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

// IndexerHealth cross-checks this API's catalog against the Farcaster mini
// app's.
//
// It was built to compare Envio endpoints, because a redeployed indexer rotated
// its URL and updating only one side left the API serving a stale catalog. Both
// sides now read the chain instead, so there is no endpoint left to drift — and
// the endpoint comparison below is kept only because a mini app that starts
// reporting one again should still be noticed.
//
// The COUNT comparison is the part that earned its keep: when the indexer was
// deleted and this API froze at five tracks against a ten-track chain, this was
// the only thing anywhere that said so, as catalog_count_differs. It was not
// wired to anything that pages. It should be.
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

	// Checked first and OUTSIDE the miniReached branch: an empty catalog is this
	// endpoint's whole reason to exist, and it is true whether or not the mini
	// app answers. Previously it could only be reported when the mini app was
	// reachable AND had songs, so the API serving nothing while the mini app was
	// down read as merely "miniapp_unreachable".
	if apiCount == 0 {
		issues = append(issues, "api_catalog_empty")
		critical = true
	}

	if miniReached {
		// Both sides read the chain now, so the mini app reports no endpoint and
		// this cannot fire. Left in place deliberately: if either side is ever
		// put back behind an indexer, a one-sided change must not be silent.
		if mini.EnvioEndpoint != "" && mini.EnvioEndpoint != apiEndpoint {
			issues = append(issues, "indexer_endpoint_mismatch")
			critical = true
		}
		if apiCount != mini.SongsCount {
			// Two chain readers disagreeing is not indexing lag any more — it is
			// a 30s cache boundary at worst, and a real divergence at best. Still
			// a warning rather than a page, because the mini app dedupes
			// differently and an off-by-one here is expected.
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
