package music

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func fakeRPC(t *testing.T, fails int, fail func(w http.ResponseWriter)) (*chainSource, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if int(atomic.AddInt32(&hits, 1)) <= fails {
			fail(w)
			return
		}
		// totalMasters() = 15
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":"0x%064x"}`, 15)
	}))
	t.Cleanup(srv.Close)
	c, err := newChainSource(srv.URL, "0x42EbcD44C2295702130f0A641633c691bA5f9480", "https://gw/ipfs/")
	if err != nil {
		t.Fatal(err)
	}
	old := rpcRetryDelays
	rpcRetryDelays = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	t.Cleanup(func() { rpcRetryDelays = old })
	return c, &hits
}

func TestCallRetriesRateLimit(t *testing.T) {
	c, hits := fakeRPC(t, 2, func(w http.ResponseWriter) { w.WriteHeader(http.StatusTooManyRequests) })
	vals, err := c.call(context.Background(), "totalMasters")
	if err != nil {
		t.Fatalf("call after two 429s: %v", err)
	}
	if vals[0].(*big.Int).Int64() != 15 || *hits != 3 {
		t.Fatalf("got %v after %d hits, want 15 after 3", vals[0], *hits)
	}
}

func TestCallRetriesJSONRPCRateLimit(t *testing.T) {
	c, _ := fakeRPC(t, 1, func(w http.ResponseWriter) {
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"error":{"code":-32005,"message":"request limit exceeded"}}`)
	})
	if _, err := c.call(context.Background(), "totalMasters"); err != nil {
		t.Fatalf("call after one -32005: %v", err)
	}
}

func TestCallGivesUpAfterRetries(t *testing.T) {
	c, hits := fakeRPC(t, 100, func(w http.ResponseWriter) { w.WriteHeader(http.StatusTooManyRequests) })
	if _, err := c.call(context.Background(), "totalMasters"); err == nil {
		t.Fatal("expected an error after exhausting retries")
	}
	if *hits != int32(len(rpcRetryDelays)+1) {
		t.Fatalf("hits=%d, want %d", *hits, len(rpcRetryDelays)+1)
	}
}

func TestCallDoesNotRetryRevert(t *testing.T) {
	c, hits := fakeRPC(t, 100, func(w http.ResponseWriter) {
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"error":{"code":3,"message":"execution reverted"}}`)
	})
	if _, err := c.call(context.Background(), "totalMasters"); err == nil || *hits != 1 {
		t.Fatalf("revert: err=%v hits=%d, want error after 1 hit", err, *hits)
	}
}
