package openai_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/model/adapters/openai"
)

const openaiCacheKey = "1723376043e8a9d0090025f147440b99" // SHA-256("conv-123"), first 16 bytes

func TestEncodeRequestUsesStableCacheKeyAndReplaysRawBlocks(t *testing.T) {
	req := model.StreamRequest{
		Model:          "gpt-test",
		PromptCacheKey: openaiCacheKey,
		Messages: []model.Message{
			{Role: "system", Content: "static system"},
			{Role: "user", Content: "q1"},
			{Role: "assistant", Blocks: []json.RawMessage{
				json.RawMessage(`{"signature":"sig-abc","type":"thinking","thinking":"hmm"}`),
				json.RawMessage(`{"arguments":{"b":1,"a":2},"name":"f","type":"tool_call","id":"c1"}`),
			}},
		},
	}
	got, err := openai.EncodeRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"model":"gpt-test","stream":true,"stream_options":{"include_usage":true},"prompt_cache_key":"` + openaiCacheKey + `","messages":[{"role":"system","content":"static system"},{"role":"user","content":"q1"},{"role":"assistant","content":[{"signature":"sig-abc","type":"thinking","thinking":"hmm"},{"arguments":{"b":1,"a":2},"name":"f","type":"tool_call","id":"c1"}]}]}`
	if string(got) != want {
		t.Fatalf("request bytes mismatch\n got: %s\nwant: %s", got, want)
	}
	// OpenAI has no explicit breakpoints: canonical and wire bytes are equal.
	canonical, err := openai.CanonicalRequest(req)
	if err != nil || !bytes.Equal(canonical, got) {
		t.Fatalf("canonical = %s (err %v), wire = %s", canonical, err, got)
	}
}

func TestCanonicalPrefixExtendsAcrossTurnsWithRawBlocks(t *testing.T) {
	turn1 := []model.Message{
		{Role: "system", Content: "static system"},
		{Role: "user", Content: "q1"},
	}
	turn2 := append(append([]model.Message{}, turn1...),
		model.Message{Role: "assistant", Blocks: []json.RawMessage{
			json.RawMessage(`{"signature":"sig","type":"thinking","thinking":"hmm"}`),
		}},
		model.Message{Role: "user", Content: "q2"},
	)
	req := func(msgs []model.Message) model.StreamRequest {
		return model.StreamRequest{Model: "m", PromptCacheKey: openaiCacheKey, Messages: msgs}
	}
	p1, err := openai.CanonicalRequest(req(turn1))
	if err != nil {
		t.Fatal(err)
	}
	p2, err := openai.CanonicalRequest(req(turn2))
	if err != nil {
		t.Fatal(err)
	}
	stable := func(b []byte) []byte { return b[:len(b)-2] }
	if !bytes.HasPrefix(stable(p2), stable(p1)) {
		t.Fatalf("golden prefix broken\nturn 1: %s\nturn 2: %s", p1, p2)
	}
}

func TestStreamParsesProviderSpecificCacheFields(t *testing.T) {
	cases := []struct {
		name       string
		usageLine  string
		wantRead   int64
		wantWrite  int64
		wantUncach int64
		wantEst    bool
	}{
		{
			name:       "deepseek hit and miss",
			usageLine:  `{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":2,"prompt_cache_hit_tokens":6,"prompt_cache_miss_tokens":4}}`,
			wantRead:   6,
			wantUncach: 4,
		},
		{
			name:       "llama.cpp cache_n",
			usageLine:  `{"choices":[],"usage":{"prompt_tokens":9,"completion_tokens":2},"timings":{"cache_n":5}}`,
			wantRead:   5,
			wantUncach: 4,
		},
		{
			name:      "only cached tokens are reported",
			usageLine: `{"choices":[],"usage":{"prompt_tokens_details":{"cached_tokens":3}}}`,
			wantRead:  3,
			wantEst:   true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", tc.usageLine)
			}))
			defer srv.Close()
			var usage *model.Usage
			adapter := openai.Adapter{BaseURL: srv.URL, ProviderID: "openai"}
			if err := adapter.Stream(t.Context(), model.StreamRequest{Model: "m", Messages: []model.Message{{Role: "user", Content: "hi"}}},
				func(ev model.StreamEvent) {
					if ev.Kind == model.EventUsage {
						usage = ev.Usage
					}
				}); err != nil {
				t.Fatal(err)
			}
			if usage == nil {
				t.Fatal("no usage event")
			}
			if usage.CacheReadTokens != tc.wantRead || usage.CacheWriteTokens != tc.wantWrite ||
				usage.UncachedTokens != tc.wantUncach || usage.Estimated != tc.wantEst {
				t.Fatalf("usage = %+v, want read=%d write=%d uncached=%d estimated=%v",
					usage, tc.wantRead, tc.wantWrite, tc.wantUncach, tc.wantEst)
			}
		})
	}
}
