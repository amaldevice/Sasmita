package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/amaldevice/sasmita/internal/tools"
)

// query_table is a discovery-only bounded tool: it is callable through
// call_tool but its schema is not native until compaction promotes it.
type queryTableTool struct{}

func (queryTableTool) Spec() tools.Spec {
	return tools.Spec{
		Name:        "query_table",
		Description: "Run a bounded aggregate (count, sum, mean, min, max) over a small inline table of rows supplied in the arguments.",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"required":["rows","column","op"],` +
			`"properties":{"rows":{"type":"array","items":{"type":"object"}},"column":{"type":"string"},` +
			`"op":{"type":"string","enum":["count","sum","mean","min","max"]}}}`),
		Risk: tools.RiskCompute,
	}
}

func (queryTableTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Rows   []map[string]any `json:"rows"`
		Column string           `json:"column"`
		Op     string           `json:"op"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("query_table: invalid arguments: %w", err)
	}
	if len(in.Rows) > 500 {
		return "", fmt.Errorf("query_table: at most 500 rows")
	}
	if in.Op == "count" {
		out, _ := json.Marshal(map[string]any{"value": len(in.Rows)})
		return string(out), nil
	}
	values := make([]float64, 0, len(in.Rows))
	for _, row := range in.Rows {
		n, ok := numeric(row[in.Column])
		if !ok {
			return "", fmt.Errorf("query_table: column %q is not numeric", in.Column)
		}
		values = append(values, n)
	}
	if len(values) == 0 {
		return "", fmt.Errorf("query_table: no numeric values in %q", in.Column)
	}
	value := values[0]
	switch in.Op {
	case "sum", "mean":
		value = 0
		for _, v := range values {
			value += v
		}
		if in.Op == "mean" {
			value /= float64(len(values))
		}
	case "min":
		for _, v := range values[1:] {
			value = math.Min(value, v)
		}
	case "max":
		for _, v := range values[1:] {
			value = math.Max(value, v)
		}
	default:
		return "", fmt.Errorf("query_table: unknown operation %q", in.Op)
	}
	out, err := json.Marshal(map[string]any{"value": value})
	if err != nil {
		return "", err
	}
	return string(out), nil
}
