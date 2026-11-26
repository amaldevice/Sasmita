package integration_test

import (
	"context"
	"encoding/json"
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
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/model/adapters/anthropic"
	"github.com/amaldevice/sasmita/internal/persistence/migrations"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
)

const anthropicKey = "sk-anthropic-seam-secret"

func anthropicSSE(answer string) string {
	return fmt.Sprintf(strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"usage":{"input_tokens":7,"output_tokens":0,"cache_creation_input_tokens":1,"cache_read_input_tokens":2}}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":%q}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":4}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")+"\n", answer)
}

func newAnthropicSeamService(t *testing.T, provider model.ModelProvider, store *repositories.Store, id func() string) *appsvc.Service {
	t.Helper()
	return appsvc.New(store, appsvc.Config{
		Provider: provider, DefaultModel: "claude-test", CoalesceSize: 8,
		NewID: id,
	})
}

// Seam 1: application-service commands and the RunEvent stream against an
// httptest provider speaking the Anthropic-compatible wire format.
func TestSeam1AnthropicStreamingAndNoKeyInEvents(t *testing.T) {
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

	var recorded []byte
	var authHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		recorded = body
		authHeader = r.Header.Get("X-Api-Key")
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, anthropicSSE("Hello from Anthropic."))
	}))
	defer srv.Close()

	adapter := anthropic.Adapter{BaseURL: srv.URL, APIKey: anthropicKey, ProviderID: "anthropic"}
	svc := newAnthropicSeamService(t, adapter, store, newIDGenerator())
	conv, err := svc.CreateConversation(ctx, "p1", "Anthropic chat", "chat")
	if err != nil {
		t.Fatal(err)
	}
	handle, err := svc.StartRun(ctx, domain.StartRunRequest{
		ConversationID: conv.ID, Text: "hi", ModelRef: "claude-test", ModeID: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	events := drain(t, handle)
	if got := coalescedText(events); got != "Hello from Anthropic." {
		t.Fatalf("coalesced text = %q", got)
	}
	if events[len(events)-1].Type != domain.EventRunCompleted {
		t.Fatalf("terminal event = %q", events[len(events)-1].Type)
	}
	if authHeader != anthropicKey {
		t.Fatalf("x-api-key = %q", authHeader)
	}
	if !strings.Contains(string(recorded), `"cache_control":{"type":"ephemeral","ttl":"1h"}`) {
		t.Fatalf("missing 1h cache breakpoint in request: %s", recorded)
	}
	for _, ev := range events {
		if strings.Contains(string(ev.Payload), anthropicKey) {
			t.Fatalf("RunEvent leaked the key: %+v", ev)
		}
	}
	usage, err := store.ListUsage(ctx, handle.RunID)
	if err != nil || len(usage) != 1 || usage[0].InputTokens != 7 || usage[0].OutputTokens != 4 ||
		usage[0].CacheReadTokens != 2 || usage[0].CacheWriteTokens != 1 {
		t.Fatalf("usage = %+v (err %v)", usage, err)
	}
}

func TestSeam1AnthropicBadKeyFailsRunWithoutKeyInMessage(t *testing.T) {
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
	if err := store.CreateProject(ctx, &domain.Project{ID: "p1", Name: "Work"}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid api key"}}`)
	}))
	defer srv.Close()

	svc := newAnthropicSeamService(t,
		anthropic.Adapter{BaseURL: srv.URL, APIKey: anthropicKey, ProviderID: "anthropic"},
		store, newIDGenerator())
	conv, err := svc.CreateConversation(ctx, "p1", "Bad key", "chat")
	if err != nil {
		t.Fatal(err)
	}
	handle, err := svc.StartRun(ctx, domain.StartRunRequest{ConversationID: conv.ID, Text: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	events := drain(t, handle)
	last := events[len(events)-1]
	if last.Type != domain.EventRunFailed {
		t.Fatalf("terminal event = %q", last.Type)
	}
	var payload domain.ErrorPayload
	if err := json.Unmarshal(last.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(payload.Message, anthropicKey) {
		t.Fatalf("failure message leaked the key: %q", payload.Message)
	}
	run, err := store.GetRun(ctx, handle.RunID)
	if err != nil || run.Status != domain.RunFailed {
		t.Fatalf("run = %+v (err %v)", run, err)
	}
}

func TestSeam1AnthropicRetryHonorsRetryAfter(t *testing.T) {
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
	if err := store.CreateProject(ctx, &domain.Project{ID: "p1", Name: "Work"}); err != nil {
		t.Fatal(err)
	}
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, anthropicSSE("After retry."))
	}))
	defer srv.Close()

	var sleeps []time.Duration
	adapter := anthropic.Adapter{
		BaseURL: srv.URL, APIKey: anthropicKey, ProviderID: "anthropic",
		Sleep: func(_ context.Context, d time.Duration) error {
			sleeps = append(sleeps, d)
			return nil
		},
	}
	svc := newAnthropicSeamService(t, adapter, store, newIDGenerator())
	conv, err := svc.CreateConversation(ctx, "p1", "Retry chat", "chat")
	if err != nil {
		t.Fatal(err)
	}
	handle, err := svc.StartRun(ctx, domain.StartRunRequest{ConversationID: conv.ID, Text: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	events := drain(t, handle)
	if got := coalescedText(events); got != "After retry." {
		t.Fatalf("answer after retry = %q", got)
	}
	if events[len(events)-1].Type != domain.EventRunCompleted {
		t.Fatalf("terminal event = %q", events[len(events)-1].Type)
	}
	if attempts.Load() != 2 || fmt.Sprint(sleeps) != fmt.Sprint([]time.Duration{3 * time.Second}) {
		t.Fatalf("attempts = %d, sleeps = %v", attempts.Load(), sleeps)
	}
}
