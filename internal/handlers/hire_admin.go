package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Admin endpoints (Bearer SITE_ADMIN_TOKEN): we read every project, quote
// tasks, move work to "building", and record peso payments (SPEI/CoDi) that
// arrive in the bank, until the bank integration does it automatically.

func (h *HireHandler) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.adminOK(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *HireHandler) AdminProjects(w http.ResponseWriter, r *http.Request) {
	type row struct {
		hireProject
		Client string `db:"login" json:"client"`
	}
	rows := []row{}
	if err := h.DB.SelectContext(r.Context(), &rows, `SELECT p.id, p.client_id, p.repo_full_name, p.title, p.brief, c.login
		FROM hire_projects p JOIN hire_clients c ON c.id = p.client_id ORDER BY p.id DESC LIMIT 200`); err != nil {
		writeError(w, http.StatusInternalServerError, "could not load")
		return
	}
	for i := range rows {
		rows[i].Tasks, _ = h.loadTasks(r.Context(), rows[i].ID)
		rows[i].Events = []hireEvent{}
	}
	writeJSON(w, http.StatusOK, rows)
}

// AdminCreateTask quotes a new task on a project.
func (h *HireHandler) AdminCreateTask(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ProjectID  int64    `json:"project_id"`
		Title      string   `json:"title"`
		What       string   `json:"what"`
		Acceptance []string `json:"acceptance"`
		PriceMXN   int64    `json:"price_mxn"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || in.PriceMXN <= 0 || in.PriceMXN > 10_000_000 || len(in.Acceptance) == 0 || len(in.Acceptance) > 12 {
		writeError(w, http.StatusBadRequest, "title, a price above zero, and 1-12 acceptance checks")
		return
	}
	for i, a := range in.Acceptance {
		in.Acceptance[i] = clip(strings.TrimSpace(a), 300)
	}
	ac, _ := json.Marshal(in.Acceptance)
	tx, err := h.DB.BeginTxx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "try again")
		return
	}
	defer tx.Rollback()
	var exists int
	tx.GetContext(r.Context(), &exists, `SELECT COUNT(*) FROM hire_projects WHERE id = ?`, in.ProjectID)
	if exists == 0 {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	res, err := tx.ExecContext(r.Context(), `INSERT INTO hire_tasks (project_id, kind, title, what, acceptance, price_mxn, stage, created_at, updated_at)
		VALUES (?, 'feature', ?, ?, ?, ?, 'quoted', ?, ?)`, in.ProjectID, clip(in.Title, 160), clip(strings.TrimSpace(in.What), 2000), string(ac), in.PriceMXN, h.now(), h.now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not save")
		return
	}
	id, _ := res.LastInsertId()
	h.event(r.Context(), tx, in.ProjectID, &id, "quoted", "New task "+strconv.FormatInt(id, 10)+" quoted at "+mxnText(in.PriceMXN))
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "branch": "et/task-" + strconv.FormatInt(id, 10)})
}

// AdminQuoteBug prices a reported bug so the client can approve it.
func (h *HireHandler) AdminQuoteBug(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var in struct {
		PriceMXN   int64    `json:"price_mxn"`
		Acceptance []string `json:"acceptance"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384)).Decode(&in) != nil || in.PriceMXN <= 0 || in.PriceMXN > 10_000_000 || len(in.Acceptance) == 0 || len(in.Acceptance) > 12 {
		writeError(w, http.StatusBadRequest, "a price above zero and 1-12 acceptance checks")
		return
	}
	for i, a := range in.Acceptance {
		in.Acceptance[i] = clip(strings.TrimSpace(a), 300)
	}
	ac, _ := json.Marshal(in.Acceptance)
	h.adminMove(w, r, id, StageBug, StageQuoted, `, price_mxn = ?, acceptance = ?`, []any{in.PriceMXN, string(ac)}, "Bug task "+strconv.FormatInt(id, 10)+" quoted at "+mxnText(in.PriceMXN))
}

// AdminStage moves an approved task to building, or cancels an unstarted one.
// Review and merged come only from GitHub; paid only from a payment.
func (h *HireHandler) AdminStage(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var in struct{ From, To string }
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in) != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if in.To != StageBuilding && in.To != StageCancelled {
		writeError(w, http.StatusBadRequest, "admin can only set building or cancelled")
		return
	}
	h.adminMove(w, r, id, in.From, in.To, "", nil, "Task "+strconv.FormatInt(id, 10)+": "+in.To)
}

// AdminPesoPaid records a SPEI/CoDi payment seen in the bank for a merged task.
func (h *HireHandler) AdminPesoPaid(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var in struct {
		Method    string `json:"method"`
		Reference string `json:"reference"` // SPEI clave de rastreo / CoDi reference
		AmountMXN int64  `json:"amount_mxn"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&in) != nil || (in.Method != "spei" && in.Method != "codi") {
		writeError(w, http.StatusBadRequest, "method spei or codi, a reference and the amount")
		return
	}
	in.Reference = strings.TrimSpace(in.Reference)
	if in.Reference == "" || len(in.Reference) > 64 {
		writeError(w, http.StatusBadRequest, "the bank reference (clave de rastreo)")
		return
	}
	var price *int64
	var projectID int64
	row := h.DB.QueryRowxContext(r.Context(), `SELECT price_mxn, project_id FROM hire_tasks WHERE id = ? AND stage = 'merged'`, id)
	if row.Scan(&price, &projectID) != nil || price == nil {
		writeError(w, http.StatusConflict, "task is not merged and waiting for payment")
		return
	}
	if in.AmountMXN != *price {
		writeError(w, http.StatusConflict, "amount must equal the task price "+mxnText(*price))
		return
	}
	tx, err := h.DB.BeginTxx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "try again")
		return
	}
	defer tx.Rollback()
	ok, err := h.moveTask(r.Context(), tx, id, StageMerged, StagePaid, `, paid_at = ?`, h.now())
	if err != nil || !ok {
		writeError(w, http.StatusConflict, "task is not waiting for payment")
		return
	}
	if _, err := tx.ExecContext(r.Context(), `INSERT INTO hire_payments (task_id, method, amount_mxn, reference, status, confirmed_at, created_at) VALUES (?, ?, ?, ?, 'confirmed', ?, ?)`,
		id, in.Method, in.AmountMXN, in.Reference, h.now(), h.now()); err != nil {
		writeError(w, http.StatusConflict, "this task already has a payment")
		return
	}
	tx.ExecContext(r.Context(), `UPDATE hire_payments SET status = 'expired' WHERE task_id = ? AND status = 'pending'`, id)
	h.event(r.Context(), tx, projectID, &id, "paid", "Task "+strconv.FormatInt(id, 10)+" paid by "+strings.ToUpper(in.Method))
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "try again")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"stage": StagePaid})
}

func (h *HireHandler) adminMove(w http.ResponseWriter, r *http.Request, id int64, from, to, extra string, args []any, msg string) {
	tx, err := h.DB.BeginTxx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "try again")
		return
	}
	defer tx.Rollback()
	var projectID int64
	if tx.GetContext(r.Context(), &projectID, `SELECT project_id FROM hire_tasks WHERE id = ?`, id) != nil {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	ok, err := h.moveTask(r.Context(), tx, id, from, to, extra, args...)
	if err != nil || !ok {
		writeError(w, http.StatusConflict, "task is not in stage "+from+", or "+from+" -> "+to+" is not allowed")
		return
	}
	h.event(r.Context(), tx, projectID, &id, to, msg)
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "try again")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"stage": to})
}
