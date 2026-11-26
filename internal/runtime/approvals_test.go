package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/policy"
	"github.com/amaldevice/sasmita/internal/runtime"
)

type approvalStoreFake struct {
	approval *domain.Approval
	grants   []*domain.PolicyGrant
}

func (f *approvalStoreFake) CreateApproval(_ context.Context, a *domain.Approval) error {
	copy := *a
	f.approval = &copy
	return nil
}
func (f *approvalStoreFake) GetApproval(_ context.Context, _ string) (*domain.Approval, error) {
	if f.approval == nil {
		return nil, domain.ErrApprovalNotFound
	}
	copy := *f.approval
	return &copy, nil
}
func (f *approvalStoreFake) FindApproval(_ context.Context, _, _, _, _ string) (*domain.Approval, error) {
	return f.GetApproval(context.Background(), "")
}
func (f *approvalStoreFake) ResolveApproval(_ context.Context, _, status, decision string, at time.Time) error {
	f.approval.Status = status
	f.approval.Decision = decision
	f.approval.ResolvedAt = &at
	return nil
}
func (f *approvalStoreFake) SupersedeApproval(_ context.Context, _ string) error {
	f.approval.Status = domain.ApprovalSuperseded
	return nil
}
func (f *approvalStoreFake) AddGrant(_ context.Context, g *domain.PolicyGrant) error {
	f.grants = append(f.grants, g)
	return nil
}
func (f *approvalStoreFake) HasGrant(_ context.Context, _, _, _, _ string) (bool, error) {
	return false, nil
}

// Both bindings are re-verified at resolve time, not just when the card is
// created: a policy change invalidates an approval that is still pending.
func TestResolveRechecksPolicySnapshot(t *testing.T) {
	ctx := context.Background()
	args := json.RawMessage(`{"name":"chart","overwrite":true}`)
	engine := policy.New()
	store := &approvalStoreFake{}
	if err := store.CreateApproval(ctx, &domain.Approval{
		ID: "apr-1", RunID: "run-1", Status: domain.ApprovalPending,
		Arguments: args, ArgsHash: policy.HashArguments(args), PolicyHash: engine.Snapshot().Hash,
	}); err != nil {
		t.Fatal(err)
	}
	coordinator := runtime.NewApprovalCoordinator(store, engine)

	engine.Version = "v2"
	if err := coordinator.Resolve(ctx, "apr-1", domain.ApprovalAllowOnce, args); !errors.Is(err, domain.ErrApprovalStale) {
		t.Fatalf("resolve under a changed policy snapshot = %v, want ErrApprovalStale", err)
	}
	if store.approval.Status != domain.ApprovalPending {
		t.Fatalf("stale decision was persisted: %+v", store.approval)
	}

	engine.Version = "v1"
	if err := coordinator.Resolve(ctx, "apr-1", domain.ApprovalAllowOnce, args); err != nil {
		t.Fatalf("resolve under the bound snapshot: %v", err)
	}
}
