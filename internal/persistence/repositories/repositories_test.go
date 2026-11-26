package repositories_test

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/persistence/migrations"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
)

func newStore(t *testing.T, path string) *repositories.Store {
	t.Helper()
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := migrations.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return repositories.New(db)
}

func TestConversationRoundtripAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sasmita.db")
	store := newStore(t, path)

	ctx := t.Context()
	if err := store.CreateProject(ctx, &domain.Project{ID: "p1", Name: "Work", RootURI: "file:///work", CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	conv := &domain.Conversation{ID: "c1", ProjectID: "p1", Title: "First", ActiveModeID: "chat", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := store.CreateConversation(ctx, conv); err != nil {
		t.Fatal(err)
	}
	msgs := []domain.Message{
		{ID: "m1", ConversationID: "c1", Role: domain.RoleUser, Content: domain.TextContent("question"), CreatedAt: time.Now()},
		{ID: "m2", ConversationID: "c1", Role: domain.RoleAssistant, Content: domain.TextContent("answer"), CreatedAt: time.Now()},
	}
	for i := range msgs {
		if err := store.AddMessage(ctx, &msgs[i]); err != nil {
			t.Fatal(err)
		}
	}

	// Simulate an application restart: close and reopen the same file.
	// (newStore registers its own cleanup; open a second handle explicitly.)
	rereadDB, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer rereadDB.Close()
	reread := repositories.New(rereadDB)

	got, err := reread.GetConversation(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "First" || got.ProjectID != "p1" {
		t.Fatalf("conversation = %+v", got)
	}
	loaded, err := reread.ListMessages(ctx, "c1", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 || loaded[0].Text() != "question" || loaded[1].Text() != "answer" {
		t.Fatalf("messages after restart = %+v", loaded)
	}
}

func TestRunEventsMonotonicSeq(t *testing.T) {
	store := newStore(t, filepath.Join(t.TempDir(), "sasmita.db"))
	ctx := t.Context()
	now := time.Now()
	if err := store.CreateProject(ctx, &domain.Project{ID: "p1", Name: "Work", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateConversation(ctx, &domain.Conversation{ID: "c1", ProjectID: "p1", Title: "First", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRun(ctx, &domain.Run{ID: "r1", ConversationID: "c1", ModelRef: "m", ModeID: "chat", Status: domain.RunQueued, StartedAt: now}); err != nil {
		t.Fatal(err)
	}
	for _, typ := range []string{domain.EventRunQueued, domain.EventRunStarted, domain.EventRunCompleted} {
		if _, err := store.AppendRunEvent(ctx, "r1", typ, json.RawMessage(`{"version":1}`)); err != nil {
			t.Fatal(err)
		}
	}
	events, err := store.ListRunEvents(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}
	for i, ev := range events {
		if ev.Seq != uint64(i+1) {
			t.Errorf("event %d seq = %d, want %d", i, ev.Seq, i+1)
		}
	}
}

func seedRun(t *testing.T, store *repositories.Store, id string, status domain.RunStatus) {
	t.Helper()
	if err := store.CreateRun(t.Context(), &domain.Run{ID: id, ConversationID: "c1", Status: status,
		ModelRef: "m", ModeID: "chat", StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
}

func seedConversation(t *testing.T, store *repositories.Store) {
	t.Helper()
	now := time.Now()
	ctx := t.Context()
	if err := store.CreateProject(ctx, &domain.Project{ID: "p1", Name: "Work", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateConversation(ctx, &domain.Conversation{ID: "c1", ProjectID: "p1", Title: "First", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
}

// Persistence must reject transitions outside TAD §8.3 and must never replace
// a terminal (or recovered/interrupted) state with another state.
func TestRunTransitionPersistence(t *testing.T) {
	store := newStore(t, filepath.Join(t.TempDir(), "sasmita.db"))
	ctx := t.Context()
	seedConversation(t, store)
	seedRun(t, store, "r1", domain.RunQueued)

	if err := store.UpdateRunStatus(ctx, "r1", domain.RunPreparing, nil); err != nil {
		t.Fatalf("queued -> preparing: %v", err)
	}
	if err := store.UpdateRunStatus(ctx, "r1", domain.RunRunning, nil); err != nil {
		t.Fatalf("preparing -> running: %v", err)
	}
	finished := time.Now()
	if err := store.UpdateRunStatus(ctx, "r1", domain.RunCompleted, &finished); err != nil {
		t.Fatalf("running -> completed: %v", err)
	}
	if err := store.UpdateRunStatus(ctx, "r1", domain.RunRunning, nil); !errors.Is(err, domain.ErrTerminalRun) {
		t.Fatalf("completed -> running error = %v, want ErrTerminalRun", err)
	}

	seedRun(t, store, "r2", domain.RunQueued)
	if err := store.UpdateRunStatus(ctx, "r2", domain.RunCompleted, nil); !errors.Is(err, domain.ErrIllegalTransition) {
		t.Fatalf("queued -> completed error = %v, want ErrIllegalTransition", err)
	}
	got, err := store.GetRun(ctx, "r2")
	if err != nil || got.Status != domain.RunQueued {
		t.Fatalf("illegal transition changed status: %+v (err %v)", got, err)
	}

	if err := store.UpdateRunStatus(ctx, "r1", domain.RunCancelled, nil); !errors.Is(err, domain.ErrTerminalRun) {
		t.Fatalf("terminal overwrite error = %v, want ErrTerminalRun", err)
	}
}

// A resume is a new run attempt explicitly tied to the original run.
func TestRetryRunCarriesOriginalID(t *testing.T) {
	store := newStore(t, filepath.Join(t.TempDir(), "sasmita.db"))
	ctx := t.Context()
	seedConversation(t, store)
	seedRun(t, store, "original", domain.RunInterrupted)

	retry := &domain.Run{ID: "retry", ConversationID: "c1", Status: domain.RunQueued,
		ModelRef: "m", ModeID: "chat", StartedAt: time.Now(), OriginalRunID: "original"}
	if err := store.StartRetryTx(ctx, retry, time.Now()); err != nil {
		t.Fatalf("StartRetryTx: %v", err)
	}
	got, err := store.GetRun(ctx, "retry")
	if err != nil || got.OriginalRunID != "original" {
		t.Fatalf("retry run = %+v (err %v)", got, err)
	}
	original, err := store.GetRun(ctx, "original")
	if err != nil || original.Status != domain.RunInterrupted {
		t.Fatalf("original run changed: %+v (err %v)", original, err)
	}
}
