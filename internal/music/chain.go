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
	"os"
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

// trackMetadata is the ERC-721 document behind a tokenURI. Only the fields a
// listener sees are read; anything else in the document is ignored.
const (
	metadataAttempts   = 3
	metadataRetryDelay = 400 * time.Millisecond
)

type trackMetadata struct {
	Name         string `json:"name"`
	Image        string `json:"image"`
	AnimationURL string `json:"animation_url"`
	AudioURL     string `json:"audio_url"`
	ExternalURL  string `json:"external_url"`
}

type chainSource struct {
	rpcURL     string
	registry   common.Address
	profiles   common.Address
	gateway    string
	abi        abi.ABI
	profileAbi abi.ABI
	client     *http.Client

	metaMu sync.Mutex
	meta   map[string]trackMetadata // tokenURI -> document, immutable once resolved
}

// The ProfileRegistry's one function this needs. A name registered here is NOT a
// Farcaster handle: registration is open and first-come, and the contract's own
// comment warns that homoglyphs are registerable. So the source of a name is
// reported alongside it (Song.ArtistNameSource) and a client showing a
// "profile" name should show the address too.
const profileABIJSON = `[
 {"name":"displayNameOf","type":"function","stateMutability":"view","inputs":[{"type":"address"}],"outputs":[{"type":"string"}]}
]`

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
	profiles := os.Getenv("PROFILE_REGISTRY_ADDRESS")
	if profiles == "" {
		profiles = "0xf4C27308f2183E7Cb07c32FAF449a259831E16EC"
	}
	profileAbi, err := abi.JSON(strings.NewReader(profileABIJSON))
	if err != nil {
		return nil, fmt.Errorf("parse profile abi: %w", err)
	}

	return &chainSource{
		rpcURL:     rpcURL,
		profiles:   common.HexToAddress(profiles),
		profileAbi: profileAbi,
		registry:   common.HexToAddress(registry),
		gateway:    gateway,
		abi:        parsed,
		client:     &http.Client{Timeout: 20 * time.Second},
		meta:       make(map[string]trackMetadata),
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
	return c.callAt(ctx, c.registry, c.abi, method, args...)
}

// callAt is the same eth_call against an arbitrary contract, so the catalog and
// the profile lookup share one transport rather than growing a second copy.
//
// Transient failures are retried with backoff. The public RPC answers a burst
// with HTTP 429, and Railway's egress IP is shared, so a scan of ~44 sequential
// calls used to lose most of them: on 2026-11-09 the API served "no tracks yet"
// while every master was readable from anywhere else. A revert or a malformed
// answer is not transient and fails at once.
func (c *chainSource) callAt(ctx context.Context, to common.Address, a abi.ABI, method string, args ...interface{}) ([]interface{}, error) {
	data, err := a.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("pack %s: %w", method, err)
	}

	var lastErr error
	for attempt := 0; ; attempt++ {
		vals, retry, err := c.callOnce(ctx, to, a, method, data)
		if err == nil {
			return vals, nil
		}
		lastErr = err
		if !retry || attempt >= len(rpcRetryDelays) {
			break
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%w (gave up: %v)", lastErr, ctx.Err())
		case <-time.After(rpcRetryDelays[attempt]):
		}
	}
	return nil, lastErr
}

// rpcRetryDelays is the wait before each retry of a transient RPC failure.
// Worst case adds ~2s to one call; a scan is cached, so listeners never wait on
// it more than once per TTL.
var rpcRetryDelays = []time.Duration{250 * time.Millisecond, 600 * time.Millisecond, 1200 * time.Millisecond}

// retryableRPCError reports whether a JSON-RPC error object is a rate limit or
// an overloaded node rather than a real answer (a revert, a bad argument).
func retryableRPCError(code int, msg string) bool {
	switch code {
	case 429, -32005, -32007: // too many requests / limit exceeded
		return true
	}
	m := strings.ToLower(msg)
	return strings.Contains(m, "rate limit") || strings.Contains(m, "too many") || strings.Contains(m, "limit exceeded")
}

// callOnce is a single eth_call. retry reports whether the failure is
// transient and worth another attempt.
func (c *chainSource) callOnce(ctx context.Context, to common.Address, a abi.ABI, method string, data []byte) (vals []interface{}, retry bool, err error) {

	body, err := json.Marshal(rpcReq{
		JSONRPC: "2.0", ID: 1, Method: "eth_call",
		Params: []interface{}{
			map[string]string{"to": to.Hex(), "data": "0x" + hex.EncodeToString(data)},
			"latest",
		},
	})
	if err != nil {
		return nil, false, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.rpcURL, bytes.NewReader(body))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("call %s: %w", method, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		retry := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return nil, retry, fmt.Errorf("call %s: rpc status %d", method, resp.StatusCode)
	}

	var parsed rpcResp
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, true, fmt.Errorf("decode %s: %w", method, err)
	}
	if parsed.Error != nil {
		return nil, retryableRPCError(parsed.Error.Code, parsed.Error.Message),
			fmt.Errorf("call %s: rpc error %d: %s", method, parsed.Error.Code, parsed.Error.Message)
	}

	raw, err := hex.DecodeString(strings.TrimPrefix(parsed.Result, "0x"))
	if err != nil {
		return nil, false, fmt.Errorf("decode %s result: %w", method, err)
	}
	// A reverted or non-existent call returns "0x". Unpacking that yields a
	// confusing error, so name the real problem.
	if len(raw) == 0 {
		return nil, false, fmt.Errorf("call %s returned no data (reverted or wrong address)", method)
	}

	vals, err = a.Unpack(method, raw)
	if err != nil {
		return nil, false, fmt.Errorf("unpack %s: %w", method, err)
	}
	return vals, false, nil
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
	unread := 0 // masters skipped because a read failed, not because they are unplayable
	// skip records a master lost to a failed read. The error is logged because
	// a count alone ("15 master(s) unreadable") cannot tell a rate limit from a
	// wrong address from a dead gateway.
	skip := func(id int64, what string, err error) {
		unread++
		log.Printf("[music] master %d: %s unreadable: %v", id, what, err)
	}
	for i := total.Int64(); i >= 1; i-- {
		id := big.NewInt(i)

		// A suspended or purged master is not playable. Checked before the
		// metadata fetch so a takedown costs no gateway round trip.
		b, err := c.call(ctx, "masterSuspended", id)
		if err != nil || len(b) == 0 {
			skip(i, "masterSuspended", err)
			continue
		}
		if v, _ := b[0].(bool); v {
			continue
		}
		b, err = c.call(ctx, "masterPurged", id)
		if err != nil || len(b) == 0 {
			skip(i, "masterPurged", err)
			continue
		}
		if v, _ := b[0].(bool); v {
			continue
		}

		m, err := c.call(ctx, "getMaster", id)
		if err != nil || len(m) < 2 {
			skip(i, "getMaster", err)
			continue // one unreadable master must not empty the catalog
		}
		artist, _ := m[0].(common.Address)
		fid, _ := m[1].(*big.Int)

		u, err := c.call(ctx, "tokenURI", id)
		if err != nil || len(u) == 0 {
			skip(i, "tokenURI", err)
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
			unread++
			continue
		}
		// external_url is the full recording; animation_url is the 3-second
		// preview that marketplaces and cast embeds show. Preferring the
		// preview shipped every track to the apps as a 3-second clip.
		audio := md.ExternalURL
		if audio == "" {
			audio = md.AudioURL
		}
		if audio == "" {
			audio = md.AnimationURL
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
	if unread > 0 {
		return songs, &IncompleteError{Unread: unread}
	}
	return songs, nil
}

// IncompleteError reports a scan that finished but skipped masters it could
// not read (RPC or gateway failure). The songs returned alongside it are real
// but partial. Treating that as success is how the app came up with three of
// seven tracks: the short list was cached and served like a full one.
type IncompleteError struct{ Unread int }

func (e *IncompleteError) Error() string {
	return fmt.Sprintf("catalog scan incomplete: %d master(s) unreadable", e.Unread)
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

// displayName returns the name this address registered on the ProfileRegistry,
// or "" if it has none. This is the tier that was missing: an artist who minted
// from a wallet with no Farcaster account had no name anywhere, so every client
// fell through to a bare hex address — which is what "Ganado" showed instead of
// "Earvin Gallardo".
func (c *chainSource) displayName(ctx context.Context, addr string) string {
	if !common.IsHexAddress(addr) {
		return ""
	}
	vals, err := c.callAt(ctx, c.profiles, c.profileAbi, "displayNameOf", common.HexToAddress(addr))
	if err != nil || len(vals) == 0 {
		return ""
	}
	name, _ := vals[0].(string)
	return strings.TrimSpace(name)
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
