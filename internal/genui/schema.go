// Package genui validates Sasmita-owned UI block specs (`sasmita.ui/v1`) and
// exposes them as bounded built-in tools. Only approved Svelte components are
// rendered; no model-supplied code is stored, compiled or executed here.
package genui

import "encoding/json"

// SchemaVersion is the only accepted UI block contract version.
const SchemaVersion = "sasmita.ui/v1"

// Payload guardrails (TAD §12A.12). These are initial targets, not measured
// performance claims; exceeding one fails the spec closed to text.
const (
	MaxSpecBytes            = 256 << 10
	MaxComponents           = 100
	MaxControls             = 20
	MaxTableRows            = 200
	MaxChartPointsPerSeries = 5000
	MaxGridCells            = 4096
	MaxPatchBytes           = 32 << 10
)

// Spec is the internal UI block contract (TAD §12A.5). JSON tags are the wire
// names; unknown input fields are ignored, but unknown component types and
// schema versions are rejected by Validate.
type Spec struct {
	SchemaVersion string                     `json:"schemaVersion"`
	BlockID       string                     `json:"blockId"`
	Kind          string                     `json:"kind"`
	Title         string                     `json:"title,omitempty"`
	Summary       string                     `json:"summary"`
	Layout        Layout                     `json:"layout,omitempty"`
	State         map[string]json.RawMessage `json:"state,omitempty"`
	Data          map[string]DataRef         `json:"data,omitempty"`
	Components    []Component                `json:"components"`
	Actions       []Action                   `json:"actions,omitempty"`
}

// Action is reserved for future typed backend handlers. Tier 1 blocks are
// view-local, so any action in a spec is rejected rather than silently ignored.
type Action struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Handler string `json:"handler,omitempty"`
}

type Layout struct {
	Mode    string `json:"mode,omitempty"`
	Columns int    `json:"columns,omitempty"`
}

// DataRef is an opaque host-resolved reference. Renderers fetch large payloads
// through a bounded data API instead of putting them in the prompt.
type DataRef struct {
	Source   string `json:"source,omitempty"`
	Encoding string `json:"encoding,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
}

type Component struct {
	ID      string            `json:"id"`
	Type    string            `json:"type"`
	Label   string            `json:"label,omitempty"`
	Bind    string            `json:"bind,omitempty"`
	DataRef string            `json:"dataRef,omitempty"`
	Options []string          `json:"options,omitempty"`
	Min     *float64          `json:"min,omitempty"`
	Max     *float64          `json:"max,omitempty"`
	Step    *float64          `json:"step,omitempty"`
	Source  string            `json:"source,omitempty"`
	Values  []json.RawMessage `json:"values,omitempty"`
	Series  int               `json:"series,omitempty"`
	Rows    int               `json:"rows,omitempty"`
	Columns int               `json:"columns,omitempty"`
}

// Result is the validator outcome. Reason is a short, user-safe explanation
// used by the text fallback; it never includes raw model JSON.
type Result struct {
	OK     bool
	Spec   Spec
	Reason string
}

// BlockEvent is the persisted ui.block RunEvent payload. Either Spec or Reason
// is set, never both: a failed validation is a text fallback rather than a
// partially rendered block.
type BlockEvent struct {
	SchemaVersion string          `json:"schemaVersion"`
	BlockID       string          `json:"blockId"`
	Spec          json.RawMessage `json:"spec,omitempty"`
	Reason        string          `json:"reason,omitempty"`
}

// UIStateEvent is the persisted ui.state RunEvent payload for a user-committed
// view-local state change (debounced; never one event per pointer move).
type UIStateEvent struct {
	SchemaVersion string          `json:"schemaVersion"`
	BlockID       string          `json:"blockId"`
	RunID         string          `json:"runId,omitempty"`
	Patch         json.RawMessage `json:"patch"`
}

// UIData is the bounded result of the host data API used by renderers.
type UIData struct {
	Ref       string          `json:"ref"`
	Rows      json.RawMessage `json:"rows"`
	Truncated bool            `json:"truncated"`
}
