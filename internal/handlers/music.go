package handlers

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"time"

	"github.com/empowertours/empowertours-app/internal/music"
	"github.com/go-chi/chi/v5"
)

// MusicHandler serves the public, no-auth music streaming surface: a JSON
// catalog API plus a friendly web player page. It is intentionally open so
// anyone can open api.empowertours.xyz and listen to the artists' music.
type MusicHandler struct {
	Svc        *music.Service
	StaticDir  string
	MiniappURL string // Farcaster mini app base URL, for the indexer drift check
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

// IndexerHealth cross-checks that this API and the Farcaster mini app are
// reading the SAME Envio indexer. If the indexer is redeployed, its URL rotates;
// if only one side's env var is updated they "drift" and the API silently shows
// a stale or empty catalog. This endpoint makes that visible (and returns 503 on
// confirmed drift so an uptime monitor can page).
// GET /api/v1/health/indexer
func (h *MusicHandler) IndexerHealth(w http.ResponseWriter, r *http.Request) {
	apiEndpoint := h.Svc.Endpoint()
	songs, _ := h.Svc.Songs(r.Context())
	apiCount := len(songs)

	// The mini app's debug route reports the indexer it uses + its song count.
	var mini struct {
		EnvioEndpoint string `json:"envioEndpoint"`
		SongsCount    int    `json:"songsCount"`
	}
	miniReached := false
	if h.MiniappURL != "" {
		client := &http.Client{Timeout: 15 * time.Second}
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet,
			h.MiniappURL+"/api/live-radio?action=debug-songs", nil)
		if err == nil {
			if resp, derr := client.Do(req); derr == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK &&
					json.NewDecoder(resp.Body).Decode(&mini) == nil {
					miniReached = true
				}
			}
		}
	}

	issues := []string{}
	critical := false
	if miniReached {
		if mini.EnvioEndpoint != "" && mini.EnvioEndpoint != apiEndpoint {
			issues = append(issues, "indexer_endpoint_mismatch")
			critical = true
		}
		if apiCount == 0 && mini.SongsCount > 0 {
			issues = append(issues, "api_catalog_empty")
			critical = true
		} else if apiCount != mini.SongsCount {
			// Usually just indexing lag or the 30s cache — a warning, not a page.
			issues = append(issues, "catalog_count_differs")
		}
	} else {
		issues = append(issues, "miniapp_unreachable")
	}

	status := http.StatusOK
	if critical {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, map[string]any{
		"ok":               !critical,
		"apiEndpoint":      apiEndpoint,
		"miniappEndpoint":  mini.EnvioEndpoint,
		"apiCount":         apiCount,
		"miniappCount":     mini.SongsCount,
		"miniappReachable": miniReached,
		"issues":           issues,
	})
}
