package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// PlaysHandler records and reports track play counts. This is ANALYTICS ONLY:
// recording a play never mints tokens, never pays a listener, and never touches
// the on-chain reward pools. Free-app plays are counted here purely for display;
// economic payouts remain on the web/miniapp where identity and anti-abuse
// controls exist. Keeping reward logic out of this path is deliberate — a fixed
// per-play reward driven by anonymous free plays would be trivially drained.
type PlaysHandler struct {
	DB *sqlx.DB
}

// RecordPlay increments the play counter for a track.
// POST /api/v1/plays/{tokenId}
func (h *PlaysHandler) RecordPlay(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "tokenId")
	if tokenID == "" || len(tokenID) > 64 {
		writeError(w, http.StatusBadRequest, "invalid tokenId")
		return
	}
	_, err := h.DB.Exec(`
		INSERT INTO music_plays (token_id, play_count, updated_at)
		VALUES (?, 1, CURRENT_TIMESTAMP)
		ON CONFLICT(token_id) DO UPDATE SET
			play_count = play_count + 1,
			updated_at = CURRENT_TIMESTAMP`, tokenID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not record play")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ListPlays returns play counts keyed by tokenId.
// GET /api/v1/plays
func (h *PlaysHandler) ListPlays(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Queryx(`SELECT token_id, play_count FROM music_plays`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load plays")
		return
	}
	defer rows.Close()

	counts := map[string]int64{}
	for rows.Next() {
		var id string
		var n int64
		if err := rows.Scan(&id, &n); err == nil {
			counts[id] = n
		}
	}
	w.Header().Set("Cache-Control", "public, max-age=15")
	writeJSON(w, http.StatusOK, map[string]any{"counts": counts})
}
