package context_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	ctxlayout "github.com/amaldevice/sasmita/internal/context"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/model"
)

type recordingProvider struct {
	requests []model.StreamRequest
	text     string
}

func (p *recordingProvider) Name() string { return "fake" }
func (p *recordingProvider) Stream(_ context.Context, req model.StreamRequest, emit func(model.StreamEvent)) error {
	p.requests = append(p.requests, req)
	emit(model.StreamEvent{Kind: model.EventTextDelta, Text: p.text})
	return nil
}

func message(id string, role domain.MessageRole, text string) domain.Message {
	return domain.Message{ID: id, Role: role, Content: domain.TextContent(text)}
}

// Compaction cuts before the last two user turns and never inside a tool-call
// pair: the assistant tool_use and its user tool_result stay in the kept tail
// (TAD §11.5, ADR-0001).
func TestSplitKeepsLastTwoUserTurnsWithCompleteToolPairs(t *testing.T) {
	rawUse := json.RawMessage(`{"type":"tool_use","id":"tu-1","name":"calculator","input":{}}`)
	rawResult := json.RawMessage(`{"type":"tool_result","tool_use_id":"tu-1","content":"20"}`)
	history := []domain.Message{
		message("u1", domain.RoleUser, "first question"),
		{ID: "a1", Role: domain.RoleAssistant, Content: []domain.ContentPart{{Type: "tool_use", Raw: rawUse}}},
		{ID: "u2", Role: domain.RoleUser, Content: []domain.ContentPart{{Type: "tool_result", Raw: rawResult}}},
		message("a2", domain.RoleAssistant, "answer one"),
		message("u3", domain.RoleUser, "second question"),
		message("a3", domain.RoleAssistant, "answer two"),
	}

	intro, tail := ctxlayout.Split(history, 2)
	if got := ids(intro); fmt.Sprint(got) != "[u1]" {
		t.Fatalf("omitted = %v, want [u1]", got)
	}
	if got := ids(tail); fmt.Sprint(got) != "[a1 u2 a2 u3 a3]" {
		t.Fatalf("kept = %v, want [a1 u2 a2 u3 a3]", got)
	}
	if len(tail) > 0 && string(tail[0].Content[0].Raw) != string(rawUse) {
		t.Fatalf("tool pair was split: first kept message = %+v", tail[0])
	}

	// Too little history: nothing is omitted.
	intro, tail = ctxlayout.Split(history[:2], 2)
	if len(intro) != 0 || fmt.Sprint(ids(tail)) != "[u1 a1]" {
		t.Fatalf("short history: omitted=%v kept=%v", ids(intro), ids(tail))
	}
}

// Auto compaction triggers at ~75% of the input budget, not at a fixed number
// of messages or on every turn (TAD §11.3, §19 config).
func TestShouldCompactAtThreeQuartersOfInputBudget(t *testing.T) {
	compactor := ctxlayout.Compactor{InputBudgetTokens: 100}
	below := []domain.Message{message("u1", domain.RoleUser, strings.Repeat("x", 280))} // 74 estimated tokens
	if compactor.ShouldCompact(below) {
		t.Fatal("compacted below the 75% threshold")
	}
	at := []domain.Message{message("u1", domain.RoleUser, strings.Repeat("x", 284))} // 75 estimated tokens
	if !compactor.ShouldCompact(at) {
		t.Fatal("did not compact at the 75% threshold")
	}
	strict := ctxlayout.Compactor{InputBudgetTokens: 100, Threshold: 0.9}
	if strict.ShouldCompact(at) {
		t.Fatal("ignored a custom threshold")
	}
}

// The summary request is appended to the cached prefix with the same model and
// frozen tools; its result becomes a deterministically structured message
// followed by the verbatim tail (TAD §11.5).
func TestCompactAppendsSummaryRequestToCachedPrefixAndKeepsTailVerbatim(t *testing.T) {
	rawUse := json.RawMessage(`{"type":"tool_use","id":"tu-1","name":"calculator","input":{}}`)
	rawResult := json.RawMessage(`{"type":"tool_result","tool_use_id":"tu-1","content":"20"}`)
	history := []domain.Message{
		message("u1", domain.RoleUser, "first question"),
		{ID: "a1", Role: domain.RoleAssistant, Content: []domain.ContentPart{{Type: "tool_use", Raw: rawUse}}},
		{ID: "u2", Role: domain.RoleUser, Content: []domain.ContentPart{{Type: "tool_result", Raw: rawResult}}},
		message("a2", domain.RoleAssistant, "answer one"),
		message("u3", domain.RoleUser, "second question"),
		message("a3", domain.RoleAssistant, "answer two"),
	}
	provider := &recordingProvider{text: `{"goal":["Ship v1"],"decisions":["Append-only"],"plan":["Verify cache"]}`}
	schema := json.RawMessage(`{"type":"object"}`)
	tools := []model.ToolDefinition{{Name: "calculator", InputSchema: schema}}
	compactor := ctxlayout.Compactor{
		Provider: provider,
		Builder:  ctxlayout.Builder{Tools: tools},
		NewID:    func() string { return "cmp-1" },
		Now:      func() time.Time { return time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC) },
	}

	record, active, err := compactor.Compact(context.Background(), "conv-1", "gpt-test", "run-2", history)
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(provider.requests))
	}
	req := provider.requests[0]
	if req.Model != "gpt-test" || req.PromptCacheKey != ctxlayout.SessionKey("conv-1") {
		t.Fatalf("summary request model=%q key=%q", req.Model, req.PromptCacheKey)
	}
	if !reflect.DeepEqual(req.Tools, tools) {
		t.Fatalf("summary request tools = %+v, want %+v", req.Tools, tools)
	}
	cached := compactor.Builder.Build("conv-1", "gpt-test", "run-1", history)
	if len(req.Messages) != len(cached.Messages)+1 {
		t.Fatalf("summary request has %d messages, want %d", len(req.Messages), len(cached.Messages)+1)
	}
	for i := range cached.Messages {
		if !reflect.DeepEqual(req.Messages[i], cached.Messages[i]) {
			t.Fatalf("cached prefix changed at %d: %+v vs %+v", i, req.Messages[i], cached.Messages[i])
		}
	}
	if req.Messages[len(req.Messages)-1].Role != "user" ||
		!strings.Contains(req.Messages[len(req.Messages)-1].Content, "compact-summary-request") {
		t.Fatalf("summary request not appended: %+v", req.Messages[len(req.Messages)-1])
	}

	if fmt.Sprint(record.OmittedMessageIDs) != "[u1]" {
		t.Fatalf("omitted = %v, want [u1]", record.OmittedMessageIDs)
	}
	if fmt.Sprint(record.KeptMessageIDs) != "[a1 u2 a2 u3 a3]" {
		t.Fatalf("kept = %v", record.KeptMessageIDs)
	}
	if record.Summary == "" || !strings.Contains(record.Summary, "goal:") ||
		!strings.Contains(record.Summary, "Ship v1") || !strings.Contains(record.Summary, "plan:") {
		t.Fatalf("summary is not structured: %q", record.Summary)
	}
	if record.ID != "cmp-1" || record.CreatedAt.IsZero() || record.EstimatedTokens <= 0 {
		t.Fatalf("record = %+v", record)
	}
	if len(active) != 6 || active[0].Role != domain.RoleSystem || active[0].Text() != record.Summary {
		t.Fatalf("new prefix = %+v", active)
	}
	if fmt.Sprint(ids(active[1:])) != "[a1 u2 a2 u3 a3]" {
		t.Fatalf("kept tail = %v", ids(active[1:]))
	}
	if string(active[1].Content[0].Raw) != string(rawUse) || string(active[2].Content[0].Raw) != string(rawResult) {
		t.Fatalf("tool pair was not kept verbatim: %+v", active[1:4])
	}
}

func ids(messages []domain.Message) []string {
	out := make([]string, 0, len(messages))
	for _, m := range messages {
		out = append(out, m.ID)
	}
	return out
}
