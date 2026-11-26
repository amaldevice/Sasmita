package modes_test

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/amaldevice/sasmita/internal/modes"
)

// The frontend mirror is a deliberate seam-2 fake: Playwright has no Go process.
// This test is the single source of truth check — both sides read the shared
// fixture, so a Mode added on one side only fails here.
func TestCatalogMatchesFrontendFixture(t *testing.T) {
	raw, err := os.ReadFile("../../frontend/src/fixtures/modes.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture []modes.Mode
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	got := modes.List()
	if !reflect.DeepEqual(got, fixture) {
		t.Fatalf("Mode catalog drifted from frontend fixture\nbackend: %+v\nfixture: %+v", got, fixture)
	}
	for _, mode := range got {
		for _, tool := range mode.ToolsetIDs {
			if tool == "query_table" {
				t.Fatalf("out-of-scope tool query_table is enabled in Mode %s", mode.ID)
			}
		}
	}
}
