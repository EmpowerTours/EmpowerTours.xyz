// Package music provides a read-only catalog of the artists' Music NFTs
// that are minted on the EmpowerTours Farcaster miniapp. It sources the
// catalog from the same Envio (HyperIndex) GraphQL indexer the miniapp uses,
// so the "friendly" streaming site at api.empowertours.xyz stays in sync with
// what artists publish, without duplicating any on-chain indexing.
//
// Artists are identified on-chain by wallet address, so the service resolves
// each artist's Farcaster display name (from their fid) for a friendly label,
// caching results since names rarely change.
package music

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"
)

// Song is one playable track in the catalog.
type Song struct {
	TokenID    string `json:"tokenId"`
	Name       string `json:"name"`
	Artist     string `json:"artist"`     // on-chain artist wallet address
	ArtistFid  string `json:"artistFid"`  // Farcaster ID of the artist
	ArtistName string `json:"artistName"` // resolved Farcaster display name ("" if unknown)
	AudioURL   string `json:"audioUrl"`   // resolved IPFS gateway URL (streamable)
	ImageURL   string `json:"imageUrl"`   // cover art URL
}

// Service fetches and caches the catalog.
type Service struct {
	src    *chainSource
	client *http.Client
	ttl    time.Duration

	mu       sync.Mutex
	cache    []Song
	fetchedA time.Time

	// artist fid -> display name, cached (names rarely change).
	nameMu  sync.Mutex
	names   map[string]nameEntry
	nameTTL time.Duration
}

type nameEntry struct {
	name string
	at   time.Time
}

// NewService creates a catalog service reading the v3 LicenseRegistry directly.
//
// It used to take an Envio GraphQL endpoint. That indexer was deleted and now
// answers 404, and because Songs() falls back to the last good cache the API
// went on serving a frozen catalog rather than failing — see chain.go.
//
// An unusable registry address is returned as an error rather than swallowed:
// a catalog service that cannot read the catalog should refuse to start, not
// come up and serve nothing.
func NewService(rpcURL, registry, gateway string) (*Service, error) {
	src, err := newChainSource(rpcURL, registry, gateway)
	if err != nil {
		return nil, err
	}
	return &Service{
		src:     src,
		client:  &http.Client{Timeout: 20 * time.Second},
		ttl:     30 * time.Second,
		names:   make(map[string]nameEntry),
		nameTTL: 6 * time.Hour,
	}, nil
}

// Songs returns the catalog, using a short-lived cache. On a fetch error it
// falls back to the last good cache (if any) so the player keeps working
// through transient indexer hiccups.
func (s *Service) Songs(ctx context.Context) ([]Song, error) {
	s.mu.Lock()
	if s.cache != nil && time.Since(s.fetchedA) < s.ttl {
		cached := s.cache
		s.mu.Unlock()
		return cached, nil
	}
	s.mu.Unlock()

	songs, err := s.fetch(ctx)
	if err != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.cache != nil {
			return s.cache, nil // serve stale rather than fail
		}
		return nil, err
	}

	s.mu.Lock()
	s.cache = songs
	s.fetchedA = time.Now()
	s.mu.Unlock()
	return songs, nil
}

// Endpoint describes where the catalog comes from. Named for the health
// handler that reports it; it is no longer a URL you can query by hand.
func (s *Service) Endpoint() string { return s.src.describe() }

// Song returns a single track by tokenId.
func (s *Service) Song(ctx context.Context, tokenID string) (*Song, bool, error) {
	songs, err := s.Songs(ctx)
	if err != nil {
		return nil, false, err
	}
	for i := range songs {
		if songs[i].TokenID == tokenID {
			return &songs[i], true, nil
		}
	}
	return nil, false, nil
}

func (s *Service) fetch(ctx context.Context) ([]Song, error) {
	songs, err := s.src.songs(ctx)
	if err != nil {
		return nil, err
	}

	// Newest first (tokenId descending, numeric when possible).
	sort.Slice(songs, func(i, j int) bool {
		a, errA := strconv.Atoi(songs[i].TokenID)
		b, errB := strconv.Atoi(songs[j].TokenID)
		if errA == nil && errB == nil {
			return a > b
		}
		return songs[i].TokenID > songs[j].TokenID
	})

	// Enrich with the artist's Farcaster display name (best effort, cached).
	for i := range songs {
		if name := s.resolveName(ctx, songs[i].ArtistFid); name != "" {
			songs[i].ArtistName = name
		}
	}

	return songs, nil
}

// resolveName maps a Farcaster fid to a friendly display name, cached for
// nameTTL. It never blocks the catalog: on any failure it returns "" and the
// caller falls back to a shortened wallet address.
func (s *Service) resolveName(ctx context.Context, fid string) string {
	if fid == "" || fid == "0" {
		return ""
	}

	s.nameMu.Lock()
	if e, ok := s.names[fid]; ok && time.Since(e.at) < s.nameTTL {
		s.nameMu.Unlock()
		return e.name
	}
	s.nameMu.Unlock()

	name := s.fetchName(ctx, fid)

	// Cache even empty results to avoid hammering the name servers for fids
	// that don't resolve.
	s.nameMu.Lock()
	s.names[fid] = nameEntry{name: name, at: time.Now()}
	s.nameMu.Unlock()
	return name
}

// fetchName tries the Warpcast user endpoint (friendly display name) first,
// then the official Farcaster fname server (handle) as a fallback. Both are
// public and require no API key.
func (s *Service) fetchName(ctx context.Context, fid string) string {
	if n := s.fetchWarpcastName(ctx, fid); n != "" {
		return n
	}
	return s.fetchFnameHandle(ctx, fid)
}

func (s *Service) fetchWarpcastName(ctx context.Context, fid string) string {
	u := "https://api.warpcast.com/v2/user-by-fid?fid=" + url.QueryEscape(fid)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return ""
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var out struct {
		Result struct {
			User struct {
				Username    string `json:"username"`
				DisplayName string `json:"displayName"`
			} `json:"user"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ""
	}
	if out.Result.User.DisplayName != "" {
		return out.Result.User.DisplayName
	}
	if out.Result.User.Username != "" {
		return "@" + out.Result.User.Username
	}
	return ""
}

func (s *Service) fetchFnameHandle(ctx context.Context, fid string) string {
	u := "https://fnames.farcaster.xyz/transfers?fid=" + url.QueryEscape(fid)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return ""
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var out struct {
		Transfers []struct {
			Username string `json:"username"`
			To       int64  `json:"to"`
		} `json:"transfers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ""
	}
	// The current username is the last transfer TO this fid.
	name := ""
	for _, t := range out.Transfers {
		if strconv.FormatInt(t.To, 10) == fid && t.Username != "" {
			name = t.Username
		}
	}
	if name != "" {
		return "@" + name
	}
	return ""
}
