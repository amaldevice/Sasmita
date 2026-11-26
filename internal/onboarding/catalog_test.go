package onboarding_test

import (
	"testing"

	"github.com/amaldevice/sasmita/internal/onboarding"
)

// The catalog is data: each entry must be localisable and carry the visual
// intent a later UI-block/artifact ticket can attach a rendering to.
func TestExamplePromptCatalogIsSeededForVisualAnswers(t *testing.T) {
	prompts := onboarding.ListExamplePrompts()
	if len(prompts) != 4 {
		t.Fatalf("got %d example prompts, want 4", len(prompts))
	}

	seen := map[string]bool{}
	for _, prompt := range prompts {
		if seen[prompt.ID] {
			t.Fatalf("duplicate prompt id %q", prompt.ID)
		}
		seen[prompt.ID] = true
		if prompt.ID == "" || prompt.Prompt == "" || prompt.VisualHint == "" {
			t.Fatalf("incomplete catalog entry: %+v", prompt)
		}
		if prompt.TitleID == "" || prompt.TitleEN == "" || prompt.DetailID == "" || prompt.DetailEN == "" {
			t.Fatalf("catalog entry is missing a locale string: %+v", prompt)
		}
	}

	// The returned slice must be safe for callers to mutate.
	prompts[0].ID = "mutated"
	if onboarding.ListExamplePrompts()[0].ID == "mutated" {
		t.Fatal("catalog leaked its internal slice")
	}
}
