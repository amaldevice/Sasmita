package migrations_test

import (
	"path/filepath"
	"testing"

	"github.com/amaldevice/sasmita/internal/persistence/migrations"
	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
)

func TestMigrateFromEmptyAndIdempotent(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "sasmita.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := migrations.Migrate(db); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	v, err := migrations.Version(db)
	if err != nil {
		t.Fatal(err)
	}
	if v != migrations.Latest {
		t.Fatalf("version = %d, want %d", v, migrations.Latest)
	}

	// Re-running must be a no-op, never re-apply migration 001.
	if err := migrations.Migrate(db); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	v, err = migrations.Version(db)
	if err != nil || v != migrations.Latest {
		t.Fatalf("version after re-migrate = %d (err %v)", v, err)
	}

	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		found[name] = true
	}
	for _, table := range []string{
		"schema_migrations", "projects", "conversations", "messages",
		"runs", "run_events", "usage_records", "provider_profiles",
	} {
		if !found[table] {
			t.Errorf("table %q was not created", table)
		}
	}
}
