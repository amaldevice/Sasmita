package agent_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/amaldevice/sasmita/internal/agent"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/tools"
)

// scriptedProvider crosses only the app-owned model port: no ADK type appears
// in this test's request or event vocabulary.
type scriptedProvider struct{}

func (scriptedProvider) Name() string { return "fake" }

func (scriptedProvider) Stream(_ context.Context, req model.StreamRequest, emit func(model.StreamEvent)) error {
	if len(req.Messages) <= 2 {
		emit(model.StreamEvent{Kind: model.EventTextDelta, Text: "calling "})
		emit(model.StreamEvent{Kind: model.EventToolUse, ToolCall: &model.ToolCall{
			ID: "c1", Name: "calculator", Input: json.RawMessage(`{"expression":"6*7"}`),
		}})
		return nil
	}
	emit(model.StreamEvent{Kind: model.EventTextDelta, Text: "42"})
	return nil
}

func TestEngineRunsToolLoopThroughDomainSignals(t *testing.T) {
	registry := tools.New(tools.Config{ArtifactsDir: t.TempDir()})
	engine := agent.New(agent.Config{Provider: scriptedProvider{}, Tools: registry, NewID: func() string { return "id" }})
	var signals []domain.AgentSignal
	result, err := engine.Run(context.Background(), agent.Request{
		ConversationID: "conv-1", RunID: "run-1", ModelRef: "fake-model",
		History: []domain.Message{{ID: "u1", Role: domain.RoleUser, Content: domain.TextContent("compute")}},
	}, func(sig domain.AgentSignal) { signals = append(signals, sig) })
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "calling 42" {
		t.Fatalf("result text = %q", result.Text)
	}
	seen := map[domain.AgentSignalKind]domain.ToolActivity{}
	for _, sig := range signals {
		seen[sig.Kind] = sig.Tool
	}
	for _, kind := range []domain.AgentSignalKind{
		domain.AgentTextDelta, domain.AgentToolProposed, domain.AgentToolExecuted,
		domain.AgentToolPair,
	} {
		if _, ok := seen[kind]; !ok {
			t.Errorf("missing signal %q", kind)
		}
	}
	if seen[domain.AgentToolProposed].Executed {
		t.Fatal("a proposal must not be marked executed")
	}
	if !seen[domain.AgentToolExecuted].Executed || !strings.Contains(seen[domain.AgentToolExecuted].Output, "42") {
		t.Fatalf("executed activity = %+v", seen[domain.AgentToolExecuted])
	}
	if len(seen[domain.AgentToolPair].AssistantBlocks) != 1 || len(seen[domain.AgentToolPair].ResultBlocks) != 1 {
		t.Fatalf("tool pair = %+v", seen[domain.AgentToolPair])
	}
}
