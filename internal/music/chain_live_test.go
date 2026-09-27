package music

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Hits Monad mainnet. Skipped with -short.
func TestChainSourceLive(t *testing.T) {
	if testing.Short() {
		t.Skip("network test")
	}
	src, err := newChainSource(
		"https://rpc.monad.xyz",
		"0x42EbcD44C2295702130f0A641633c691bA5f9480",
		"https://harlequin-used-hare-224.mypinata.cloud/ipfs/",
	)
	if err != nil {
		t.Fatalf("newChainSource: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	songs, err := src.songs(ctx)
	if err != nil {
		t.Fatalf("songs: %v", err)
	}
	t.Logf("catalog: %d playable tracks", len(songs))
	for _, s := range songs {
		t.Logf("  #%s %-24q artist=%s fid=%s audio=%.60s", s.TokenID, s.Name, s.Artist, s.ArtistFid, s.AudioURL)
	}
	if len(songs) == 0 {
		t.Fatal("catalog is empty - the whole point of this change was that it should not be")
	}
	// Every track must be the full recording, not the 3-second preview. The
	// previews are 576,044-byte WAVs; the shortest full track is ~5 MB. This
	// is the check that would have caught the catalog serving clips.
	const minFullTrackBytes = 1 << 20
	for _, s := range songs {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.AudioURL, nil)
		req.Header.Set("Range", "bytes=0-0")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("#%s %q: fetch audio: %v", s.TokenID, s.Name, err)
			continue
		}
		resp.Body.Close()
		cr := resp.Header.Get("Content-Range") // "bytes 0-0/<total>"
		total, _ := strconv.ParseInt(cr[strings.LastIndex(cr, "/")+1:], 10, 64)
		if total < minFullTrackBytes {
			t.Errorf("#%s %q: audio is %d bytes (Content-Range %q) - a preview clip, not the full track", s.TokenID, s.Name, total, cr)
		}
	}

	// The republished masters are the newest ids. If a superseded copy shows up
	// instead, dedupe picked the wrong winner or its metadata failed to resolve.
	for _, s := range songs {
		if s.Name == "Sloppy" && s.TokenID != "10" {
			t.Errorf("Sloppy resolved to #%s, expected the republished #10 (artist %s)", s.TokenID, s.Artist)
		}
		if s.Name == "Killah" && s.TokenID != "9" {
			t.Errorf("Killah resolved to #%s, expected the republished #9", s.TokenID)
		}
	}
}
