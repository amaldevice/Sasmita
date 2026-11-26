package desktop_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/amaldevice/sasmita/internal/appsvc"
	"github.com/amaldevice/sasmita/internal/desktop"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/persistence/migrations"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
)

func usageBindingService(t *testing.T) (*desktop.Service, *repositories.Store) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "usage-binding.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := migrations.Migrate(db); err != nil {
		t.Fatal(err)
	}
	store := repositories.New(db)
	now := time.Now()
	if err := store.CreateProject(t.Context(), &domain.Project{ID: "p1", Name: "Work", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateConversation(t.Context(), &domain.Conversation{ID: "c1", ProjectID: "p1", Title: "Usage", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	return desktop.NewService(appsvc.New(store, appsvc.Config{})), store
}

// The typed IPC surface exposes the Diagnostics read model, with estimates
// still marked as estimates when it crosses the boundary.
func TestBindingsGetUsageReport(t *testing.T) {
	bindings, store := usageBindingService(t)
	started := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	if err := store.CreateRun(t.Context(), &domain.Run{
		ID: "r1", ConversationID: "c1", Status: domain.RunCompleted,
		ModelRef: "gpt-test", HarnessID: "chat", StartedAt: started,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddUsage(t.Context(), "r1", domain.Usage{
		ProviderID: "openai", ModelID: "gpt-test",
		InputTokens: 100, OutputTokens: 20, CacheReadTokens: 25, UncachedTokens: 75,
	}); err != nil {
		t.Fatal(err)
	}

	report, err := bindings.GetUsageReport(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Days) != 1 || report.Days[0].Key != "2026-11-26" {
		t.Fatalf("days = %+v", report.Days)
	}
	if len(report.Models) != 1 || report.Models[0].HitRate != 0.25 || !report.Models[0].CostEstimated {
		t.Fatalf("models = %+v, want hit rate 0.25 and an estimated cost", report.Models)
	}
}

// Seam 1: an answer's usage travels with its persisted message, so a hover
// after restart still shows provider figures and app estimates separately.
func TestBindingsGetConversationCarriesAnswerUsage(t *testing.T) {
	bindings, store := usageBindingService(t)
	started := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	run := &domain.Run{ID: "r1", ConversationID: "c1", Status: domain.RunQueued,
		ModelRef: "gpt-test", HarnessID: "chat", StartedAt: started}
	if err := store.CreateRun(t.Context(), run); err != nil {
		t.Fatal(err)
	}
	for _, status := range []domain.RunStatus{domain.RunPreparing, domain.RunRunning} {
		if err := store.UpdateRunStatus(t.Context(), "r1", status, nil); err != nil {
			t.Fatal(err)
		}
	}
	finished := started.Add(1200 * time.Millisecond)
	if err := store.UpdateRunStatus(t.Context(), "r1", domain.RunCompleted, &finished); err != nil {
		t.Fatal(err)
	}
	if err := store.AddUsage(t.Context(), "r1", domain.Usage{
		ProviderID: "openai", ModelID: "gpt-test",
		InputTokens: 100, OutputTokens: 20, CacheReadTokens: 25, UncachedTokens: 75,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddMessage(t.Context(), &domain.Message{
		ID: "a1", ConversationID: "c1", Role: domain.RoleAssistant,
		Content: domain.TextContent("answer"), CreatedAt: finished, RunID: "r1",
	}); err != nil {
		t.Fatal(err)
	}

	detail, err := bindings.GetConversation(t.Context(), "c1")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Messages) != 1 || detail.Messages[0].Usage == nil {
		t.Fatalf("messages = %+v, want one answer carrying usage", detail.Messages)
	}
	usage := detail.Messages[0].Usage
	if usage.ProviderID != "openai" || usage.HitRate != 0.25 || !usage.CostEstimated {
		t.Fatalf("answer usage = %+v", usage)
	}
	if usage.DurationMs != 1200 {
		t.Fatalf("duration = %d ms, want 1200", usage.DurationMs)
	}
}
