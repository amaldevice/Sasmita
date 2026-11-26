package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amaldevice/sasmita/internal/appsvc"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/harness"
	"github.com/amaldevice/sasmita/internal/model/adapters/anthropic"
	"github.com/amaldevice/sasmita/internal/model/adapters/openai"
	"github.com/amaldevice/sasmita/internal/persistence/migrations"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
)

// Seam 1: a model switch rebuilds the active context from canonical history,
// and a provider switch drops blocks belonging to the old provider without
// rewriting canonical history or the new provider's append-only prefix.
func TestSeam1ModelAndProviderSwitchReSerializeFromCanonicalHistory(t *testing.T) {
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

	const rawThinking = `{"signature":"sig-abc","type":"thinking","thinking":"hidden reasoning"}`
	var anthropicBodies, openaiBodies [][]byte
	anthropicServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		anthropicBodies = append(anthropicBodies, body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, anthropicStreamWithBlocks(`[`+rawThinking+`,{"type":"text","text":"first answer"}]`,
			`{"input_tokens":7,"output_tokens":0}`, "first answer"))
	}))
	defer anthropicServer.Close()
	openAIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		openaiBodies = append(openaiBodies, body)
		writeSSE(w, "next answer")
	}))
	defer openAIServer.Close()

	from := appsvc.New(store, appsvc.Config{
		Provider:     anthropic.Adapter{BaseURL: anthropicServer.URL, ProviderID: "anthropic"},
		DefaultModel: "claude-old", NewID: newIDGenerator(),
	})
	conv, err := from.CreateConversation(ctx, "p1", "Switch", "chat")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ text, model string }{
		{"q1", ""}, {"q2", "claude-new"},
	} {
		h, err := from.StartRun(ctx, domain.StartRunRequest{
			ConversationID: conv.ID, Text: tc.text, ModelRef: tc.model,
		})
		if err != nil {
			t.Fatal(err)
		}
		drain(t, h)
	}
	if len(anthropicBodies) != 2 {
		t.Fatalf("provider received %d requests, want 2", len(anthropicBodies))
	}

	type requestBody struct {
		Model    string            `json:"model"`
		Messages []json.RawMessage `json:"messages"`
	}
	decode := func(b []byte) requestBody {
		t.Helper()
		var parsed requestBody
		if err := json.Unmarshal(b, &parsed); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return parsed
	}
	// A model switch changes only the model field; every history turn is
	// re-serialized from canonical history with identical bytes.
	// Breakpoints are wire decorations that move with the last block (#17), so
	// byte stability is asserted over the canonical, breakpoint-free request.
	first := decode(stripAnthropicBreakpoints(anthropicBodies[0]))
	second := decode(stripAnthropicBreakpoints(anthropicBodies[1]))
	if first.Model != "claude-old" || second.Model != "claude-new" {
		t.Fatalf("models after switch = %q, %q", first.Model, second.Model)
	}
	canonical := func(messages []json.RawMessage) []byte {
		out, err := json.Marshal(messages)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	if len(second.Messages) <= len(first.Messages) ||
		!bytes.Equal(canonical(first.Messages), canonical(second.Messages[:len(first.Messages)])) {
		t.Fatalf("model switch rewrote canonical history\nfirst:  %s\nsecond: %s", anthropicBodies[0], anthropicBodies[1])
	}
	if !bytes.Contains(anthropicBodies[1], []byte(rawThinking)) {
		t.Fatalf("same-provider switch lost the stored raw block: %s", anthropicBodies[1])
	}

	// Switching provider configures a different adapter over the same canonical
	// history. Anthropic-only thinking/signature blocks must not leak into the
	// OpenAI-compatible wire request.
	to := appsvc.New(store, appsvc.Config{
		Provider:     openai.Adapter{BaseURL: openAIServer.URL, ProviderID: "openai"},
		DefaultModel: "gpt-new", NewID: newIDGenerator(),
	})
	for _, text := range []string{"q3", "q4"} {
		h, err := to.StartRun(ctx, domain.StartRunRequest{ConversationID: conv.ID, Text: text, ModelRef: "gpt-new"})
		if err != nil {
			t.Fatal(err)
		}
		drain(t, h)
	}
	if len(openaiBodies) != 2 {
		t.Fatalf("new provider received %d requests, want 2", len(openaiBodies))
	}
	for _, banned := range []string{"hidden reasoning", "sig-abc", `"type":"thinking"`, `"type":"tool_use"`} {
		if bytes.Contains(openaiBodies[0], []byte(banned)) {
			t.Fatalf("provider-specific block %q leaked after provider switch: %s", banned, openaiBodies[0])
		}
	}
	if !bytes.Contains(openaiBodies[0], []byte("first answer")) || !bytes.Contains(openaiBodies[0], []byte("q1")) {
		t.Fatalf("provider switch dropped canonical text: %s", openaiBodies[0])
	}
	// After the switch, the new provider's request prefix grows append-only.
	stable := func(b []byte) []byte { return b[:len(b)-2] }
	if !bytes.HasPrefix(stable(openaiBodies[1]), stable(openaiBodies[0])) {
		t.Fatalf("new provider prefix was corrupted by the switch\nfirst:  %s\nsecond: %s", openaiBodies[0], openaiBodies[1])
	}
	messages, err := store.ListMessages(ctx, conv.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	rawPreserved := false
	for _, m := range messages {
		for _, part := range m.Content {
			if string(part.Raw) == rawThinking {
				rawPreserved = true
			}
		}
	}
	if !rawPreserved {
		t.Fatal("canonical history was rewritten by a provider switch")
	}
}

// Seam 1: the built-in Mode catalog exposes availability, unavailable Modes
// cannot be started, and the re-read warning only fires past the threshold.
func TestSeam1BuiltInModesAndRereadThreshold(t *testing.T) {
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
	svc := appsvc.New(store, appsvc.Config{
		Provider:     openai.Adapter{BaseURL: srv.URL, ProviderID: "openai"},
		DefaultModel: "gpt-test", NewID: newIDGenerator(),
	})
	conv, err := svc.CreateConversation(ctx, "p1", "Modes", "chat")
	if err != nil {
		t.Fatal(err)
	}

	modes := svc.ListModes(ctx)
	wantAvailable := map[string]bool{
		"chat": true, "riset": true, "penulisan": true, "analis-data": false, "coding": false,
	}
	if len(modes) != len(wantAvailable) {
		t.Fatalf("catalog has %d Modes, want %d", len(modes), len(wantAvailable))
	}
	for _, mode := range modes {
		want, known := wantAvailable[mode.ID]
		if !known || mode.Available != want {
			t.Fatalf("Mode %q available=%v, want %v", mode.ID, mode.Available, want)
		}
	}
	if _, err := svc.StartRun(ctx, domain.StartRunRequest{
		ConversationID: conv.ID, Text: "q", HarnessID: "coding",
	}); err == nil {
		t.Fatal("an unavailable Mode was accepted for a run")
	}

	short, err := svc.EstimateSwitch(ctx, conv.ID, "gpt-test", "chat")
	if err != nil {
		t.Fatal(err)
	}
	if short.Warn || !short.Estimated || short.EstimatedTokens > harness.RereadWarningTokens {
		t.Fatalf("short conversation should not nag: %+v", short)
	}
	if err := store.AddMessage(ctx, &domain.Message{
		ID: "long-turn", ConversationID: conv.ID, Role: domain.RoleUser,
		Content: domain.TextContent(strings.Repeat("a", 20000)), CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	long, err := svc.EstimateSwitch(ctx, conv.ID, "gpt-new", "riset")
	if err != nil {
		t.Fatal(err)
	}
	if !long.Warn || long.EstimatedTokens <= harness.RereadWarningTokens {
		t.Fatalf("long conversation should warn: %+v", long)
	}

	h, err := svc.StartRun(ctx, domain.StartRunRequest{
		ConversationID: conv.ID, Text: "q", ModelRef: "gpt-new", HarnessID: "riset",
	})
	if err != nil {
		t.Fatal(err)
	}
	drain(t, h)
	if len(bodies) != 1 {
		t.Fatalf("provider received %d requests, want 1", len(bodies))
	}
	mode, err := harness.Select("riset")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(bodies[0], []byte(mode.Instructions)) {
		t.Fatalf("Mode instructions missing from active context: %s", bodies[0])
	}
	if n := bytes.Count(bodies[0], []byte(`"role":"system"`)); n != 2 {
		t.Fatalf("request has %d system messages, want product rules + Mode instructions: %s", n, bodies[0])
	}
}
