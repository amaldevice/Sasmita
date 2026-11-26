package sqlite_test

import (
	"path/filepath"
	"testing"

	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
)

// modernc.org/sqlite's FTS5 support was flagged as "appears compiled but
// unconfirmed" in research/core-dependencies.md. Conversation search (ticket
// #20) depends on it, so pin the fact with a test rather than assume it.
func TestFTS5Available(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "fts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var enabled int
	if err := db.QueryRow(`SELECT sqlite_compileoption_used('ENABLE_FTS5')`).Scan(&enabled); err != nil {
		t.Fatal(err)
	}
	if enabled != 1 {
		t.Fatal("modernc.org/sqlite was built without FTS5; conversation search needs a different strategy")
	}
	if _, err := db.Exec(`CREATE VIRTUAL TABLE probe USING fts5(text, tokenize = 'unicode61 remove_diacritics 2')`); err != nil {
		t.Fatalf("create FTS5 table: %v", err)
	}

	// The FTS triggers extract plain text from messages.content_json.
	var text string
	if err := db.QueryRow(`SELECT json_extract('[{"type":"text","text":"hello"}]', '$[0].text')`).Scan(&text); err != nil {
		t.Fatalf("json_extract unavailable: %v", err)
	}
	if text != "hello" {
		t.Fatalf("json_extract = %q, want hello", text)
	}
}
