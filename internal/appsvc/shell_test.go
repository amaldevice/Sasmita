package appsvc_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/amaldevice/sasmita/internal/appsvc"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/persistence/migrations"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
)

func newShellService(t *testing.T) (*appsvc.Service, *repositories.Store) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "sasmita.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := migrations.Migrate(db); err != nil {
		t.Fatal(err)
	}
	store := repositories.New(db)
	now := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	ctx := t.Context()
	for _, id := range []string{"p1", "p2"} {
		if err := store.CreateProject(ctx, &domain.Project{ID: id, Name: id, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	svc := appsvc.New(store, appsvc.Config{Now: func() time.Time { return now }})
	return svc, store
}

func TestShellConversationCommands(t *testing.T) {
	svc, store := newShellService(t)
	ctx := t.Context()

	conv, err := svc.CreateConversation(ctx, "p1", "First", "chat")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddMessage(ctx, &domain.Message{
		ID: "m1", ConversationID: conv.ID, Role: domain.RoleUser,
		Content: domain.TextContent("how does compaction work?"), CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	// Rename trims surrounding whitespace and rejects a blank title.
	renamed, err := svc.RenameConversation(ctx, conv.ID, "  New title  ")
	if err != nil || renamed.Title != "New title" {
		t.Fatalf("rename = %+v (err %v)", renamed, err)
	}
	if _, err := svc.RenameConversation(ctx, conv.ID, "   "); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("blank rename err = %v, want ErrInvalidArgument", err)
	}
	if _, err := svc.RenameConversation(ctx, "missing", "x"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing rename err = %v, want ErrNotFound", err)
	}

	// Move into a project, then out of one. Out of a project is allowed and is
	// represented by the empty NoProjectID.
	moved, err := svc.MoveConversation(ctx, conv.ID, "p2")
	if err != nil || moved.ProjectID != "p2" {
		t.Fatalf("move = %+v (err %v)", moved, err)
	}
	moved, err = svc.MoveConversation(ctx, conv.ID, domain.NoProjectID)
	if err != nil || moved.ProjectID != domain.NoProjectID {
		t.Fatalf("move to no project = %+v (err %v)", moved, err)
	}
	if _, err := svc.MoveConversation(ctx, "missing", "p1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing move err = %v, want ErrNotFound", err)
	}

	// Search sees the renamed title and the message text.
	for _, q := range []string{"new", "compaction"} {
		results, err := svc.SearchConversations(ctx, q, 0)
		if err != nil || len(results) == 0 || results[0].ConversationID != conv.ID {
			t.Fatalf("SearchConversations(%q) = %+v (err %v)", q, results, err)
		}
	}

	projects, err := svc.ListProjects(ctx)
	if err != nil || len(projects) != 2 {
		t.Fatalf("ListProjects = %+v (err %v)", projects, err)
	}

	// A conversation may be created without a project; it has no folder access.
	loose, err := svc.CreateConversation(ctx, domain.NoProjectID, "Loose", "chat")
	if err != nil || loose.ProjectID != domain.NoProjectID {
		t.Fatalf("CreateConversation with no project = %+v (err %v)", loose, err)
	}
}
