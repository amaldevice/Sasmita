package appsvc

import (
	"context"
	"encoding/json"
	"errors"

	ctxlayout "github.com/amaldevice/sasmita/internal/context"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/modes"
)

// latestCompaction reads the durable prefix reset recorded in run events.
// Compaction records are kept separately from canonical history (TAD §11.5).
func (s *Service) latestCompaction(ctx context.Context, conversationID string) (*domain.Compaction, error) {
	runs, err := s.store.ListRuns(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	var latest *domain.Compaction
	for _, run := range runs {
		events, err := s.store.ListRunEvents(ctx, run.ID)
		if err != nil {
			return nil, err
		}
		for _, ev := range events {
			if ev.Type != domain.EventContextCompacted {
				continue
			}
			var payload domain.CompactionPayload
			if json.Unmarshal(ev.Payload, &payload) != nil {
				continue
			}
			if latest == nil || payload.Compaction.CreatedAt.After(latest.CreatedAt) {
				record := payload.Compaction
				latest = &record
			}
		}
	}
	return latest, nil
}

// activeContext rebuilds the derived view from canonical history, applying the
// last compaction first. Later messages stay in append order after the tail.
func (s *Service) activeContext(ctx context.Context, conversationID, modeID string, kind domain.ProviderKind) (canonical, view []domain.Message, err error) {
	canonical, err = s.store.ListMessages(ctx, conversationID, 0)
	if err != nil {
		return nil, nil, err
	}
	record, err := s.latestCompaction(ctx, conversationID)
	if err != nil {
		return nil, nil, err
	}
	if record != nil {
		canonical = ctxlayout.Apply(*record, canonical)
	}
	mode, err := modes.Select(modeID)
	if err != nil {
		return nil, nil, err
	}
	return canonical, modes.ActiveHistory(canonical, mode, kind), nil
}

// systemMessages are the Mode instructions that precede canonical history in
// the cached request. The summary call must include them to hit the cache.
func (s *Service) systemMessages(modeID string) []domain.Message {
	mode, err := modes.Select(modeID)
	if err != nil || mode.Instructions == "" {
		return nil
	}
	return []domain.Message{{Role: domain.RoleSystem, Content: domain.TextContent(mode.Instructions)}}
}

func (s *Service) compact(ctx context.Context, conversationID, modelRef, runID, modeID string, provider model.ModelProvider, kind domain.ProviderKind) (*domain.Compaction, error) {
	canonical, _, err := s.activeContext(ctx, conversationID, modeID, kind)
	if err != nil {
		return nil, err
	}
	compactor := s.compactor
	compactor.Provider = provider
	compactor.SystemMessages = s.systemMessages(modeID)
	record, _, err := compactor.Compact(ctx, conversationID, modelRef, runID, canonical)
	if err != nil {
		return nil, err
	}
	// Compaction is the only point where the tool list may change. Tools used
	// through call_tool become native definitions for the next run.
	if s.discovery != nil {
		s.discovery.Promote(conversationID)
	}
	return &record, nil
}

func (s *Service) maybeCompact(ctx context.Context, stream *eventStream, run *domain.Run, provider model.ModelProvider, kind domain.ProviderKind) {
	if !s.budgetReached(ctx, run, kind) {
		return
	}
	record, err := s.compact(ctx, run.ConversationID, run.ModelRef, run.ID, run.ModeID, provider, kind)
	if errors.Is(err, ctxlayout.ErrNothingToCompact) {
		return
	}
	if err != nil {
		// Never fail a completed run because of a summary request. The warning
		// makes the missed compaction observable in the run stream.
		raw, _ := json.Marshal(domain.ErrorPayload{
			Envelope: envelope(), Message: "compaction failed: " + err.Error(),
		})
		_ = s.emitRaw(ctx, stream, run.ID, domain.EventContextBudgetWarning, raw, false)
		return
	}
	raw, err := json.Marshal(domain.CompactionPayload{Envelope: envelope(), Compaction: *record})
	if err != nil {
		return
	}
	_ = s.emitRaw(ctx, stream, run.ID, domain.EventContextCompacted, raw, true)
}

// budgetReached estimates the active context after the assistant turn was
// persisted. A zero budget leaves automatic compaction disabled.
func (s *Service) budgetReached(ctx context.Context, run *domain.Run, kind domain.ProviderKind) bool {
	if s.compactor.InputBudgetTokens <= 0 {
		return false
	}
	_, view, err := s.activeContext(ctx, run.ConversationID, run.ModeID, kind)
	if err != nil {
		return false
	}
	return s.compactor.ShouldCompact(view)
}

// CompactConversation is the user-requested compaction command. It records the
// new prefix on the latest run so debugging and recovery keep a durable trail.
func (s *Service) CompactConversation(ctx context.Context, conversationID string) (*domain.Compaction, error) {
	conv, err := s.store.GetConversation(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	runs, err := s.store.ListRuns(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return nil, domain.ErrNotFound
	}
	last := runs[len(runs)-1]
	modelRef := last.ModelRef
	if modelRef == "" {
		modelRef = s.cfg.DefaultModel
	}
	provider, kind, err := s.providerForRun(ctx, last.ProviderID)
	if err != nil {
		return nil, err
	}
	record, err := s.compact(ctx, conversationID, modelRef, last.ID, conv.ActiveModeID, provider, kind)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(domain.CompactionPayload{Envelope: envelope(), Compaction: *record})
	if err != nil {
		return nil, err
	}
	if _, err := s.store.AppendRunEvent(ctx, last.ID, domain.EventContextCompacted, raw); err != nil {
		return nil, err
	}
	return record, nil
}
