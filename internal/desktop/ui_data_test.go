package desktop_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amaldevice/sasmita/internal/desktop"
)

// Seam 1: large UI data is fetched through a bounded, read-only host API. The
// reference is opaque, so a model-supplied path cannot escape the data dir.
func TestGetUIDataIsBoundedAndRefIsOpaque(t *testing.T) {
	dir := t.TempDir()
	all := `[{"label":"a","value":1},{"label":"b","value":2},{"label":"c","value":3}]`
	if err := os.WriteFile(filepath.Join(dir, "grid.json"), []byte(all), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := desktop.NewService(nil)
	svc.SetUIDataDir(dir)
	ctx := context.Background()

	limited, err := svc.GetUIData(ctx, "artifact://grid", 2)
	if err != nil {
		t.Fatal(err)
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(limited.Rows, &rows); err != nil || len(rows) != 2 || !limited.Truncated {
		t.Fatalf("bounded result = %s, truncated=%v (err %v)", limited.Rows, limited.Truncated, err)
	}

	full, err := svc.GetUIData(ctx, "artifact://grid", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(full.Rows, &rows); err != nil || len(rows) != 3 || full.Truncated {
		t.Fatalf("full result = %s, truncated=%v (err %v)", full.Rows, full.Truncated, err)
	}

	for _, ref := range []string{"../../etc/passwd", "file:///etc/passwd", "artifact://../grid", "artifact://grid/../../x", "/abs/path"} {
		if _, err := svc.GetUIData(ctx, ref, 10); err == nil {
			t.Errorf("ref %q escaped containment", ref)
		}
	}
	if _, err := svc.GetUIData(ctx, "artifact://missing", 10); err == nil {
		t.Error("missing artifact should fail, not render an empty chart")
	}
	if !strings.Contains(string(full.Rows), `"label":"a"`) {
		t.Fatalf("rows = %s", full.Rows)
	}
}
