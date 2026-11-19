package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// HireGitHub is the "EmpowerTours Builder" GitHub App. The API only needs the
// app's OAuth client (sign-in) and webhook secret (merge detection). The app's
// private key, used to push branches and open pull requests, never lives here.
type HireGitHub struct {
	Slug          string // github.com/apps/<slug>
	ClientID      string
	ClientSecret  string
	WebhookSecret string
	CallbackURL   string // https://api.empowertours.xyz/api/v1/hire/auth/callback
	APIBase       string // https://api.github.com (tests override)
	WebBase       string // https://github.com (tests override)
}

func (g HireGitHub) Enabled() bool {
	return g.Slug != "" && g.ClientID != "" && g.ClientSecret != "" && g.WebhookSecret != ""
}

func (g HireGitHub) InstallURL() string {
	if g.Slug == "" {
		return ""
	}
	return "https://github.com/apps/" + url.PathEscape(g.Slug) + "/installations/new"
}

func (g HireGitHub) api() string {
	if g.APIBase != "" {
		return g.APIBase
	}
	return "https://api.github.com"
}

func (g HireGitHub) web() string {
	if g.WebBase != "" {
		return g.WebBase
	}
	return "https://github.com"
}

const hireStateCookie = "et_hire_state"

// AuthStart sends the visitor to GitHub. ?to=install installs the app (repo
// picker, then sign-in); anything else just signs in.
func (h *HireHandler) AuthStart(w http.ResponseWriter, r *http.Request) {
	if !h.GitHub.Enabled() {
		writeError(w, http.StatusServiceUnavailable, "GitHub sign-in is not configured yet")
		return
	}
	state, err := randomToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "try again")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: hireStateCookie, Value: state, Path: "/api/v1/hire/auth", MaxAge: 600,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	target := h.GitHub.web() + "/login/oauth/authorize?" + url.Values{
		"client_id": {h.GitHub.ClientID}, "redirect_uri": {h.GitHub.CallbackURL}, "state": {state},
	}.Encode()
	if r.URL.Query().Get("to") == "install" {
		target = h.GitHub.InstallURL() + "?" + url.Values{"state": {state}}.Encode()
	}
	http.Redirect(w, r, target, http.StatusFound)
}

// AuthCallback finishes sign-in: checks state, swaps the code for a user token,
// records the user and the installations they can reach, then drops the token.
func (h *HireHandler) AuthCallback(w http.ResponseWriter, r *http.Request) {
	back := func(reason string) {
		http.Redirect(w, r, h.SiteURL+"?signin="+url.QueryEscape(reason), http.StatusFound)
	}
	c, err := r.Cookie(hireStateCookie)
	q := r.URL.Query()
	if err != nil || q.Get("state") == "" || subtle.ConstantTimeCompare([]byte(c.Value), []byte(q.Get("state"))) != 1 {
		back("expired")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: hireStateCookie, Value: "", Path: "/api/v1/hire/auth", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	code := q.Get("code")
	if code == "" || len(code) > 100 {
		back("denied")
		return
	}
	ctx := r.Context()
	tok, err := h.exchangeCode(ctx, code)
	if err != nil {
		log.Printf("hire oauth exchange: %v", err)
		back("failed")
		return
	}
	var u struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := h.ghGet(ctx, tok, "/user", &u); err != nil || u.ID == 0 {
		log.Printf("hire oauth user: %v", err)
		back("failed")
		return
	}
	var clientID int64
	err = h.DB.GetContext(ctx, &clientID, `INSERT INTO hire_clients (github_id, login, name, email) VALUES (?, ?, ?, ?)
		ON CONFLICT(github_id) DO UPDATE SET login = excluded.login, name = excluded.name,
		email = CASE WHEN excluded.email != '' THEN excluded.email ELSE hire_clients.email END RETURNING id`,
		u.ID, clip(u.Login, 64), clip(u.Name, 120), clip(u.Email, 254))
	if err != nil {
		log.Printf("hire client upsert: %v", err)
		back("failed")
		return
	}
	if err := h.syncInstallations(ctx, tok, u.ID); err != nil {
		log.Printf("hire installation sync: %v", err) // sign-in still works; webhooks fill repos in
	}
	if err := h.startSession(w, r, clientID); err != nil {
		back("failed")
		return
	}
	http.Redirect(w, r, h.SiteURL, http.StatusFound)
}

func (h *HireHandler) exchangeCode(ctx context.Context, code string) (string, error) {
	form := url.Values{"client_id": {h.GitHub.ClientID}, "client_secret": {h.GitHub.ClientSecret}, "code": {code}, "redirect_uri": {h.GitHub.CallbackURL}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, h.GitHub.web()+"/login/oauth/access_token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := h.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("no token (%s)", out.Error)
	}
	return out.AccessToken, nil
}

func (h *HireHandler) client() *http.Client {
	if h.Client != nil {
		return h.Client
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (h *HireHandler) ghGet(ctx context.Context, tok, path string, out any) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, h.GitHub.api()+path, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := h.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %d", path, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out)
}

type ghRepo struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
}

// syncInstallations records every installation of our app this user can reach
// and its repos. An installation already claimed by another user stays theirs.
func (h *HireHandler) syncInstallations(ctx context.Context, tok string, githubID int64) error {
	var inst struct {
		Installations []struct {
			ID      int64 `json:"id"`
			Account struct {
				Login string `json:"login"`
			} `json:"account"`
		} `json:"installations"`
	}
	if err := h.ghGet(ctx, tok, "/user/installations?per_page=100", &inst); err != nil {
		return err
	}
	for _, in := range inst.Installations {
		var rs struct {
			Repositories []ghRepo `json:"repositories"`
		}
		if err := h.ghGet(ctx, tok, "/user/installations/"+strconv.FormatInt(in.ID, 10)+"/repositories?per_page=100", &rs); err != nil {
			return err
		}
		if err := h.storeInstallation(ctx, in.ID, githubID, in.Account.Login, rs.Repositories, true); err != nil {
			return err
		}
	}
	return nil
}

var reFullName = regexp.MustCompile(`^[A-Za-z0-9-]{1,39}/[A-Za-z0-9._-]{1,100}$`)

func (h *HireHandler) storeInstallation(ctx context.Context, id, sender int64, account string, repos []ghRepo, replace bool) error {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO hire_installations (installation_id, sender_github_id, account_login) VALUES (?, ?, ?)
		ON CONFLICT(installation_id) DO UPDATE SET account_login = excluded.account_login, removed_at = NULL`, id, sender, clip(account, 64)); err != nil {
		return err
	}
	if replace {
		if _, err := tx.ExecContext(ctx, `DELETE FROM hire_repos WHERE installation_id = ?`, id); err != nil {
			return err
		}
	}
	if err := addRepos(ctx, tx, id, repos); err != nil {
		return err
	}
	return tx.Commit()
}

func addRepos(ctx context.Context, tx *sqlx.Tx, inst int64, repos []ghRepo) error {
	for _, r := range repos {
		if r.ID == 0 || !reFullName.MatchString(r.FullName) {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hire_repos (repo_id, installation_id, full_name) VALUES (?, ?, ?)
			ON CONFLICT(repo_id) DO UPDATE SET installation_id = excluded.installation_id, full_name = excluded.full_name`, r.ID, inst, r.FullName); err != nil {
			return err
		}
	}
	return nil
}

// ---------- webhook ----------

// verifySignature checks X-Hub-Signature-256 (HMAC-SHA256 of the raw body).
func (g HireGitHub) verifySignature(body []byte, header string) bool {
	if g.WebhookSecret == "" || !strings.HasPrefix(header, "sha256=") {
		return false
	}
	got, err := hex.DecodeString(strings.TrimPrefix(header, "sha256="))
	if err != nil {
		return false
	}
	m := hmac.New(sha256.New, []byte(g.WebhookSecret))
	m.Write(body)
	return hmac.Equal(got, m.Sum(nil))
}

var reTaskBranch = regexp.MustCompile(`^et/task-([0-9]{1,9})(?:-[a-z0-9-]{0,60})?$`)

type ghWebhook struct {
	Action       string `json:"action"`
	Installation struct {
		ID      int64 `json:"id"`
		Account struct {
			Login string `json:"login"`
		} `json:"account"`
	} `json:"installation"`
	Sender struct {
		ID int64 `json:"id"`
	} `json:"sender"`
	Repositories        []ghRepo `json:"repositories"`
	RepositoriesAdded   []ghRepo `json:"repositories_added"`
	RepositoriesRemoved []ghRepo `json:"repositories_removed"`
	Repository          ghRepo   `json:"repository"`
	PullRequest         struct {
		Number         int64  `json:"number"`
		HTMLURL        string `json:"html_url"`
		Merged         bool   `json:"merged"`
		MergeCommitSHA string `json:"merge_commit_sha"`
		Head           struct {
			Ref  string `json:"ref"`
			Repo ghRepo `json:"repo"`
		} `json:"head"`
	} `json:"pull_request"`
}

// Webhook receives GitHub App events: installs, repo selection changes, and
// pull requests on et/task-<id> branches (opened = in review, merged = payable).
func (h *HireHandler) Webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil || !h.GitHub.verifySignature(body, r.Header.Get("X-Hub-Signature-256")) {
		writeError(w, http.StatusUnauthorized, "bad signature")
		return
	}
	var ev ghWebhook
	if err := json.Unmarshal(body, &ev); err != nil {
		writeError(w, http.StatusBadRequest, "bad payload")
		return
	}
	ctx := r.Context()
	switch r.Header.Get("X-GitHub-Event") {
	case "installation":
		switch ev.Action {
		case "created", "new_permissions_accepted", "unsuspend":
			err = h.storeInstallation(ctx, ev.Installation.ID, ev.Sender.ID, ev.Installation.Account.Login, ev.Repositories, ev.Action == "created")
		case "deleted", "suspend":
			_, err = h.DB.ExecContext(ctx, `UPDATE hire_installations SET removed_at = ? WHERE installation_id = ?`, h.now(), ev.Installation.ID)
		}
	case "installation_repositories":
		err = h.changeRepos(ctx, ev)
	case "pull_request":
		err = h.onPullRequest(ctx, ev)
	}
	if err != nil {
		log.Printf("hire webhook %s/%s: %v", r.Header.Get("X-GitHub-Event"), ev.Action, err)
		writeError(w, http.StatusInternalServerError, "retry")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *HireHandler) changeRepos(ctx context.Context, ev ghWebhook) error {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var known int
	tx.GetContext(ctx, &known, `SELECT COUNT(*) FROM hire_installations WHERE installation_id = ?`, ev.Installation.ID)
	if known == 0 {
		if _, err := tx.ExecContext(ctx, `INSERT INTO hire_installations (installation_id, sender_github_id, account_login) VALUES (?, ?, ?)`,
			ev.Installation.ID, ev.Sender.ID, clip(ev.Installation.Account.Login, 64)); err != nil {
			return err
		}
	}
	if err := addRepos(ctx, tx, ev.Installation.ID, ev.RepositoriesAdded); err != nil {
		return err
	}
	for _, rp := range ev.RepositoriesRemoved {
		if _, err := tx.ExecContext(ctx, `DELETE FROM hire_repos WHERE repo_id = ? AND installation_id = ?`, rp.ID, ev.Installation.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (h *HireHandler) onPullRequest(ctx context.Context, ev ghWebhook) error {
	m := reTaskBranch.FindStringSubmatch(ev.PullRequest.Head.Ref)
	if m == nil {
		return nil // not one of ours
	}
	// The branch must live in the client's repo itself (we push there through the
	// app), so a fork can't move a task by naming a branch et/task-N.
	if ev.PullRequest.Head.Repo.ID != ev.Repository.ID {
		return nil
	}
	id, _ := strconv.ParseInt(m[1], 10, 64)
	var t struct {
		Stage     string `db:"stage"`
		ProjectID int64  `db:"project_id"`
	}
	err := h.DB.GetContext(ctx, &t, `SELECT t.stage, t.project_id FROM hire_tasks t JOIN hire_projects p ON p.id = t.project_id
		JOIN hire_repos r ON r.full_name = p.repo_full_name JOIN hire_installations i ON i.installation_id = r.installation_id
		JOIN hire_clients c ON c.id = p.client_id
		WHERE t.id = ? AND r.repo_id = ? AND i.installation_id = ? AND i.sender_github_id = c.github_id`, id, ev.Repository.ID, ev.Installation.ID)
	if err != nil {
		return nil // unknown task or repo mismatch: ignore, don't make GitHub retry
	}
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	pr := ev.PullRequest
	num := strconv.FormatInt(pr.Number, 10)
	prURL := ""
	if strings.HasPrefix(pr.HTMLURL, "https://github.com/") {
		prURL = pr.HTMLURL
	}
	switch {
	case ev.Action == "opened" || ev.Action == "reopened" || ev.Action == "ready_for_review" || (ev.Action == "closed" && pr.Merged):
		if t.Stage == StageApproved || t.Stage == StageBuilding {
			if ok, err := h.moveTask(ctx, tx, id, t.Stage, StageReview, `, pr_number = ?, pr_url = ?`, pr.Number, prURL); err != nil {
				return err
			} else if ok {
				h.event(ctx, tx, t.ProjectID, &id, "review", "Pull request #"+num+" opened for task "+m[1])
				t.Stage = StageReview
			}
		}
		if ev.Action == "closed" && pr.Merged && t.Stage == StageReview {
			sha := pr.MergeCommitSHA
			if !reHexSHA.MatchString(sha) {
				sha = ""
			}
			ok, err := h.moveTask(ctx, tx, id, StageReview, StageMerged, `, pr_number = ?, pr_url = ?, merged_at = ?, merge_sha = ?`, pr.Number, prURL, h.now(), sha)
			if err != nil {
				return err
			}
			if ok {
				h.event(ctx, tx, t.ProjectID, &id, "merged", "You merged #"+num+"; task "+m[1]+" is ready to pay")
			}
		}
	case ev.Action == "closed" && !pr.Merged && t.Stage == StageReview:
		if ok, err := h.moveTask(ctx, tx, id, StageReview, StageBuilding, ""); err != nil {
			return err
		} else if ok {
			h.event(ctx, tx, t.ProjectID, &id, "building", "Pull request #"+num+" closed without merging; back to building")
		}
	}
	return tx.Commit()
}

var reHexSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)
