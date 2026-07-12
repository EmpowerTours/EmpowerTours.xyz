// Package music provides a read-only catalog of the artists' Music NFTs
// that are minted on the EmpowerTours Farcaster miniapp. It sources the
// catalog from the same Envio (HyperIndex) GraphQL indexer the miniapp uses,
// so the "friendly" streaming site at api.empowertours.xyz stays in sync with
// what artists publish, without duplicating any on-chain indexing.
package music

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"
)

// Song is one playable track in the catalog.
type Song struct {
	TokenID   string `json:"tokenId"`
	Name      string `json:"name"`
	Artist    string `json:"artist"`    // on-chain artist wallet address
	ArtistFid string `json:"artistFid"` // Farcaster ID of the artist
	AudioURL  string `json:"audioUrl"`  // resolved IPFS gateway URL (streamable)
	ImageURL  string `json:"imageUrl"`  // cover art URL
}

// Service fetches and caches the catalog.
type Service struct {
	endpoint string
	client   *http.Client
	ttl      time.Duration

	mu       sync.Mutex
	cache    []Song
	fetchedA time.Time
}

// NewService creates a catalog service for the given Envio GraphQL endpoint.
func NewService(endpoint string) *Service {
	return &Service{
		endpoint: endpoint,
		client:   &http.Client{Timeout: 20 * time.Second},
		ttl:      30 * time.Second,
	}
}

// graphQL request/response shapes for the MusicNFT entity.
const catalogQuery = `query GetMusicNFTs {
  MusicNFT(where: {isBurned: {_eq: false}}, limit: 200) {
    tokenId
    name
    artist
    artistFid
    fullAudioUrl
    imageUrl
  }
}`

type gqlRequest struct {
	Query string `json:"query"`
}

type gqlResponse struct {
	Data struct {
		MusicNFT []struct {
			TokenID      string `json:"tokenId"`
			Name         string `json:"name"`
			Artist       string `json:"artist"`
			ArtistFid    string `json:"artistFid"`
			FullAudioURL string `json:"fullAudioUrl"`
			ImageURL     string `json:"imageUrl"`
		} `json:"MusicNFT"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
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
	body, _ := json.Marshal(gqlRequest{Query: catalogQuery})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("indexer request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("indexer returned status %d", resp.StatusCode)
	}

	var parsed gqlResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode indexer response: %w", err)
	}
	if len(parsed.Errors) > 0 {
		return nil, fmt.Errorf("indexer error: %s", parsed.Errors[0].Message)
	}

	songs := make([]Song, 0, len(parsed.Data.MusicNFT))
	for _, m := range parsed.Data.MusicNFT {
		audio := m.FullAudioURL
		if audio == "" {
			continue // no playable audio -> skip
		}
		songs = append(songs, Song{
			TokenID:   m.TokenID,
			Name:      m.Name,
			Artist:    m.Artist,
			ArtistFid: m.ArtistFid,
			AudioURL:  audio,
			ImageURL:  m.ImageURL,
		})
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

	return songs, nil
}
