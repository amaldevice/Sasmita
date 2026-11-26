package appsvc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/amaldevice/sasmita/internal/domain"
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
	harness := req.HarnessID
	if harness == "" {
		harness = defaultHarness
	}
	now := s.cfg.Now()
	run := &domain.Run{
		ID:             s.cfg.NewID(),
		ConversationID: req.ConversationID,
		Status:         domain.RunQueued,
		ModelRef:       modelRef,
		HarnessID:      harness,
		StartedAt:      now,
		OriginalRunID:  originalRunID,
	}
	content := domain.TextContent(req.Text)
	if s.cfg.ContextNote != nil {
		if note := strings.TrimSpace(s.cfg.ContextNote()); note != "" {
			// Dynamic information is a tail context note on the new user turn,
			// never part of the static system instruction (ADR-0001).
			content = append(content, domain.ContentPart{Type: "context_note", Text: note})
		}
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

	// Full canonical history: no sliding window. Compaction is a separate
	// ticket and is the only event allowed to change the prefix (ADR-0001).
	history, err := s.store.ListMessages(persistCtx, run.ConversationID, 0)
	if err != nil {
		s.fail(ctx, persistCtx, run, stream, err)
		return
	}
	request := s.builder.Build(run.ConversationID, run.ModelRef, run.ID, history)

	var full, pending strings.Builder
	var usage *model.Usage
	var contentBlocks []json.RawMessage
	flush := func() {
		if pending.Len() == 0 {
			return
		}
		s.emit(persistCtx, stream, run.ID, domain.EventModelTextDelta,
			domain.TextDeltaPayload{Envelope: envelope(), Text: pending.String()})
		pending.Reset()
	}

	streamErr := s.cfg.Provider.Stream(ctx, request, func(ev model.StreamEvent) {
		switch ev.Kind {
		case model.EventTextDelta:
			full.WriteString(ev.Text)
			pending.WriteString(ev.Text)
			if pending.Len() >= s.cfg.CoalesceSize {
				flush()
			}
		case model.EventUsage:
			usage = ev.Usage
			s.emit(persistCtx, stream, run.ID, domain.EventModelUsage,
				domain.UsagePayload{Envelope: envelope(), Usage: toDomainUsage(*ev.Usage)})
		case model.EventContentBlocks:
			contentBlocks = append(contentBlocks, ev.ContentBlocks...)
		}
	})
	flush()

	if full.Len() > 0 || len(contentBlocks) > 0 {
		s.persistAssistant(persistCtx, run, full.String(), contentBlocks)
	}
	s.persistUsage(persistCtx, run, usage)

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
		RunID:          run.ID,
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
