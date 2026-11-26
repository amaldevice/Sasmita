package artifacts_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/amaldevice/sasmita/internal/artifacts"
	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/tools"
)

// The Tier 2 commands reuse #22's registry: schemas are frozen, results flow
// through the same bounded Result type, and edit rejections come back to the
// model as tool errors rather than being swallowed by the host.
func TestSeam1ArtifactToolsReuseRegistryResultPath(t *testing.T) {
	svc := &artifacts.Service{Store: artifacts.NewStore(t.TempDir())}
	registry := tools.New(tools.Config{ArtifactsDir: t.TempDir(), Artifacts: svc})
	ctx := context.Background()

	defs := registry.Definitions()
	found := map[string]bool{}
	for _, def := range defs {
		found[def.Name] = true
	}
	for _, name := range []string{"create_artifact", "edit_artifact", "rewrite_artifact"} {
		if !found[name] {
			t.Fatalf("registry missing %s: %+v", name, defs)
		}
	}

	res, err := registry.Execute(ctx, model.ToolCall{
		Name: "create_artifact",
		Input: json.RawMessage(`{"source":"<p>cat cat</p>"}`),
	})
	if err != nil || !strings.Contains(res.Output, "artifact://") || !strings.Contains(res.Output, "version 1") {
		t.Fatalf("create result = %+v (err %v)", res, err)
	}
	id := strings.TrimPrefix(strings.Fields(res.Output)[0], "artifact://")

	_, err = registry.Execute(ctx, model.ToolCall{
		Name: "edit_artifact",
		Input: json.RawMessage(`{"id":"` + id + `","edits":[{"old":"cat","new":"dog"}]}`),
	})
	if err == nil || !strings.Contains(err.Error(), "matches 2 times") {
		t.Fatalf("ambiguous edit err = %v", err)
	}

	res, err = registry.Execute(ctx, model.ToolCall{
		Name: "edit_artifact",
		Input: json.RawMessage(`{"id":"` + id + `","edits":[{"old":"<p>cat cat</p>","new":"<p>dog dog</p>"}]}`),
	})
	if err != nil || !strings.Contains(res.Output, "version 2") {
		t.Fatalf("edit result = %+v (err %v)", res, err)
	}
}

func TestSeam1ArtifactToolsAreOptIn(t *testing.T) {
	registry := tools.New(tools.Config{ArtifactsDir: t.TempDir()})
	for _, def := range registry.Definitions() {
		if def.Name == "create_artifact" || def.Name == "edit_artifact" || def.Name == "rewrite_artifact" {
			t.Fatalf("artifact tool %s registered when not configured", def.Name)
		}
	}
}
