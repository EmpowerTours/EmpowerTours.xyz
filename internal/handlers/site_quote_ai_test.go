package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

// The real SDK client against a local server: checks the request we send and how we read
// the reply. No network.
func TestClaudeDrafterWire(t *testing.T) {
	var got map[string]any
	stop := "end_turn"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &got)
		out, _ := json.Marshal(map[string]any{"summary": "s", "included": []string{"a"}, "questions": []string{}, "flags": []string{}, "client_email": "Hi"})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-opus-5-5",
			"content":     []any{map[string]any{"type": "text", "text": string(out)}},
			"stop_reason": stop, "usage": map[string]any{"input_tokens": 11, "output_tokens": 22},
		})
	}))
	defer srv.Close()
	d := &ClaudeDrafter{client: anthropic.NewClient(option.WithAPIKey("test-key"), option.WithBaseURL(srv.URL), option.WithMaxRetries(0))}

	draft, in, out, err := d.Draft(context.Background(), "sys", "user")
	if err != nil || draft.ClientEmail != "Hi" || in != 11 || out != 22 {
		t.Fatalf("draft %+v in %d out %d err %v", draft, in, out, err)
	}
	if got["model"] != "claude-opus-5-5" {
		t.Errorf("model %v", got["model"])
	}
	if _, ok := got["thinking"]; ok {
		t.Error("thinking must be omitted")
	}
	oc, _ := got["output_config"].(map[string]any)
	format, _ := oc["format"].(map[string]any)
	if oc["effort"] != "low" || format["type"] != "json_schema" || format["schema"] == nil {
		t.Errorf("output_config %v", got["output_config"])
	}

	for _, s := range []string{"refusal", "max_tokens"} {
		stop = s
		if _, _, _, err := d.Draft(context.Background(), "sys", "user"); err == nil {
			t.Errorf("stop_reason %s accepted", s)
		}
	}
}
