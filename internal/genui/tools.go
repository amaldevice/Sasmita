package genui

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/amaldevice/sasmita/internal/tools"
)

// toolEnvelope is deliberately small: the model sees a status, never the full
// UI specification (TAD §12A.11).
type toolEnvelope struct {
	Kind    string `json:"kind"`
	BlockID string `json:"blockId,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// genericRenderSchema is one schema for every Tier 1 component. Detailed field
// rules are loaded on demand with load_skill, not injected into every prompt.
const genericRenderSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["schemaVersion", "blockId", "summary", "components"],
  "properties": {
    "schemaVersion": {"type": "string", "const": "sasmita.ui/v1"},
    "blockId": {"type": "string", "pattern": "^[A-Za-z0-9_-]{1,64}$"},
    "kind": {"type": "string", "enum": ["static", "interactive"]},
    "title": {"type": "string"},
    "summary": {"type": "string", "description": "One-sentence accessible fallback text."},
    "layout": {"type": "object", "properties": {
      "mode": {"type": "string", "enum": ["inline", "two-column", "left", "right"]},
      "columns": {"type": "integer", "minimum": 1, "maximum": 4}
    }},
    "state": {"type": "object", "description": "Declared view-local state values."},
    "data": {"type": "object", "description": "Opaque artifact references for large data."},
    "components": {"type": "array", "items": {
      "type": "object", "required": ["id", "type"],
      "properties": {
        "id": {"type": "string"},
        "type": {"type": "string", "enum": ["chart", "table", "mermaid", "kpi", "math", "slider", "tabs", "toggle"]},
        "label": {"type": "string"},
        "bind": {"type": "string", "description": "state.<key> for interactive controls."},
        "dataRef": {"type": "string", "description": "data.<key> for host-resolved data."},
        "options": {"type": "array", "items": {"type": "string"}},
        "min": {"type": "number"}, "max": {"type": "number"}, "step": {"type": "number"},
        "source": {"type": "string", "description": "Inline diagram or formula text."},
        "values": {"type": "array", "items": {"type": "object"}},
        "series": {"type": "integer"}, "rows": {"type": "integer"}, "columns": {"type": "integer"}
      }
    }}
  }
}`

var _ tools.Tool = (*renderTool)(nil)

type renderTool struct{ store *Store }

// NewRenderTool returns the bounded render_ui built-in. Validated specs are
// kept in store for the host renderer; only a short acknowledgement is returned.
func NewRenderTool(store *Store) tools.Tool {
	if store == nil {
		store = NewStore()
	}
	return &renderTool{store: store}
}

func (r *renderTool) Spec() tools.Spec {
	return tools.Spec{
		Name: "render_ui",
		Description: "Render a Tier 1 UI block inline from one generic sasmita.ui/v1 spec. " +
			"Call load_skill(component) for the exact rules of each component type before authoring. " +
			"Use artifact references in data for large datasets.",
		InputSchema: json.RawMessage(genericRenderSchema),
		Risk:        tools.RiskCompute,
	}
}

func (r *renderTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	res := Validate(args)
	var env toolEnvelope
	if !res.OK {
		env = toolEnvelope{Kind: "ui_fallback", Reason: res.Reason}
	} else {
		r.store.Put(res.Spec.BlockID, args)
		env = toolEnvelope{Kind: "ui_block", BlockID: res.Spec.BlockID}
	}
	out, err := json.Marshal(env)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

var _ tools.Tool = (*skillTool)(nil)

type skillTool struct{}

// NewSkillTool returns the progressive-disclosure built-in for component docs.
func NewSkillTool() tools.Tool { return &skillTool{} }

func (s *skillTool) Spec() tools.Spec {
	return tools.Spec{
		Name:        "load_skill",
		Description: "Load the compact authoring rules for one UI block component before calling render_ui.",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"required":["component"],` +
			`"properties":{"component":{"type":"string","enum":["chart","table","mermaid","kpi","math","slider","tabs","toggle"]}}}`),
		Risk: tools.RiskReadOnly,
	}
}

func (s *skillTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var req struct {
		Component string `json:"component"`
	}
	if err := json.Unmarshal(args, &req); err != nil {
		return "unknown component skill: pass a component name", nil
	}
	doc, ok := skillDocs[req.Component]
	if !ok {
		names := make([]string, 0, len(skillDocs))
		for name := range skillDocs {
			names = append(names, name)
		}
		return "unknown component skill " + strconvQuote(req.Component) + "; available: " + strings.Join(sortedStrings(names), ", "), nil
	}
	return doc, nil
}

func strconvQuote(s string) string {
	out, _ := json.Marshal(s)
	return string(out)
}

func sortedStrings(in []string) []string {
	for i := 1; i < len(in); i++ {
		for j := i; j > 0 && in[j] < in[j-1]; j-- {
			in[j], in[j-1] = in[j-1], in[j]
		}
	}
	return in
}
