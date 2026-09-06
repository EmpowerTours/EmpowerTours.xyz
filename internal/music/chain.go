package music

// The catalog, read straight from the v3 LicenseRegistry.
//
// WHY THIS REPLACED THE INDEXER
//
// This service used to query an Envio subgraph. That indexer was deleted on
// 2026-08-29 and its endpoint now answers 404, but the failure was invisible:
// Songs() falls back to the last good cache on a fetch error, so the API kept
// serving a frozen five-track catalog while the chain had ten. A stale answer
// and a correct one look identical from outside — the only clue was
// /api/v1/health/indexer reporting catalog_count_differs.
//
// The contracts are the only source that cannot drift from itself, so the
// catalog is assembled here the same way the mini app does it since dropping
// the same indexer: token ids and artists from the registry, then the name,
// cover and audio from the JSON document behind each tokenURI.
//
// Reads are per-master and unbatched. That is fine at this catalog's size —
// ten masters, four calls each — and the ceiling is a real one: when it stops
// being affordable the answer is an indexer again, not a contract change.
//
// A tokenURI is an IPFS CID and a CID is a hash of its content, so a resolved
// document can never become wrong. Metadata is therefore cached for the life of
// the process and never invalidated.

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// eth_call over plain JSON-RPC rather than go-ethereum's ethclient.
//
// ethclient pulls in the whole geth RPC stack — websockets, OpenTelemetry,
// gopsutil — none of which a service making forty read-only calls needs, and
// all of which would land in this API's image. accounts/abi does the only hard
// part, which is encoding and decoding; the transport is a POST.

// Only the five functions the catalog needs. Kept as a literal rather than a
// generated binding so this file is readable next to the mini app's
// REGISTRY_ABI, which lists exactly the same five.
const registryABIJSON = `[
 {"name":"totalMasters","type":"function","stateMutability":"view","inputs":[],"outputs":[{"type":"uint256"}]},
 {"name":"tokenURI","type":"function","stateMutability":"view","inputs":[{"type":"uint256"}],"outputs":[{"type":"string"}]},
 {"name":"masterSuspended","type":"function","stateMutability":"view","inputs":[{"type":"uint256"}],"outputs":[{"type":"bool"}]},
 {"name":"masterPurged","type":"function","stateMutability":"view","inputs":[{"type":"uint256"}],"outputs":[{"type":"bool"}]},
 {"name":"getMaster","type":"function","stateMutability":"view","inputs":[{"type":"uint256"}],"outputs":[
   {"name":"artist","type":"address"},{"name":"artistFid","type":"uint256"},{"name":"createdAt","type":"uint64"},
   {"name":"maxCollectorEditions","type":"uint32"},{"name":"collectorsMinted","type":"uint32"},{"name":"nftType","type":"uint8"},
   {"name":"referrer","type":"address"},{"name":"royaltyShareBps","type":"uint96"},{"name":"royaltyShareSink","type":"address"}]}
]`

// trackMetadata is the ERC-721 document behind a tokenURI. Only the three
// fields a listener sees are read; anything else in the document is ignored.
const (
	metadataAttempts   = 3
	metadataRetryDelay = 400 * time.Millisecond
)

type trackMetadata struct {
	Name         string `json:"name"`
	Image        string `json:"image"`
	AnimationURL string `json:"animation_url"`
	AudioURL     string `json:"audio_url"`
}

type chainSource struct {
	rpcURL   string
	registry common.Address
	gateway  string
	abi      abi.ABI
	client   *http.Client

	metaMu sync.Mutex
	meta   map[string]trackMetadata // tokenURI -> document, immutable once resolved
}

func newChainSource(rpcURL, registry, gateway string) (*chainSource, error) {
	parsed, err := abi.JSON(strings.NewReader(registryABIJSON))
	if err != nil {
		return nil, fmt.Errorf("parse registry abi: %w", err)
	}
	if !common.IsHexAddress(registry) {
		return nil, fmt.Errorf("license registry address %q is not an address", registry)
	}
	if !strings.HasSuffix(gateway, "/") {
		gateway += "/"
	}
	return &chainSource{
		rpcURL:   rpcURL,
		registry: common.HexToAddress(registry),
		gateway:  gateway,
		abi:      parsed,
		client:   &http.Client{Timeout: 20 * time.Second},
		meta:     make(map[string]trackMetadata),
	}, nil
}

func (c *chainSource) describe() string {
	return fmt.Sprintf("onchain:%s@%s", c.registry.Hex(), c.rpcURL)
}

type rpcReq struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      int           `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

type rpcResp struct {
	Result string `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// call performs one eth_call against the registry and unpacks the result.
func (c *chainSource) call(ctx context.Context, method string, args ...interface{}) ([]interface{}, error) {
	data, err := c.abi.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("pack %s: %w", method, err)
	}

	body, err := json.Marshal(rpcReq{
		JSONRPC: "2.0", ID: 1, Method: "eth_call",
		Params: []interface{}{
			map[string]string{"to": c.registry.Hex(), "data": "0x" + hex.EncodeToString(data)},
			"latest",
		},
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.rpcURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call %s: %w", method, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("call %s: rpc status %d", method, resp.StatusCode)
	}

	var parsed rpcResp
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode %s: %w", method, err)
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("call %s: rpc error %d: %s", method, parsed.Error.Code, parsed.Error.Message)
	}

	raw, err := hex.DecodeString(strings.TrimPrefix(parsed.Result, "0x"))
	if err != nil {
		return nil, fmt.Errorf("decode %s result: %w", method, err)
	}
	// A reverted or non-existent call returns "0x". Unpacking that yields a
	// confusing error, so name the real problem.
	if len(raw) == 0 {
		return nil, fmt.Errorf("call %s returned no data (reverted or wrong address)", method)
	}

	vals, err := c.abi.Unpack(method, raw)
	if err != nil {
		return nil, fmt.Errorf("unpack %s: %w", method, err)
	}
	return vals, nil
}

// songs reads the whole catalog. Newest first, matching the order the indexer
// used to return and the order the player expects.
func (c *chainSource) songs(ctx context.Context) ([]Song, error) {
	vals, err := c.call(ctx, "totalMasters")
	if err != nil {
		return nil, err
	}
	total, ok := vals[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("totalMasters returned %T", vals[0])
	}

	songs := make([]Song, 0, total.Int64())
	seen := make(map[string]bool, total.Int64())
	for i := total.Int64(); i >= 1; i-- {
		id := big.NewInt(i)

		// A suspended or purged master is not playable. Checked before the
		// metadata fetch so a takedown costs no gateway round trip.
		if b, err := c.call(ctx, "masterSuspended", id); err == nil {
			if v, _ := b[0].(bool); v {
				continue
			}
		}
		if b, err := c.call(ctx, "masterPurged", id); err == nil {
			if v, _ := b[0].(bool); v {
				continue
			}
		}

		m, err := c.call(ctx, "getMaster", id)
		if err != nil || len(m) < 2 {
			continue // one unreadable master must not empty the catalog
		}
		artist, _ := m[0].(common.Address)
		fid, _ := m[1].(*big.Int)

		u, err := c.call(ctx, "tokenURI", id)
		if err != nil || len(u) == 0 {
			continue
		}
		uri, _ := u[0].(string)
		if uri == "" {
			continue
		}

		md, ok := c.metadata(ctx, uri)
		if !ok {
			// A gateway hiccup here is NOT harmless. Dedupe runs newest-first,
			// so dropping master N lets the superseded copy of the same
			// recording through underneath it — which is how the player ends up
			// showing the deployer as the artist again. Logged rather than
			// swallowed, because the symptom (right song, wrong artist) looks
			// nothing like the cause.
			log.Printf("[music] master %d: metadata unresolved at %s - a superseded copy may surface in its place", i, uri)
			continue
		}
		audio := md.AnimationURL
		if audio == "" {
			audio = md.AudioURL
		}
		if audio == "" {
			// Nothing to play. Listing it would put a dead row in the player.
			continue
		}

		fidStr := ""
		if fid != nil && fid.Sign() > 0 {
			fidStr = fid.String()
		}

		resolved := c.resolveIPFS(audio)

		// One row per recording, newest master wins.
		//
		// The v3 migration re-minted the existing catalog, so several tracks
		// exist twice: once as the original master (whose artist is the
		// DEPLOYER, because the migration minted them) and once as the
		// artist's own republish. Both are live and unsuspended, so a straight
		// read of the registry lists the same song twice with two different
		// artists — and the copy the player happened to show was the wrong-
		// artist one.
		//
		// Deduped on the audio URI because that is the recording's identity: a
		// tokenURI is a content hash, so two masters pointing at the same audio
		// ARE the same recording, whatever their titles say. The scan runs from
		// the highest token id down, so the first copy seen is the newest and
		// later ones are skipped.
		//
		// This is a display rule, not a repair. The underlying fix is to
		// suspend the superseded masters on the registry — masterSuspended()
		// exists for exactly this — after which this loop would have nothing
		// to collapse.
		if seen[resolved] {
			continue
		}
		seen[resolved] = true

		songs = append(songs, Song{
			TokenID:   fmt.Sprintf("%d", i),
			Name:      md.Name,
			Artist:    strings.ToLower(artist.Hex()),
			ArtistFid: fidStr,
			AudioURL:  resolved,
			ImageURL:  c.resolveIPFS(md.Image),
		})
	}
	return songs, nil
}

// metadata resolves and permanently caches the document behind a tokenURI.
//
// Retried, because the cost of one failure is not "one missing track" — it is a
// superseded master surfacing in place of the newest one (see the call site).
// A CID is immutable, so a success can be cached for the life of the process; a
// failure is never cached, so a gateway that 504s recovers without a restart.
func (c *chainSource) metadata(ctx context.Context, uri string) (trackMetadata, bool) {
	c.metaMu.Lock()
	if md, ok := c.meta[uri]; ok {
		c.metaMu.Unlock()
		return md, true
	}
	c.metaMu.Unlock()

	var md trackMetadata
	var ok bool
	for attempt := 0; attempt < metadataAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return trackMetadata{}, false
			case <-time.After(time.Duration(attempt) * metadataRetryDelay):
			}
		}
		md, ok = c.fetchMetadata(ctx, uri)
		if ok {
			break
		}
	}
	if !ok {
		return trackMetadata{}, false
	}

	c.metaMu.Lock()
	c.meta[uri] = md
	c.metaMu.Unlock()
	return md, true
}

func (c *chainSource) fetchMetadata(ctx context.Context, uri string) (trackMetadata, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.resolveIPFS(uri), nil)
	if err != nil {
		return trackMetadata{}, false
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return trackMetadata{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return trackMetadata{}, false
	}
	var md trackMetadata
	if err := json.NewDecoder(resp.Body).Decode(&md); err != nil {
		return trackMetadata{}, false
	}
	return md, true
}

func (c *chainSource) resolveIPFS(u string) string {
	if u == "" {
		return ""
	}
	if strings.HasPrefix(u, "ipfs://") {
		return c.gateway + strings.TrimPrefix(u, "ipfs://")
	}
	return u
}
