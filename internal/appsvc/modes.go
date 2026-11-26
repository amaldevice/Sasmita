package appsvc

import (
	"context"
	"strings"

	"github.com/amaldevice/sasmita/internal/harness"
)

// ListModes exposes the built-in Mode catalog, including the Modes that are
// listed but not yet available, so the UI can show "coming later".
func (s *Service) ListModes(_ context.Context) []harness.Mode {
	return harness.List()
}

// EstimateSwitch is the preflight for a model or Mode change. Both reset the
// cached prefix, so both use the same re-read warning (TAD §9.3, §10.1).
func (s *Service) EstimateSwitch(ctx context.Context, conversationID, modelRef, modeID string) (harness.SwitchEstimate, error) {
	if _, err := s.store.GetConversation(ctx, conversationID); err != nil {
		return harness.SwitchEstimate{}, err
	}
	if strings.TrimSpace(modelRef) == "" {
		modelRef = s.cfg.DefaultModel
	}
	if _, err := harness.Select(modeID); err != nil {
		return harness.SwitchEstimate{}, err
	}
	history, err := s.store.ListMessages(ctx, conversationID, 0)
	if err != nil {
		return harness.SwitchEstimate{}, err
	}
	return harness.EstimateSwitch(history), nil
}
