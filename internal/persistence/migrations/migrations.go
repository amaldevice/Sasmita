// Package migrations applies versioned, transactional SQLite migrations.
package migrations

import (
	"database/sql"
	"embed"
	"fmt"
)

// Latest is the highest migration version known to this build.
const Latest = 1

//go:embed *.sql
var files embed.FS

// Migrate brings db up to Latest. Each migration is applied once, in its own
// transaction, together with its version stamp.
func Migrate(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	current, err := Version(db)
	if err != nil {
		return err
	}
	for version := current + 1; version <= Latest; version++ {
		name := fmt.Sprintf("%03d_init.sql", version)
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
		if _, err := tx.Exec(`INSERT INTO schema_migrations(version) VALUES(?)`, version); err != nil {
			tx.Rollback()
			return fmt.Errorf("stamp %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Version returns the highest applied migration version, or 0 for a fresh DB.
func Version(db *sql.DB) (int, error) {
	var version int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return version, nil
}
