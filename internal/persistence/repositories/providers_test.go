package repositories_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/persistence/migrations"
	"github.com/amaldevice/sasmita/internal/persistence/repositories"
	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
)

func profile(id string) *domain.ProviderProfile {
	now := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	return &domain.ProviderProfile{
		ID: id, Kind: domain.ProviderAnthropicCompatible,
		DisplayName: "Anthropic", BaseURL: "https://api.anthropic.com",
		SecretRef: "sasmita-provider-" + id, CreatedAt: now, UpdatedAt: now,
	}
}

func TestProviderProfileCRUD(t *testing.T) {
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

	want := profile("p1")
	if err := store.CreateProviderProfile(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetProviderProfile(ctx, "p1")
	if err != nil || got.DisplayName != "Anthropic" || got.SecretRef != "sasmita-provider-p1" {
		t.Fatalf("GetProviderProfile = %+v (err %v)", got, err)
	}
	if got.Kind != domain.ProviderAnthropicCompatible {
		t.Fatalf("Kind = %q", got.Kind)
	}

	if _, err := store.GetProviderProfile(ctx, "missing"); err != domain.ErrNotFound {
		t.Fatalf("missing error = %v", err)
	}

	all, err := store.ListProviderProfiles(ctx)
	if err != nil || len(all) != 1 || all[0].ID != "p1" {
		t.Fatalf("ListProviderProfiles = %+v (err %v)", all, err)
	}

	got.DisplayName = "Anthropic renamed"
	got.BaseURL = "https://proxy.example.test"
	if err := store.UpdateProviderProfile(ctx, got); err != nil {
		t.Fatal(err)
	}
	updated, err := store.GetProviderProfile(ctx, "p1")
	if err != nil || updated.DisplayName != "Anthropic renamed" || updated.BaseURL != "https://proxy.example.test" {
		t.Fatalf("updated profile = %+v (err %v)", updated, err)
	}

	if err := store.DeleteProviderProfile(ctx, "p1"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetProviderProfile(ctx, "p1"); err != domain.ErrNotFound {
		t.Fatalf("error after delete = %v", err)
	}
}

// The schema must have nowhere to put a raw key: only an opaque reference.
func TestProviderProfilesTableHasNoKeyColumn(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "sasmita.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrations.Migrate(db); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`PRAGMA table_info(provider_profiles)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(name), "key") || strings.Contains(strings.ToLower(name), "token") {
			t.Fatalf("provider_profiles must not store secret material, found column %q", name)
		}
	}
}
