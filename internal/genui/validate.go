package genui

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var (
	idPattern    = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	bindPattern  = regexp.MustCompile(`^state\.[A-Za-z0-9_-]{1,64}$`)
	dataPattern  = regexp.MustCompile(`^data\.[A-Za-z0-9_-]{1,64}$`)
	artifactURI  = regexp.MustCompile(`^artifact://[A-Za-z0-9_-]{1,128}$`)
)

// approvedComponents is the entire Tier 1 catalog. Anything else fails closed.
var approvedComponents = map[string]bool{
	"chart": true, "table": true, "mermaid": true, "kpi": true, "math": true,
	"slider": true, "tabs": true, "toggle": true,
}

var controlComponents = map[string]bool{
	"slider": true, "tabs": true, "toggle": true,
}

func fail(reason string) Result { return Result{Reason: reason} }

// Validate parses and checks a raw UI block spec. A rejected spec is never
// partially rendered: the caller receives a short reason for a text fallback.
func Validate(raw json.RawMessage) Result {
	if len(raw) > MaxSpecBytes {
		return fail(fmt.Sprintf("UI spec payload exceeds %d KB", MaxSpecBytes>>10))
	}
	var spec Spec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return fail("invalid UI spec: " + shortJSONError(err))
	}
	if spec.SchemaVersion != SchemaVersion {
		return fail(fmt.Sprintf("unknown schema version %q", spec.SchemaVersion))
	}
	if !idPattern.MatchString(spec.BlockID) {
		return fail("invalid blockId")
	}
	if strings.TrimSpace(spec.Summary) == "" {
		return fail("UI block requires a summary")
	}
	switch spec.Kind {
	case "", "static", "interactive":
	default:
		return fail(fmt.Sprintf("unknown block kind %q", spec.Kind))
	}
	switch spec.Layout.Mode {
	case "", "inline", "two-column", "left", "right":
	default:
		return fail(fmt.Sprintf("unknown layout mode %q", spec.Layout.Mode))
	}
	if len(spec.Actions) > 0 {
		return fail("actions are not supported in Tier 1 UI blocks")
	}
	if len(spec.Components) == 0 {
		return fail("UI block requires at least one component")
	}
	if len(spec.Components) > MaxComponents {
		return fail(fmt.Sprintf("too many components (max %d)", MaxComponents))
	}
	for key, ref := range spec.Data {
		if !idPattern.MatchString(key) {
			return fail(fmt.Sprintf("invalid data key %q", key))
		}
		if ref.Source != "" && !artifactURI.MatchString(ref.Source) {
			return fail(fmt.Sprintf("data %q source is not an opaque artifact reference", key))
		}
	}

	seen := make(map[string]bool, len(spec.Components))
	controls := 0
	for i := range spec.Components {
		c := &spec.Components[i]
		if !approvedComponents[c.Type] {
			return fail(fmt.Sprintf("unknown component type %q", c.Type))
		}
		if !idPattern.MatchString(c.ID) || seen[c.ID] {
			if !idPattern.MatchString(c.ID) {
				return fail(fmt.Sprintf("invalid component id %q", c.ID))
			}
			return fail(fmt.Sprintf("duplicate component id %q", c.ID))
		}
		seen[c.ID] = true

		if c.Bind != "" {
			key, ok := stateKey(c.Bind)
			if !ok {
				return fail(fmt.Sprintf("binding %q must reference state.<key>", c.Bind))
			}
			if _, ok := spec.State[key]; !ok {
				return fail(fmt.Sprintf("binding %q is not declared in state", c.Bind))
			}
		}
		if c.DataRef != "" {
			key, ok := dataKey(c.DataRef)
			if !ok {
				return fail(fmt.Sprintf("dataRef %q must reference data.<key>", c.DataRef))
			}
			if _, ok := spec.Data[key]; !ok {
				return fail(fmt.Sprintf("dataRef %q is not declared in data", c.DataRef))
			}
		}
		if controlComponents[c.Type] {
			controls++
			if c.Bind == "" {
				return fail(fmt.Sprintf("%s requires a state binding", c.Type))
			}
			switch c.Type {
			case "tabs":
				if len(c.Options) < 2 || len(c.Options) > 20 {
					return fail("tabs require 2 to 20 options")
				}
			case "slider":
				if c.Min == nil || c.Max == nil || *c.Min >= *c.Max {
					return fail("slider requires min < max")
				}
				if c.Step != nil && *c.Step <= 0 {
					return fail("slider requires a positive step")
				}
			}
		}

		if quotaReason := checkQuotas(c); quotaReason != "" {
			return fail(quotaReason)
		}
	}
	if controls > MaxControls {
		return fail(fmt.Sprintf("too many interactive controls (max %d)", MaxControls))
	}
	return Result{OK: true, Spec: spec}
}

func checkQuotas(c *Component) string {
	switch c.Type {
	case "table":
		if len(c.Values) > MaxTableRows || c.Rows > MaxTableRows {
			return fmt.Sprintf("table rows exceed %d", MaxTableRows)
		}
	case "chart":
		series := c.Series
		if series < 1 {
			series = 1
		}
		points := len(c.Values)
		if (points+series-1)/series > MaxChartPointsPerSeries {
			return fmt.Sprintf("chart points per series exceed %d", MaxChartPointsPerSeries)
		}
	}
	if c.Rows > 0 && c.Columns > 0 && c.Rows > MaxGridCells/c.Columns {
		return fmt.Sprintf("grid cells exceed %d", MaxGridCells)
	}
	return ""
}

func stateKey(bind string) (string, bool) {
	m := bindPattern.FindStringSubmatch(bind)
	if m == nil {
		return "", false
	}
	return strings.TrimPrefix(bind, "state."), true
}

func dataKey(ref string) (string, bool) {
	m := dataPattern.FindStringSubmatch(ref)
	if m == nil {
		return "", false
	}
	return strings.TrimPrefix(ref, "data."), true
}

func shortJSONError(err error) string {
	msg := err.Error()
	if i := strings.Index(msg, ":"); i >= 0 {
		msg = msg[i+1:]
	}
	msg = strings.TrimSpace(msg)
	if len(msg) > 60 {
		msg = msg[:60] + "…"
	}
	return msg
}

// ValidatePatch checks a small, typed state patch against the declared state
// keys. Patches may not introduce keys or change their JSON type.
func ValidatePatch(raw json.RawMessage, spec Spec) error {
	if len(raw) > MaxPatchBytes {
		return fmt.Errorf("UI state patch exceeds %d KB (max %d bytes)", MaxPatchBytes>>10, MaxPatchBytes)
	}
	var patch map[string]json.RawMessage
	if err := json.Unmarshal(raw, &patch); err != nil {
		return fmt.Errorf("invalid UI state patch: %w", err)
	}
	for key, value := range patch {
		current, ok := spec.State[key]
		if !ok {
			return fmt.Errorf("UI state patch key %q is not declared", key)
		}
		if jsonKind(value) != jsonKind(current) {
			return fmt.Errorf("UI state patch key %q changes its type", key)
		}
	}
	return nil
}

func jsonKind(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "empty"
	}
	switch trimmed[0] {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	default:
		return "number"
	}
}

// FallbackText keeps the agent's textual answer visible when a UI block cannot
// be rendered. The reason is short and safe for display in the chat window.
func FallbackText(title, reason string) string {
	if title == "" {
		return "UI block unavailable: " + reason
	}
	return title + " — UI block unavailable: " + reason
}
