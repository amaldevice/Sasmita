package appsvc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/amaldevice/sasmita/internal/agent"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/harness"
	"github.com/amaldevice/sasmita/internal/model"
)

// eventStream is a bounded channel for UI delivery. Coalescable events are
// dropped under backpressure; terminal and approval events block instead so
// they are never lost for a consumer that is still reading (TAD §14.3).
type eventStream struct {
	ch        chan domain.RunEvent
	done      chan struct{}
	closeOnce sync.Once
	doneOnce  sync.Once
}

func newEventStream(size int) *eventStream {
	return &eventStream{ch: make(chan domain.RunEvent, size), done: make(chan struct{})}
}

func (s *eventStream) send(ev domain.RunEvent, important, terminal bool) bool {
	if important {
		select {
		case s.ch <- ev:
			if terminal {
				s.closeOnce.Do(func() { close(s.ch) })
			}
			return true
		case <-s.done:
			return false
		}
	}
	select {
	case s.ch <- ev:
		return true
	default:
		return false
	}
}

func (s *eventStream) abort() {
	s.doneOnce.Do(func() { close(s.done) })
}

// RunHandle is the typed stream handle returned by StartRun/ResumeRun.
type RunHandle struct {
	RunID          string
	ConversationID string
	stream         *eventStream
}

func (h *RunHandle) Events() <-chan domain.RunEvent { return h.stream.ch }

// Abort releases the sender when the UI stops reading; terminal state is still
// persisted in the run record and recoverable via GetRunSnapshot.
func (h *RunHandle) Abort() { h.stream.abort() }

func envelope() domain.Envelope { return domain.Envelope{Version: domain.RunEventSchemaVersion} }

// StartRun persists the run record and user message first, then returns a
// handle whose event channel carries the versioned RunEvent stream.
func (s *Service) StartRun(ctx context.Context, req domain.StartRunRequest) (*RunHandle, error) {
	return s.start(ctx, req, "")
}

// ResumeRun starts a new attempt tied to an interrupted run. The original user
// turn stays canonical; the new run reuses it as active context (TAD §8.3).
func (s *Service) ResumeRun(ctx context.Context, originalRunID string) (*RunHandle, error) {
	original, err := s.store.GetRun(ctx, originalRunID)
	if err != nil {
		return nil, err
	}
	if original.Status != domain.RunInterrupted {
		return nil, domain.ErrIllegalTransition
	}
	messages, err := s.store.ListMessages(ctx, original.ConversationID, 0)
	if err != nil {
		return nil, err
	}
	text := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == domain.RoleUser {
			text = messages[i].Text()
			break
		}
	}
	if strings.TrimSpace(text) == "" {
		return nil, domain.ErrInvalidArgument
	}
	return s.start(ctx, domain.StartRunRequest{
		ConversationID: original.ConversationID,
		Text:           text,
		ModelRef:       original.ModelRef,
		HarnessID:      original.HarnessID,
	}, original.ID)
}

func (s *Service) start(ctx context.Context, req domain.StartRunRequest, originalRunID string) (*RunHandle, error) {
	if strings.TrimSpace(req.Text) == "" {
		return nil, domain.ErrInvalidArgument
	}
	if _, err := s.store.GetConversation(ctx, req.ConversationID); err != nil {
		return nil, err
	}
	modelRef := req.ModelRef
	if modelRef == "" {
		modelRef = s.cfg.DefaultModel
	}
	modeID := req.HarnessID
	if modeID == "" {
		modeID = defaultHarness
	}
	if _, err := harness.Select(modeID); err != nil {
		return nil, err
	}
	now := s.cfg.Now()
	run := &domain.Run{
		ID:             s.cfg.NewID(),
		ConversationID: req.ConversationID,
		Status:         domain.RunQueued,
		ModelRef:       modelRef,
		HarnessID:      modeID,
		StartedAt:      now,
		OriginalRunID:  originalRunID,
	}
	content := domain.TextContent(req.Text)
	// Dynamic information is a tail context note on the new user turn, never
	// part of the static system instruction (ADR-0001). It carries only deltas
	// since the previous note (GLOSSARY "context note").
	var note string
	if s.cfg.NoteSource != nil {
		note = s.notes.Next(now, s.cfg.NoteSource())
	} else if s.cfg.ContextNote != nil {
		note = s.notes.Observe(now, s.cfg.ContextNote())
	}
	if note = strings.TrimSpace(note); note != "" {
		content = append(content, domain.ContentPart{Type: "context_note", Text: note})
	}
	userMessage := &domain.Message{
		ID:             s.cfg.NewID(),
		ConversationID: req.ConversationID,
		Role:           domain.RoleUser,
		Content:        content,
		CreatedAt:      now,
	}
	if originalRunID != "" {
		if err := s.store.StartRetryTx(ctx, run, now); err != nil {
			return nil, err
		}
	} else if err := s.store.StartRunTx(ctx, run, userMessage, now); err != nil {
		return nil, err
	}

	stream := newEventStream(128)
	if err := s.emit(ctx, stream, run.ID, domain.EventRunQueued,
		domain.StatusPayload{Envelope: envelope(), Status: string(domain.RunQueued)}); err != nil {
		return nil, err
	}

	// A disconnected frontend must not kill an in-flight run; CancelRun is the
	// explicit cancellation path from UI to provider (TAD §14.3).
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	handle := &RunHandle{RunID: run.ID, ConversationID: run.ConversationID, stream: stream}
	s.runtime.Register(run.ID, cancel, func(eventType string, payload json.RawMessage, important bool) error {
		return s.emitRaw(context.WithoutCancel(ctx), stream, run.ID, eventType, payload, important)
	})
	go s.execute(runCtx, run, stream, cancel)
	return handle, nil
}

// CancelRun cancels the provider context for an in-flight run. The run reaches
// its cancelled terminal state asynchronously; drain Events or read the
// snapshot to observe it.
func (s *Service) CancelRun(ctx context.Context, runID string) error {
	if err := s.runtime.Cancel(runID); err == nil {
		return nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if run.Status.Terminal() || run.Status == domain.RunInterrupted {
		return domain.ErrTerminalRun
	}
	return domain.ErrNotFound
}

// MarkAwaitingApproval represents a sensitive-tool wait in run state. It does
// not decide policy: the approval engine (ticket #23) decides when to call it.
func (s *Service) MarkAwaitingApproval(ctx context.Context, runID, message string) error {
	if err := s.store.UpdateRunStatus(ctx, runID, domain.RunAwaitingApproval, nil); err != nil {
		return err
	}
	raw, err := json.Marshal(domain.ApprovalPayload{
		Envelope: envelope(), Status: string(domain.RunAwaitingApproval), Message: message,
	})
	if err != nil {
		return err
	}
	return s.runtime.Emit(runID, domain.EventApprovalRequired, raw, true)
}

// RecoverStaleRuns is called at startup: every run still active in the database
// belonged to a process that is gone, so it becomes interrupted. The event is
// appended with the next monotonic seq, keeping terminal events intact.
func (s *Service) RecoverStaleRuns(ctx context.Context) ([]domain.Run, error) {
	runs, err := s.store.ListActiveRuns(ctx)
	if err != nil {
		return nil, err
	}
	recovered := make([]domain.Run, 0, len(runs))
	for _, run := range runs {
		now := s.cfg.Now()
		if err := s.store.UpdateRunStatus(ctx, run.ID, domain.RunInterrupted, &now); err != nil {
			if errors.Is(err, domain.ErrTerminalRun) || errors.Is(err, domain.ErrNotFound) {
				continue
			}
			return nil, err
		}
		_ = s.runtime.Cancel(run.ID)
		raw, err := json.Marshal(domain.StatusPayload{
			Envelope: envelope(), Status: string(domain.RunInterrupted),
		})
		if err != nil {
			return nil, err
		}
		if _, err := s.store.AppendRunEvent(ctx, run.ID, domain.EventRunInterrupted, raw); err != nil {
			return nil, err
		}
		run.Status, run.FinishedAt = domain.RunInterrupted, &now
		recovered = append(recovered, run)
	}
	return recovered, nil
}

func (s *Service) emit(ctx context.Context, stream *eventStream, runID, eventType string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.emitRaw(ctx, stream, runID, eventType, raw, domain.IsImportantEvent(eventType))
}

func (s *Service) emitRaw(ctx context.Context, stream *eventStream, runID, eventType string, payload json.RawMessage, important bool) error {
	ev, err := s.store.AppendRunEvent(ctx, runID, eventType, payload)
	if err != nil {
		return err
	}
	stream.send(ev, important, false)
	return nil
}

// emitTerminal persists and delivers the final event without blocking on ctx.
// A terminal event is never dropped for ordinary stream pressure.
func (s *Service) emitTerminal(ctx context.Context, stream *eventStream, runID, eventType string, payload any) {
	raw, _ := json.Marshal(payload)
	ev, err := s.store.AppendRunEvent(ctx, runID, eventType, raw)
	if err != nil {
		// The authoritative terminal state lives in the runs row; still emit
		// an event so the UI can close the stream.
		ev = domain.RunEvent{RunID: runID, Type: eventType, Payload: raw}
	}
	stream.send(ev, true, true)
}

func (s *Service) execute(ctx context.Context, run *domain.Run, stream *eventStream, cancel context.CancelFunc) {
	defer cancel()
	defer s.runtime.Finish(run.ID)
	persistCtx := context.WithoutCancel(ctx)

	if err := s.store.UpdateRunStatus(persistCtx, run.ID, domain.RunPreparing, nil); err != nil {
		s.fail(ctx, persistCtx, run, stream, err)
		return
	}
	s.emit(persistCtx, stream, run.ID, domain.EventRunPreparing,
		domain.StatusPayload{Envelope: envelope(), Status: string(domain.RunPreparing)})
	if err := s.store.UpdateRunStatus(persistCtx, run.ID, domain.RunRunning, nil); err != nil {
		s.fail(ctx, persistCtx, run, stream, err)
		return
	}
	s.emit(persistCtx, stream, run.ID, domain.EventRunStarted,
		domain.StatusPayload{Envelope: envelope(), Status: string(domain.RunRunning)})
	s.emit(persistCtx, stream, run.ID, domain.EventModelRequestStarted, envelope())

	// Active context is a derived view of canonical history: Mode instructions,
	// provider filtering and compaction never rewrite stored messages. The ADK
	// agent builds each request from this view plus appended tool pairs.
	_, view, err := s.activeContext(persistCtx, run.ConversationID, run.HarnessID)
	if err != nil {
		s.fail(ctx, persistCtx, run, stream, err)
		return
	}

	var full, pending strings.Builder
	var contentBlocks []json.RawMessage
	flush := func() {
		if pending.Len() == 0 {
			return
		}
		s.emit(persistCtx, stream, run.ID, domain.EventModelTextDelta,
			domain.TextDeltaPayload{Envelope: envelope(), Text: pending.String()})
		pending.Reset()
	}

	// The ADK-backed agent owns the tool loop. It appends tool call/result
	// pairs to canonical history through the signals below; the earlier prefix
	// is never rewritten (ADR-0001).
	result, streamErr := s.cfg.Agent.Run(ctx, agent.Request{
		ConversationID: run.ConversationID, RunID: run.ID, ModelRef: run.ModelRef, History: view,
	}, func(sig domain.AgentSignal) {
		switch sig.Kind {
		case domain.AgentTextDelta:
			full.WriteString(sig.Text)
			pending.WriteString(sig.Text)
			if pending.Len() >= s.cfg.CoalesceSize {
				flush()
			}
		case domain.AgentToolProposed, domain.AgentToolExecuted, domain.AgentToolArtifact, domain.AgentToolFailed:
			s.emitToolSignal(persistCtx, stream, run.ID, sig)
		case domain.AgentToolPair:
			s.appendToolPair(persistCtx, run, sig.Tool)
		}
	})
	flush()
	if result.Text != "" && full.Len() == 0 {
		full.WriteString(result.Text)
		s.emit(persistCtx, stream, run.ID, domain.EventModelTextDelta,
			domain.TextDeltaPayload{Envelope: envelope(), Text: result.Text})
	}
	if result.Usage != nil {
		s.emit(persistCtx, stream, run.ID, domain.EventModelUsage,
			domain.UsagePayload{Envelope: envelope(), Usage: toDomainUsage(*result.Usage)})
	}
	contentBlocks = result.Blocks

	if full.Len() > 0 || len(contentBlocks) > 0 {
		s.persistAssistant(persistCtx, run, full.String(), contentBlocks)
	}
	s.persistUsage(persistCtx, run, result.Usage)

	if ctx.Err() != nil || errors.Is(streamErr, context.Canceled) {
		// running -> cancelling -> cancelled (TAD §8.3). Each step is persisted.
		_ = s.store.UpdateRunStatus(persistCtx, run.ID, domain.RunCancelling, nil)
		s.emit(persistCtx, stream, run.ID, domain.EventRunCancelling,
			domain.StatusPayload{Envelope: envelope(), Status: string(domain.RunCancelling)})
		now := s.cfg.Now()
		_ = s.store.UpdateRunStatus(persistCtx, run.ID, domain.RunCancelled, &now)
		s.emitTerminal(persistCtx, stream, run.ID, domain.EventRunCancelled,
			domain.ErrorPayload{Envelope: envelope(), Message: "run cancelled"})
		return
	}
	if streamErr != nil {
		s.fail(ctx, persistCtx, run, stream, streamErr)
		return
	}

	// Compaction is the only allowed prefix reset; it happens at a safe
	// boundary after the assistant turn is persisted (ADR-0001, TAD §11.5).
	s.maybeCompact(persistCtx, stream, run)

	now := s.cfg.Now()
	_ = s.store.UpdateRunStatus(persistCtx, run.ID, domain.RunCompleted, &now)
	s.emitTerminal(persistCtx, stream, run.ID, domain.EventRunCompleted,
		domain.StatusPayload{Envelope: envelope(), Status: string(domain.RunCompleted)})
}

func (s *Service) fail(ctx, persistCtx context.Context, run *domain.Run, stream *eventStream, cause error) {
	now := s.cfg.Now()
	_ = s.store.UpdateRunStatus(persistCtx, run.ID, domain.RunFailed, &now)
	s.emitTerminal(persistCtx, stream, run.ID, domain.EventRunFailed,
		domain.ErrorPayload{Envelope: envelope(), Message: cause.Error()})
}

func (s *Service) persistAssistant(ctx context.Context, run *domain.Run, text string, blocks []json.RawMessage) {
	parts := make([]domain.ContentPart, 0, len(blocks)+1)
	hasText := false
	for _, raw := range blocks {
		var probe struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Thinking string `json:"thinking"`
		}
		if json.Unmarshal(raw, &probe) != nil {
			continue
		}
		part := domain.ContentPart{Type: probe.Type, Raw: append(json.RawMessage(nil), raw...)}
		switch probe.Type {
		case "text":
			part.Text, hasText = probe.Text, true
		case "thinking":
			part.Text = probe.Thinking
		}
		parts = append(parts, part)
	}
	if !hasText {
		parts = append([]domain.ContentPart{{Type: "text", Text: text}}, parts...)
	}
	message := &domain.Message{
		ID:             s.cfg.NewID(),
		ConversationID: run.ConversationID,
		Role:           domain.RoleAssistant,
		Content:        parts,
		CreatedAt:      s.cfg.Now(),
	}
	_ = s.store.AddMessage(ctx, message)
}

func toDomainUsage(u model.Usage) domain.Usage {
	return domain.Usage{
		ProviderID:       u.ProviderID,
		ModelID:          u.ModelID,
		InputTokens:      u.InputTokens,
		OutputTokens:     u.OutputTokens,
		CacheReadTokens:  u.CacheReadTokens,
		CacheWriteTokens: u.CacheWriteTokens,
		UncachedTokens:   u.UncachedTokens,
		Estimated:        u.Estimated,
	}
}

func (s *Service) persistUsage(ctx context.Context, run *domain.Run, u *model.Usage) {
	if u == nil {
		return
	}
	_ = s.store.AddUsage(ctx, run.ID, toDomainUsage(*u))
}

// emitToolSignal turns a domain-level tool step into a versioned RunEvent.
// The policy engine (#23) consumes proposed vs executed state here; this
// ticket only reports it.
func (s *Service) emitToolSignal(ctx context.Context, stream *eventStream, runID string, sig domain.AgentSignal) {
	eventType := domain.EventToolExecuted
	status := "executed"
	switch sig.Kind {
	case domain.AgentToolProposed:
		eventType, status = domain.EventToolProposed, "proposed"
	case domain.AgentToolArtifact:
		eventType, status = domain.EventToolArtifact, "artifact"
	case domain.AgentToolFailed:
		eventType, status = domain.EventToolFailed, "failed"
	}
	payload := domain.ToolPayload{
		Envelope: envelope(), CallID: sig.Tool.CallID, Name: sig.Tool.Name,
		Risk: sig.Tool.Risk, Input: sig.Tool.Input, Output: sig.Tool.Output,
		Summary: sig.Tool.Summary, ArtifactRef: sig.Tool.ArtifactRef,
		Truncated: sig.Tool.Truncated, Executed: sig.Tool.Executed,
		DurationMS: sig.Tool.DurationMS, Status: status, Error: sig.Tool.Error,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_ = s.emitRaw(ctx, stream, runID, eventType, raw, false)
}

// appendToolPair stores the assistant tool_use turn and its tool_result turn
// without touching any earlier message. Canonical history is append-only.
func (s *Service) appendToolPair(ctx context.Context, run *domain.Run, activity domain.ToolActivity) {
	now := s.cfg.Now()
	parts := rawParts(activity.AssistantBlocks)
	if len(parts) > 0 {
		_ = s.store.AddMessage(ctx, &domain.Message{
			ID: s.cfg.NewID(), ConversationID: run.ConversationID,
			Role: domain.RoleAssistant, Content: parts, CreatedAt: now,
		})
	}
	parts = rawParts(activity.ResultBlocks)
	if len(parts) > 0 {
		_ = s.store.AddMessage(ctx, &domain.Message{
			ID: s.cfg.NewID(), ConversationID: run.ConversationID,
			Role: domain.RoleUser, Content: parts, CreatedAt: now,
		})
	}
}

func rawParts(blocks []json.RawMessage) []domain.ContentPart {
	parts := make([]domain.ContentPart, 0, len(blocks))
	for _, raw := range blocks {
		var probe struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(raw, &probe) != nil {
			continue
		}
		parts = append(parts, domain.ContentPart{Type: probe.Type, Raw: append(json.RawMessage(nil), raw...)})
	}
	return parts
}
