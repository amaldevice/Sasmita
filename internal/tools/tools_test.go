package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amaldevice/sasmita/internal/model"
)

func TestCalculatorIsArithmeticOnly(t *testing.T) {
	cases := []struct {
		expr string
		want string
		err  bool
	}{
		{"(2+3)*4", "20", false},
		{"-2 + 3*2", "4", false},
		{"10/4", "2.5", false},
		{"1/0", "", true},
		{"__import__('os')", "", true},
		{"1; rm -rf /", "", true},
	}
	for _, tc := range cases {
		args, _ := json.Marshal(map[string]string{"expression": tc.expr})
		got, err := calculator{}.Execute(context.Background(), args)
		if tc.err {
			if err == nil {
				t.Errorf("%q: expected rejection, got %q", tc.expr, got)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("%q: got %q (err %v), want %q", tc.expr, got, err, tc.want)
		}
	}
}

func TestFileToolCannotEscapeAllowedRoots(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ok.txt"), []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	f := fileTool{roots: []string{root}, readLimit: 1024}
	for _, path := range []string{
		"../" + filepath.Base(outside) + "/secret.txt",
		filepath.Join(outside, "secret.txt"),
		"link.txt",
		"",
	} {
		args, _ := json.Marshal(map[string]string{"path": path})
		if out, err := f.Execute(context.Background(), args); err == nil {
			t.Errorf("path %q escaped containment: %q", path, out)
		}
	}
	args, _ := json.Marshal(map[string]string{"path": "ok.txt"})
	if out, err := f.Execute(context.Background(), args); err != nil || out != "inside" {
		t.Fatalf("allowed read = %q (err %v)", out, err)
	}
}

type longTool struct{}

func (longTool) Spec() Spec {
	return Spec{Name: "long", InputSchema: json.RawMessage(`{}`), MaxOutputBytes: 8}
}
func (longTool) Execute(context.Context, json.RawMessage) (string, error) {
	return strings.Repeat("x", 1024), nil
}

func TestRegistryCapsOutputIntoArtifact(t *testing.T) {
	dir := t.TempDir()
	r := New(Config{ArtifactsDir: dir, OutputCap: 16})
	r.register(longTool{})
	res, err := r.Execute(context.Background(), model.ToolCall{Name: "long"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Truncated || !strings.HasPrefix(res.ArtifactRef, "artifact://") {
		t.Fatalf("result = %+v", res)
	}
	stored, err := os.ReadFile(filepath.Join(dir, res.ArtifactID+".txt"))
	if err != nil || len(stored) != 1024 {
		t.Fatalf("artifact = %d bytes (err %v)", len(stored), err)
	}
	if len(res.Summary) >= len(stored)/2 {
		t.Fatalf("summary is not short: %d bytes", len(res.Summary))
	}
}

func TestBuiltinRegistrationIncludesRiskTimeoutAndCaps(t *testing.T) {
	r := New(Config{ArtifactsDir: t.TempDir()})
	want := []string{"calculator", "read_project_file", "search", "write_artifact"}
	defs := r.Definitions()
	if len(defs) != len(want) {
		t.Fatalf("definitions = %+v", defs)
	}
	for i, name := range want {
		if defs[i].Name != name {
			t.Fatalf("definition %d = %q, want %q", i, defs[i].Name, name)
		}
		spec, ok := r.Spec(name)
		if !ok || spec.Risk == "" || spec.Timeout <= 0 || spec.MaxOutputBytes <= 0 || len(spec.InputSchema) == 0 {
			t.Fatalf("spec %s = %+v", name, spec)
		}
	}
}
