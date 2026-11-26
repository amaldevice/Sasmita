package desktop_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amaldevice/sasmita/internal/appsvc"
	"github.com/amaldevice/sasmita/internal/desktop"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/persistence/migrations"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
)

func newShellBindings(t *testing.T) *desktop.Service {
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
	now := time.Now()
	if err := store.CreateProject(context.Background(), &domain.Project{
		ID: "p1", Name: "Work", RootURI: "file:///work", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	return desktop.NewService(appsvc.New(store, appsvc.Config{Provider: stubProvider{}}))
}

func TestBindingsShellCommands(t *testing.T) {
	ctx := context.Background()
	bindings := newShellBindings(t)

	projects, err := bindings.ListProjects(ctx)
	if err != nil || len(projects) != 1 || projects[0].ID != "p1" || projects[0].RootURI != "file:///work" {
		t.Fatalf("ListProjects = %+v (err %v)", projects, err)
	}

	loose, err := bindings.CreateConversation(ctx, domain.NoProjectID, "Loose", "chat")
	if err != nil || loose.ProjectID != "" {
		t.Fatalf("CreateConversation(no project) = %+v (err %v)", loose, err)
	}
	renamed, err := bindings.RenameConversation(ctx, loose.ID, "Renamed chat")
	if err != nil || renamed.Title != "Renamed chat" {
		t.Fatalf("RenameConversation = %+v (err %v)", renamed, err)
	}
	moved, err := bindings.MoveConversation(ctx, loose.ID, "p1")
	if err != nil || moved.ProjectID != "p1" {
		t.Fatalf("MoveConversation = %+v (err %v)", moved, err)
	}

	hits, err := bindings.SearchConversations(ctx, "renamed", 10)
	if err != nil || len(hits) != 1 || hits[0].ConversationID != loose.ID || hits[0].Snippet == "" {
		t.Fatalf("SearchConversations = %+v (err %v)", hits, err)
	}

	// The generated TypeScript client relies on these exact JSON field names.
	raw, err := json.Marshal(struct {
		Project desktop.ProjectDTO      `json:"project"`
		Hit     desktop.SearchResultDTO `json:"hit"`
	}{Project: projects[0], Hit: hits[0]})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"projectId"`, `"rootUri"`, `"conversationId"`, `"snippet"`} {
		if !strings.Contains(string(raw), field) {
			t.Errorf("JSON %s missing %s", raw, field)
		}
	}
}
