package handlers

import (
	"net/http"
	"path/filepath"

	"github.com/empowertours/empowertours-app/internal/music"
	"github.com/go-chi/chi/v5"
)

// MusicHandler serves the public, no-auth music streaming surface: a JSON
// catalog API plus a friendly web player page. It is intentionally open so
// anyone can open api.empowertours.xyz and listen to the artists' music.
type MusicHandler struct {
	Svc       *music.Service
	StaticDir string
}

// ListCatalog returns the full catalog of artists' songs.
// GET /api/v1/music
func (h *MusicHandler) ListCatalog(w http.ResponseWriter, r *http.Request) {
	songs, err := h.Svc.Songs(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "could not load catalog: "+err.Error())
		return
	}
	// Cache at the CDN/browser for 30s; the service also caches server-side.
	w.Header().Set("Cache-Control", "public, max-age=30")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"count": len(songs),
		"songs": songs,
	})
}

// GetSong returns a single track by tokenId.
// GET /api/v1/music/{tokenId}
func (h *MusicHandler) GetSong(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "tokenId")
	song, ok, err := h.Svc.Song(r.Context(), tokenID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "could not load catalog: "+err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "song not found")
		return
	}
	writeJSON(w, http.StatusOK, song)
}

// Stream redirects to the track's audio so clients get a stable, shareable
// stream URL that survives IPFS gateway changes.
// GET /stream/{tokenId}
func (h *MusicHandler) Stream(w http.ResponseWriter, r *http.Request) {
	tokenID := chi.URLParam(r, "tokenId")
	song, ok, err := h.Svc.Song(r.Context(), tokenID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "could not load catalog: "+err.Error())
		return
	}
	if !ok || song.AudioURL == "" {
		writeError(w, http.StatusNotFound, "song not found")
		return
	}
	http.Redirect(w, r, song.AudioURL, http.StatusFound)
}

// Player serves the friendly web player page.
// GET / and GET /listen
func (h *MusicHandler) Player(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, filepath.Join(h.StaticDir, "player.html"))
}
