package appsvc

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"sync"

	"github.com/amaldevice/sasmita/internal/agent"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/genui"
	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/modes"
	"github.com/amaldevice/sasmita/internal/tools"
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
		ModeID:         original.ModeID,
		ProviderID:     original.ProviderID,
		Attachments:    original.Attachments,
	}, original.ID)
}

// providerForRun resolves the adapter and wire kind for one run. An empty
// profile ID keeps the shell's single configured adapter.
func (s *Service) providerForRun(ctx context.Context, providerID string) (model.ModelProvider, domain.ProviderKind, error) {
	if providerID == "" {
		return s.cfg.Provider, s.providerKind, nil
	}
	if s.providerFor == nil {
		return nil, "", ErrProviderUnavailable
	}
	provider, err := s.providerFor(ctx, providerID)
	if err != nil {
		return nil, "", err
	}
	var kind domain.ProviderKind
	if aware, ok := provider.(model.ProviderKindAware); ok {
		kind = domain.ProviderKind(aware.ProviderKind())
	}
	return provider, kind, nil
}

func (s *Service) start(ctx context.Context, req domain.StartRunRequest, originalRunID string) (*RunHandle, error) {
	if strings.TrimSpace(req.Text) == "" {
		return nil, domain.ErrInvalidArgument
	}
	provider, kind, err := s.providerForRun(ctx, req.ProviderID)
	if err != nil {
		return nil, err
	}
	if _, err := s.store.GetConversation(ctx, req.ConversationID); err != nil {
		return nil, err
	}
	modelRef := req.ModelRef
	if modelRef == "" {
		modelRef = s.cfg.DefaultModel
	}
	modeID := req.ModeID
	if modeID == "" {
		modeID = defaultMode
	}
	if _, err := modes.Select(modeID); err != nil {
		return nil, err
	}
	now := s.cfg.Now()
	run := &domain.Run{
		ID:             s.cfg.NewID(),
		ConversationID: req.ConversationID,
		Status:         domain.RunQueued,
		ModelRef:       modelRef,
		ModeID:         modeID,
		ProviderID:     req.ProviderID,
		Attachments:    append([]string(nil), req.Attachments...),
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
	go s.execute(runCtx, run, stream, cancel, provider, kind)
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

func (s *Service) execute(ctx context.Context, run *domain.Run, stream *eventStream, cancel context.CancelFunc, provider model.ModelProvider, kind domain.ProviderKind) {
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
	_, view, err := s.activeContext(persistCtx, run.ConversationID, run.ModeID, kind)
	if err != nil {
		s.fail(ctx, persistCtx, run, stream, err)
		return
	}
	// Approval scope and the checkpoint ID come from the stored run: a resumed
	// attempt reuses the original run's approval decisions and checkpoint.
	conv, err := s.store.GetConversation(persistCtx, run.ConversationID)
	if err != nil {
		s.fail(ctx, persistCtx, run, stream, err)
		return
	}
	// Folder access comes from the conversation's project, never from a
	// service-wide setting. A conversation without a project has no folder at
	// all; only files explicitly attached to this run are readable (story 16).
	roots := make([]string, 0, 1+len(run.Attachments))
	if conv.ProjectID != "" {
		if project, err := s.store.GetProject(persistCtx, conv.ProjectID); err == nil {
			if root := projectRoot(project.RootURI); root != "" {
				roots = append(roots, root)
			}
		}
	}
	attached := append([]string(nil), run.Attachments...)
	roots = append(roots, attached...)
	scope := tools.Scope{Roots: roots, Attached: attached}
	agentCtx := tools.WithScope(ctx, scope)
	checkpointID := run.ID
	if run.OriginalRunID != "" {
		checkpointID = run.OriginalRunID
	}

	var full, pending strings.Builder
	var contentBlocks []json.RawMessage
	var uiBlocks []json.RawMessage
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
	result, streamErr := s.cfg.Agent.Run(agentCtx, agent.Request{
		ConversationID: run.ConversationID, RunID: run.ID, CheckpointID: checkpointID,
		Mode: run.ModeID, ProjectID: conv.ProjectID, ScopeRoots: roots,
		ModelRef: run.ModelRef, History: view, Provider: provider,
	}, func(sig domain.AgentSignal) {
		switch sig.Kind {
		case domain.AgentTextDelta:
			full.WriteString(sig.Text)
			pending.WriteString(sig.Text)
			if pending.Len() >= s.cfg.CoalesceSize {
				flush()
			}
		case domain.AgentApprovalRequired:
			s.emitApprovalRequired(persistCtx, stream, run.ID, sig.Approval)
		case domain.AgentToolProposed, domain.AgentToolExecuted, domain.AgentToolArtifact,
			domain.AgentToolFailed, domain.AgentToolDenied:
			s.emitToolSignal(persistCtx, stream, run.ID, sig)
		case domain.AgentToolPair:
			s.appendToolPair(persistCtx, run, sig.Tool)
		case domain.AgentUIBlock:
			s.emitUIBlock(persistCtx, stream, run.ID, sig.UI)
			if sig.UI != nil && len(sig.UI.Spec) > 0 {
				uiBlocks = append(uiBlocks, uiBlockContent(*sig.UI))
			}
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
	contentBlocks = append(result.Blocks, uiBlocks...)

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
	s.maybeCompact(persistCtx, stream, run, provider, kind)

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

// emitApprovalRequired persists the waiting state and streams the inline card.
// The exact arguments travel with the card so the UI can echo them back; the
// stored approval remains the source of truth for revalidation at resolve.
func (s *Service) emitApprovalRequired(ctx context.Context, stream *eventStream, runID string, a *domain.Approval) {
	if a == nil {
		return
	}
	if err := s.store.UpdateRunStatus(ctx, runID, domain.RunAwaitingApproval, nil); err != nil {
		return
	}
	raw, err := json.Marshal(domain.ApprovalPayload{
		Envelope: envelope(), ApprovalID: a.ID, CallID: a.CallID, ToolID: a.ToolID,
		Status: string(domain.RunAwaitingApproval), Action: a.Action, Resource: a.Resource,
		Impact: a.Impact, Message: a.Message, Arguments: a.Arguments,
		ArgsHash: a.ArgsHash, PolicyHash: a.PolicyHash,
	})
	if err != nil {
		return
	}
	_ = s.emitRaw(ctx, stream, runID, domain.EventApprovalRequired, raw, true)
}

// emitToolSignal turns a domain-level tool step into a versioned RunEvent.
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
	case domain.AgentToolDenied:
		eventType, status = domain.EventToolDenied, "denied"
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

// emitUIBlock persists the validated spec as a versioned RunEvent so a reload
// can re-render the block from stored spec and artifact references.
func (s *Service) emitUIBlock(ctx context.Context, stream *eventStream, runID string, block *domain.UIBlockSignal) {
	if block == nil {
		return
	}
	payload := genui.BlockEvent{
		SchemaVersion: genui.SchemaVersion, BlockID: block.BlockID,
		Spec: block.Spec, Reason: block.Reason,
	}
	_ = s.emit(ctx, stream, runID, domain.EventUIBlock, payload)
}

func uiBlockContent(block domain.UIBlockSignal) json.RawMessage {
	encoded, _ := model.MarshalDeterministic(struct {
		Type    string          `json:"type"`
		BlockID string          `json:"blockId"`
		Spec    json.RawMessage `json:"spec,omitempty"`
		Reason  string          `json:"reason,omitempty"`
	}{Type: "ui_block", BlockID: block.BlockID, Spec: block.Spec, Reason: block.Reason})
	return encoded
}

// SaveUIState appends a small, user-committed state patch. It validates keys
// against the stored spec when available and never treats a patch as a tool
// call or a model request.
func (s *Service) SaveUIState(ctx context.Context, runID, blockID string, patch json.RawMessage) error {
	if len(patch) == 0 || len(patch) > genui.MaxPatchBytes {
		return domain.ErrInvalidArgument
	}
	if s.genui != nil {
		if stored, ok := s.genui.Get(blockID); ok {
			var spec genui.Spec
			if err := json.Unmarshal(stored, &spec); err != nil {
				return domain.ErrInvalidArgument
			}
			if err := genui.ValidatePatch(patch, spec); err != nil {
				return err
			}
		}
	}
	payload := genui.UIStateEvent{
		SchemaVersion: genui.SchemaVersion, BlockID: blockID, RunID: runID, Patch: patch,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = s.store.AppendRunEvent(ctx, runID, domain.EventUIState, raw)
	return err
}

// projectRoot converts a project root_uri to a filesystem path. An empty or
// non-file URI yields no root, which keeps the no-project sentinel isolated.
func projectRoot(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	u, err := url.Parse(trimmed)
	if err == nil && u.Scheme == "file" {
		return u.Path
	}
	if err == nil && u.Scheme != "" {
		return ""
	}
	return trimmed
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
