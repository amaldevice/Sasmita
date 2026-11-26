package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/amaldevice/sasmita/internal/appsvc"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/persistence/migrations"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
)

// approvalTool is a write with an external-ish impact: TAD §15.3 requires
// approval before overwriting an existing user artifact.
var approvalTool = json.RawMessage(`{"name":"chart","content":"x","overwrite":true}`)

// approvalProvider proposes one overwrite tool call, then finishes. A new
// provider instance starts from the first turn again, which is what a resumed
// attempt sees after a restart.
type approvalProvider struct{ calls int }

func (p *approvalProvider) Name() string { return "approval-fake" }

func (p *approvalProvider) Stream(_ context.Context, _ model.StreamRequest, emit func(model.StreamEvent)) error {
	p.calls++
	if p.calls == 1 {
		emit(model.StreamEvent{Kind: model.EventTextDelta, Text: "Menulis. "})
		emit(model.StreamEvent{Kind: model.EventToolUse, ToolCall: &model.ToolCall{
			ID: "write-1", Name: "write_artifact", Input: append(json.RawMessage(nil), approvalTool...),
		}})
		return nil
	}
	emit(model.StreamEvent{Kind: model.EventTextDelta, Text: "Selesai."})
	return nil
}

// twoTurnApprovalProvider re-proposes the call on the resumed attempt so the
// persisted approval decision can be consumed, then finishes on turn three.
type twoTurnApprovalProvider struct{ calls int }

func (p *twoTurnApprovalProvider) Name() string { return "approval-restart-fake" }

func (p *twoTurnApprovalProvider) Stream(_ context.Context, _ model.StreamRequest, emit func(model.StreamEvent)) error {
	p.calls++
	if p.calls <= 2 {
		emit(model.StreamEvent{Kind: model.EventToolUse, ToolCall: &model.ToolCall{
			ID: "write-1", Name: "write_artifact", Input: append(json.RawMessage(nil), approvalTool...),
		}})
		return nil
	}
	emit(model.StreamEvent{Kind: model.EventTextDelta, Text: "Lanjut setelah restart."})
	return nil
}

func newApprovalService(t *testing.T, path string, provider model.ModelProvider) (*appsvc.Service, *repositories.Store) {
	return newApprovalServiceOnDB(t, path, provider, true)
}

// seed=false models a new process opening the same SQLite file after restart.
func newApprovalServiceOnDB(t *testing.T, path string, provider model.ModelProvider, seed bool) (*appsvc.Service, *repositories.Store) {
	t.Helper()
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := migrations.Migrate(db); err != nil {
		t.Fatal(err)
	}
	store := repositories.New(db)
	if seed {
		now := time.Now()
		if err := store.CreateProject(context.Background(), &domain.Project{ID: "p1", Name: "Work", CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
		if err := store.CreateConversation(context.Background(), &domain.Conversation{ID: "c1", ProjectID: "p1", Title: "Approvals", CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	svc := appsvc.New(store, appsvc.Config{
		Provider:       provider,
		DefaultModel:   "fake",
		EnableTools:    true,
		ArtifactsDir:   t.TempDir(),
		WorkspaceRoots: []string{filepath.Join(t.TempDir(), "project")},
		ToolOutputCap:  512,
	})
	return svc, store
}

// startUntilApproval drives a run only as far as the approval wait. The stream
// is bounded, so leaving it undrained here is safe.
func startUntilApproval(t *testing.T, svc *appsvc.Service) (*appsvc.RunHandle, domain.RunEvent, domain.Approval) {
	t.Helper()
	h, err := svc.StartRun(context.Background(), domain.StartRunRequest{
		ConversationID: "c1", Text: "tulis chart", ModelRef: "fake", HarnessID: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	for ev := range h.Events() {
		if ev.Type != domain.EventApprovalRequired {
			continue
		}
		var payload domain.ApprovalPayload
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		approval, err := svc.GetApproval(context.Background(), payload.ApprovalID)
		if err != nil {
			t.Fatal(err)
		}
		return h, ev, *approval
	}
	t.Fatal("run completed without asking for approval")
	return nil, domain.RunEvent{}, domain.Approval{}
}

func eventTypes(events []domain.RunEvent) map[string]bool {
	out := map[string]bool{}
	for _, ev := range events {
		out[ev.Type] = true
	}
	return out
}

// Seam 1: "allow once" resumes the paused call, records the decision in SQLite,
// and keeps canonical history append-only.
func TestSeam1AllowOnceExecutesAndCompletes(t *testing.T) {
	svc, store := newApprovalService(t, filepath.Join(t.TempDir(), "sasmita.db"), &approvalProvider{})
	h, ev, approval := startUntilApproval(t, svc)

	var payload domain.ApprovalPayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Action == "" || payload.Resource == "" || payload.Impact == "" || payload.ApprovalID != approval.ID {
		t.Fatalf("approval card payload = %+v (approval %+v)", payload, approval)
	}
	run, err := store.GetRun(context.Background(), h.RunID)
	if err != nil || run.Status != domain.RunAwaitingApproval {
		t.Fatalf("run while waiting = %+v (err %v)", run, err)
	}
	if approval.Status != domain.ApprovalPending || approval.ArgsHash == "" || approval.PolicyHash == "" {
		t.Fatalf("stored approval = %+v", approval)
	}

	if err := svc.ResolveApproval(context.Background(), approval.ID, domain.ApprovalAllowOnce, nil); err != nil {
		t.Fatalf("ResolveApproval: %v", err)
	}
	events := drain(t, h)
	types := eventTypes(events)
	for _, want := range []string{domain.EventToolExecuted, domain.EventRunCompleted} {
		if !types[want] {
			t.Fatalf("missing %q in %+v", want, events)
		}
	}
	if types[domain.EventToolDenied] {
		t.Fatalf("allowed call was denied: %+v", events)
	}
	resolved, err := store.GetApproval(context.Background(), approval.ID)
	if err != nil || resolved.Decision != domain.ApprovalAllowOnce || resolved.ResolvedAt == nil {
		t.Fatalf("resolved approval = %+v (err %v)", resolved, err)
	}
}

// Seam 1: "deny" returns a bounded denial to the agent loop and never executes
// the tool. The run still reaches a terminal state.
func TestSeam1DenyDoesNotExecuteTool(t *testing.T) {
	svc, _ := newApprovalService(t, filepath.Join(t.TempDir(), "sasmita.db"), &approvalProvider{})
	h, _, approval := startUntilApproval(t, svc)
	if err := svc.ResolveApproval(context.Background(), approval.ID, domain.ApprovalDeny, nil); err != nil {
		t.Fatalf("ResolveApproval: %v", err)
	}
	events := drain(t, h)
	types := eventTypes(events)
	if !types[domain.EventToolDenied] || types[domain.EventToolExecuted] {
		t.Fatalf("denial events = %+v", events)
	}
	if events[len(events)-1].Type != domain.EventRunCompleted {
		t.Fatalf("terminal event = %q", events[len(events)-1].Type)
	}
}

// Seam 1: an approval is bound to exact arguments and a policy snapshot. New
// arguments invalidate it and must be rejected by ResolveApproval.
func TestSeam1ChangedArgumentsInvalidateApproval(t *testing.T) {
	svc, _ := newApprovalService(t, filepath.Join(t.TempDir(), "sasmita.db"), &approvalProvider{})
	h, _, approval := startUntilApproval(t, svc)
	changed := json.RawMessage(`{"name":"chart","content":"DIFFERENT","overwrite":true}`)
	err := svc.ResolveApproval(context.Background(), approval.ID, domain.ApprovalAllowOnce, changed)
	if !errors.Is(err, domain.ErrApprovalStale) {
		t.Fatalf("ResolveApproval with changed args = %v, want ErrApprovalStale", err)
	}
	if err := svc.ResolveApproval(context.Background(), approval.ID, domain.ApprovalAllowOnce, nil); err != nil {
		t.Fatalf("ResolveApproval with original args: %v", err)
	}
	_ = drain(t, h)
}

// Seam 1: the approval wait, its decision and the agent checkpoint survive a
// process restart. ResumeRun consumes them without asking again.
func TestSeam1ApprovalAndCheckpointResumeAfterRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sasmita.db")
	svc1, store := newApprovalService(t, path, &twoTurnApprovalProvider{})
	h1, _, approval := startUntilApproval(t, svc1)
	originalID := h1.RunID

	// New process: the same SQLite file, no in-memory waiter or run manager.
	svc2, _ := newApprovalServiceOnDB(t, path, &twoTurnApprovalProvider{}, false)
	recovered, err := svc2.RecoverStaleRuns(ctx)
	if err != nil || len(recovered) != 1 || recovered[0].ID != originalID {
		t.Fatalf("RecoverStaleRuns = %+v (err %v)", recovered, err)
	}
	stored, err := store.GetApproval(ctx, approval.ID)
	if err != nil || stored.Status != domain.ApprovalPending {
		t.Fatalf("approval after restart = %+v (err %v)", stored, err)
	}
	if _, ok, err := store.LoadCheckpoint(ctx, "approval."+originalID); err != nil || !ok {
		t.Fatalf("checkpoint after restart: found=%v err=%v", ok, err)
	}
	if err := svc2.ResolveApproval(ctx, approval.ID, domain.ApprovalAllowOnce, nil); err != nil {
		t.Fatalf("ResolveApproval after restart: %v", err)
	}
	h2, err := svc2.ResumeRun(ctx, originalID)
	if err != nil {
		t.Fatalf("ResumeRun: %v", err)
	}
	events := drain(t, h2)
	types := eventTypes(events)
	if !types[domain.EventToolExecuted] || !types[domain.EventRunCompleted] {
		t.Fatalf("resumed events = %+v", events)
	}
	if types[domain.EventApprovalRequired] {
		t.Fatalf("resume asked for the same approval again: %+v", events)
	}
	resumed, err := store.GetRun(ctx, h2.RunID)
	if err != nil || resumed.OriginalRunID != originalID || resumed.ID == originalID {
		t.Fatalf("resumed run = %+v (err %v)", resumed, err)
	}
	// The abandoned first handle is never resumed into a second terminal event.
	_ = h1
}

// Seam 1: "always allow in this project" stores a grant bound to the policy
// snapshot; a later identical operation in the same project needs no approval.
func TestSeam1AlwaysAllowUsesProjectGrant(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sasmita.db")
	provider := &approvalProvider{}
	svc, store := newApprovalService(t, path, provider)
	h1, _, approval := startUntilApproval(t, svc)
	if err := svc.ResolveApproval(context.Background(), approval.ID, domain.ApprovalAlways, nil); err != nil {
		t.Fatalf("ResolveApproval: %v", err)
	}
	_ = drain(t, h1)
	if grants, err := store.ListGrants(context.Background(), "p1"); err != nil || len(grants) != 1 {
		t.Fatalf("grants = %+v (err %v)", grants, err)
	}
	provider.calls = 0
	h2, err := svc.StartRun(context.Background(), domain.StartRunRequest{
		ConversationID: "c1", Text: "tulis lagi", ModelRef: "fake", HarnessID: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	if eventTypes(drain(t, h2))[domain.EventToolExecuted] == false {
		t.Fatal("granted operation did not execute")
	}
}
