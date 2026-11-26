package migrations_test

import (
	"fmt"
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
		"schema_migrations", "schema_migration_files",
		"projects", "conversations", "messages", "runs", "run_events", "usage_records",
		"provider_profiles",
	} {
		if !found[table] {
			t.Errorf("table %q was not created", table)
		}
	}

	// Migration 004 adds the retry linkage on top of #16/#20's schema.
	columns, err := db.Query(`PRAGMA table_info(runs)`)
	if err != nil {
		t.Fatal(err)
	}
	defer columns.Close()
	hasOriginal := false
	for columns.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt any
		if err := columns.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if name == "original_run_id" {
			hasOriginal = true
		}
	}
	if !hasOriginal {
		t.Error("migration 004 did not add runs.original_run_id")
	}
}

// A fresh database must apply all four ticket migrations exactly once, in
// filename order, and end at version 4 (TAD §13.4).
func TestMigrateAppliesAllFourFilesExactlyOnce(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "sasmita.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrations.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := migrations.Migrate(db); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}

	rows, err := db.Query(`SELECT name FROM schema_migration_files ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var applied []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		applied = append(applied, name)
	}
	want := []string{"001_init.sql", "002_init.sql", "003_init.sql", "004_init.sql"}
	if fmt.Sprint(applied) != fmt.Sprint(want) {
		t.Fatalf("applied migrations = %v, want %v", applied, want)
	}
	if migrations.Latest != 4 {
		t.Fatalf("migrations.Latest = %d, want 4", migrations.Latest)
	}
	v, err := migrations.Version(db)
	if err != nil || v != 4 {
		t.Fatalf("schema version = %d (err %v), want 4", v, err)
	}
	for _, table := range []string{"provider_profiles", "conversation_fts"} {
		var name string
		if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name); err != nil {
			t.Errorf("table from #16/#20 migration %q missing: %v", table, err)
		}
	}
}
