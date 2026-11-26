package anthropic_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/model/adapters/anthropic"
)

// anthropicServer is a fake provider that speaks the Anthropic Messages wire
// format and records the exact request bytes (seam 1).
func anthropicServer(t *testing.T, record *[]byte, headers *http.Header) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if record != nil {
			*record = body
		}
		if headers != nil {
			*headers = r.Header.Clone()
		}
		w.Header().Set("Content-Type", "text/event-stream")
		lines := []string{
			`event: message_start`,
			`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-test","usage":{"input_tokens":10,"output_tokens":0,"cache_creation_input_tokens":2,"cache_read_input_tokens":4}}}`,
			``,
			`event: content_block_start`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":", "}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"world"}}`,
			``,
			`event: content_block_delta`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"!"}}`,
			``,
			`event: content_block_stop`,
			`data: {"type":"content_block_stop","index":0}`,
			``,
			`event: message_delta`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`,
			``,
			`event: message_stop`,
			`data: {"type":"message_stop"}`,
			``,
		}
		for _, line := range lines {
			fmt.Fprint(w, line+"\n")
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
}

func TestStreamScriptedSSEAndExactRequestBytes(t *testing.T) {
	var recorded []byte
	var headers http.Header
	srv := anthropicServer(t, &recorded, &headers)
	defer srv.Close()

	adapter := anthropic.Adapter{BaseURL: srv.URL, APIKey: "secret-key", ProviderID: "anthropic"}
	var deltas []string
	var usage *model.Usage
	var tools []model.ToolCall
	err := adapter.Stream(t.Context(), model.StreamRequest{
		Model:          "claude-test",
		PromptCacheKey: "conv-1",
		Messages: []model.Message{
			{Role: "system", Content: "You are Sasmita. Answer clearly and concisely."},
			{Role: "user", Content: "Hi"},
		},
	}, func(ev model.StreamEvent) {
		switch ev.Kind {
		case model.EventTextDelta:
			deltas = append(deltas, ev.Text)
		case model.EventUsage:
			usage = ev.Usage
		case model.EventToolUse:
			tools = append(tools, *ev.ToolCall)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(deltas, ""); got != "Hello, world!" {
		t.Fatalf("streamed text = %q", got)
	}
	if len(tools) != 0 {
		t.Fatalf("unexpected tool calls: %+v", tools)
	}
	if usage == nil {
		t.Fatal("usage event was not emitted")
	}
	if usage.InputTokens != 10 || usage.OutputTokens != 3 || usage.CacheReadTokens != 4 || usage.CacheWriteTokens != 2 {
		t.Fatalf("usage = %+v", usage)
	}
	if usage.ProviderID != "anthropic" || usage.ModelID != "claude-test" {
		t.Fatalf("usage ids = %+v", usage)
	}

	if got := headers.Get("X-Api-Key"); got != "secret-key" {
		t.Fatalf("x-api-key = %q", got)
	}
	if got := headers.Get("Anthropic-Version"); got != "2023-06-01" {
		t.Fatalf("anthropic-version = %q", got)
	}

	want := `{"model":"claude-test","max_tokens":1024,"stream":true,"system":[{"type":"text","text":"You are Sasmita. Answer clearly and concisely.","cache_control":{"type":"ephemeral","ttl":"1h"}}],"messages":[{"role":"user","content":"Hi"}]}`
	if string(recorded) != want {
		t.Fatalf("request bytes mismatch\n got: %s\nwant: %s", recorded, want)
	}
}

func TestStreamAuthErrorIsClassifiedAndNeverEchoesKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid api key"}}`)
	}))
	defer srv.Close()

	adapter := anthropic.Adapter{BaseURL: srv.URL, APIKey: "secret-key-should-not-leak", ProviderID: "anthropic"}
	err := adapter.Stream(t.Context(), model.StreamRequest{Model: "m", Messages: []model.Message{{Role: "user", Content: "hi"}}}, func(model.StreamEvent) {})
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !errors.Is(err, model.ErrUnauthorized) {
		t.Fatalf("error = %v, want ErrUnauthorized", err)
	}
	if strings.Contains(err.Error(), "secret-key-should-not-leak") {
		t.Fatalf("error leaked the API key: %v", err)
	}
}

func TestStreamRetriesWithRetryAfter(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`+"\n\n")
		fmt.Fprint(w, `data: {"type":"message_stop"}`+"\n\n")
	}))
	defer srv.Close()

	var sleeps []time.Duration
	adapter := anthropic.Adapter{
		BaseURL: srv.URL, APIKey: "secret-key", ProviderID: "anthropic",
		Sleep: func(_ context.Context, d time.Duration) error {
			sleeps = append(sleeps, d)
			return nil
		},
	}
	var deltas []string
	err := adapter.Stream(t.Context(), model.StreamRequest{Model: "m", Messages: []model.Message{{Role: "user", Content: "hi"}}},
		func(ev model.StreamEvent) {
			if ev.Kind == model.EventTextDelta {
				deltas = append(deltas, ev.Text)
			}
		})
	if err != nil {
		t.Fatal(err)
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}
	if fmt.Sprint(sleeps) != fmt.Sprint([]time.Duration{2 * time.Second}) {
		t.Fatalf("sleeps = %v, want [2s]", sleeps)
	}
	if got := strings.Join(deltas, ""); got != "ok" {
		t.Fatalf("deltas = %q, want ok (no duplicate from retried attempt)", got)
	}
}

func TestStreamTransportFailureIsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	adapter := anthropic.Adapter{BaseURL: url, APIKey: "secret-key", ProviderID: "anthropic"}
	err := adapter.Stream(t.Context(), model.StreamRequest{Model: "m", Messages: []model.Message{{Role: "user", Content: "hi"}}}, func(model.StreamEvent) {})
	if err == nil {
		t.Fatal("expected transport error")
	}
	if !errors.Is(err, model.ErrUnreachable) {
		t.Fatalf("error = %v, want ErrUnreachable", err)
	}
}

func TestStableRequestPrefixIsByteExactPrefix(t *testing.T) {
	req := func(messages []model.Message) model.StreamRequest {
		return model.StreamRequest{Model: "m", Messages: messages}
	}
	turn1 := []model.Message{
		{Role: "system", Content: "static system"},
		{Role: "user", Content: "q1"},
	}
	turn2 := append(append([]model.Message{}, turn1...),
		model.Message{Role: "assistant", Content: "a1"},
		model.Message{Role: "user", Content: "q2"},
	)
	p1, err := anthropic.EncodeRequest(req(turn1))
	if err != nil {
		t.Fatal(err)
	}
	p2, err := anthropic.EncodeRequest(req(turn2))
	if err != nil {
		t.Fatal(err)
	}
	// Strip the trailing `]}` envelope: appending a turn must only extend the
	// earlier bytes (ADR-0001 golden prefix).
	stable := func(b []byte) []byte { return b[:len(b)-2] }
	if !strings.HasPrefix(string(stable(p2)), string(stable(p1))) {
		t.Fatalf("golden prefix broken\nturn 1: %s\nturn 2: %s", p1, p2)
	}
}
