package appsvc_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/amaldevice/sasmita/internal/appsvc"
	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/persistence/migrations"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
)

func usageTestService(t *testing.T) (*appsvc.Service, *repositories.Store) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "usage.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := migrations.Migrate(db); err != nil {
		t.Fatal(err)
	}
	store := repositories.New(db)
	ctx := t.Context()
	now := time.Now()
	if err := store.CreateProject(ctx, &domain.Project{ID: "p1", Name: "Work", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateConversation(ctx, &domain.Conversation{ID: "c1", ProjectID: "p1", Title: "Usage", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	return appsvc.New(store, appsvc.Config{}), store
}

func seedUsageRun(t *testing.T, store *repositories.Store, id, day string, cacheRead int64) {
	t.Helper()
	started, err := time.Parse(time.RFC3339, day+"T10:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRun(t.Context(), &domain.Run{
		ID: id, ConversationID: "c1", Status: domain.RunCompleted,
		ModelRef: "gpt-test", ModeID: "chat", StartedAt: started,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddUsage(t.Context(), id, domain.Usage{
		ProviderID: "openai", ModelID: "gpt-test",
		InputTokens: 1000, OutputTokens: 100, CacheReadTokens: cacheRead,
		UncachedTokens: 1000 - cacheRead,
	}); err != nil {
		t.Fatal(err)
	}
}

// Seam 1: the application-service read command aggregates stored usage by
// model and by the run's day, and surfaces a hit-rate drop in the report only.
func TestGetUsageReport(t *testing.T) {
	svc, store := usageTestService(t)
	seedUsageRun(t, store, "r1", "2026-11-24", 800)
	seedUsageRun(t, store, "r2", "2026-11-25", 300)

	report, err := svc.GetUsageReport(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Days) != 2 || report.Days[0].Key != "2026-11-24" || report.Days[1].Key != "2026-11-25" {
		t.Fatalf("days = %+v, want the two run days in order", report.Days)
	}
	if len(report.Models) != 1 || report.Models[0].Key != "gpt-test" || report.Models[0].Requests != 2 {
		t.Fatalf("models = %+v, want one model with two requests", report.Models)
	}
	// (800 + 300) / 2000 input tokens = 55 % across both runs.
	if got := report.Models[0].HitRate; got < 0.5499 || got > 0.5501 {
		t.Fatalf("model hit rate = %v, want 0.55", got)
	}
	if !report.Models[0].CostEstimated {
		t.Fatal("cost derived from token counts must be labelled an estimate")
	}
	if report.Drop == nil || report.Drop.Day != "2026-11-25" {
		t.Fatalf("drop = %+v, want a warning on 2026-11-25", report.Drop)
	}
}
