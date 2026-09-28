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

	cases := []struct {
		name     string
		cache    []Song
		age      time.Duration
		fresh    []Song
		err      error
		wantLen  int
		wantKeep bool
	}{
		{"complete scan replaces cache", partial, time.Minute, full, nil, 7, false},
		// The bug: a 3-of-7 scan used to be cached and served as the catalog.
		{"partial scan keeps recent full cache", full, time.Minute, partial, inc, 7, true},
		{"partial scan past maxStale is accepted", full, maxStale + time.Second, partial, inc, 3, false},
		{"partial scan with no cache is served", nil, 0, partial, inc, 3, false},
		{"failed scan serves old cache", full, time.Hour, nil, down, 7, true},
		{"failed scan with no cache serves nothing", nil, 0, nil, down, 0, false},
	}
	for _, c := range cases {
		serve, keep := choose(c.cache, now.Add(-c.age), now, c.fresh, c.err)
		if len(serve) != c.wantLen || keep != c.wantKeep {
			t.Errorf("%s: got %d tracks keep=%v, want %d keep=%v", c.name, len(serve), keep, c.wantLen, c.wantKeep)
		}
	}
}
