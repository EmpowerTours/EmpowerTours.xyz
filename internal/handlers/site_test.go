package handlers

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func siteTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db := sqlx.MustConnect("sqlite3", ":memory:")
	db.SetMaxOpenConns(1)
	sql, err := os.ReadFile("../database/migrations/022_site_inbox.sql")
	if err != nil {
		t.Fatal(err)
	}
	db.MustExec(string(sql))
	return db
}

func post(h http.HandlerFunc, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
	return w
}

func count(t *testing.T, db *sqlx.DB, table string) int {
	var n int
	if err := db.Get(&n, "SELECT COUNT(*) FROM "+table); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestInquiry(t *testing.T) {
	db := siteTestDB(t)
	h := &SiteHandler{DB: db}
	cases := []struct {
		name, body string
		code, rows int
	}{
		{"valid", `{"name":"Ana","email":"ana@example.com","message":"Build a booking app"}`, 200, 1},
		{"honeypot stores nothing", `{"name":"Bot","email":"b@example.com","message":"x","website":"spam"}`, 200, 1},
		{"bad email", `{"name":"Ana","email":"not-an-email","message":"x"}`, 400, 1},
		{"display-name email rejected", `{"name":"Ana","email":"Ana <ana@example.com>","message":"x"}`, 400, 1},
		{"missing message", `{"name":"Ana","email":"ana@example.com","message":"  "}`, 400, 1},
		{"not json", `name=Ana`, 400, 1},
		{"html is stored as text, not rejected", `{"name":"<b>A</b>","email":"a@example.com","message":"<script>x</script>"}`, 200, 2},
	}
	for _, c := range cases {
		w := post(h.Inquiry, c.body)
		if w.Code != c.code {
			t.Errorf("%s: code %d, want %d (%s)", c.name, w.Code, c.code, w.Body)
		}
		if n := count(t, db, "site_inquiries"); n != c.rows {
			t.Errorf("%s: rows %d, want %d", c.name, n, c.rows)
		}
	}
	// Oversized fields are clipped, not stored whole.
	post(h.Inquiry, `{"name":"`+strings.Repeat("n", 500)+`","email":"a@example.com","message":"`+strings.Repeat("m", 20000)+`"}`)
	var name, msg string
	db.QueryRow("SELECT name, message FROM site_inquiries ORDER BY id DESC LIMIT 1").Scan(&name, &msg)
	if len(name) != 120 || len(msg) != 8000 {
		t.Errorf("clip: name %d msg %d", len(name), len(msg))
	}
}

func TestInquiryDailyCap(t *testing.T) {
	db := siteTestDB(t)
	h := &SiteHandler{DB: db}
	for i := 0; i < maxInquiriesPerDay; i++ {
		db.MustExec(`INSERT INTO site_inquiries (name, email, message) VALUES ('a','a@example.com','m')`)
	}
	if w := post(h.Inquiry, `{"name":"Ana","email":"ana@example.com","message":"hi"}`); w.Code != http.StatusTooManyRequests {
		t.Fatalf("over cap: code %d", w.Code)
	}
}

func appBody(ekLen, ivLen int, kid string) string {
	b := func(n int) string { return base64.StdEncoding.EncodeToString(make([]byte, n)) }
	return `{"v":1,"kid":"` + kid + `","ek":"` + b(ekLen) + `","iv":"` + b(ivLen) + `","ct":"` + b(64) + `"}`
}

func TestApplicationStoresOnlyWellFormedCiphertext(t *testing.T) {
	db := siteTestDB(t)
	h := &SiteHandler{DB: db}
	if w := post(h.Application, appBody(512, 12, "0123456789abcdef")); w.Code != 200 {
		t.Fatalf("valid: %d %s", w.Code, w.Body)
	}
	bad := []string{
		appBody(256, 12, "0123456789abcdef"), // wrong RSA size
		appBody(512, 16, "0123456789abcdef"), // wrong IV size
		appBody(512, 12, "../../etc/passwd"), // bad kid
		`{"v":2,"kid":"0123456789abcdef","ek":"","iv":"","ct":""}`,
		`{"v":1,"name":"plaintext should never be accepted"}`,
	}
	for i, body := range bad {
		if w := post(h.Application, body); w.Code != 400 {
			t.Errorf("bad %d: code %d", i, w.Code)
		}
	}
	if n := count(t, db, "site_applications"); n != 1 {
		t.Fatalf("rows %d, want 1", n)
	}
}

func TestAdminReadNeedsStrongToken(t *testing.T) {
	db := siteTestDB(t)
	get := func(h *SiteHandler, auth string) int {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if auth != "" {
			r.Header.Set("Authorization", auth)
		}
		h.ListInquiries(w, r)
		return w.Code
	}
	strong := strings.Repeat("k", 40)
	if c := get(&SiteHandler{DB: db}, ""); c != 401 {
		t.Errorf("no token configured: %d", c)
	}
	if c := get(&SiteHandler{DB: db, AdminToken: "short"}, "Bearer short"); c != 401 {
		t.Errorf("short token must stay disabled: %d", c)
	}
	h := &SiteHandler{DB: db, AdminToken: strong}
	if c := get(h, "Bearer wrong"); c != 401 {
		t.Errorf("wrong token: %d", c)
	}
	if c := get(h, "Bearer "+strong); c != 200 {
		t.Errorf("right token: %d", c)
	}
}

func TestRepoNameGuard(t *testing.T) {
	for _, ok := range []string{"empowertours-hunt", "EmpowerTours.xyz", "a_b"} {
		if !reRepoName.MatchString(ok) {
			t.Errorf("%q should pass", ok)
		}
	}
	for _, bad := range []string{"../x", "a/b", "a?b", "", "a b"} {
		if reRepoName.MatchString(bad) {
			t.Errorf("%q should fail", bad)
		}
	}
}
