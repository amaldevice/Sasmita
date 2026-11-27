package openai_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/model/adapters/openai"
)

func scriptedServer(t *testing.T, record *[]byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*record = body
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, line := range []string{
			`data: {"choices":[{"delta":{"content":"Hello"}}]}`,
			`data: {"choices":[{"delta":{"content":", "}}]}`,
			`data: {"choices":[{"delta":{"content":"world"}}]}`,
			`data: {"choices":[{"delta":{"content":"!"}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":3,"prompt_tokens_details":{"cached_tokens":4},"cache_write_tokens":2}}`,
			`data: [DONE]`,
		} {
			fmt.Fprint(w, line+"\n\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}))
}

func TestStreamScriptedSSEAndExactRequestBytes(t *testing.T) {
	var recorded []byte
	srv := scriptedServer(t, &recorded)
	defer srv.Close()

	adapter := openai.Adapter{BaseURL: srv.URL, APIKey: "test-key", ProviderID: "openai"}
	var deltas []string
	var usage *model.Usage
	err := adapter.Stream(t.Context(), model.StreamRequest{
		Model:          "gpt-test",
		PromptCacheKey: "conv-1",
		Messages: []model.Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hi"},
		},
	}, func(ev model.StreamEvent) {
		switch ev.Kind {
		case model.EventTextDelta:
			deltas = append(deltas, ev.Text)
		case model.EventUsage:
			usage = ev.Usage
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(deltas, ""); got != "Hello, world!" {
		t.Fatalf("streamed text = %q", got)
	}
	if usage == nil {
		t.Fatal("usage event was not emitted")
	}
	if usage.InputTokens != 10 || usage.OutputTokens != 3 || usage.CacheReadTokens != 4 || usage.CacheWriteTokens != 2 {
		t.Fatalf("usage = %+v", usage)
	}
	if usage.ProviderID != "openai" || usage.ModelID != "gpt-test" {
		t.Fatalf("usage ids = %+v", usage)
	}

	want := `{"model":"gpt-test","stream":true,"stream_options":{"include_usage":true},"prompt_cache_key":"conv-1","messages":[{"role":"system","content":"You are helpful."},{"role":"user","content":"Hi"}]}`
	if string(recorded) != want {
		t.Fatalf("request bytes mismatch\n got: %s\nwant: %s", recorded, want)
	}
}

// The serialized stable prefix is the request body minus its trailing `]}`.
// Appending a turn must only extend those bytes, never rewrite them.
func TestStableRequestPrefixIsByteExactPrefix(t *testing.T) {
	req := func(messages []model.Message) model.StreamRequest {
		return model.StreamRequest{Model: "m", PromptCacheKey: "session", Messages: messages}
	}
	turn1 := []model.Message{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "q1"},
	}
	turn2 := append(append([]model.Message{}, turn1...),
		model.Message{Role: "assistant", Content: "a1"},
		model.Message{Role: "user", Content: "q2"},
	)
	p1 := openai.StableRequestPrefix(req(turn1))
	p2 := openai.StableRequestPrefix(req(turn2))
	if !bytes.HasPrefix(p2, p1) {
		t.Fatalf("turn 1 prefix is not a byte-exact prefix of turn 2\np1: %s\np2: %s", p1, p2)
	}
	if bytes.Equal(p1, p2) {
		t.Fatal("adding a turn did not extend the request prefix")
	}
}

func TestStreamCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"choices":[{"delta":{"content":"partial"}}]}`+"\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	adapter := openai.Adapter{BaseURL: srv.URL, ProviderID: "openai"}
	err := adapter.Stream(ctx, model.StreamRequest{Model: "m", Messages: []model.Message{{Role: "user", Content: "hi"}}},
		func(ev model.StreamEvent) {
			if ev.Kind == model.EventTextDelta {
				cancel()
			}
		})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("unexpected cancellation error: %v", err)
	}
}
