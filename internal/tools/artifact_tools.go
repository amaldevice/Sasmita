package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/amaldevice/sasmita/internal/artifacts"
)

// ArtifactEditor is the narrow capability the Tier 2 tools need. It is
// satisfied by *artifacts.Service; the tools package never touches SQL or the
// artifact filesystem itself.
type ArtifactEditor interface {
	Create(context.Context, artifacts.CreateInput) (artifacts.Artifact, error)
	Edit(context.Context, string, []artifacts.Edit) (artifacts.Artifact, error)
	Rewrite(context.Context, string, artifacts.RewriteInput) (artifacts.Artifact, error)
}

// These tools are opt-in: adding them to the frozen registry only when an
// editor is configured keeps #22's tool set and request bytes unchanged.
func registerArtifactTools(r *Registry, editor ArtifactEditor) {
	r.register(createArtifactTool{editor: editor})
	r.register(editArtifactTool{editor: editor})
	r.register(rewriteArtifactTool{editor: editor})
}

type createArtifactTool struct{ editor ArtifactEditor }

func (createArtifactTool) Spec() Spec {
	return Spec{
		Name: "create_artifact",
		Description: "Store a single-file HTML or JSX artifact as an immutable version and return its artifact:// reference.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"source":{"type":"string"},"sourceKind":{"type":"string","enum":["html","jsx"]},"manifest":{"type":"object"},"projectId":{"type":"string"}},"required":["source"]}`),
		Risk: RiskWrite,
	}
}

func (t createArtifactTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var in artifacts.CreateInput
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("create_artifact: invalid arguments: %w", err)
	}
	a, err := t.editor.Create(ctx, in)
	if err != nil {
		return "", fmt.Errorf("create_artifact: %w", err)
	}
	return artifactResult(a), nil
}

type editArtifactTool struct{ editor ArtifactEditor }

func (editArtifactTool) Spec() Spec {
	return Spec{
		Name: "edit_artifact",
		Description: "Apply exact-match replacements to an artifact and store the result as a new immutable version. Each old string must match exactly once.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"},"edits":{"type":"array","items":{"type":"object","properties":{"old":{"type":"string"},"new":{"type":"string"}},"required":["old","new"]}}},"required":["id","edits"]}`),
		Risk: RiskWrite,
	}
}

func (t editArtifactTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		ID    string         `json:"id"`
		Edits []artifacts.Edit `json:"edits"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("edit_artifact: invalid arguments: %w", err)
	}
	a, err := t.editor.Edit(ctx, in.ID, in.Edits)
	if err != nil {
		return "", fmt.Errorf("edit_artifact: %w", err)
	}
	return artifactResult(a), nil
}

type rewriteArtifactTool struct{ editor ArtifactEditor }

func (rewriteArtifactTool) Spec() Spec {
	return Spec{
		Name: "rewrite_artifact",
		Description: "Replace an artifact's whole source with a new immutable version.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"},"source":{"type":"string"},"sourceKind":{"type":"string","enum":["html","jsx"]},"manifest":{"type":"object"}},"required":["id","source"]}`),
		Risk: RiskWrite,
	}
}

func (t rewriteArtifactTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		ID    string               `json:"id"`
		Input artifacts.RewriteInput `json:"input"`
	}
	// The model calls rewrite_artifact(id, source, ...): accept both a flat
	// argument object and a nested input object.
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("rewrite_artifact: invalid arguments: %w", err)
	}
	payload := in.Input
	if payload.Source == "" {
		var flat artifacts.RewriteInput
		if err := json.Unmarshal(args, &flat); err != nil {
			return "", fmt.Errorf("rewrite_artifact: invalid arguments: %w", err)
		}
		payload = flat
	}
	a, err := t.editor.Rewrite(ctx, in.ID, payload)
	if err != nil {
		return "", fmt.Errorf("rewrite_artifact: %w", err)
	}
	return artifactResult(a), nil
}

func artifactResult(a artifacts.Artifact) string {
	v, err := a.Latest()
	if err != nil {
		return a.Ref
	}
	sum := v.SHA256
	if len(sum) > 12 {
		sum = sum[:12]
	}
	return fmt.Sprintf("%s version %d (%s, sha256 %s)", a.Ref, v.Number, v.Kind, sum)
}
