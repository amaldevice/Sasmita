package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/amaldevice/sasmita/internal/appsvc"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/persistence/migrations"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
)

// gatedProvider emits one delta and then waits for its request context to be
// cancelled, modelling a provider stream that is still running mid-answer.
type gatedProvider struct {
	started    chan struct{}
	cancelled  chan struct{}
	startOnce  sync.Once
	cancelOnce sync.Once
}

func newGatedProvider() *gatedProvider {
	return &gatedProvider{started: make(chan struct{}), cancelled: make(chan struct{})}
}

func (p *gatedProvider) Name() string { return "gated" }

func (p *gatedProvider) Stream(ctx context.Context, _ model.StreamRequest, emit func(model.StreamEvent)) error {
	emit(model.StreamEvent{Kind: model.EventTextDelta, Text: "partial "})
	p.startOnce.Do(func() { close(p.started) })
	<-ctx.Done()
	p.cancelOnce.Do(func() { close(p.cancelled) })
	return ctx.Err()
}

func openStateDB(t *testing.T, path string, provider model.ModelProvider, coalesce int) (*appsvc.Service, *repositories.Store) {
	t.Helper()
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := migrations.Migrate(db); err != nil {
		t.Fatal(err)
	}
	store := repositories.New(db)
	svc := appsvc.New(store, appsvc.Config{
		Provider: provider, DefaultModel: "gated", NewID: newIDGenerator(), CoalesceSize: coalesce,
	})
	return svc, store
}

func seedState(t *testing.T, store *repositories.Store) {
	t.Helper()
	now := time.Now()
	if err := store.CreateProject(context.Background(), &domain.Project{ID: "p1", Name: "Work", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateConversation(context.Background(), &domain.Conversation{ID: "c1", ProjectID: "p1", Title: "Chat", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
}

func stateTestService(t *testing.T, path string, provider model.ModelProvider) (*appsvc.Service, *repositories.Store) {
	t.Helper()
	svc, store := openStateDB(t, path, provider, 32)
	seedState(t, store)
	return svc, store
}

// Seam 1: CancelRun must stop provider work, end the run as cancelled, and
// deliver the terminal event without losing earlier deltas.
func TestSeam1CancelRunMidStream(t *testing.T) {
	ctx := context.Background()
	provider := newGatedProvider()
	svc, store := stateTestService(t, filepath.Join(t.TempDir(), "sasmita.db"), provider)

	handle, err := svc.StartRun(ctx, domain.StartRunRequest{ConversationID: "c1", Text: "question"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-provider.started:
	case <-time.After(5 * time.Second):
		t.Fatal("provider never started")
	}

	var events []domain.RunEvent
	for ev := range handle.Events() {
		events = append(events, ev)
		// The single small delta is still in the coalescing buffer at this
		// point; request_started proves the provider stream is live.
		if ev.Type == domain.EventModelRequestStarted {
			break
		}
	}
	if err := svc.CancelRun(ctx, handle.RunID); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	select {
	case <-provider.cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("provider stream was not cancelled")
	}
	for ev := range handle.Events() {
		events = append(events, ev)
	}

	if len(events) == 0 || events[len(events)-1].Type != domain.EventRunCancelled {
		t.Fatalf("terminal event = %+v, want run.cancelled", events)
	}
	for i, ev := range events {
		if ev.Seq != uint64(i+1) {
			t.Fatalf("event %d seq = %d, want %d", i, ev.Seq, i+1)
		}
	}
	run, err := store.GetRun(ctx, handle.RunID)
	if err != nil || run.Status != domain.RunCancelled || run.FinishedAt == nil {
		t.Fatalf("run after cancel = %+v (err %v)", run, err)
	}
	snapshot, err := svc.GetRunSnapshot(ctx, handle.RunID)
	if err != nil || len(snapshot.Events) == 0 || snapshot.Events[len(snapshot.Events)-1].Type != domain.EventRunCancelled {
		t.Fatalf("snapshot after cancel = %+v (err %v)", snapshot, err)
	}
	if snapshot.Answer != "partial " {
		t.Fatalf("snapshot answer = %q", snapshot.Answer)
	}
	if err := svc.CancelRun(ctx, handle.RunID); !errors.Is(err, domain.ErrTerminalRun) {
		t.Fatalf("second CancelRun = %v, want ErrTerminalRun", err)
	}
}

type answerProvider struct{ answer string }

func (p answerProvider) Name() string { return "answer" }
func (p answerProvider) Stream(_ context.Context, _ model.StreamRequest, emit func(model.StreamEvent)) error {
	emit(model.StreamEvent{Kind: model.EventTextDelta, Text: p.answer})
	return nil
}

// Seam 1: a run left active by a previous process is interrupted at startup,
// its terminal event survives in the snapshot, and Continue starts a new
// attempt tied to the original run without duplicating the user turn.
func TestSeam1RestartRecoveryAndResume(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sasmita.db")
	_, store := stateTestService(t, path, answerProvider{answer: "unused"})

	if err := store.CreateRun(ctx, &domain.Run{ID: "stale", ConversationID: "c1", Status: domain.RunRunning,
		ModelRef: "gated", ModeID: "chat", StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRun(ctx, &domain.Run{ID: "done", ConversationID: "c1", Status: domain.RunCompleted,
		ModelRef: "gated", ModeID: "chat", StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddMessage(ctx, &domain.Message{ID: "u1", ConversationID: "c1", Role: domain.RoleUser,
		Content: domain.TextContent("original question"), CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	for _, typ := range []string{domain.EventRunQueued, domain.EventRunStarted, domain.EventModelTextDelta} {
		if _, err := store.AppendRunEvent(ctx, "stale", typ, json.RawMessage(`{"version":1}`)); err != nil {
			t.Fatal(err)
		}
	}

	// Fresh service = application restart.
	svc2, _ := openStateDB(t, path, answerProvider{answer: "resumed answer"}, 32)
	recovered, err := svc2.RecoverStaleRuns(ctx)
	if err != nil || len(recovered) != 1 || recovered[0].ID != "stale" {
		t.Fatalf("RecoverStaleRuns = %+v (err %v)", recovered, err)
	}
	staleRun, err := store.GetRun(ctx, "stale")
	if err != nil || staleRun.Status != domain.RunInterrupted || staleRun.FinishedAt == nil {
		t.Fatalf("stale run = %+v (err %v)", staleRun, err)
	}
	doneRun, err := store.GetRun(ctx, "done")
	if err != nil || doneRun.Status != domain.RunCompleted {
		t.Fatalf("terminal run was changed: %+v (err %v)", doneRun, err)
	}
	snapshot, err := svc2.GetRunSnapshot(ctx, "stale")
	if err != nil || len(snapshot.Events) != 4 || snapshot.Events[3].Type != domain.EventRunInterrupted {
		t.Fatalf("recovery snapshot = %+v (err %v)", snapshot, err)
	}
	for i, ev := range snapshot.Events {
		if ev.Seq != uint64(i+1) {
			t.Fatalf("recovery event %d seq = %d", i, ev.Seq)
		}
	}

	handle, err := svc2.ResumeRun(ctx, "stale")
	if err != nil {
		t.Fatalf("ResumeRun: %v", err)
	}
	events := drain(t, handle)
	if events[len(events)-1].Type != domain.EventRunCompleted {
		t.Fatalf("resume terminal = %q", events[len(events)-1].Type)
	}
	resumed, err := store.GetRun(ctx, handle.RunID)
	if err != nil || resumed.OriginalRunID != "stale" || resumed.ID == "stale" {
		t.Fatalf("resumed run = %+v (err %v)", resumed, err)
	}
	messages, err := store.ListMessages(ctx, "c1", 0)
	if err != nil || len(messages) != 2 || messages[0].Text() != "original question" ||
		messages[1].Text() != "resumed answer" {
		t.Fatalf("messages after resume = %+v (err %v)", messages, err)
	}
	if _, err := svc2.ResumeRun(ctx, "done"); !errors.Is(err, domain.ErrIllegalTransition) {
		t.Fatalf("ResumeRun on a completed run = %v, want ErrIllegalTransition", err)
	}
}

// Approval transitions are representable and their events are delivered to a
// live stream instead of being dropped by backpressure.
func TestSeam1ApprovalWaitEventIsDelivered(t *testing.T) {
	ctx := context.Background()
	provider := newGatedProvider()
	svc, store := stateTestService(t, filepath.Join(t.TempDir(), "sasmita.db"), provider)
	handle, err := svc.StartRun(ctx, domain.StartRunRequest{ConversationID: "c1", Text: "question"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-provider.started:
	case <-time.After(5 * time.Second):
		t.Fatal("provider never started")
	}
	if err := svc.MarkAwaitingApproval(ctx, handle.RunID, "tool needs approval"); err != nil {
		t.Fatalf("MarkAwaitingApproval: %v", err)
	}
	run, err := store.GetRun(ctx, handle.RunID)
	if err != nil || run.Status != domain.RunAwaitingApproval {
		t.Fatalf("run status = %+v (err %v)", run, err)
	}

	if err := svc.CancelRun(ctx, handle.RunID); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	events := drain(t, handle)
	var approval, terminal domain.RunEvent
	for _, ev := range events {
		if ev.Type == domain.EventApprovalRequired {
			approval = ev
		}
		terminal = ev
	}
	if approval.Seq == 0 {
		t.Fatalf("approval event was dropped: %+v", events)
	}
	if terminal.Type != domain.EventRunCancelled {
		t.Fatalf("terminal event = %q", terminal.Type)
	}
	snapshot, err := svc.GetRunSnapshot(ctx, handle.RunID)
	if err != nil || len(snapshot.Events) == 0 || snapshot.Events[len(snapshot.Events)-1].Type != domain.EventRunCancelled {
		t.Fatalf("snapshot = %+v (err %v)", snapshot, err)
	}
	found := false
	for _, ev := range snapshot.Events {
		if ev.Type == domain.EventApprovalRequired {
			found = true
		}
	}
	if !found {
		t.Fatal("approval event missing from snapshot")
	}
}

type manyDeltaProvider struct{}

func (manyDeltaProvider) Name() string { return "many" }
func (manyDeltaProvider) Stream(_ context.Context, _ model.StreamRequest, emit func(model.StreamEvent)) error {
	for range 200 {
		emit(model.StreamEvent{Kind: model.EventTextDelta, Text: "x"})
	}
	return nil
}

// Terminal events must survive a full bounded channel: deltas may be
// coalesced/dropped, the final state may not.
func TestSeam1TerminalEventSurvivesBackpressure(t *testing.T) {
	svc, store := openStateDB(t, filepath.Join(t.TempDir(), "sasmita.db"), manyDeltaProvider{}, 1000)
	seedState(t, store)
	handle, err := svc.StartRun(context.Background(), domain.StartRunRequest{ConversationID: "c1", Text: "question"})
	if err != nil {
		t.Fatal(err)
	}
	events := drain(t, handle)
	if len(events) < 2 || events[len(events)-1].Type != domain.EventRunCompleted {
		t.Fatalf("terminal event lost under backpressure: %d events, last %q", len(events), events[len(events)-1].Type)
	}
	for i, ev := range events {
		if ev.Seq != uint64(i+1) {
			t.Fatalf("event %d seq = %d, want %d", i, ev.Seq, i+1)
		}
	}
}
