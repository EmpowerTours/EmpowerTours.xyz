package music

import (
	"errors"
	"testing"
	"time"
)

func TestChoose(t *testing.T) {
	now := time.Now()
	full := []Song{{TokenID: "1"}, {TokenID: "2"}, {TokenID: "3"}, {TokenID: "4"}, {TokenID: "5"}, {TokenID: "6"}, {TokenID: "7"}}
	partial := full[:3]
	inc := &IncompleteError{Unread: 4}
	down := errors.New("rpc down")

	snap := func(songs []Song, age time.Duration, complete bool) snapshot {
		if songs == nil {
			return snapshot{}
		}
		return snapshot{songs: songs, at: now.Add(-age), complete: complete}
	}

	cases := []struct {
		name         string
		cache        snapshot
		fresh        []Song
		err          error
		wantLen      int
		wantErr      bool
		wantComplete bool
		wantCacheLen int
	}{
		{"complete scan replaces cache", snap(partial, time.Minute, false), full, nil, 7, false, true, 7},
		{"complete scan of zero is a real empty catalog", snap(full, time.Minute, true), []Song{}, nil, 0, false, true, 0},
		// 2026-09-27: a 3-of-7 scan was cached and served as the catalog.
		{"partial scan keeps recent complete cache", snap(full, time.Minute, true), partial, inc, 7, false, true, 7},
		{"partial scan past maxStale is accepted", snap(full, maxStale+time.Second, true), partial, inc, 3, false, false, 3},
		{"partial scan with no cache is served", snap(nil, 0, false), partial, inc, 3, false, false, 3},
		// 2026-11-09: an empty partial was cached and pinned "no tracks yet".
		{"empty partial with no cache is an error, not zero tracks", snap(nil, 0, false), []Song{}, inc, 0, true, false, 0},
		{"bigger partial beats a cached empty-ish partial", snap(partial[:1], time.Minute, false), partial, inc, 3, false, false, 3},
		{"bigger recent partial cache beats smaller partial", snap(partial, time.Minute, false), partial[:1], inc, 3, false, false, 3},
		{"over-age partial cache loses to smaller partial", snap(partial, maxStale+time.Second, false), partial[:1], inc, 1, false, false, 1},
		{"empty partial falls back to recent partial cache", snap(partial, time.Minute, false), []Song{}, inc, 3, false, false, 3},
		{"failed scan serves old cache", snap(full, time.Hour, true), nil, down, 7, false, true, 7},
		{"failed scan with no cache is an error", snap(nil, 0, false), nil, down, 0, true, false, 0},
	}
	for _, c := range cases {
		serve, next, err := choose(c.cache, now, c.fresh, c.err)
		if len(serve) != c.wantLen || (err != nil) != c.wantErr || next.complete != c.wantComplete || len(next.songs) != c.wantCacheLen {
			t.Errorf("%s: got %d tracks err=%v cache=%d complete=%v; want %d err=%v cache=%d complete=%v",
				c.name, len(serve), err, len(next.songs), next.complete, c.wantLen, c.wantErr, c.wantCacheLen, c.wantComplete)
		}
	}
}

// A kept cache must keep its age, or a run of partial scans could hold a
// taken-down track in the catalog indefinitely.
func TestChooseKeptCacheKeepsItsAge(t *testing.T) {
	now := time.Now()
	born := now.Add(-time.Minute)
	cache := snapshot{songs: []Song{{TokenID: "1"}, {TokenID: "2"}}, at: born}
	_, next, _ := choose(cache, now, []Song{{TokenID: "1"}}, &IncompleteError{Unread: 1})
	if !next.at.Equal(born) {
		t.Fatalf("kept cache age refreshed: at=%v, want %v", next.at, born)
	}
}
