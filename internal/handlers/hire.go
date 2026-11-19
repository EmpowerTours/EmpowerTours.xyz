package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// HireHandler serves the client portal at empowertours.xyz/hire.
//
// Pricing model: one task = one pull request = one fixed price, paid only after
// the client merges that pull request. There is no project total and nothing is
// free: a bug report becomes a new task that we price like any other.
type HireHandler struct {
	DB         *sqlx.DB
	AdminToken string   // same token as the site admin endpoints; empty disables admin
	Origins    []string // allowed Origin values for writes (CSRF guard)
	SiteURL    string   // where the portal lives, e.g. https://www.empowertours.xyz/hire
	GitHub     HireGitHub
	MON        HireMON
	Notify     SiteSMTP
	Client     *http.Client
	Now        func() time.Time // tests override
}

const (
	hireCookie     = "et_hire"
	hireSessionTTL = 30 * 24 * time.Hour
)

// Task stages, in order. "bug" is a reported bug waiting for a price.
const (
	StageBug       = "bug"
	StageQuoted    = "quoted"
	StageApproved  = "approved"
	StageBuilding  = "building"
	StageReview    = "review"
	StageMerged    = "merged"
	StagePaid      = "paid"
	StageCancelled = "cancelled"
)

// hireTransitions lists every allowed stage change. Anything else is refused.
var hireTransitions = map[string][]string{
	StageBug:      {StageQuoted, StageCancelled},
	StageQuoted:   {StageApproved, StageCancelled},
	StageApproved: {StageBuilding, StageReview, StageCancelled},
	StageBuilding: {StageReview, StageCancelled},
	StageReview:   {StageBuilding, StageMerged},
	StageMerged:   {StagePaid},
}

func canMove(from, to string) bool {
	for _, s := range hireTransitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

func (h *HireHandler) now() time.Time {
	if h.Now != nil {
		return h.Now().UTC()
	}
	return time.Now().UTC()
}

// moveTask changes a task's stage only if it is still in `from`. It returns
// false when another request moved it first (or it never was in `from`).
func (h *HireHandler) moveTask(ctx context.Context, tx *sqlx.Tx, id int64, from, to string, extra string, args ...any) (bool, error) {
	if !canMove(from, to) {
		return false, nil
	}
	q := `UPDATE hire_tasks SET stage = ?, updated_at = ?` + extra + ` WHERE id = ? AND stage = ?`
	all := append([]any{to, h.now()}, args...)
	all = append(all, id, from)
	res, err := tx.ExecContext(ctx, q, all...)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (h *HireHandler) event(ctx context.Context, tx *sqlx.Tx, projectID int64, taskID *int64, kind, text string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO hire_events (project_id, task_id, kind, text, created_at) VALUES (?, ?, ?, ?, ?)`,
		projectID, taskID, kind, clip(text, 300), h.now())
	return err
}

// ---------- sessions ----------

type hireClient struct {
	ID       int64  `db:"id" json:"id"`
	GitHubID int64  `db:"github_id" json:"github_id"`
	Login    string `db:"login" json:"login"`
	Name     string `db:"name" json:"name"`
	Email    string `db:"email" json:"email"`
}

func hashToken(t string) string {
	s := sha256.Sum256([]byte(t))
	return hex.EncodeToString(s[:])
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (h *HireHandler) startSession(w http.ResponseWriter, r *http.Request, clientID int64) error {
	tok, err := randomToken()
	if err != nil {
		return err
	}
	exp := h.now().Add(hireSessionTTL)
	if _, err := h.DB.ExecContext(r.Context(), `INSERT INTO hire_sessions (token_hash, client_id, expires_at) VALUES (?, ?, ?)`, hashToken(tok), clientID, exp); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: hireCookie, Value: tok, Path: "/api/v1/hire", Expires: exp,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	return nil
}

var reHexToken = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (h *HireHandler) sessionClient(r *http.Request) (*hireClient, error) {
	c, err := r.Cookie(hireCookie)
	if err != nil || !reHexToken.MatchString(c.Value) {
		return nil, errors.New("no session")
	}
	var cl hireClient
	err = h.DB.GetContext(r.Context(), &cl, `SELECT c.id, c.github_id, c.login, c.name, c.email
		FROM hire_sessions s JOIN hire_clients c ON c.id = s.client_id
		WHERE s.token_hash = ? AND s.expires_at > ?`, hashToken(c.Value), h.now())
	if err != nil {
		return nil, errors.New("no session")
	}
	return &cl, nil
}

type hireCtxKey struct{}

// RequireClient guards client routes: a valid session, and for writes a JSON
// body from one of our own origins (the cookie is SameSite=Lax; the Origin
// check closes the remaining same-site CSRF gap).
func (h *HireHandler) RequireClient(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if !h.originAllowed(r.Header.Get("Origin")) {
				writeError(w, http.StatusForbidden, "bad origin")
				return
			}
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				writeError(w, http.StatusBadRequest, "expected JSON")
				return
			}
		}
		cl, err := h.sessionClient(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "sign in with GitHub")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), hireCtxKey{}, cl)))
	})
}

func (h *HireHandler) originAllowed(o string) bool {
	for _, a := range h.Origins {
		if o == a {
			return true
		}
	}
	return false
}

func clientFrom(r *http.Request) *hireClient { return r.Context().Value(hireCtxKey{}).(*hireClient) }

func (h *HireHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(hireCookie); err == nil {
		h.DB.ExecContext(r.Context(), `DELETE FROM hire_sessions WHERE token_hash = ?`, hashToken(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: hireCookie, Value: "", Path: "/api/v1/hire", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------- read models ----------

type hireTask struct {
	ID           int64      `db:"id" json:"id"`
	ProjectID    int64      `db:"project_id" json:"project_id"`
	Kind         string     `db:"kind" json:"kind"`
	ParentTaskID *int64     `db:"parent_task_id" json:"parent_task_id,omitempty"`
	Title        string     `db:"title" json:"title"`
	What         string     `db:"what" json:"what"`
	AcceptanceJS string     `db:"acceptance" json:"-"`
	Acceptance   []string   `db:"-" json:"acceptance"`
	PriceMXN     *int64     `db:"price_mxn" json:"price_mxn"`
	Stage        string     `db:"stage" json:"stage"`
	PRNumber     *int64     `db:"pr_number" json:"pr_number,omitempty"`
	PRURL        string     `db:"pr_url" json:"pr_url,omitempty"`
	MergedAt     *time.Time `db:"merged_at" json:"merged_at,omitempty"`
	PaidAt       *time.Time `db:"paid_at" json:"paid_at,omitempty"`
	PaidBy       string     `db:"-" json:"paid_by,omitempty"`
}

type hireProject struct {
	ID       int64       `db:"id" json:"id"`
	ClientID int64       `db:"client_id" json:"-"`
	Repo     string      `db:"repo_full_name" json:"repo"`
	Title    string      `db:"title" json:"title"`
	Brief    string      `db:"brief" json:"brief"`
	Tasks    []hireTask  `db:"-" json:"tasks"`
	Events   []hireEvent `db:"-" json:"events"`
}

type hireEvent struct {
	TaskID *int64    `db:"task_id" json:"task_id,omitempty"`
	Kind   string    `db:"kind" json:"kind"`
	Text   string    `db:"text" json:"text"`
	At     time.Time `db:"created_at" json:"at"`
}

type hireBilling struct {
	RFC       string `db:"rfc" json:"rfc"`
	LegalName string `db:"legal_name" json:"legal_name"`
	Regimen   string `db:"regimen" json:"regimen"`
	CP        string `db:"cp" json:"cp"`
	UsoCFDI   string `db:"uso_cfdi" json:"uso_cfdi"`
	Email     string `db:"email" json:"email"`
}

const hireTaskCols = `id, project_id, kind, parent_task_id, title, what, acceptance, price_mxn, stage, pr_number, pr_url, merged_at, paid_at`

func (h *HireHandler) loadTasks(ctx context.Context, projectID int64) ([]hireTask, error) {
	var ts []hireTask
	if err := h.DB.SelectContext(ctx, &ts, `SELECT `+hireTaskCols+` FROM hire_tasks WHERE project_id = ? ORDER BY id`, projectID); err != nil {
		return nil, err
	}
	for i := range ts {
		json.Unmarshal([]byte(ts[i].AcceptanceJS), &ts[i].Acceptance)
		if ts[i].Acceptance == nil {
			ts[i].Acceptance = []string{}
		}
		if ts[i].Stage == StagePaid {
			h.DB.GetContext(ctx, &ts[i].PaidBy, `SELECT method FROM hire_payments WHERE task_id = ? AND status = 'confirmed'`, ts[i].ID)
		}
	}
	if ts == nil {
		ts = []hireTask{}
	}
	return ts, nil
}

// ownedTask loads a task only if it belongs to the signed-in client.
func (h *HireHandler) ownedTask(r *http.Request) (*hireTask, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		return nil, sql.ErrNoRows
	}
	var t hireTask
	err = h.DB.GetContext(r.Context(), &t, `SELECT t.id, t.project_id, t.kind, t.parent_task_id, t.title, t.what, t.acceptance, t.price_mxn, t.stage,
		t.pr_number, t.pr_url, t.merged_at, t.paid_at
		FROM hire_tasks t JOIN hire_projects p ON p.id = t.project_id WHERE t.id = ? AND p.client_id = ?`, id, clientFrom(r).ID)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ---------- client endpoints ----------

// Me returns the signed-in client, their billing details and connected repos.
func (h *HireHandler) Me(w http.ResponseWriter, r *http.Request) {
	cl := clientFrom(r)
	var billing *hireBilling
	var b hireBilling
	if err := h.DB.GetContext(r.Context(), &b, `SELECT rfc, legal_name, regimen, cp, uso_cfdi, email FROM hire_billing WHERE client_id = ?`, cl.ID); err == nil {
		billing = &b
	}
	repos := []string{}
	h.DB.SelectContext(r.Context(), &repos, `SELECT r.full_name FROM hire_repos r JOIN hire_installations i ON i.installation_id = r.installation_id
		WHERE i.sender_github_id = ? AND i.removed_at IS NULL ORDER BY r.full_name`, cl.GitHubID)
	writeJSON(w, http.StatusOK, map[string]any{
		"client": cl, "billing": billing, "repos": repos,
		"install_url": h.GitHub.InstallURL(),
		"pay_methods": h.payMethods(),
	})
}

func (h *HireHandler) payMethods() []string {
	m := []string{}
	if h.MON.Enabled() {
		m = append(m, "mon")
	}
	return m
}

var (
	reRFC = regexp.MustCompile(`^[A-ZÑ&]{3,4}[0-9]{6}[A-Z0-9]{3}$`)
	reCP  = regexp.MustCompile(`^[0-9]{5}$`)
	reSAT = regexp.MustCompile(`^[A-Z0-9]{3,4}$`) // c_RegimenFiscal (601) / c_UsoCFDI (G03)
)

// PutBilling stores the client's CFDI 4.0 receptor data.
func (h *HireHandler) PutBilling(w http.ResponseWriter, r *http.Request) {
	var b hireBilling
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	b.RFC = strings.ToUpper(strings.TrimSpace(b.RFC))
	b.LegalName = strings.ToUpper(strings.TrimSpace(b.LegalName))
	b.Regimen, b.UsoCFDI, b.CP = strings.TrimSpace(b.Regimen), strings.ToUpper(strings.TrimSpace(b.UsoCFDI)), strings.TrimSpace(b.CP)
	b.Email = strings.TrimSpace(b.Email)
	switch {
	case !reRFC.MatchString(b.RFC):
		writeError(w, http.StatusBadRequest, "RFC must be 12 (company) or 13 (person) characters")
	case b.LegalName == "" || len(b.LegalName) > 254:
		writeError(w, http.StatusBadRequest, "legal name as it appears on the Constancia de Situación Fiscal")
	case !reSAT.MatchString(b.Regimen) || !reSAT.MatchString(b.UsoCFDI):
		writeError(w, http.StatusBadRequest, "regimen and CFDI use must be SAT catalog keys, e.g. 601 and G03")
	case !reCP.MatchString(b.CP):
		writeError(w, http.StatusBadRequest, "postal code must be 5 digits")
	case !validEmail(b.Email):
		writeError(w, http.StatusBadRequest, "a valid email for the factura")
	default:
		_, err := h.DB.ExecContext(r.Context(), `INSERT INTO hire_billing (client_id, rfc, legal_name, regimen, cp, uso_cfdi, email, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(client_id) DO UPDATE SET rfc = excluded.rfc, legal_name = excluded.legal_name,
			regimen = excluded.regimen, cp = excluded.cp, uso_cfdi = excluded.uso_cfdi, email = excluded.email, updated_at = excluded.updated_at`,
			clientFrom(r).ID, b.RFC, b.LegalName, b.Regimen, b.CP, b.UsoCFDI, b.Email, h.now())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not save")
			return
		}
		writeJSON(w, http.StatusOK, b)
	}
}

// Projects lists the client's projects with their tasks and recent activity.
func (h *HireHandler) Projects(w http.ResponseWriter, r *http.Request) {
	ps := []hireProject{}
	if err := h.DB.SelectContext(r.Context(), &ps, `SELECT id, client_id, repo_full_name, title, brief FROM hire_projects WHERE client_id = ? ORDER BY id DESC`, clientFrom(r).ID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not load projects")
		return
	}
	for i := range ps {
		ts, err := h.loadTasks(r.Context(), ps[i].ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not load tasks")
			return
		}
		ps[i].Tasks = ts
		ps[i].Events = []hireEvent{}
		h.DB.SelectContext(r.Context(), &ps[i].Events, `SELECT task_id, kind, text, created_at FROM hire_events WHERE project_id = ? ORDER BY id DESC LIMIT 20`, ps[i].ID)
	}
	writeJSON(w, http.StatusOK, ps)
}

// CreateProject starts a project on one of the client's connected repos.
func (h *HireHandler) CreateProject(w http.ResponseWriter, r *http.Request) {
	var in struct{ Repo, Title, Brief string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	in.Title, in.Brief = strings.TrimSpace(in.Title), strings.TrimSpace(in.Brief)
	if in.Title == "" || in.Brief == "" {
		writeError(w, http.StatusBadRequest, "a title and a short brief")
		return
	}
	cl := clientFrom(r)
	var n int
	h.DB.GetContext(r.Context(), &n, `SELECT COUNT(*) FROM hire_repos r JOIN hire_installations i ON i.installation_id = r.installation_id
		WHERE r.full_name = ? AND i.sender_github_id = ? AND i.removed_at IS NULL`, in.Repo, cl.GitHubID)
	if n == 0 {
		writeError(w, http.StatusBadRequest, "connect this repo to EmpowerTours on GitHub first")
		return
	}
	var open int
	h.DB.GetContext(r.Context(), &open, `SELECT COUNT(*) FROM hire_projects WHERE client_id = ?`, cl.ID)
	if open >= 20 {
		writeError(w, http.StatusTooManyRequests, "project limit reached; contact us")
		return
	}
	tx, err := h.DB.BeginTxx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save")
		return
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(r.Context(), `INSERT INTO hire_projects (client_id, repo_full_name, title, brief, created_at) VALUES (?, ?, ?, ?, ?)`,
		cl.ID, in.Repo, clip(in.Title, 120), clip(in.Brief, 8000), h.now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save")
		return
	}
	pid, _ := res.LastInsertId()
	h.event(r.Context(), tx, pid, nil, "project", "Project started on "+in.Repo)
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save")
		return
	}
	h.notify("New hire project: "+in.Title, "Client: "+cl.Login+"\nRepo: "+in.Repo+"\n\n"+in.Brief)
	writeJSON(w, http.StatusOK, map[string]int64{"id": pid})
}

// Approve accepts a task's fixed price so we begin building.
func (h *HireHandler) Approve(w http.ResponseWriter, r *http.Request) {
	t, err := h.ownedTask(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if t.Stage != StageQuoted || t.PriceMXN == nil || *t.PriceMXN <= 0 {
		writeError(w, http.StatusConflict, "this task has no price to approve yet")
		return
	}
	tx, err := h.DB.BeginTxx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "try again")
		return
	}
	defer tx.Rollback()
	ok, err := h.moveTask(r.Context(), tx, t.ID, t.Stage, StageApproved, `, approved_at = ?`, h.now())
	if err != nil || !ok {
		writeError(w, http.StatusConflict, "this task can't be approved now")
		return
	}
	msg := "You approved task " + strconv.FormatInt(t.ID, 10) + " at " + mxnText(*t.PriceMXN)
	h.event(r.Context(), tx, t.ProjectID, &t.ID, "approved", msg)
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "try again")
		return
	}
	h.notify("Task approved: "+t.Title, clientFrom(r).Login+": "+msg)
	writeJSON(w, http.StatusOK, map[string]string{"stage": StageApproved})
}

// Question sends a question about a task to us and logs it on the board.
func (h *HireHandler) Question(w http.ResponseWriter, r *http.Request) {
	t, err := h.ownedTask(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	var in struct{ Text string }
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&in) != nil || strings.TrimSpace(in.Text) == "" {
		writeError(w, http.StatusBadRequest, "write your question")
		return
	}
	tx, err := h.DB.BeginTxx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "try again")
		return
	}
	defer tx.Rollback()
	h.event(r.Context(), tx, t.ProjectID, &t.ID, "question", "You asked about task "+strconv.FormatInt(t.ID, 10))
	tx.Commit()
	h.notify("Question on task "+strconv.FormatInt(t.ID, 10)+": "+t.Title, clientFrom(r).Login+" asks:\n\n"+clip(in.Text, 4000))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ReportBug opens a bug task against a paid task. We price it like any other
// task; the client approves that price before any work starts.
func (h *HireHandler) ReportBug(w http.ResponseWriter, r *http.Request) {
	t, err := h.ownedTask(r)
	if err != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if t.Stage != StagePaid {
		writeError(w, http.StatusConflict, "bugs can be reported on paid tasks")
		return
	}
	var in struct{ Title, Text string }
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&in) != nil || strings.TrimSpace(in.Text) == "" {
		writeError(w, http.StatusBadRequest, "describe what goes wrong")
		return
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = "Bug in: " + t.Title
	}
	var open int
	h.DB.GetContext(r.Context(), &open, `SELECT COUNT(*) FROM hire_tasks WHERE parent_task_id = ? AND stage IN ('bug','quoted')`, t.ID)
	if open >= 5 {
		writeError(w, http.StatusTooManyRequests, "several bugs on this task are already waiting; we'll get to them")
		return
	}
	tx, err := h.DB.BeginTxx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "try again")
		return
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(r.Context(), `INSERT INTO hire_tasks (project_id, kind, parent_task_id, title, what, acceptance, price_mxn, stage, created_at, updated_at)
		VALUES (?, 'bug', ?, ?, ?, '[]', NULL, 'bug', ?, ?)`, t.ProjectID, t.ID, clip(title, 160), clip(strings.TrimSpace(in.Text), 4000), h.now(), h.now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "try again")
		return
	}
	id, _ := res.LastInsertId()
	h.event(r.Context(), tx, t.ProjectID, &id, "bug", "Bug reported on task "+strconv.FormatInt(t.ID, 10)+"; we'll send a price")
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "try again")
		return
	}
	h.notify("Bug reported on task "+strconv.FormatInt(t.ID, 10), clientFrom(r).Login+" (needs a price):\n\n"+clip(in.Text, 4000))
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

// ---------- helpers ----------

func mxnText(n int64) string {
	s := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return "$" + b.String() + " MXN"
}

func validEmail(s string) bool {
	return reEmail.MatchString(s) && len(s) <= 254
}

var reEmail = regexp.MustCompile(`^[^@\s<>"]+@[^@\s<>"]+\.[^@\s<>"]+$`)

func (h *HireHandler) notify(subject, body string) {
	if !h.Notify.ready() {
		return
	}
	go h.Notify.send("", subject, body)
}

// adminOK checks the bearer token in constant time; short tokens never work.
func (h *HireHandler) adminOK(r *http.Request) bool {
	if len(h.AdminToken) < 32 {
		return false
	}
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	return subtle.ConstantTimeCompare([]byte(got), []byte(h.AdminToken)) == 1
}
