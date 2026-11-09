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
	"errors"
	"fmt"
	"log"
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
	ArtistName string `json:"artistName"` // resolved display name ("" if unknown)
	// Where ArtistName came from: "farcaster", "profile", or "" when unresolved.
	//
	// These are NOT equivalent claims and a client must not present them as one.
	// A Farcaster handle is tied to an account; a ProfileRegistry name is open,
	// first-come registration and the contract warns homoglyphs are registerable.
	// So a "profile" name should be shown WITH the address, a "farcaster" one need
	// not be.
	ArtistNameSource string `json:"artistNameSource,omitempty"`
	AudioURL         string `json:"audioUrl"` // resolved IPFS gateway URL (streamable)
	ImageURL         string `json:"imageUrl"` // cover art URL
}

// Service fetches and caches the catalog.
type Service struct {
	src    *chainSource
	client *http.Client
	ttl    time.Duration

	mu    sync.Mutex
	cache snapshot

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

// snapshot is a cached catalog and how it was obtained. complete is what makes
// it trustworthy: a partial scan is real data but not the catalog.
type snapshot struct {
	songs    []Song
	at       time.Time
	complete bool
}

// ErrCatalogUnavailable is returned when no scan has produced a usable catalog.
// The handler turns it into a 502, which the apps show as "couldn't reach the
// studio" - true - instead of "no tracks yet", which was false.
var ErrCatalogUnavailable = errors.New("catalog unavailable: no complete scan and the partial one found nothing")

// Songs returns the catalog, using a short-lived cache. On a fetch error it
// falls back to the last good cache (if any) so the player keeps working
// through transient RPC hiccups.
func (s *Service) Songs(ctx context.Context) ([]Song, error) {
	s.mu.Lock()
	if s.cache.songs != nil && time.Since(s.cache.at) < s.ttl {
		cached := s.cache.songs
		s.mu.Unlock()
		return cached, nil
	}
	s.mu.Unlock()

	songs, err := s.fetch(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()
	serve, next, serveErr := choose(s.cache, time.Now(), songs, err)
	if err != nil {
		log.Printf("[music] %v - serving %d tracks (cache complete=%v)", err, len(serve), next.complete)
	}
	s.cache = next
	if serveErr != nil {
		return nil, serveErr
	}
	return serve, nil
}

// maxStale bounds how long a complete catalog is preferred over a partial
// scan. Without a bound, one master whose metadata is gone for good would
// freeze the catalog at its last complete state forever.
const maxStale = 10 * time.Minute

// choose decides what a request is served after a scan, and what is cached.
//
//   - complete scan: serve and cache it, marked complete.
//   - partial scan, complete cache younger than maxStale: serve the cache and
//     leave it untouched; its age is not refreshed, so the next request rescans.
//   - partial scan otherwise: serve whichever of the fresh scan and a cache
//     younger than maxStale holds more tracks; a fresh partial is cached as
//     partial. A partial cache
//     never outranks a complete scan, and an earlier empty partial can no
//     longer pin the catalog at zero - which is how "no tracks yet" stuck for
//     ten minutes at a time on 2026-11-09.
//   - partial scan that found nothing, with nothing better cached: an error,
//     not an empty catalog. Zero tracks from a scan that could not read the
//     registry is not an answer.
//   - failed scan: serve any cache, however old; else an error.
func choose(cache snapshot, now time.Time, fresh []Song, err error) (serve []Song, next snapshot, serveErr error) {
	if err == nil {
		return fresh, snapshot{songs: fresh, at: now, complete: true}, nil
	}

	var inc *IncompleteError
	if !errors.As(err, &inc) {
		if cache.songs != nil {
			return cache.songs, cache, nil
		}
		return nil, cache, err
	}

	if cache.complete && cache.songs != nil && now.Sub(cache.at) < maxStale {
		return cache.songs, cache, nil
	}
	// A cache that wins keeps its original age, so it ages out at maxStale
	// like any other: a track taken down on-chain cannot be kept alive by a
	// run of partial scans that each happen to read fewer masters.
	if now.Sub(cache.at) < maxStale && len(cache.songs) > len(fresh) {
		return cache.songs, cache, nil
	}
	if len(fresh) == 0 {
		// Cache nothing: the next request must rescan, not serve this.
		return nil, snapshot{}, fmt.Errorf("%w (%v)", ErrCatalogUnavailable, err)
	}
	return fresh, snapshot{songs: fresh, at: now, complete: false}, nil
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
	var inc *IncompleteError
	if err != nil && !errors.As(err, &inc) {
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

	// Name the artist, in descending strength of claim:
	//
	//   1. Farcaster, by the artistFid the master carries.
	//   2. ProfileRegistry, by the artist's address.
	//   3. nothing — the client shortens the address.
	//
	// Tier 2 was missing entirely, which is why a track minted from a wallet with
	// no Farcaster account rendered as a bare hex address in the API, the web
	// player and the mobile app alike. The mini app has had this order for a
	// while; the API only ever asked Farcaster.
	for i := range songs {
		if name := s.resolveName(ctx, songs[i].ArtistFid); name != "" {
			songs[i].ArtistName = name
			songs[i].ArtistNameSource = "farcaster"
			continue
		}
		if name := s.src.displayName(ctx, songs[i].Artist); name != "" {
			songs[i].ArtistName = name
			songs[i].ArtistNameSource = "profile"
		}
	}

	return songs, err
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
