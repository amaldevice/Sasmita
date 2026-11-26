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

// eventStream is a bounded channel for UI delivery. Non-terminal events are
// dropped under backpressure; the terminal event is always delivered unless
// the consumer explicitly aborts.
type eventStream struct {
	ch        chan domain.RunEvent
	done      chan struct{}
	closeOnce sync.Once
	doneOnce  sync.Once
}

func newEventStream(size int) *eventStream {
	return &eventStream{ch: make(chan domain.RunEvent, size), done: make(chan struct{})}
}

func (s *eventStream) send(ev domain.RunEvent, terminal bool) bool {
	if terminal {
		select {
		case s.ch <- ev:
			s.closeOnce.Do(func() { close(s.ch) })
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

// RunHandle is the typed stream handle returned by StartRun.
type RunHandle struct {
	RunID          string
	ConversationID string
	stream         *eventStream
	cancel         context.CancelFunc
}

func (h *RunHandle) Events() <-chan domain.RunEvent { return h.stream.ch }

// Cancel propagates cancellation from the UI all the way to the provider.
func (h *RunHandle) Cancel() { h.cancel() }

// Abort releases the sender when the UI stops reading; terminal state is still
// persisted in the run record and recoverable via GetRunSnapshot.
func (h *RunHandle) Abort() { h.stream.abort() }

func envelope() domain.Envelope { return domain.Envelope{Version: domain.RunEventSchemaVersion} }

// StartRun persists the run record and user message first, then returns a
// handle whose event channel carries the versioned RunEvent stream.
func (s *Service) StartRun(ctx context.Context, req domain.StartRunRequest) (*RunHandle, error) {
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
	if err := s.store.StartRunTx(ctx, run, userMessage, now); err != nil {
		return nil, err
	}

	stream := newEventStream(128)
	if _, err := s.emit(ctx, stream, run.ID, domain.EventRunQueued,
		domain.StatusPayload{Envelope: envelope(), Status: string(domain.RunQueued)}); err != nil {
		return nil, err
	}

	// A disconnected frontend must not kill an in-flight run; the handle's
	// Cancel() is the explicit cancellation path to the provider.
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	handle := &RunHandle{
		RunID:          run.ID,
		ConversationID: run.ConversationID,
		stream:         stream,
		cancel:         cancel,
	}
	go s.execute(runCtx, run, stream, cancel)
	return handle, nil
}

func (s *Service) emit(ctx context.Context, stream *eventStream, runID, eventType string, payload any) (domain.RunEvent, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return domain.RunEvent{}, err
	}
	ev, err := s.store.AppendRunEvent(ctx, runID, eventType, raw)
	if err != nil {
		return domain.RunEvent{}, err
	}
	stream.send(ev, false)
	return ev, nil
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
	stream.send(ev, true)
}

func (s *Service) execute(ctx context.Context, run *domain.Run, stream *eventStream, cancel context.CancelFunc) {
	defer cancel()
	persistCtx := context.WithoutCancel(ctx)

	if err := s.store.UpdateRunStatus(persistCtx, run.ID, domain.RunPreparing, nil); err != nil {
		s.fail(ctx, persistCtx, run, stream, err)
		return
	}
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

	if streamErr != nil {
		status, eventType, message := domain.RunFailed, domain.EventRunFailed, streamErr.Error()
		if errors.Is(streamErr, context.Canceled) {
			status, eventType, message = domain.RunCancelled, domain.EventRunCancelled, "run cancelled"
		}
		now := s.cfg.Now()
		s.store.UpdateRunStatus(persistCtx, run.ID, status, &now)
		s.emitTerminal(persistCtx, stream, run.ID, eventType,
			domain.ErrorPayload{Envelope: envelope(), Message: message})
		return
	}

	now := s.cfg.Now()
	s.store.UpdateRunStatus(persistCtx, run.ID, domain.RunCompleted, &now)
	s.emitTerminal(persistCtx, stream, run.ID, domain.EventRunCompleted,
		domain.StatusPayload{Envelope: envelope(), Status: string(domain.RunCompleted)})
}

func (s *Service) fail(ctx, persistCtx context.Context, run *domain.Run, stream *eventStream, cause error) {
	now := s.cfg.Now()
	s.store.UpdateRunStatus(persistCtx, run.ID, domain.RunFailed, &now)
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
	s.store.AddMessage(ctx, message)
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
	s.store.AddUsage(ctx, run.ID, toDomainUsage(*u))
}
