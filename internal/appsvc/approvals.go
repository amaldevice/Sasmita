package appsvc

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/amaldevice/sasmita/internal/agent"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/runtime"
)

// approvalGateAdapter keeps the runtime's typed request out of the agent
// package: the agent knows only its own small port.
type approvalGateAdapter struct{ coordinator *runtime.ApprovalCoordinator }

func (a approvalGateAdapter) Authorize(ctx context.Context, req agent.ApprovalRequest) (agent.ApprovalResult, error) {
	result, err := a.coordinator.Authorize(ctx, runtime.ApprovalRequest{
		RunID: req.RunID, CheckpointID: req.CheckpointID, ConversationID: req.ConversationID,
		CallID: req.CallID, ToolID: req.ToolID, Mode: req.Mode, ProjectID: req.ProjectID,
		Arguments: req.Arguments, ScopeRoots: req.ScopeRoots, OnPending: req.OnPending,
	})
	return agent.ApprovalResult{Allowed: result.Allowed, Denied: result.Denied, Reason: result.Reason}, err
}

// GetApproval returns the stored decision record, including its argument hash
// and policy snapshot. It is the typed read used after a restart.
func (s *Service) GetApproval(ctx context.Context, id string) (*domain.Approval, error) {
	return s.store.GetApproval(ctx, id)
}

// PendingApprovals lists approvals that still need an answer after a restart.
func (s *Service) PendingApprovals(ctx context.Context, conversationID string) ([]domain.Approval, error) {
	all, err := s.store.ListApprovals(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Approval, 0, len(all))
	for _, a := range all {
		if a.Status == domain.ApprovalPending {
			out = append(out, a)
		}
	}
	return out, nil
}

// ResolveApproval records allow once / always in this project / deny. If the
// arguments on the table differ from the bound hash, the decision is rejected
// and the run keeps waiting for a fresh approval (TAD §15.8).
func (s *Service) ResolveApproval(ctx context.Context, approvalID, decision string, arguments json.RawMessage) error {
	approval, err := s.store.GetApproval(ctx, approvalID)
	if err != nil {
		return err
	}
	if err := s.approvals.Resolve(ctx, approvalID, decision, arguments); err != nil {
		return err
	}
	// The in-process waiter resumes; move the run back to running and keep the
	// UI stream in sync. A run interrupted by a restart has no live waiter.
	run, err := s.store.GetRun(ctx, approval.RunID)
	if err != nil {
		return err
	}
	if run.Status != domain.RunAwaitingApproval || !s.runtime.Has(run.ID) {
		return nil
	}
	if err := s.store.UpdateRunStatus(ctx, run.ID, domain.RunRunning, nil); err != nil {
		if errors.Is(err, domain.ErrIllegalTransition) || errors.Is(err, domain.ErrTerminalRun) {
			return nil
		}
		return err
	}
	raw, err := json.Marshal(domain.StatusPayload{Envelope: envelope(), Status: string(domain.RunRunning)})
	if err != nil {
		return err
	}
	return s.runtime.Emit(run.ID, domain.EventRunStarted, raw, false)
}
