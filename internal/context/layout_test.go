package context_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	ctxlayout "github.com/amaldevice/sasmita/internal/context"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/model"
)

func TestSessionKeyIsStableHashOfConversationID(t *testing.T) {
	// SHA-256("conv-123"), first 16 bytes, hex — an independent known value.
	const want = "1723376043e8a9d0090025f147440b99"
	if got := ctxlayout.SessionKey("conv-123"); got != want {
		t.Fatalf("SessionKey = %q, want %q", got, want)
	}
	if ctxlayout.SessionKey("conv-123") != ctxlayout.SessionKey("conv-123") {
		t.Fatal("SessionKey is not deterministic")
	}
	if ctxlayout.SessionKey("conv-123") == ctxlayout.SessionKey("conv-124") {
		t.Fatal("different conversations share a session key")
	}
}

func TestBuildUsesFrozenToolsStaticSystemAndAppendOnlyHistory(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`)
	builder := ctxlayout.Builder{
		Tools: []model.ToolDefinition{{Name: "lookup", Description: "find things", InputSchema: schema}},
	}
	first := []domain.Message{
		{ID: "m1", Role: domain.RoleUser, Content: domain.TextContent("q1")},
		{ID: "m2", Role: domain.RoleAssistant, Content: domain.TextContent("a1")},
	}
	second := append(append([]domain.Message{}, first...), domain.Message{
		ID: "m3", Role: domain.RoleUser, Content: []domain.ContentPart{
			{Type: "text", Text: "q2"},
			{Type: "context_note", Text: "mode:coding"},
		},
	})

	req1 := builder.Build("conv-123", "gpt-test", "run-1", first)
	req2 := builder.Build("conv-123", "gpt-test", "run-2", second)

	if req1.PromptCacheKey != ctxlayout.SessionKey("conv-123") || req2.PromptCacheKey != req1.PromptCacheKey {
		t.Fatalf("cache keys = %q, %q", req1.PromptCacheKey, req2.PromptCacheKey)
	}
	if len(req1.Tools) != 1 || req1.Tools[0].Name != "lookup" || string(req1.Tools[0].InputSchema) != string(schema) {
		t.Fatalf("tools = %+v", req1.Tools)
	}
	if len(req1.Messages) != len(first)+1 || len(req2.Messages) != len(second)+1 {
		t.Fatalf("message counts = %d, %d", len(req1.Messages), len(req2.Messages))
	}
	system := req1.Messages[0]
	if system.Role != "system" || system.Content != ctxlayout.StaticSystemPrompt {
		t.Fatalf("system message = %+v", system)
	}
	// No per-request state in the system instruction (ADR-0001 anti-patterns).
	for _, banned := range []string{time.Now().Format("2006-01-02"), "cwd", "git", "workspace"} {
		if strings.Contains(strings.ToLower(system.Content), strings.ToLower(banned)) {
			t.Errorf("system prompt contains dynamic %q: %q", banned, system.Content)
		}
	}

	// The dynamic note lives only on the final user turn, as structured blocks.
	last := req2.Messages[len(req2.Messages)-1]
	if last.Role != "user" || len(last.Blocks) != 2 {
		t.Fatalf("final user turn = %+v", last)
	}
	if !bytes.Contains(last.Blocks[1], []byte("mode:coding")) {
		t.Fatalf("context note missing from user turn: %s", last.Blocks[1])
	}

	// Every earlier message survives the next turn byte-for-byte.
	b1, err := json.Marshal(req1.Messages)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := json.Marshal(req2.Messages)
	if err != nil {
		t.Fatal(err)
	}
	stable := func(b []byte) []byte { return b[:len(b)-2] }
	if !bytes.HasPrefix(stable(b2), stable(b1)) {
		t.Fatalf("history not append-only\nturn 1: %s\nturn 2: %s", b1, b2)
	}
}

func TestBuildReplaysRawProviderBlocksByteIdentically(t *testing.T) {
	// Deliberately unusual key order: a provider block must be replayed exactly
	// as stored, not re-serialized through a fixed struct (ADR-0002).
	raw := json.RawMessage(`{"signature":"sig-abc","type":"thinking","thinking":"hmm"}`)
	builder := ctxlayout.Builder{}
	history := []domain.Message{
		{ID: "m1", Role: domain.RoleUser, Content: domain.TextContent("q")},
		{ID: "m2", Role: domain.RoleAssistant, Content: []domain.ContentPart{{Type: "thinking", Raw: raw}}},
	}
	req := builder.Build("conv-123", "m", "run", history)
	assistant := req.Messages[len(req.Messages)-1]
	if len(assistant.Blocks) != 1 || !bytes.Equal(assistant.Blocks[0], []byte(raw)) {
		t.Fatalf("raw block not replayed byte-identically: %s", assistant.Blocks)
	}
}
