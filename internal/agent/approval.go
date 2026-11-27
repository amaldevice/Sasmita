package agent

import (
	"context"
	"encoding/json"

	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/model"
)

// ApprovalGate is the app-owned boundary between the agent loop and the policy
// engine. It may block until the user decides; cancellation flows in from the
// run context. No policy or provider type crosses this interface.
type ApprovalGate interface {
	Authorize(ctx context.Context, req ApprovalRequest) (ApprovalResult, error)
}

type ApprovalRequest struct {
	RunID          string
	CheckpointID   string
	ConversationID string
	CallID         string
	ToolID         string
	Mode           string
	ProjectID      string
	Arguments      json.RawMessage
	ScopeRoots     []string
	OnPending      func(*domain.Approval) error
}

type ApprovalResult struct {
	Allowed bool
	Denied  bool
	Reason  string
}

// agentCheckpoint is the minimal state needed to continue a paused turn. The
// exact approval decision lives in SQLite beside it, keyed by checkpoint ID.
type agentCheckpoint struct {
	Working []domain.Message `json:"working"`
	Usage   *model.Usage     `json:"usage,omitempty"`
}

func checkpointKey(id string) string { return "approval." + id }

func (l *loopAgent) checkpointID() string {
	if l.req.CheckpointID != "" {
		return l.req.CheckpointID
	}
	return l.req.RunID
}

func (l *loopAgent) saveCheckpoint(ctx context.Context) {
	store := l.engine.cfg.States
	if store == nil || l.checkpointID() == "" {
		return
	}
	raw, err := json.Marshal(agentCheckpoint{Working: l.working})
	if err != nil {
		return
	}
	_ = store.Set(ctx, checkpointKey(l.checkpointID()), raw)
}

func (l *loopAgent) restoreCheckpoint(ctx context.Context) {
	store := l.engine.cfg.States
	if store == nil || l.checkpointID() == "" {
		return
	}
	raw, ok, err := store.Get(ctx, checkpointKey(l.checkpointID()))
	if err != nil || !ok {
		return
	}
	var state agentCheckpoint
	if json.Unmarshal(raw, &state) == nil && len(state.Working) > 0 {
		l.working = state.Working
	}
}
