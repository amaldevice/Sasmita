package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"

	ctxlayout "github.com/amaldevice/sasmita/internal/context"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/tools"
)

// loopAgent is an ADK Agent implementation that runs the app-owned provider
// port and tool dispatcher. Eino owns the run lifecycle; our adapters own the
// wire bytes (ADR-0002), so schema.Message never becomes a request body.
type loopAgent struct {
	engine  *engine
	req     Request
	emit    func(domain.AgentSignal)
	working []domain.Message
	builder ctxlayout.Builder
}

func (l *loopAgent) Name(context.Context) string { return "sasmita.agent" }
func (l *loopAgent) Description(context.Context) string {
	return "Runs built-in tools while answering."
}

func (l *loopAgent) Run(ctx context.Context, _ *adk.AgentInput, _ ...adk.AgentRunOption) *adk.AsyncIterator[*adk.AgentEvent] {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	go func() {
		defer gen.Close()
		result, err := l.execute(ctx, func(sig domain.AgentSignal) {
			gen.Send(&adk.AgentEvent{Output: &adk.AgentOutput{CustomizedOutput: sig}})
		})
		if err != nil {
			gen.Send(&adk.AgentEvent{Err: err})
			return
		}
		gen.Send(&adk.AgentEvent{Output: &adk.AgentOutput{CustomizedOutput: result}})
	}()
	return iter
}

func (l *loopAgent) execute(ctx context.Context, emit func(domain.AgentSignal)) (Result, error) {
	var result Result
	if l.req.CheckpointID != "" {
		// A resumed attempt continues from the state saved at the approval
		// wait, not from a fresh active context.
		l.restoreCheckpoint(ctx)
	}
	provider := l.req.Provider
	if provider == nil {
		provider = l.engine.cfg.Provider
	}
	var allText strings.Builder
	for turn := 0; turn < l.engine.cfg.MaxTurns; turn++ {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		request := l.builder.Build(l.req.ConversationID, l.req.ModelRef, l.req.RunID, l.working)
		var calls []model.ToolCall
		var blocks []json.RawMessage
		var text strings.Builder
		err := provider.Stream(ctx, request, func(ev model.StreamEvent) {
			switch ev.Kind {
			case model.EventTextDelta:
				text.WriteString(ev.Text)
				emit(domain.AgentSignal{Kind: domain.AgentTextDelta, Text: ev.Text})
			case model.EventUsage:
				if ev.Usage != nil {
					addUsage(&result.Usage, ev.Usage)
				}
			case model.EventToolUse:
				if ev.ToolCall != nil {
					calls = append(calls, *ev.ToolCall)
				}
			case model.EventContentBlocks:
				blocks = append(blocks, ev.ContentBlocks...)
			}
		})
		if err != nil {
			return result, err
		}
		if err := ctx.Err(); err != nil {
			return result, err
		}
		allText.WriteString(text.String())
		if len(calls) == 0 {
			result.Text = allText.String()
			result.Blocks = blocks
			return result, nil
		}
		if turn == l.engine.cfg.MaxTurns-1 {
			return result, fmt.Errorf("agent exceeded %d tool turns", l.engine.cfg.MaxTurns)
		}
		if err := l.dispatchCalls(ctx, calls, blocks, emit); err != nil {
			return result, err
		}
	}
	return result, fmt.Errorf("agent exceeded %d tool turns", l.engine.cfg.MaxTurns)
}

func (l *loopAgent) dispatchCalls(ctx context.Context, calls []model.ToolCall, blocks []json.RawMessage, emit func(domain.AgentSignal)) error {
	rawByID := make(map[string]json.RawMessage, len(blocks))
	for _, raw := range blocks {
		var probe struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		if json.Unmarshal(raw, &probe) == nil && probe.Type == "tool_use" {
			rawByID[probe.ID] = append(json.RawMessage(nil), raw...)
		}
	}
	assistantBlocks := make([]json.RawMessage, 0, len(calls))
	resultBlocks := make([]json.RawMessage, 0, len(calls))
	for _, call := range calls {
		if err := ctx.Err(); err != nil {
			return err
		}
		// A meta-tool call is resolved to the real tool before policy, approval
		// and execution. The assistant block stays the envelope the model sent.
		effective := call
		viaMeta := false
		var err error
		if call.Name == "call_tool" && l.engine.cfg.Discovery != nil {
			var real model.ToolCall
			real, err = l.engine.cfg.Discovery.Resolve(call)
			if err != nil {
				activity := domain.ToolActivity{CallID: call.ID, Name: call.Name, Executed: false, Error: err.Error()}
				emit(domain.AgentSignal{Kind: domain.AgentToolFailed, Tool: activity})
				resultBlocks = append(resultBlocks, toolResultBlock(call.ID, "error: "+err.Error()))
				continue
			}
			effective, viaMeta = real, true
			l.engine.cfg.Discovery.Note(l.req.ConversationID, real.Name)
		}
		risk := "unknown"
		if l.engine.cfg.Tools != nil {
			if spec, ok := l.engine.cfg.Tools.Spec(effective.Name); ok {
				risk = string(spec.Risk)
			}
		}
		emit(domain.AgentSignal{
			Kind: domain.AgentToolProposed,
			Tool: domain.ToolActivity{
				CallID: call.ID, Name: effective.Name, Risk: risk, Input: effective.Input,
				Executed: false,
			},
		})
		assistantBlocks = append(assistantBlocks, toolUseBlock(call, rawByID[call.ID]))

		if gate := l.engine.cfg.Approvals; gate != nil {
			// Persist the pause state before blocking: the process may die
			// while the approval card is still on screen (TAD §14.3).
			l.saveCheckpoint(ctx)
			verdict, err := gate.Authorize(ctx, ApprovalRequest{
				RunID: l.req.RunID, CheckpointID: l.checkpointID(),
				ConversationID: l.req.ConversationID, CallID: call.ID, ToolID: effective.Name,
				Mode: l.req.Mode, ProjectID: l.req.ProjectID,
				Arguments: effective.Input, ScopeRoots: l.req.ScopeRoots,
				OnPending: func(a *domain.Approval) error {
					emit(domain.AgentSignal{Kind: domain.AgentApprovalRequired, Approval: a})
					return nil
				},
			})
			if err != nil {
				return err
			}
			if verdict.Denied {
				reason := verdict.Reason
				if reason == "" {
					reason = "tool was not approved"
				}
				activity := domain.ToolActivity{
					CallID: call.ID, Name: effective.Name, Risk: risk, Input: effective.Input,
					Executed: false, Error: reason,
				}
				emit(domain.AgentSignal{Kind: domain.AgentToolDenied, Tool: activity})
				resultBlocks = append(resultBlocks, toolResultBlock(call.ID, "denied: "+reason))
				continue
			}
		}

		started := time.Now()
		var out tools.Result
		if viaMeta {
			out, err = l.engine.cfg.Discovery.Call(ctx, effective)
		} else {
			out, err = l.engine.cfg.Tools.Execute(ctx, call)
		}
		activity := domain.ToolActivity{
			CallID: call.ID, Name: effective.Name, Risk: risk, Input: effective.Input,
			Executed: err == nil, DurationMS: time.Since(started).Milliseconds(),
		}
		if err != nil {
			activity.Error = err.Error()
			emit(domain.AgentSignal{Kind: domain.AgentToolFailed, Tool: activity})
			resultBlocks = append(resultBlocks, toolResultBlock(call.ID, "error: "+err.Error()))
			continue
		}
		activity.Output, activity.Summary = out.Output, out.Summary
		activity.ArtifactID, activity.ArtifactRef, activity.Truncated = out.ArtifactID, out.ArtifactRef, out.Truncated
		if out.Truncated {
			emit(domain.AgentSignal{Kind: domain.AgentToolArtifact, Tool: activity})
		} else {
			emit(domain.AgentSignal{Kind: domain.AgentToolExecuted, Tool: activity})
		}
		if effective.Name == "render_ui" {
			l.emitUIBlock(out.Output, emit)
		}
		content := out.Output
		if out.Truncated {
			content += " " + out.ArtifactRef
		}
		resultBlocks = append(resultBlocks, toolResultBlock(call.ID, content))
	}
	l.working = append(l.working,
		domain.Message{
			ID: l.newID(), ConversationID: l.req.ConversationID, Role: domain.RoleAssistant,
			Content: rawParts(assistantBlocks), CreatedAt: time.Now(),
		},
		domain.Message{
			ID: l.newID(), ConversationID: l.req.ConversationID, Role: domain.RoleUser,
			Content: rawParts(resultBlocks), CreatedAt: time.Now(),
		},
	)
	emit(domain.AgentSignal{
		Kind: domain.AgentToolPair,
		Tool: domain.ToolActivity{
			AssistantBlocks: assistantBlocks, ResultBlocks: resultBlocks, Executed: true,
		},
	})
	return nil
}

// emitUIBlock turns the small render_ui acknowledgement into a signal carrying
// the host-side spec. The spec never travelled back through the model request.
func (l *loopAgent) emitUIBlock(output string, emit func(domain.AgentSignal)) {
	var ack struct {
		Kind    string `json:"kind"`
		BlockID string `json:"blockId"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(output), &ack); err != nil {
		return
	}
	block := &domain.UIBlockSignal{BlockID: ack.BlockID, Reason: ack.Reason}
	if ack.Kind == "ui_block" {
		if l.engine.cfg.GenUI == nil {
			block.Reason = "UI block store is not configured"
		} else if spec, ok := l.engine.cfg.GenUI.Get(ack.BlockID); ok {
			block.Spec = spec
		} else {
			block.Reason = "UI block spec expired"
		}
	}
	emit(domain.AgentSignal{Kind: domain.AgentUIBlock, UI: block})
}

func (l *loopAgent) newID() string {
	if l.engine.cfg.NewID != nil {
		return l.engine.cfg.NewID()
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func rawParts(blocks []json.RawMessage) []domain.ContentPart {
	parts := make([]domain.ContentPart, 0, len(blocks))
	for _, raw := range blocks {
		var probe struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(raw, &probe)
		parts = append(parts, domain.ContentPart{Type: probe.Type, Raw: append(json.RawMessage(nil), raw...)})
	}
	return parts
}

func toolUseBlock(call model.ToolCall, raw json.RawMessage) json.RawMessage {
	if len(raw) > 0 {
		return raw
	}
	encoded, _ := model.MarshalDeterministic(struct {
		Type  string          `json:"type"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	}{Type: "tool_use", ID: call.ID, Name: call.Name, Input: call.Input})
	return encoded
}

func toolResultBlock(callID, content string) json.RawMessage {
	encoded, _ := model.MarshalDeterministic(struct {
		Type      string `json:"type"`
		ToolUseID string `json:"tool_use_id"`
		Content   string `json:"content"`
	}{Type: "tool_result", ToolUseID: callID, Content: content})
	return encoded
}

func addUsage(total **model.Usage, u *model.Usage) {
	if u == nil {
		return
	}
	if *total == nil {
		copied := *u
		*total = &copied
		return
	}
	t := *total
	t.InputTokens += u.InputTokens
	t.OutputTokens += u.OutputTokens
	t.CacheReadTokens += u.CacheReadTokens
	t.CacheWriteTokens += u.CacheWriteTokens
	t.UncachedTokens += u.UncachedTokens
	t.Estimated = t.Estimated || u.Estimated
}
