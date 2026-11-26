package policy_test

import (
	"testing"

	"github.com/amaldevice/sasmita/internal/policy"
)

func TestDefaultMatrixEvaluatesToolArgumentsScopeAndMode(t *testing.T) {
	engine := policy.New()
	root := "/work/project"
	cases := []struct {
		name string
		req  policy.Request
		want policy.Outcome
	}{
		{
			name: "read inside approved workspace",
			req:  policy.Request{ToolID: "read_project_file", Arguments: []byte(`{"path":"notes.txt"}`), Mode: policy.ModeChat, Scope: policy.Scope{Roots: []string{root}}},
			want: policy.Allow,
		},
		{
			name: "read outside workspace",
			req:  policy.Request{ToolID: "read_project_file", Arguments: []byte(`{"path":"../secrets.txt"}`), Mode: policy.ModeChat, Scope: policy.Scope{Roots: []string{root}}},
			want: policy.Deny,
		},
		{
			name: "secret store is never readable",
			req:  policy.Request{ToolID: "read_secret_store", Arguments: []byte(`{}`), Mode: policy.ModeCoding, Scope: policy.Scope{Roots: []string{root}}},
			want: policy.Deny,
		},
		{
			name: "bulk delete denied by default",
			req:  policy.Request{ToolID: "delete_many", Arguments: []byte(`{"paths":["a","b"]}`), Mode: policy.ModeCoding, Scope: policy.Scope{Roots: []string{root}}},
			want: policy.Deny,
		},
		{
			name: "validated UI block renders without approval",
			req:  policy.Request{ToolID: "render_ui", Arguments: []byte(`{"schemaVersion":"sasmita.ui/v1"}`), Mode: policy.ModeChat, Scope: policy.Scope{Roots: []string{root}}},
			want: policy.Allow,
		},
		{
			name: "loading a skill reads only the catalog",
			req:  policy.Request{ToolID: "load_skill", Arguments: []byte(`{"name":"rounding"}`), Mode: policy.ModeChat},
			want: policy.Allow,
		},
		{
			name: "immutable artifact edit is reversible",
			req:  policy.Request{ToolID: "edit_artifact", Arguments: []byte(`{"id":"art-1","edits":[]}`), Mode: policy.ModeChat},
			want: policy.Allow,
		},
		{
			name: "create artifact is reversible",
			req:  policy.Request{ToolID: "write_artifact", Arguments: []byte(`{"name":"chart","content":"x"}`), Mode: policy.ModeChat, Scope: policy.Scope{Roots: []string{root}}},
			want: policy.Allow,
		},
		{
			name: "overwrite needs approval",
			req:  policy.Request{ToolID: "write_artifact", Arguments: []byte(`{"name":"chart","content":"x","overwrite":true}`), Mode: policy.ModeChat, Scope: policy.Scope{Roots: []string{root}}},
			want: policy.Approval,
		},
		{
			name: "external write needs approval",
			req:  policy.Request{ToolID: "send_email", Arguments: []byte(`{}`), Mode: policy.ModeChat},
			want: policy.Approval,
		},
		{
			name: "shell needs Coding Mode",
			req:  policy.Request{ToolID: "shell", Arguments: []byte(`{"command":"ls"}`), Mode: policy.ModeChat},
			want: policy.Deny,
		},
		{
			name: "shell needs approval even in Coding Mode",
			req:  policy.Request{ToolID: "shell", Arguments: []byte(`{"command":"ls"}`), Mode: policy.ModeCoding},
			want: policy.Approval,
		},
		{
			name: "unknown tool fails closed",
			req:  policy.Request{ToolID: "mystery", Arguments: []byte(`{}`), Mode: policy.ModeChat},
			want: policy.Deny,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := engine.Evaluate(tc.req, false)
			if got.Outcome != tc.want {
				t.Fatalf("outcome = %q, want %q (reason %q)", got.Outcome, tc.want, got.Reason)
			}
			if got.Action == "" || got.Resource == "" || got.Impact == "" {
				t.Fatalf("decision is missing plain-language fields: %+v", got)
			}
		})
	}
}

func TestProjectGrantOverridesApprovalButNeverShell(t *testing.T) {
	engine := policy.New()
	req := policy.Request{ToolID: "send_email", Arguments: []byte(`{}`), Mode: policy.ModeChat}
	if got := engine.Evaluate(req, true).Outcome; got != policy.Allow {
		t.Fatalf("granted external write = %q, want allow", got)
	}
	shell := policy.Request{ToolID: "shell", Arguments: []byte(`{"command":"ls"}`), Mode: policy.ModeCoding}
	if got := engine.Evaluate(shell, true).Outcome; got != policy.Approval {
		t.Fatalf("granted shell = %q, want approval", got)
	}
}

func TestArgumentsHashIsCanonicalAndPolicySnapshotIsStable(t *testing.T) {
	a := policy.HashArguments([]byte(`{"a":1,"b":2}`))
	b := policy.HashArguments([]byte(`{"b":2,"a":1}`))
	c := policy.HashArguments([]byte(`{"a":2,"b":2}`))
	if a == "" || a != b || a == c {
		t.Fatalf("hashes = %q %q %q", a, b, c)
	}
	first, second := policy.New().Snapshot(), policy.New().Snapshot()
	if first.Version == "" || first.Hash == "" || first != second {
		t.Fatalf("snapshots = %+v / %+v", first, second)
	}
	changed := policy.Engine{Version: "v2"}
	if changed.Snapshot() == first {
		t.Fatal("snapshot did not change with policy version")
	}
}
