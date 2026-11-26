package desktop_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amaldevice/sasmita/internal/appsvc"
	"github.com/amaldevice/sasmita/internal/desktop"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/persistence/migrations"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
)

type stubProvider struct{}

func (stubProvider) Name() string { return "stub" }
func (stubProvider) Stream(_ context.Context, _ model.StreamRequest, emit func(model.StreamEvent)) error {
	emit(model.StreamEvent{Kind: model.EventTextDelta, Text: "stub answer"})
	return nil
}

// The typed bindings seam must expose all four commands and the run stream.
func TestBindingsCommandsEndToEnd(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "sasmita.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrations.Migrate(db); err != nil {
		t.Fatal(err)
	}
	store := repositories.New(db)
	svc := appsvc.New(store, appsvc.Config{Provider: stubProvider{}, DefaultModel: "stub-model"})
	bindings := desktop.NewService(svc)

	if err := store.CreateProject(ctx, &domain.Project{ID: "p1", Name: "Work"}); err != nil {
		t.Fatal(err)
	}
	conv, err := bindings.CreateConversation(ctx, "p1", "Typed chat", "chat")
	if err != nil {
		t.Fatal(err)
	}
	page, err := bindings.ListConversations(ctx, "p1", "", 10)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != conv.ID {
		t.Fatalf("ListConversations = %+v (err %v)", page, err)
	}
	detail, err := bindings.GetConversation(ctx, conv.ID)
	if err != nil || detail.Conversation.Title != "Typed chat" {
		t.Fatalf("GetConversation = %+v (err %v)", detail, err)
	}

	handle, err := bindings.StartRun(ctx, desktop.StartRunRequestDTO{ConversationID: conv.ID, Text: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	var answer strings.Builder
	var last domain.RunEvent
	for ev := range handle.Events {
		last = ev
		if ev.Type == domain.EventModelTextDelta {
			var p domain.TextDeltaPayload
			json.Unmarshal(ev.Payload, &p)
			answer.WriteString(p.Text)
		}
	}
	if last.Type != domain.EventRunCompleted || answer.String() != "stub answer" {
		t.Fatalf("terminal = %q, answer = %q", last.Type, answer.String())
	}
	snapshot, err := bindings.GetRunSnapshot(ctx, handle.RunID)
	if err != nil || snapshot.Answer != "stub answer" || snapshot.Status != string(domain.RunCompleted) {
		t.Fatalf("snapshot = %+v (err %v)", snapshot, err)
	}
}
