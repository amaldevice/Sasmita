// Package migrations applies versioned, transactional SQLite migrations.
package migrations

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
)

// Latest is the highest migration version known to this build.
const Latest = 4

//go:embed *.sql
var files embed.FS

// Migrate brings db up to Latest. Each migration is applied once, in its own
// transaction, together with its version stamp. Applied files are tracked by
// name as well: migrations for parallel tickets can land in any order, so a
// missing version must not permanently hide a later migration (or vice versa).
func Migrate(db *sql.DB) error {
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS schema_migration_files (
			name TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
	} {
		if _, err := db.Exec(ddl); err != nil {
			return fmt.Errorf("create migration metadata: %w", err)
		}
	}

	names, err := fs.Glob(files, "[0-9][0-9][0-9]_init.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)

	applied, err := loadAppliedFiles(db)
	if err != nil {
		return err
	}
	for _, name := range names {
		version, err := strconv.Atoi(name[:3])
		if err != nil || version > Latest {
			continue
		}
		if applied[name] {
			continue
		}
		script, err := files.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(script)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT OR REPLACE INTO schema_migrations(version) VALUES(?)`, version); err != nil {
			tx.Rollback()
			return fmt.Errorf("stamp %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migration_files(name) VALUES(?)`, name); err != nil {
			tx.Rollback()
			return fmt.Errorf("record %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func loadAppliedFiles(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query(`SELECT name FROM schema_migration_files`)
	if err != nil {
		return nil, fmt.Errorf("read applied migrations: %w", err)
	}
	defer rows.Close()
	applied := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		applied[name] = true
	}
	return applied, rows.Err()
}

// Version returns the highest applied migration version, or 0 for a fresh DB.
func Version(db *sql.DB) (int, error) {
	var version int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return version, nil
}
