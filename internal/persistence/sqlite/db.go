// Package sqlite opens the application database with the lifecycle pragmas
// required by TAD §13.4.
package sqlite

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Open returns a database handle with foreign keys, a busy timeout and (for
// file databases) WAL enabled. modernc.org/sqlite v1.63.0 uses SQLITE_OPEN_NOMUTEX,
// so a single connection is used to keep pragma scope and write ordering simple.
func Open(path string) (*sql.DB, error) {
	dsn := "file:" + path
	if path == ":memory:" {
		dsn = ":memory:"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
	}
	if path != ":memory:" {
		pragmas = append(pragmas, "PRAGMA journal_mode = WAL")
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("%s: %w", pragma, err)
		}
	}
	return db, nil
}
