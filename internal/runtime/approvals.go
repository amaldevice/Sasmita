package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/policy"
)

// ApprovalStore is the SQLite-backed seam used by the coordinator. It contains
// no provider or shell dependency.
type ApprovalStore interface {
	CreateApproval(ctx context.Context, a *domain.Approval) error
	GetApproval(ctx context.Context, id string) (*domain.Approval, error)
	FindApproval(ctx context.Context, runID, callID, argsHash, policyHash string) (*domain.Approval, error)
	ResolveApproval(ctx context.Context, id, status, decision string, at time.Time) error
	SupersedeApproval(ctx context.Context, id string) error
	AddGrant(ctx context.Context, g *domain.PolicyGrant) error
	HasGrant(ctx context.Context, projectID, toolID, resourceHash, policyHash string) (bool, error)
}

// ApprovalRequest is one proposed tool call as seen by the approval gate. Args
// are the exact bytes from the model; the gate never rewrites them.
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

// ApprovalResult is what the agent loop needs: proceed, skip with a denial, or
// (internally) wait for the user. It carries no policy or desktop type.
type ApprovalResult struct {
	Allowed    bool
	Denied     bool
	ApprovalID string
	Reason     string
}

// ApprovalCoordinator binds an approval to exact arguments and a policy
// snapshot, blocks the run while it waits, and resolves decisions. Waiters are
// process-local; decisions and checkpoints are not (TAD §15.8, §14.3).
type ApprovalCoordinator struct {
	store   ApprovalStore
	engine  *policy.Engine
	mu      sync.Mutex
	waiters map[string]chan ApprovalResult
	now     func() time.Time
	newID   func() string
}

func NewApprovalCoordinator(store ApprovalStore, engine *policy.Engine) *ApprovalCoordinator {
	if engine == nil {
		engine = policy.New()
	}
	return &ApprovalCoordinator{
		store: store, engine: engine, waiters: make(map[string]chan ApprovalResult),
		now: time.Now, newID: randomApprovalID,
	}
}

// Authorize evaluates policy and, if needed, waits for a user decision. A
// previously resolved decision is reused by a resumed attempt without asking
// again; a stale pending approval is superseded and re-requested.
func (c *ApprovalCoordinator) Authorize(ctx context.Context, req ApprovalRequest) (ApprovalResult, error) {
	checkpointID := req.CheckpointID
	if checkpointID == "" {
		checkpointID = req.RunID
	}
	argsHash := policy.HashArguments(req.Arguments)
	snapshot := c.engine.Snapshot()
	grant, err := c.store.HasGrant(ctx, req.ProjectID, req.ToolID, hashString(resourceOf(req.ToolID, req.Arguments)), snapshot.Hash)
	if err != nil {
		return ApprovalResult{}, err
	}
	decision := c.engine.Evaluate(policy.Request{
		ToolID: req.ToolID, Arguments: req.Arguments,
		Mode: policy.Mode(req.Mode), Scope: policy.Scope{ProjectID: req.ProjectID, Roots: req.ScopeRoots},
	}, grant)
	switch decision.Outcome {
	case policy.Allow:
		return ApprovalResult{Allowed: true}, nil
	case policy.Deny:
		return ApprovalResult{Denied: true, Reason: decision.Reason}, nil
	}

	existing, err := c.store.FindApproval(ctx, checkpointID, req.CallID, argsHash, snapshot.Hash)
	if err != nil && !errors.Is(err, domain.ErrApprovalNotFound) {
		return ApprovalResult{}, err
	}
	if err == nil {
		switch existing.Status {
		case domain.ApprovalResolved:
			return decisionResult(existing.Decision, existing.ID, decision.Reason), nil
		case domain.ApprovalPending:
			return c.wait(ctx, existing, decision.Reason)
		default:
			_ = c.store.SupersedeApproval(ctx, existing.ID)
		}
	}

	a := &domain.Approval{
		ID: c.newID(), RunID: checkpointID, ConversationID: req.ConversationID,
		CallID: req.CallID, ToolID: req.ToolID, Mode: req.Mode, ProjectID: req.ProjectID,
		Status: domain.ApprovalPending, Arguments: append(json.RawMessage(nil), req.Arguments...),
		ArgsHash: argsHash, PolicyHash: snapshot.Hash,
		Action: decision.Action, Resource: decision.Resource, Impact: decision.Impact,
		Message: fmt.Sprintf("%s — %s", decision.Action, decision.Impact), CreatedAt: c.now(),
	}
	if err := c.store.CreateApproval(ctx, a); err != nil {
		return ApprovalResult{}, err
	}
	if req.OnPending != nil {
		if err := req.OnPending(a); err != nil {
			return ApprovalResult{}, err
		}
	}
	return c.wait(ctx, a, decision.Reason)
}

// Resolve verifies the binding before persisting the decision. Passing the
// arguments currently on the table makes a changed request fail closed.
func (c *ApprovalCoordinator) Resolve(ctx context.Context, approvalID, decision string, arguments json.RawMessage) error {
	if decision != domain.ApprovalAllowOnce && decision != domain.ApprovalAlways && decision != domain.ApprovalDeny {
		return domain.ErrInvalidDecision
	}
	a, err := c.store.GetApproval(ctx, approvalID)
	if err != nil {
		return err
	}
	if a.Status != domain.ApprovalPending {
		return domain.ErrApprovalResolved
	}
	if len(arguments) > 0 && policy.HashArguments(arguments) != a.ArgsHash {
		return domain.ErrApprovalStale
	}
	snapshot := c.engine.Snapshot()
	if snapshot.Hash != a.PolicyHash {
		return domain.ErrApprovalStale
	}
	if decision == domain.ApprovalAlways {
		g := &domain.PolicyGrant{
			ID: c.newID(), ProjectID: a.ProjectID, ToolID: a.ToolID,
			ResourceHash: hashString(a.Resource), PolicyHash: a.PolicyHash, CreatedAt: c.now(),
		}
		if err := c.store.AddGrant(ctx, g); err != nil {
			return err
		}
	}
	now := c.now()
	if err := c.store.ResolveApproval(ctx, approvalID, domain.ApprovalResolved, decision, now); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrApprovalResolved
		}
		return err
	}
	c.mu.Lock()
	ch := c.waiters[approvalID]
	if ch != nil {
		delete(c.waiters, approvalID)
	}
	c.mu.Unlock()
	if ch != nil {
		select {
		case ch <- decisionResult(decision, approvalID, ""):
		default:
		}
	}
	return nil
}

func (c *ApprovalCoordinator) wait(ctx context.Context, a *domain.Approval, reason string) (ApprovalResult, error) {
	ch := make(chan ApprovalResult, 1)
	c.mu.Lock()
	if existing := c.waiters[a.ID]; existing != nil {
		c.mu.Unlock()
		return <-existing, nil
	}
	c.waiters[a.ID] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		if c.waiters[a.ID] == ch {
			delete(c.waiters, a.ID)
		}
		c.mu.Unlock()
	}()
	select {
	case result := <-ch:
		if result.Reason == "" {
			result.Reason = reason
		}
		return result, nil
	case <-ctx.Done():
		return ApprovalResult{}, ctx.Err()
	}
}

func decisionResult(decision, approvalID, reason string) ApprovalResult {
	switch decision {
	case domain.ApprovalAllowOnce, domain.ApprovalAlways:
		return ApprovalResult{Allowed: true, ApprovalID: approvalID, Reason: reason}
	default:
		return ApprovalResult{Denied: true, ApprovalID: approvalID, Reason: reason}
	}
}

func hashString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// resourceOf extracts the affected resource for the project grant. It is the
// same field shown on the card, so a grant cannot silently widen to another
// file under the same tool ID.
func resourceOf(toolID string, args json.RawMessage) string {
	var fields struct {
		Path      string `json:"path"`
		Name      string `json:"name"`
		Command   string `json:"command"`
		Overwrite bool   `json:"overwrite"`
	}
	_ = json.Unmarshal(args, &fields)
	switch {
	case fields.Path != "":
		return fields.Path
	case fields.Name != "":
		return fields.Name
	case fields.Command != "":
		return fields.Command
	default:
		return toolID
	}
}

func randomApprovalID() string {
	return fmt.Sprintf("apr-%d", time.Now().UnixNano())
}
