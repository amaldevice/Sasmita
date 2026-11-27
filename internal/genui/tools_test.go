package genui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/amaldevice/sasmita/internal/tools"
)

func TestRenderUIToolHasGenericSchema(t *testing.T) {
	spec := NewRenderTool(NewStore()).Spec()
	if spec.Name != "render_ui" || spec.Risk != tools.RiskCompute {
		t.Fatalf("spec = %+v", spec)
	}
	schema := string(spec.InputSchema)
	for _, want := range []string{"schemaVersion", "blockId", "components"} {
		if !strings.Contains(schema, want) {
			t.Fatalf("generic schema missing %q: %s", want, schema)
		}
	}
	if !strings.Contains(spec.Description, "load_skill") {
		t.Fatalf("description does not progressive-disclose skills: %q", spec.Description)
	}
	// Per-component field details live in load_skill docs, not the tool schema.
	for _, unwanted := range []string{"outlier-threshold", "heatmap-grid", "segmented-control"} {
		if strings.Contains(schema, unwanted) {
			t.Fatalf("tool schema leaked component details: %s", schema)
		}
	}
}

func TestRenderUIKeepsFullSpecOutOfToolOutput(t *testing.T) {
	store := NewStore()
	tool := NewRenderTool(store)
	raw := validSpec()
	out, err := tool.Execute(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) >= 512 {
		t.Fatalf("tool output is not a small acknowledgement: %d bytes", len(out))
	}
	var env struct {
		Kind    string `json:"kind"`
		BlockID string `json:"blockId"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatal(err)
	}
	if env.Kind != "ui_block" || env.BlockID != "uib_rounding" || env.Reason != "" {
		t.Fatalf("envelope = %+v", env)
	}
	stored, ok := store.Get(env.BlockID)
	if !ok || string(stored) != string(raw) {
		t.Fatalf("stored spec missing or changed: ok=%v", ok)
	}
	if strings.Contains(out, `"components"`) {
		t.Fatalf("full spec leaked into the prompt: %s", out)
	}
}

func TestRenderUIInvalidSpecReturnsFailClosedReason(t *testing.T) {
	out, err := NewRenderTool(NewStore()).Execute(context.Background(), json.RawMessage(`{"schemaVersion":"nope"}`))
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		Kind    string `json:"kind"`
		Reason  string `json:"reason"`
		BlockID string `json:"blockId"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatal(err)
	}
	if env.Kind != "ui_fallback" || env.Reason == "" || env.BlockID != "" {
		t.Fatalf("envelope = %+v", env)
	}
}

func TestLoadSkillReturnsPerComponentDocs(t *testing.T) {
	tool := NewSkillTool()
	if tool.Spec().Name != "load_skill" {
		t.Fatalf("spec = %+v", tool.Spec())
	}
	for _, component := range []string{"chart", "table", "mermaid", "kpi", "math", "slider", "tabs", "toggle"} {
		args, _ := json.Marshal(map[string]string{"component": component})
		doc, err := tool.Execute(context.Background(), args)
		if err != nil || !strings.Contains(doc, component) || len(doc) < 80 {
			t.Fatalf("skill %q = %q (err %v)", component, doc, err)
		}
	}
	args, _ := json.Marshal(map[string]string{"component": "iframe"})
	doc, err := tool.Execute(context.Background(), args)
	if err != nil || !strings.Contains(doc, "unknown") {
		t.Fatalf("unknown skill = %q (err %v)", doc, err)
	}
}

func TestGenUIToolsRegisterThroughBuiltinRegistry(t *testing.T) {
	registry := tools.New(tools.Config{ArtifactsDir: t.TempDir(), OutputCap: 4096})
	registry.Register(NewRenderTool(NewStore()))
	registry.Register(NewSkillTool())
	defs := registry.Definitions()
	names := make([]string, 0, len(defs))
	for _, d := range defs {
		names = append(names, d.Name)
	}
	got := strings.Join(names, ",")
	if got != "calculator,load_skill,read_project_file,render_ui,search,write_artifact" {
		t.Fatalf("definitions = %q", got)
	}
	for _, name := range []string{"render_ui", "load_skill"} {
		spec, ok := registry.Spec(name)
		if !ok || spec.Risk == "" || spec.Timeout <= 0 || spec.MaxOutputBytes <= 0 {
			t.Fatalf("spec %s = %+v (ok %v)", name, spec, ok)
		}
	}
}
