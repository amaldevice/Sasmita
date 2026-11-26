package artifacts_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amaldevice/sasmita/internal/artifacts"
	"github.com/amaldevice/sasmita/internal/persistence/migrations"
	"github.com/amaldevice/sasmita/internal/persistence/sqlite"
)

func TestSeam1MigrationCarriesTier2Fields(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "artifacts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrations.Migrate(db); err != nil {
		t.Fatal(err)
	}
	for table, want := range map[string][]string{
		"artifacts":         {"runtime_version", "source_kind", "manifest_json", "status"},
		"artifact_versions": {"artifact_id", "version", "uri", "created_at"},
	} {
		for _, column := range want {
			if !hasColumn(t, db, table, column) {
				t.Errorf("%s has no column %s", table, column)
			}
		}
	}
}

func TestSeam1StoreMirrorsMetadataIntoDatabase(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "artifacts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrations.Migrate(db); err != nil {
		t.Fatal(err)
	}
	store := artifacts.NewStore(t.TempDir())
	store.DB = db
	svc := &artifacts.Service{Store: store}

	a, err := svc.Create(context.Background(), artifacts.CreateInput{Source: "<p>persisted</p>"})
	if err != nil {
		t.Fatal(err)
	}
	var runtime, status, sourceKind, manifest string
	if err := db.QueryRow(`SELECT runtime_version, status, source_kind, manifest_json FROM artifacts WHERE id = ?`, a.ID).
		Scan(&runtime, &status, &sourceKind, &manifest); err != nil {
		t.Fatal(err)
	}
	if sourceKind != string(artifacts.SourceHTML) || manifest != "{}" {
		t.Fatalf("source_kind = %q, manifest = %q", sourceKind, manifest)
	}
	if runtime != artifacts.RuntimeVersion || status != string(artifacts.StatusOK) {
		t.Fatalf("metadata = %q / %q", runtime, status)
	}
	var versions int
	if err := db.QueryRow(`SELECT COUNT(*) FROM artifact_versions WHERE artifact_id = ?`, a.ID).Scan(&versions); err != nil || versions != 1 {
		t.Fatalf("artifact_versions = %d (err %v)", versions, err)
	}
	list, err := svc.List(context.Background())
	if err != nil || len(list) != 1 || len(list[0].Versions) != 1 {
		t.Fatalf("library = %+v (err %v)", list, err)
	}
}

func hasColumn(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if strings.EqualFold(name, column) {
			return true
		}
	}
	return false
}
