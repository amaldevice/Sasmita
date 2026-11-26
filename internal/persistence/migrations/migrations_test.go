package migrations_test

import (
	"fmt"
	"path/filepath"
	"sort"
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

// A fresh database must apply every ticket migration exactly once, in
// filename order, and end at the highest known version (TAD §13.4).
func TestMigrateAppliesAllFilesExactlyOnce(t *testing.T) {
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
	// Derive the expectation from the files actually present so parallel
	// ticket lanes can add migrations without editing this invariant test.
	// The derived list includes 007 from #18 and 013 from this ticket.
	names, err := filepath.Glob("*_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	want := make([]string, 0, len(names))
	highest := 0
	for _, name := range names {
		want = append(want, filepath.Base(name))
		var version int
		if _, err := fmt.Sscanf(name[:3], "%d", &version); err == nil && version > highest {
			highest = version
		}
	}
	if fmt.Sprint(applied) != fmt.Sprint(want) {
		t.Fatalf("applied migrations = %v, want %v", applied, want)
	}
	if migrations.Latest != highest {
		t.Fatalf("migrations.Latest = %d, want %d", migrations.Latest, highest)
	}
	v, err := migrations.Version(db)
	if err != nil || v != highest {
		t.Fatalf("schema version = %d (err %v), want %d", v, err, highest)
	}
	for _, table := range []string{"provider_profiles", "conversation_fts"} {
		var name string
		if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name); err != nil {
			t.Errorf("table from #16/#20 migration %q missing: %v", table, err)
		}
	}
}
