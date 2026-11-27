package integration_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amaldevice/sasmita/internal/appsvc"
	ctxlayout "github.com/amaldevice/sasmita/internal/context"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/model/adapters/anthropic"
	"github.com/amaldevice/sasmita/internal/model/adapters/openai"
	"github.com/amaldevice/sasmita/internal/persistence/migrations"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
)

const id1SessionKey = "eb66c623572f8a2a5167ce017dee0541" // SHA-256("id-1"), first 16 bytes

func newLayoutService(t *testing.T, provider model.ModelProvider, store *repositories.Store, note func() string) *appsvc.Service {
	t.Helper()
	return appsvc.New(store, appsvc.Config{
		Provider: provider, DefaultModel: "test-model", CoalesceSize: 8,
		NewID: newIDGenerator(), ContextNote: note,
	})
}

func stripAnthropicBreakpoints(b []byte) []byte {
	s := string(b)
	s = strings.ReplaceAll(s, `,"cache_control":{"type":"ephemeral","ttl":"1h"}`, "")
	s = strings.ReplaceAll(s, `,"cache_control":{"type":"ephemeral","ttl":"5m"}`, "")
	return []byte(s)
}

func anthropicStreamWithBlocks(blocks, usage, answer string) string {
	return fmt.Sprintf(strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"usage":%s,"content":%s}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":%q}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","usage":{"output_tokens":4}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")+"\n", usage, blocks, answer)
}

func TestSeam1AnthropicGoldenPrefixRawReplayAndBreakpoints(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "sasmita.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrations.Migrate(db); err != nil {
		t.Fatal(err)
	}
	store := repositories.New(db)
	now := time.Now()
	if err := store.CreateProject(ctx, &domain.Project{ID: "p1", Name: "Work", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	// HTML-sensitive characters guard against json.Marshal's default escaping
	// changing stored bytes on replay.
	const rawThinking = `{"signature":"sig<raw>&x","type":"thinking","thinking":"hmm"}`
	var bodies [][]byte
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "text/event-stream")
		switch calls.Add(1) {
		case 1:
			fmt.Fprint(w, anthropicStreamWithBlocks(`[`+rawThinking+`,{"type":"text","text":"answer 1"}]`,
				`{"input_tokens":7,"output_tokens":0,"cache_creation_input_tokens":1,"cache_read_input_tokens":2}`, "answer 1"))
		default:
			fmt.Fprint(w, anthropicStreamWithBlocks(`[{"type":"text","text":"answer 2"}]`,
				`{"input_tokens":7,"output_tokens":0,"cache_creation_input_tokens":1,"cache_read_input_tokens":2}`, "answer 2"))
		}
	}))
	defer srv.Close()

	notes := []string{"note:turn-1", "note:turn-2"}
	noteIndex := 0
	svc := newLayoutService(t,
		anthropic.Adapter{BaseURL: srv.URL, ProviderID: "anthropic"}, store,
		func() string { n := notes[noteIndex]; noteIndex++; return n })
	conv, err := svc.CreateConversation(ctx, "p1", "Layout", "chat")
	if err != nil {
		t.Fatal(err)
	}
	if ctxlayout.SessionKey(conv.ID) != id1SessionKey {
		t.Fatalf("session key for %q = %q", conv.ID, ctxlayout.SessionKey(conv.ID))
	}

	h1, err := svc.StartRun(ctx, domain.StartRunRequest{ConversationID: conv.ID, Text: "q1"})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, h1)
	h2, err := svc.StartRun(ctx, domain.StartRunRequest{ConversationID: conv.ID, Text: "q2"})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, h2)

	if len(bodies) != 2 {
		t.Fatalf("provider received %d requests, want 2 (no keepalive)", len(bodies))
	}
	for i, body := range bodies {
		if !bytes.Contains(body, []byte(`"session_id":"`+id1SessionKey+`"`)) {
			t.Fatalf("request %d missing hashed session_id: %s", i+1, body)
		}
		if !bytes.Contains(body, []byte(`"cache_control":{"type":"ephemeral","ttl":"1h"}`)) ||
			!bytes.Contains(body, []byte(`"cache_control":{"type":"ephemeral","ttl":"5m"}`)) {
			t.Fatalf("request %d missing breakpoints: %s", i+1, body)
		}
		if n := bytes.Count(body, []byte(`"cache_control"`)); n != 2 {
			t.Fatalf("request %d has %d breakpoints, want 2: %s", i+1, n, body)
		}
	}

	// Golden prefix is asserted over canonical history: breakpoints are wire
	// decorations that move with the last block (see decision note).
	c1, c2 := stripAnthropicBreakpoints(bodies[0]), stripAnthropicBreakpoints(bodies[1])
	stable := func(b []byte) []byte { return b[:len(b)-2] }
	if !bytes.HasPrefix(stable(c2), stable(c1)) {
		t.Fatalf("golden prefix broken\nturn 1: %s\nturn 2: %s", bodies[0], bodies[1])
	}
	// The system instruction is static; dynamic info only appears on user turns.
	s1 := string(bodies[0])
	s2 := string(bodies[1])
	if !bytes.Contains([]byte(s2), []byte(`"text":"note:turn-1"`)) ||
		!bytes.Contains([]byte(s2), []byte(`"text":"note:turn-2"`)) ||
		strings.Index(s2, "note:turn-1") > strings.Index(s2, "note:turn-2") {
		t.Fatalf("context notes are not appended to user turns: %s", s2)
	}
	if strings.Contains(s1, "note:turn-2") || strings.Contains(s1, "2026-") {
		t.Fatalf("dynamic content leaked into turn 1: %s", s1)
	}
	// Raw thinking/signature from turn 1 must be replayed byte-identically.
	if !bytes.Contains(bodies[1], []byte(rawThinking)) {
		t.Fatalf("turn 2 did not replay the raw provider block: %s", bodies[1])
	}

	msgs, err := store.ListMessages(ctx, conv.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	assistant := msgs[1]
	if len(assistant.Content) == 0 || string(assistant.Content[0].Raw) != rawThinking {
		t.Fatalf("stored assistant content = %+v", assistant.Content)
	}
	for _, runID := range []string{h1.RunID, h2.RunID} {
		usage, err := store.ListUsage(ctx, runID)
		if err != nil || len(usage) != 1 {
			t.Fatalf("usage for %s = %+v (err %v)", runID, usage, err)
		}
		if usage[0].InputTokens != 7 || usage[0].CacheReadTokens != 2 || usage[0].CacheWriteTokens != 1 ||
			usage[0].UncachedTokens != 7 || usage[0].Estimated {
			t.Fatalf("stored usage = %+v", usage[0])
		}
	}
}

func TestSeam1OpenAIHashedPromptCacheKeyAndGoldenPrefix(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "sasmita.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrations.Migrate(db); err != nil {
		t.Fatal(err)
	}
	store := repositories.New(db)
	now := time.Now()
	if err := store.CreateProject(ctx, &domain.Project{ID: "p1", Name: "Work", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	var bodies [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, body)
		writeSSE(w, "answer")
	}))
	defer srv.Close()

	svc := newLayoutService(t, openai.Adapter{BaseURL: srv.URL, ProviderID: "openai"}, store, func() string { return "" })
	conv, err := svc.CreateConversation(ctx, "p1", "OpenAI layout", "chat")
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"q1", "q2"} {
		h, err := svc.StartRun(ctx, domain.StartRunRequest{ConversationID: conv.ID, Text: text})
		if err != nil {
			t.Fatal(err)
		}
		drain(t, h)
	}
	if len(bodies) != 2 {
		t.Fatalf("provider received %d requests, want 2", len(bodies))
	}
	for i, body := range bodies {
		if !bytes.Contains(body, []byte(`"prompt_cache_key":"`+id1SessionKey+`"`)) {
			t.Fatalf("request %d missing hashed prompt_cache_key: %s", i+1, body)
		}
	}
	stable := func(b []byte) []byte { return b[:len(b)-2] }
	if !bytes.HasPrefix(stable(bodies[1]), stable(bodies[0])) {
		t.Fatalf("golden prefix broken\nturn 1: %s\nturn 2: %s", bodies[0], bodies[1])
	}
}

func TestSeam1ProviderSpecificCacheUsageIsPersisted(t *testing.T) {
	cases := []struct {
		name       string
		usageLine  string
		wantRead   int64
		wantUncach int64
		wantEst    bool
	}{
		{"deepseek", `{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":2,"prompt_cache_hit_tokens":6,"prompt_cache_miss_tokens":4}}`, 6, 4, false},
		{"llama.cpp", `{"choices":[],"usage":{"prompt_tokens":9,"completion_tokens":2},"timings":{"cache_n":5}}`, 5, 4, false},
		{"estimated", `{"choices":[],"usage":{"prompt_tokens_details":{"cached_tokens":3}}}`, 3, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := sqlite.Open(filepath.Join(t.TempDir(), "sasmita.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err := migrations.Migrate(db); err != nil {
				t.Fatal(err)
			}
			store := repositories.New(db)
			now := time.Now()
			if err := store.CreateProject(ctx, &domain.Project{ID: "p1", Name: "Work", CreatedAt: now, UpdatedAt: now}); err != nil {
				t.Fatal(err)
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", tc.usageLine)
			}))
			defer srv.Close()
			svc := newLayoutService(t, openai.Adapter{BaseURL: srv.URL, ProviderID: tc.name}, store, nil)
			conv, err := svc.CreateConversation(ctx, "p1", tc.name, "chat")
			if err != nil {
				t.Fatal(err)
			}
			h, err := svc.StartRun(ctx, domain.StartRunRequest{ConversationID: conv.ID, Text: "q"})
			if err != nil {
				t.Fatal(err)
			}
			drain(t, h)
			usage, err := store.ListUsage(ctx, h.RunID)
			if err != nil || len(usage) != 1 {
				t.Fatalf("usage = %+v (err %v)", usage, err)
			}
			if usage[0].CacheReadTokens != tc.wantRead || usage[0].UncachedTokens != tc.wantUncach || usage[0].Estimated != tc.wantEst {
				t.Fatalf("usage = %+v, want read=%d uncached=%d estimated=%v", usage[0], tc.wantRead, tc.wantUncach, tc.wantEst)
			}
		})
	}
}
