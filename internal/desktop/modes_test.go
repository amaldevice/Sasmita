package desktop_test

import (
	"context"
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

func TestBindingsExposeModesAndSwitchEstimate(t *testing.T) {
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
	bindings := desktop.NewService(appsvc.New(store, appsvc.Config{Provider: stubProvider{}}))
	conv, err := bindings.CreateConversation(ctx, "", "Modes", "chat")
	if err != nil {
		t.Fatal(err)
	}

	modes := bindings.ListModes(ctx)
	if len(modes) != 5 {
		t.Fatalf("ListModes returned %d Modes, want 5", len(modes))
	}
	available := map[string]bool{}
	for _, mode := range modes {
		available[mode.ID] = mode.Available
	}
	for _, id := range []string{"chat", "riset", "penulisan"} {
		if !available[id] {
			t.Errorf("Mode %q should be selectable", id)
		}
	}
	for _, id := range []string{"analis-data", "coding"} {
		if available[id] {
			t.Errorf("Mode %q should be marked coming later", id)
		}
	}

	short, err := bindings.EstimateSwitch(ctx, desktop.SwitchEstimateRequestDTO{
		ConversationID: conv.ID, ModelRef: "m", ModeID: "riset",
	})
	if err != nil || short.Warn || !short.Estimated {
		t.Fatalf("short estimate = %+v (err %v)", short, err)
	}
	now := time.Now()
	if err := store.AddMessage(ctx, &domain.Message{
		ID: "long", ConversationID: conv.ID, Role: domain.RoleUser,
		Content: domain.TextContent(strings.Repeat("x", 20000)), CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	long, err := bindings.EstimateSwitch(ctx, desktop.SwitchEstimateRequestDTO{
		ConversationID: conv.ID, ModelRef: "m2", ModeID: "riset",
	})
	if err != nil || !long.Warn || long.EstimatedTokens <= 4000 {
		t.Fatalf("long estimate = %+v (err %v)", long, err)
	}
	if _, err := bindings.EstimateSwitch(ctx, desktop.SwitchEstimateRequestDTO{
		ConversationID: conv.ID, ModeID: "coding",
	}); err == nil {
		t.Fatal("unavailable Mode passed the switch preflight")
	}
}
