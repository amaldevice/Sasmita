package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amaldevice/sasmita/internal/appsvc"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/persistence/migrations"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
)

type fileReadProvider struct{ calls int }

func (p *fileReadProvider) Name() string { return "file-read-fake" }

func (p *fileReadProvider) Stream(_ context.Context, _ model.StreamRequest, emit func(model.StreamEvent)) error {
	p.calls++
	if p.calls == 1 {
		emit(model.StreamEvent{Kind: model.EventTextDelta, Text: "Membaca. "})
		emit(model.StreamEvent{Kind: model.EventToolUse, ToolCall: &model.ToolCall{
			ID: "fr-1", Name: "read_project_file", Input: []byte(`{"path":"note.txt"}`),
		}})
		return nil
	}
	emit(model.StreamEvent{Kind: model.EventTextDelta, Text: "Selesai."})
	return nil
}

func newScopeService(t *testing.T, projectRoot string, provider model.ModelProvider) (*appsvc.Service, *repositories.Store, string) {
	t.Helper()
	ctx := context.Background()
	db, err := sqlite.Open(t.TempDir() + "/sasmita.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := migrations.Migrate(db); err != nil {
		t.Fatal(err)
	}
	store := repositories.New(db)
	now := time.Now()
	if err := store.CreateProject(ctx, &domain.Project{
		ID: "p1", Name: "Work", RootURI: "file://" + projectRoot, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	// No service-level folders: every root must come from the conversation.
	svc := appsvc.New(store, appsvc.Config{
		Provider: provider, DefaultModel: "m", NewID: newIDGenerator(),
		EnableTools: true, ArtifactsDir: t.TempDir(), ToolOutputCap: 512,
	})
	return svc, store, "p1"
}

// Seam 1: a conversation in a project can read its project root; a conversation
// without a project has no folder access and the same call is denied by policy.
func TestSeam1ProjectScopeControlsFolderAccess(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("project secret 42"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc, _, projectID := newScopeService(t, root, &fileReadProvider{})
	withProject, err := svc.CreateConversation(context.Background(), projectID, "In project", "chat")
	if err != nil {
		t.Fatal(err)
	}
	h, err := svc.StartRun(context.Background(), domain.StartRunRequest{
		ConversationID: withProject.ID, Text: "read", ModelRef: "m", ModeID: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	events := drain(t, h)
	if !eventTypes(events)[domain.EventToolExecuted] {
		t.Fatalf("project file was not readable: %+v", events)
	}
	if !strings.Contains(coalescedText(events), "Selesai.") {
		t.Fatalf("answer = %q", coalescedText(events))
	}

	// A fresh service/provider so the no-project run also proposes the call.
	svcNoProject, _, _ := newScopeService(t, t.TempDir(), &fileReadProvider{})
	noProject, err := svcNoProject.CreateConversation(context.Background(), domain.NoProjectID, "No project", "chat")
	if err != nil {
		t.Fatal(err)
	}
	h2, err := svcNoProject.StartRun(context.Background(), domain.StartRunRequest{
		ConversationID: noProject.ID, Text: "read", ModelRef: "m", ModeID: "chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	events2 := drain(t, h2)
	if !eventTypes(events2)[domain.EventToolDenied] || eventTypes(events2)[domain.EventToolExecuted] {
		t.Fatalf("no-project conversation had folder access: %+v", events2)
	}
}

// Seam 1: explicit attachments are the only folder access a conversation
// without a project gets.
func TestSeam1AttachmentGrantsFileAccessWithoutProject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("attached 77"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc, _, _ := newScopeService(t, t.TempDir(), &fileReadProvider{})
	conv, err := svc.CreateConversation(context.Background(), domain.NoProjectID, "Attached", "chat")
	if err != nil {
		t.Fatal(err)
	}
	h, err := svc.StartRun(context.Background(), domain.StartRunRequest{
		ConversationID: conv.ID, Text: "read", ModelRef: "m", ModeID: "chat",
		Attachments: []string{path},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := drain(t, h)
	if !eventTypes(events)[domain.EventToolExecuted] {
		t.Fatalf("attached file was not readable: %+v", events)
	}
}
