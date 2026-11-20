package handlers

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
)

// SiteHandler serves the empowertours.xyz marketing site: the public GitHub
// activity feed, the "Start a project" inbox and the encrypted careers inbox.
type SiteHandler struct {
	DB          *sqlx.DB
	GitHubUser  string
	GitHubToken string // optional; raises GitHub's rate limit
	AdminToken  string // SITE_ADMIN_TOKEN; empty disables the read endpoints
	SMTP        SiteSMTP
	Client      *http.Client
	Drafter     QuoteDrafter // nil (no ANTHROPIC_API_KEY) disables AI quote drafts

	mailer    siteMailer     // tests replace SMTP with a stub
	bg        sync.WaitGroup // background quote drafting
	mu        sync.Mutex
	cached    []byte
	cachedAt  time.Time
	refreshMu sync.Mutex
}

const siteGitHubTTL = time.Hour

// ---------- GitHub feed ----------

type ghDay struct {
	Date  string `json:"date"`
	Level int    `json:"level"`
	Count int    `json:"count"`
}

type ghCommit struct {
	Repo    string `json:"repo"`
	SHA     string `json:"sha"`
	Message string `json:"message"`
	At      string `json:"at"`
	URL     string `json:"url"`
}

type ghFeed struct {
	Days    []ghDay    `json:"days"`
	Total   int        `json:"total"`
	Commits []ghCommit `json:"commits"`
	Updated string     `json:"updated"`
}

var (
	reCell = regexp.MustCompile(`<td[^>]*data-date="([\d-]+)"[^>]*id="([^"]+)"[^>]*data-level="(\d)"`)
	reTip  = regexp.MustCompile(`<tool-tip[^>]*for="([^"]+)"[^>]*>\s*(\d+|No) contribution`)
)

// GitHub handles GET /api/v1/site/github. Cached for an hour; on upstream
// failure it serves the last good copy.
func (h *SiteHandler) GitHub(w http.ResponseWriter, r *http.Request) {
	body, err := h.feed(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "GitHub feed unavailable")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=600")
	w.Write(body)
}

func (h *SiteHandler) feed(ctx context.Context) ([]byte, error) {
	h.mu.Lock()
	if h.cached != nil && time.Since(h.cachedAt) < siteGitHubTTL {
		b := h.cached
		h.mu.Unlock()
		return b, nil
	}
	h.mu.Unlock()

	// One refresh at a time; others wait and reuse its result.
	h.refreshMu.Lock()
	defer h.refreshMu.Unlock()
	h.mu.Lock()
	if h.cached != nil && time.Since(h.cachedAt) < siteGitHubTTL {
		b := h.cached
		h.mu.Unlock()
		return b, nil
	}
	stale := h.cached
	h.mu.Unlock()

	fresh, err := h.buildFeed(ctx)
	if err != nil {
		if stale != nil {
			return stale, nil
		}
		return nil, err
	}
	b, err := json.Marshal(map[string]any{"data": fresh})
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	h.cached, h.cachedAt = b, time.Now()
	h.mu.Unlock()
	return b, nil
}

func (h *SiteHandler) get(ctx context.Context, url string, api bool) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "empowertours-site")
	if api {
		req.Header.Set("Accept", "application/vnd.github+json")
		if h.GitHubToken != "" {
			req.Header.Set("Authorization", "Bearer "+h.GitHubToken)
		}
	}
	res, err := h.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %d", url, res.StatusCode)
	}
	return io.ReadAll(io.LimitReader(res.Body, 8<<20))
}

func (h *SiteHandler) buildFeed(ctx context.Context) (*ghFeed, error) {
	cal, err := h.get(ctx, "https://github.com/users/"+h.GitHubUser+"/contributions", false)
	if err != nil {
		return nil, err
	}
	cells := map[string]*ghDay{}
	for _, m := range reCell.FindAllStringSubmatch(string(cal), -1) {
		lvl, _ := strconv.Atoi(m[3])
		cells[m[2]] = &ghDay{Date: m[1], Level: lvl}
	}
	for _, m := range reTip.FindAllStringSubmatch(string(cal), -1) {
		if c, ok := cells[m[1]]; ok && m[2] != "No" {
			c.Count, _ = strconv.Atoi(m[2])
		}
	}
	if len(cells) < 300 {
		return nil, fmt.Errorf("calendar parse: %d days", len(cells))
	}
	feed := &ghFeed{Updated: time.Now().UTC().Format(time.RFC3339)}
	for _, d := range cells {
		feed.Days = append(feed.Days, *d)
		feed.Total += d.Count
	}
	sort.Slice(feed.Days, func(i, j int) bool { return feed.Days[i].Date < feed.Days[j].Date })

	raw, err := h.get(ctx, "https://api.github.com/users/"+h.GitHubUser+"/repos?type=owner&sort=pushed&per_page=30", true)
	if err != nil {
		return nil, err
	}
	var repos []struct {
		Name    string `json:"name"`
		Fork    bool   `json:"fork"`
		Private bool   `json:"private"`
	}
	if err := json.Unmarshal(raw, &repos); err != nil {
		return nil, err
	}
	n := 0
	for _, rp := range repos {
		if rp.Fork || rp.Private || !reRepoName.MatchString(rp.Name) {
			continue
		}
		if n++; n > 6 {
			break
		}
		raw, err := h.get(ctx, "https://api.github.com/repos/"+h.GitHubUser+"/"+rp.Name+"/commits?per_page=3", true)
		if err != nil {
			continue // one bad repo should not hide the rest
		}
		var cs []struct {
			SHA     string `json:"sha"`
			HTMLURL string `json:"html_url"`
			Commit  struct {
				Message string `json:"message"`
				Author  struct {
					Date string `json:"date"`
				} `json:"author"`
			} `json:"commit"`
		}
		if json.Unmarshal(raw, &cs) != nil {
			continue
		}
		for _, c := range cs {
			msg := strings.SplitN(c.Commit.Message, "\n", 2)[0]
			if len(msg) > 120 {
				msg = msg[:117] + "..."
			}
			sha := c.SHA
			if len(sha) > 7 {
				sha = sha[:7]
			}
			feed.Commits = append(feed.Commits, ghCommit{Repo: rp.Name, SHA: sha, Message: msg, At: c.Commit.Author.Date, URL: c.HTMLURL})
		}
	}
	sort.Slice(feed.Commits, func(i, j int) bool { return feed.Commits[i].At > feed.Commits[j].At })
	if len(feed.Commits) > 8 {
		feed.Commits = feed.Commits[:8]
	}
	return feed, nil
}

// ---------- Inquiries ----------

type inquiryIn struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Type    string `json:"type"`
	Budget  string `json:"budget"`
	When    string `json:"when"`
	Repo    string `json:"repo"`
	Message string `json:"message"`
	Source  string `json:"source"`
	Website string `json:"website"` // honeypot: real visitors never fill it
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		s = s[:n]
	}
	return s
}

// Inquiry handles POST /api/v1/site/inquiry.
func (h *SiteHandler) Inquiry(w http.ResponseWriter, r *http.Request) {
	var in inquiryIn
	if err := json.NewDecoder(io.LimitReader(r.Body, 32<<10)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "Send the form as JSON.")
		return
	}
	if in.Website != "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true}) // bot: accept silently, store nothing
		return
	}
	in.Name, in.Email, in.Message = clip(in.Name, 120), clip(in.Email, 200), clip(in.Message, 8000)
	if in.Name == "" || in.Message == "" {
		writeError(w, http.StatusBadRequest, "Add your name and what you want built.")
		return
	}
	if a, err := mail.ParseAddress(in.Email); err != nil || a.Address != in.Email {
		writeError(w, http.StatusBadRequest, "Check your email address.")
		return
	}
	if h.overDailyCap("site_inquiries", maxInquiriesPerDay) {
		writeError(w, http.StatusTooManyRequests, "We're receiving a lot of inquiries right now. Try again tomorrow.")
		return
	}
	src := "form"
	if in.Source == "scoper" {
		src = "scoper"
	}
	res, err := h.DB.Exec(`INSERT INTO site_inquiries (name, email, project_type, budget, timeline, repo, message, source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		in.Name, in.Email, clip(in.Type, 60), clip(in.Budget, 60), clip(in.When, 120), clip(in.Repo, 300), in.Message, src)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Your inquiry didn't save. Try again.")
		return
	}
	if h.SMTP.ready() {
		id, _ := res.LastInsertId()
		body := fmt.Sprintf("New project inquiry #%d (%s)\n\nName: %s\nEmail: %s\nType: %s\nBudget: %s\nWhen: %s\nRepo: %s\n\n%s\n",
			id, src, in.Name, in.Email, clip(in.Type, 60), clip(in.Budget, 60), clip(in.When, 120), clip(in.Repo, 300), in.Message)
		go h.SMTP.send(in.Email, fmt.Sprintf("New project inquiry #%d", id), body)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------- Encrypted applications ----------

type applicationIn struct {
	V       int    `json:"v"`
	Kid     string `json:"kid"`
	EK      string `json:"ek"`
	IV      string `json:"iv"`
	CT      string `json:"ct"`
	Website string `json:"website"`
}

var (
	reKid      = regexp.MustCompile(`^[0-9a-f]{16}$`)
	reRepoName = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
)

// Daily ceilings on stored rows. The per-IP limiter trusts proxy headers a caller can
// set, so these global caps are what stop a flood from filling the database.
const (
	maxInquiriesPerDay    = 300
	maxApplicationsPerDay = 100
)

func (h *SiteHandler) overDailyCap(table string, max int) bool {
	var n int
	if err := h.DB.Get(&n, "SELECT COUNT(*) FROM "+table+" WHERE created_at > datetime('now','-1 day')"); err != nil {
		return true // fail closed
	}
	return n >= max
}

func b64Len(s string) (int, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	return len(b), err
}

// Application handles POST /api/v1/site/application. It stores ciphertext only.
func (h *SiteHandler) Application(w http.ResponseWriter, r *http.Request) {
	var in applicationIn
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "Your application is too large or malformed. Send a CV under 5 MB.")
		return
	}
	if in.Website != "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	if in.V != 1 || !reKid.MatchString(in.Kid) {
		writeError(w, http.StatusBadRequest, "Unsupported application format. Reload the page and try again.")
		return
	}
	if n, err := b64Len(in.EK); err != nil || n != 512 { // RSA-4096 output
		writeError(w, http.StatusBadRequest, "Unsupported application format. Reload the page and try again.")
		return
	}
	if n, err := b64Len(in.IV); err != nil || n != 12 {
		writeError(w, http.StatusBadRequest, "Unsupported application format. Reload the page and try again.")
		return
	}
	if n, err := b64Len(in.CT); err != nil || n < 16 {
		writeError(w, http.StatusBadRequest, "Unsupported application format. Reload the page and try again.")
		return
	}
	if h.overDailyCap("site_applications", maxApplicationsPerDay) {
		writeError(w, http.StatusTooManyRequests, "We're receiving a lot of applications right now. Try again tomorrow.")
		return
	}
	_, err := h.DB.Exec(`INSERT INTO site_applications (version, kid, ek, iv, ct) VALUES (?, ?, ?, ?, ?)`,
		in.V, in.Kid, in.EK, in.IV, in.CT)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Your application didn't save. Try again.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------- Owner read access (bearer SITE_ADMIN_TOKEN) ----------

var errNoAdmin = errors.New("site admin disabled")

func (h *SiteHandler) authorized(r *http.Request) error {
	if len(h.AdminToken) < 32 {
		return errNoAdmin // unset or too short to be safe: read endpoints stay off
	}
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(got), []byte(h.AdminToken)) != 1 {
		return errors.New("unauthorized")
	}
	return nil
}

// ListInquiries handles GET /api/v1/site/admin/inquiries.
func (h *SiteHandler) ListInquiries(w http.ResponseWriter, r *http.Request) {
	if err := h.authorized(r); err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	rows := []map[string]any{}
	q, err := h.DB.Queryx(`SELECT id, name, email, project_type, budget, timeline, repo, message, source, CAST(created_at AS TEXT) AS created_at
		FROM site_inquiries ORDER BY id DESC LIMIT 200`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer q.Close()
	for q.Next() {
		m := map[string]any{}
		if q.MapScan(m) == nil {
			for k, v := range m {
				if b, ok := v.([]byte); ok {
					m[k] = string(b)
				}
			}
			rows = append(rows, m)
		}
	}
	writeJSON(w, http.StatusOK, rows)
}

// ListApplications handles GET /api/v1/site/admin/applications (ciphertext; open locally).
func (h *SiteHandler) ListApplications(w http.ResponseWriter, r *http.Request) {
	if err := h.authorized(r); err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	type row struct {
		ID        int    `db:"id" json:"id"`
		V         int    `db:"version" json:"v"`
		Kid       string `db:"kid" json:"kid"`
		EK        string `db:"ek" json:"ek"`
		IV        string `db:"iv" json:"iv"`
		CT        string `db:"ct" json:"ct"`
		CreatedAt string `db:"created_at" json:"created_at"`
	}
	rows := []row{}
	if err := h.DB.Select(&rows, `SELECT id, version, kid, ek, iv, ct, CAST(created_at AS TEXT) AS created_at FROM site_applications ORDER BY id DESC LIMIT 100`); err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

// ---------- Owner notification by email (optional) ----------

// SiteSMTP sends a copy of each inquiry to the owner. Disabled unless all fields are set.
type SiteSMTP struct {
	Host, User, Pass, To string // implicit TLS on port 465
}

func (s SiteSMTP) ready() bool { return s.Host != "" && s.User != "" && s.Pass != "" && s.To != "" }

// send runs in the background; failures are logged, never shown to the visitor (the row is already stored).
func (s SiteSMTP) send(replyTo, subject, body string) {
	if err := s.deliver("EmpowerTours site", s.To, replyTo, subject, body); err != nil {
		log.Printf("site smtp: %v", err)
	}
}

// deliver sends one plain-text message from our address (s.User) to `to`.
func (s SiteSMTP) deliver(fromName, to, replyTo, subject, body string) error {
	if strings.ContainsAny(fromName+to+replyTo+subject, "\r\n") {
		return errors.New("header value contains a line break") // never let visitor input reach a header line
	}
	msg := "From: " + fromName + " <" + s.User + ">\r\nTo: " + to + "\r\nReply-To: " + replyTo +
		"\r\nSubject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\nDate: " + time.Now().Format(time.RFC1123Z) +
		"\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" +
		strings.ReplaceAll(body, "\n", "\r\n")
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 15 * time.Second}, "tcp", net.JoinHostPort(s.Host, "465"), &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12})
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(30 * time.Second))
	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return fmt.Errorf("client: %w", err)
	}
	defer c.Close()
	steps := []func() error{
		func() error { return c.Auth(smtp.PlainAuth("", s.User, s.Pass, s.Host)) },
		func() error { return c.Mail(s.User) },
		func() error { return c.Rcpt(to) },
	}
	for _, f := range steps {
		if err := f(); err != nil {
			return err
		}
	}
	wc, err := c.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	if _, err := wc.Write([]byte(msg)); err != nil {
		wc.Close()
		return fmt.Errorf("write: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("data close: %w", err)
	}
	return c.Quit()
}
