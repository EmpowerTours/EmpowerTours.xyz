package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

const (
	testOrigin   = "https://www.empowertours.xyz"
	testSecret   = "whsec-test-secret"
	testTreasury = "0x1111111111111111111111111111111111111111"
	testAdmin    = "admin-token-admin-token-admin-token-xx"
)

func hireDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db := sqlx.MustConnect("sqlite3", ":memory:")
	db.SetMaxOpenConns(1)
	sql, err := os.ReadFile("../database/migrations/023_hire.sql")
	if err != nil {
		t.Fatal(err)
	}
	db.MustExec(string(sql))
	return db
}

type hireEnv struct {
	h      *HireHandler
	srv    *httptest.Server
	clock  time.Time
	rpcTx  map[string]any // fake chain: the one transaction it knows
	cookie map[int64]*http.Cookie
}

func newHireEnv(t *testing.T) *hireEnv {
	e := &hireEnv{clock: time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC), cookie: map[int64]*http.Cookie{}}
	rate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"monad":{"mxn":0.25}}`)
	}))
	rpc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		res := map[string]any{
			"eth_getTransactionByHash":  e.rpcTx,
			"eth_getTransactionReceipt": map[string]any{"status": "0x1"},
			"eth_blockNumber":           "0x64",
			"eth_getBlockByNumber":      map[string]any{"timestamp": fmt.Sprintf("0x%x", e.clock.Add(time.Minute).Unix())},
		}[req.Method]
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": res})
	}))
	t.Cleanup(func() { rate.Close(); rpc.Close() })
	e.h = &HireHandler{
		DB: hireDB(t), AdminToken: testAdmin, Origins: []string{testOrigin}, SiteURL: "https://www.empowertours.xyz/hire",
		GitHub: HireGitHub{Slug: "et-builder", ClientID: "cid", ClientSecret: "cs", WebhookSecret: testSecret},
		MON:    HireMON{RPCURL: rpc.URL, Treasury: testTreasury, ChainID: 143, RateURL: rate.URL, MinConf: 2},
		Now:    func() time.Time { return e.clock },
	}
	r := chi.NewRouter()
	r.Post("/webhook", e.h.Webhook)
	r.Group(func(r chi.Router) {
		r.Use(e.h.RequireClient)
		r.Get("/me", e.h.Me)
		r.Put("/billing", e.h.PutBilling)
		r.Get("/projects", e.h.Projects)
		r.Post("/projects", e.h.CreateProject)
		r.Post("/tasks/{id}/approve", e.h.Approve)
		r.Post("/tasks/{id}/bug", e.h.ReportBug)
		r.Post("/tasks/{id}/pay/mon", e.h.PayMONIntent)
		r.Post("/tasks/{id}/pay/mon/confirm", e.h.PayMONConfirm)
	})
	r.Group(func(r chi.Router) {
		r.Use(e.h.RequireAdmin)
		r.Post("/admin/tasks", e.h.AdminCreateTask)
		r.Post("/admin/tasks/{id}/quote", e.h.AdminQuoteBug)
		r.Post("/admin/tasks/{id}/stage", e.h.AdminStage)
		r.Post("/admin/tasks/{id}/paid", e.h.AdminPesoPaid)
	})
	e.srv = httptest.NewServer(r)
	t.Cleanup(e.srv.Close)
	return e
}

// client creates a GitHub user with a session, owning installation inst with repo.
func (e *hireEnv) client(t *testing.T, ghID, inst, repoID int64, repo string) int64 {
	var id int64
	e.h.DB.Get(&id, `INSERT INTO hire_clients (github_id, login) VALUES (?, ?) RETURNING id`, ghID, fmt.Sprintf("user%d", ghID))
	if err := e.h.storeInstallation(context.Background(), inst, ghID, "acct", []ghRepo{{ID: repoID, FullName: repo}}, true); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	if err := e.h.startSession(w, httptest.NewRequest("GET", "/", nil), id); err != nil {
		t.Fatal(err)
	}
	e.cookie[id] = w.Result().Cookies()[0]
	return id
}

func (e *hireEnv) do(t *testing.T, client int64, method, path, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", testOrigin)
	if c := e.cookie[client]; c != nil {
		req.AddCookie(c)
	}
	if client < 0 {
		req.Header.Set("Authorization", "Bearer "+testAdmin)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

const admin = -1

func (e *hireEnv) hook(t *testing.T, event string, payload any, sign bool) int {
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", e.srv.URL+"/webhook", strings.NewReader(string(b)))
	req.Header.Set("X-GitHub-Event", event)
	if sign {
		m := hmac.New(sha256.New, []byte(testSecret))
		m.Write(b)
		req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(m.Sum(nil)))
	} else {
		req.Header.Set("X-Hub-Signature-256", "sha256="+strings.Repeat("0", 64))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func prEvent(action string, task int64, repoID, headRepoID, inst int64, merged bool) map[string]any {
	return map[string]any{
		"action": action, "installation": map[string]any{"id": inst}, "repository": map[string]any{"id": repoID, "full_name": "acme/app"},
		"pull_request": map[string]any{"number": 7, "html_url": "https://github.com/acme/app/pull/7", "merged": merged,
			"merge_commit_sha": strings.Repeat("a", 40),
			"head":             map[string]any{"ref": fmt.Sprintf("et/task-%d", task), "repo": map[string]any{"id": headRepoID}}},
	}
}

func (e *hireEnv) stage(t *testing.T, task int64) string {
	var s string
	e.h.DB.Get(&s, `SELECT stage FROM hire_tasks WHERE id = ?`, task)
	return s
}

// setup: client 1 owns acme/app with a project and one quoted task at $1,000.
func setup(t *testing.T) (*hireEnv, int64, int64) {
	e := newHireEnv(t)
	c := e.client(t, 100, 10, 500, "acme/app")
	if code, out := e.do(t, c, "POST", "/projects", `{"repo":"acme/app","title":"Booking","brief":"Tour booking app"}`); code != 200 {
		t.Fatalf("create project: %d %v", code, out)
	}
	code, out := e.do(t, admin, "POST", "/admin/tasks", `{"project_id":1,"title":"Calendar","price_mxn":1000,"acceptance":["Shows open dates"]}`)
	if code != 200 {
		t.Fatalf("quote: %d %v", code, out)
	}
	return e, c, 1
}

func TestHireGuards(t *testing.T) {
	e, c, task := setup(t)
	other := e.client(t, 200, 20, 600, "evil/repo")

	if code, _ := e.do(t, 0, "GET", "/me", ""); code != 401 {
		t.Errorf("no session: %d", code)
	}
	req, _ := http.NewRequest("POST", e.srv.URL+fmt.Sprintf("/tasks/%d/approve", task), strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	req.AddCookie(e.cookie[c])
	if resp, _ := http.DefaultClient.Do(req); resp.StatusCode != 403 {
		t.Errorf("foreign origin: %d", resp.StatusCode)
	}
	if code, _ := e.do(t, other, "POST", fmt.Sprintf("/tasks/%d/approve", task), `{}`); code != 404 {
		t.Errorf("another client's task: %d", code)
	}
	if code, _ := e.do(t, other, "POST", "/projects", `{"repo":"acme/app","title":"x","brief":"x"}`); code != 400 {
		t.Errorf("project on a repo you didn't connect: %d", code)
	}
	if code, _ := e.do(t, c, "POST", "/admin/tasks", `{}`); code != 401 {
		t.Errorf("client on admin route: %d", code)
	}
	if code, _ := e.do(t, admin, "POST", "/admin/tasks", `{"project_id":1,"title":"Free","price_mxn":0,"acceptance":["x"]}`); code != 400 {
		t.Errorf("zero price must be refused: %d", code)
	}
	// expired session
	e.clock = e.clock.Add(31 * 24 * time.Hour)
	if code, _ := e.do(t, c, "GET", "/me", ""); code != 401 {
		t.Errorf("expired session: %d", code)
	}
}

func TestHireLifecycleAndWebhook(t *testing.T) {
	e, c, task := setup(t)

	// Can't pay before merge.
	if code, _ := e.do(t, c, "POST", "/tasks/1/pay/mon", `{}`); code != 409 {
		t.Errorf("pay before merge: %d", code)
	}
	if code, _ := e.do(t, c, "POST", "/tasks/1/approve", `{}`); code != 200 {
		t.Fatalf("approve: %d", code)
	}
	if code, _ := e.do(t, c, "POST", "/tasks/1/approve", `{}`); code != 409 {
		t.Errorf("double approve: %d", code)
	}
	if code := e.hook(t, "pull_request", prEvent("opened", task, 500, 500, 10, false), false); code != 401 {
		t.Errorf("unsigned webhook: %d", code)
	}
	// A fork's branch named et/task-1 must not move the task.
	e.hook(t, "pull_request", prEvent("opened", task, 500, 999, 10, false), true)
	if s := e.stage(t, task); s != StageApproved {
		t.Errorf("fork moved task to %s", s)
	}
	// Right branch on another client's installation must not move it either.
	e.client(t, 200, 20, 600, "evil/repo")
	e.hook(t, "pull_request", prEvent("opened", task, 600, 600, 20, false), true)
	if s := e.stage(t, task); s != StageApproved {
		t.Errorf("other repo moved task to %s", s)
	}
	e.hook(t, "pull_request", prEvent("opened", task, 500, 500, 10, false), true)
	if s := e.stage(t, task); s != StageReview {
		t.Fatalf("opened -> %s", s)
	}
	e.hook(t, "pull_request", prEvent("closed", task, 500, 500, 10, false), true)
	if s := e.stage(t, task); s != StageBuilding {
		t.Fatalf("closed unmerged -> %s", s)
	}
	e.hook(t, "pull_request", prEvent("reopened", task, 500, 500, 10, false), true)
	e.hook(t, "pull_request", prEvent("closed", task, 500, 500, 10, true), true)
	if s := e.stage(t, task); s != StageMerged {
		t.Fatalf("merged -> %s", s)
	}
	// Replayed merge webhook is harmless.
	if code := e.hook(t, "pull_request", prEvent("closed", task, 500, 500, 10, true), true); code != 200 {
		t.Errorf("replay: %d", code)
	}
}

func TestHireBugsAreNeverFree(t *testing.T) {
	e, c, _ := setup(t)
	e.h.DB.MustExec(`UPDATE hire_tasks SET stage = 'paid' WHERE id = 1`)
	code, out := e.do(t, c, "POST", "/tasks/1/bug", `{"text":"dates show twice"}`)
	if code != 200 {
		t.Fatalf("report bug: %d %v", code, out)
	}
	bug := int64(out["data"].(map[string]any)["id"].(float64))
	if code, _ := e.do(t, c, "POST", fmt.Sprintf("/tasks/%d/approve", bug), `{}`); code != 409 {
		t.Errorf("unpriced bug approved: %d", code)
	}
	if code, _ := e.do(t, admin, "POST", fmt.Sprintf("/admin/tasks/%d/quote", bug), `{"price_mxn":0,"acceptance":["x"]}`); code != 400 {
		t.Errorf("zero-price bug quote: %d", code)
	}
	if code, _ := e.do(t, admin, "POST", fmt.Sprintf("/admin/tasks/%d/quote", bug), `{"price_mxn":800,"acceptance":["Each date shows once"]}`); code != 200 {
		t.Fatalf("quote bug: %d", code)
	}
	if code, _ := e.do(t, c, "POST", fmt.Sprintf("/tasks/%d/approve", bug), `{}`); code != 200 {
		t.Errorf("approve priced bug: %d", code)
	}
	if code, _ := e.do(t, c, "POST", "/tasks/2/bug", `{"text":"x"}`); code != 409 {
		t.Errorf("bug on an unpaid task: %d", code)
	}
}

func TestHireMONPayment(t *testing.T) {
	e, c, task := setup(t)
	e.h.DB.MustExec(`UPDATE hire_tasks SET stage = 'merged' WHERE id = ?`, task)

	code, out := e.do(t, c, "POST", "/tasks/1/pay/mon", `{}`)
	if code != 200 {
		t.Fatalf("intent: %d %v", code, out)
	}
	in := out["data"].(map[string]any)
	wei, _ := new(big.Int).SetString(in["amount_wei"].(string), 10)
	whole := new(big.Int).Quo(wei, big.NewInt(1e18))
	if whole.Int64() != 4000 { // $1,000 MXN at 0.25 MXN/MON
		t.Fatalf("whole MON %v", whole)
	}
	if in["pay_to"] != testTreasury {
		t.Fatalf("pay_to %v", in["pay_to"])
	}
	// Same intent comes back while it is open.
	_, again := e.do(t, c, "POST", "/tasks/1/pay/mon", `{}`)
	if again["data"].(map[string]any)["amount_wei"] != in["amount_wei"] {
		t.Errorf("intent not reused")
	}

	hash := "0x" + strings.Repeat("ab", 32)
	tx := func(to string, v *big.Int) map[string]any {
		return map[string]any{"to": to, "value": "0x" + v.Text(16), "blockNumber": "0x60", "chainId": "0x8f"}
	}
	confirm := func(h string) int {
		code, _ := e.do(t, c, "POST", "/tasks/1/pay/mon/confirm", `{"tx_hash":"`+h+`"}`)
		return code
	}
	e.rpcTx = tx(testTreasury, new(big.Int).Sub(wei, big.NewInt(1)))
	if code := confirm(hash); code != 400 {
		t.Errorf("1 wei short accepted: %d", code)
	}
	e.rpcTx = tx("0x2222222222222222222222222222222222222222", wei)
	if code := confirm(hash); code != 400 {
		t.Errorf("wrong recipient accepted: %d", code)
	}
	e.rpcTx = map[string]any{"to": testTreasury, "value": "0x" + wei.Text(16), "blockNumber": "0x60", "chainId": "0x1"}
	if code := confirm(hash); code != 400 {
		t.Errorf("wrong chain accepted: %d", code)
	}
	e.rpcTx = tx(testTreasury, wei)
	if code := confirm("not-a-hash"); code != 400 {
		t.Errorf("bad hash: %d", code)
	}
	if code := confirm(hash); code != 200 {
		t.Fatalf("exact payment refused: %d", code)
	}
	if s := e.stage(t, task); s != StagePaid {
		t.Fatalf("stage %s", s)
	}
	if code := confirm(hash); code != 409 {
		t.Errorf("hash reuse: %d", code)
	}
}

func TestHirePesoPaymentMustMatchPrice(t *testing.T) {
	e, _, task := setup(t)
	e.h.DB.MustExec(`UPDATE hire_tasks SET stage = 'merged' WHERE id = ?`, task)
	if code, _ := e.do(t, admin, "POST", "/admin/tasks/1/paid", `{"method":"spei","reference":"ABC123","amount_mxn":999}`); code != 409 {
		t.Errorf("short peso payment: %d", code)
	}
	if code, _ := e.do(t, admin, "POST", "/admin/tasks/1/paid", `{"method":"spei","reference":"ABC123","amount_mxn":1000}`); code != 200 {
		t.Errorf("peso payment: %d", code)
	}
	if code, _ := e.do(t, admin, "POST", "/admin/tasks/1/paid", `{"method":"spei","reference":"ABC124","amount_mxn":1000}`); code != 409 {
		t.Errorf("second payment for one task: %d", code)
	}
}

func TestMONMath(t *testing.T) {
	if got := monForMXN(1000, 0.25).String(); got != "4000" {
		t.Errorf("exact: %s", got)
	}
	if got := monForMXN(1000, 0.263927).String(); got != "3789" { // 3788.92 rounds up
		t.Errorf("round up: %s", got)
	}
	if got := weiToMON("4000012300000000000000"); got != "4000.0123" {
		t.Errorf("weiToMON: %s", got)
	}
	if !reTaskBranch.MatchString("et/task-12-guide-schedule") || reTaskBranch.MatchString("et/task-12/../x") || reTaskBranch.MatchString("main") {
		t.Errorf("branch pattern")
	}
	if !(HireGitHub{WebhookSecret: "s"}).verifySignature([]byte("x"), "sha256="+hmacHex("s", "x")) || (HireGitHub{}).verifySignature([]byte("x"), "sha256="+hmacHex("", "x")) {
		t.Errorf("signature: empty secret must never verify")
	}
}

func hmacHex(k, msg string) string {
	m := hmac.New(sha256.New, []byte(k))
	m.Write([]byte(msg))
	return hex.EncodeToString(m.Sum(nil))
}
