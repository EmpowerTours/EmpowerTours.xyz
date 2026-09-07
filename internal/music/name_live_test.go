package music

import (
	"context"
	"testing"
	"time"
)

func TestProfileNameLive(t *testing.T) {
	if testing.Short() {
		t.Skip("network test")
	}
	src, err := newChainSource("https://rpc.monad.xyz",
		"0x42EbcD44C2295702130f0A641633c691bA5f9480",
		"https://harlequin-used-hare-224.mypinata.cloud/ipfs/")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	got := src.displayName(ctx, "0x05d1599622915050C4981816ef5E8d51F53dbc7D")
	t.Logf("displayNameOf(Ganado artist) = %q", got)
	if got != "Earvin Gallardo" {
		t.Fatalf("expected \"Earvin Gallardo\", got %q", got)
	}
	if n := src.displayName(ctx, "0x33fFCcb1802e13a7eead232BCd4706a2269582b0"); n != "" {
		t.Fatalf("unify34 has no registered profile name, got %q", n)
	}
	if n := src.displayName(ctx, "not-an-address"); n != "" {
		t.Fatalf("a malformed address must resolve to empty, got %q", n)
	}
}
