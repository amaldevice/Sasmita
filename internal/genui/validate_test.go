package genui

import (
	"encoding/json"
	"strings"
	"testing"
)

// validSpec is the smallest UI block that exercises every Tier 1 component and
// a view-local binding. It is the known-good literal the validator is tested
// against; the invalid cases below change exactly one property at a time.
func validSpec() json.RawMessage {
	return json.RawMessage(`{
		"schemaVersion":"sasmita.ui/v1",
		"blockId":"uib_rounding",
		"kind":"interactive",
		"title":"Rounding explorer",
		"summary":"Compare rounding modes.",
		"state":{"resolution":"cell","threshold":76,"showErrors":false},
		"data":{"source":{"source":"artifact://rows"}},
		"components":[
			{"id":"resolution","type":"tabs","label":"Resolution","bind":"state.resolution","options":["sample","cell"]},
			{"id":"threshold","type":"slider","label":"Threshold","bind":"state.threshold","min":0,"max":100,"step":1},
			{"id":"show-errors","type":"toggle","label":"Show errors","bind":"state.showErrors"},
			{"id":"series","type":"chart","label":"Error by threshold","values":[{"label":"0","value":1},{"label":"1","value":2}]},
			{"id":"rows","type":"table","label":"Samples","dataRef":"data.source"},
			{"id":"flow","type":"mermaid","label":"Flow","source":"A[Start] --> B[End]"},
			{"id":"metrics","type":"kpi","label":"Summary","values":[{"label":"Bias","value":"0.1"}]},
			{"id":"formula","type":"math","label":"Formula","source":"E = mc^2"}
		]
	}`)
}

func TestValidateAcceptsKnownGoodUIBlock(t *testing.T) {
	res := Validate(validSpec())
	if !res.OK {
		t.Fatalf("valid UI block rejected: %s", res.Reason)
	}
	if res.Spec.BlockID != "uib_rounding" || len(res.Spec.Components) != 8 {
		t.Fatalf("parsed spec = %+v", res.Spec)
	}
}

func TestValidateFailsClosedWithShortReason(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(string) string
		wantKey string
	}{
		{"unknown schema version", func(s string) string {
			return strings.Replace(s, "sasmita.ui/v1", "sasmita.ui/v2", 1)
		}, "schema version"},
		{"unknown component", func(s string) string {
			return strings.Replace(s, `"type":"chart"`, `"type":"iframe"`, 1)
		}, "component"},
		{"undeclared binding", func(s string) string {
			return strings.Replace(s, "state.threshold", "state.missing", 1)
		}, "bind"},
		{"undeclared data ref", func(s string) string {
			return strings.Replace(s, "data.source", "data.ghost", 1)
		}, "dataref"},
		{"duplicate component id", func(s string) string {
			return strings.Replace(s, `"id":"rows"`, `"id":"series"`, 1)
		}, "duplicate"},
		{"missing summary", func(s string) string {
			return strings.Replace(s, `"summary":"Compare rounding modes.",`, "", 1)
		}, "summary"},
		{"invalid slider range", func(s string) string {
			return strings.Replace(s, `"min":0,"max":100`, `"min":100,"max":0`, 1)
		}, "slider"},
		{"disallowed action", func(s string) string {
			return strings.Replace(s, `"components":[`, `"actions":[{"id":"x","type":"shell","handler":"rm"}],"components":[`, 1)
		}, "actions"},
		{"malformed json", func(string) string { return `{"schemaVersion":` }, "invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := Validate(json.RawMessage(tc.mutate(string(validSpec()))))
			if res.OK {
			t.Fatal("invalid UI block was accepted")
		}
			if res.Reason == "" {
				t.Fatal("fail-closed result has no reason")
			}
			if !strings.Contains(strings.ToLower(res.Reason), tc.wantKey) {
				t.Fatalf("reason %q does not mention %q", res.Reason, tc.wantKey)
			}
			if len(res.Reason) > 120 {
				t.Fatalf("reason is not short: %d bytes", len(res.Reason))
			}
		})
	}
}

func TestValidateEnforcesTADQuotas(t *testing.T) {
	// The quota checks are driven by independent literal specs, not by a
	// reimplementation of the validator's counting logic.
	cases := []struct {
		name   string
		spec   string
		want   string
	}{
		{"component nodes", quotaSpec(101, 0, 0, 0, 0), "components"},
		{"interactive controls", quotaSpec(0, 21, 0, 0, 0), "controls"},
		{"inline table rows", quotaSpec(0, 0, 201, 0, 0), "rows"},
		{"chart points per series", quotaSpec(0, 0, 0, 5001, 0), "points"},
		{"grid cells", quotaSpec(0, 0, 0, 0, 4097), "cells"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := Validate(json.RawMessage(tc.spec))
			if res.OK || !strings.Contains(strings.ToLower(res.Reason), tc.want) {
				t.Fatalf("result = ok:%v reason:%q, want %q", res.OK, res.Reason, tc.want)
			}
		})
	}

	large := `{"schemaVersion":"sasmita.ui/v1","blockId":"uib_big","summary":"Big.","components":[{"id":"x","type":"math","source":"` +
		strings.Repeat("x", MaxSpecBytes+1) + `"}]}`
	if res := Validate(json.RawMessage(large)); res.OK || !strings.Contains(res.Reason, "payload") {
		t.Fatalf("oversized payload: ok:%v reason:%q", res.OK, res.Reason)
	}
}

// quotaSpec builds a literal spec with exactly the requested excess. controls,
// rows, points and cells are separate so each quota test isolates one rule.
func quotaSpec(components, controls, rows, points, cells int) string {
	parts := make([]string, 0, components+controls+3)
	for i := 0; i < components; i++ {
		parts = append(parts, `{"id":"c`+itoa(i)+`","type":"math","source":"x"}`)
	}
	states := make([]string, 0, controls+1)
	for i := 0; i < controls; i++ {
		parts = append(parts, `{"id":"k`+itoa(i)+`","type":"toggle","bind":"state.k`+itoa(i)+`"}`)
		states = append(states, `"k`+itoa(i)+`":false`)
	}
	if rows > 0 {
		values := make([]string, 0, rows)
		for i := 0; i < rows; i++ {
			values = append(values, `{"a":1}`)
		}
		parts = append(parts, `{"id":"table","type":"table","values":[`+strings.Join(values, ",")+`]}`)
	}
	if points > 0 {
		values := make([]string, 0, points)
		for i := 0; i < points; i++ {
			values = append(values, `{"label":"p","value":1}`)
		}
		parts = append(parts, `{"id":"chart","type":"chart","series":1,"values":[`+strings.Join(values, ",")+`]}`)
	}
	if cells > 0 {
		parts = append(parts, `{"id":"grid","type":"chart","rows":`+itoa(cells)+`,"columns":1}`)
	}
	return `{"schemaVersion":"sasmita.ui/v1","blockId":"uib_quota","summary":"Quota.",` +
		`"state":{` + strings.Join(states, ",") + `},"components":[` + strings.Join(parts, ",") + `]}`
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}

func TestValidatePatchRejectsOversizedAndUnknownKeys(t *testing.T) {
	res := Validate(validSpec())
	if !res.OK {
		t.Fatal(res.Reason)
	}
	if err := ValidatePatch(json.RawMessage(`{"threshold":80}`), res.Spec); err != nil {
		t.Fatalf("valid patch rejected: %v", err)
	}
	if err := ValidatePatch(json.RawMessage(`{"ghost":1}`), res.Spec); err == nil {
		t.Fatal("unknown state key accepted")
	}
	big := json.RawMessage(`{"threshold":"` + strings.Repeat("x", MaxPatchBytes+1) + `"}`)
	if err := ValidatePatch(big, res.Spec); err == nil || !strings.Contains(err.Error(), "patch") {
		t.Fatalf("oversized patch: %v", err)
	}
}

func TestFallbackTextPreservesSummaryAndReason(t *testing.T) {
	text := FallbackText("Rounding explorer", "unknown component: iframe")
	if !strings.Contains(text, "Rounding explorer") || !strings.Contains(text, "unknown component: iframe") {
		t.Fatalf("fallback = %q", text)
	}
}
