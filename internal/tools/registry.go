// Package tools owns the built-in, bounded tools available to an agent run.
// No tool executes a shell, evaluates arbitrary code, or escapes its allowed
// roots. Risk, timeout and output caps are part of every registered schema.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/model"
)

const defaultTimeout = 2 * time.Second

type Risk string

const (
	RiskCompute  Risk = "compute"
	RiskReadOnly Risk = "read_only"
	RiskWrite    Risk = "write"
)

// Spec is the provider-neutral registration record for one tool.
type Spec struct {
	Name           string
	Description    string
	InputSchema    json.RawMessage
	Risk           Risk
	Timeout        time.Duration
	MaxOutputBytes int
}

// Tool is implemented only by built-ins in this package.
type Tool interface {
	Spec() Spec
	Execute(ctx context.Context, args json.RawMessage) (string, error)
}

// ConversationSearcher is the repository capability the search tool needs;
// no tool talks to SQL directly.
type ConversationSearcher interface {
	SearchConversations(ctx context.Context, query string, limit int) ([]domain.SearchResult, error)
}

type Config struct {
	ArtifactsDir   string
	WorkspaceRoots []string
	AttachedFiles  []string
	Search         ConversationSearcher
	OutputCap      int // 0 means 4096 bytes
	NewID          func() string
	// Artifacts opts the registry into the Tier 2 artifact tools (#28). Nil
	// keeps #22's frozen tool set and request bytes unchanged.
	Artifacts ArtifactEditor
}

// Result is a normalized tool outcome. Oversized output never leaves the
// registry: it becomes an artifact plus a short summary.
type Result struct {
	Output      string
	Summary     string
	ArtifactID  string
	ArtifactRef string
	Truncated   bool
	Duration    time.Duration
}

type Registry struct {
	tools     map[string]*boundedTool
	defs      []model.ToolDefinition
	artifacts *ArtifactStore
	outputCap int
}

// New builds the built-in toolset. Definitions are frozen at construction so
// later discovery cannot change request bytes (ADR-0001, ADR-0003).
func New(cfg Config) *Registry {
	if cfg.OutputCap <= 0 {
		cfg.OutputCap = 4096
	}
	if cfg.NewID == nil {
		cfg.NewID = randomArtifactID
	}
	r := &Registry{
		tools:     make(map[string]*boundedTool),
		artifacts: NewArtifactStore(cfg.ArtifactsDir),
		outputCap: cfg.OutputCap,
	}
	r.artifacts.NewID = cfg.NewID
	r.register(fileTool{roots: cfg.WorkspaceRoots, attached: cfg.AttachedFiles, readLimit: cfg.OutputCap * 4})
	r.register(searchTool{
		roots: cfg.WorkspaceRoots, search: cfg.Search, readLimit: cfg.OutputCap / 2,
	})
	r.register(calculator{})
	r.register(artifactTool{artifacts: r.artifacts})
	if cfg.Artifacts != nil {
		registerArtifactTools(r, cfg.Artifacts)
	}
	return r
}

// Register adds an additional built-in after New. Definitions are re-sorted so
// the provider request layout stays deterministic (ADR-0001).
func (r *Registry) Register(t Tool) { r.register(t) }

func (r *Registry) register(t Tool) {
	spec := t.Spec()
	if spec.Timeout <= 0 {
		spec.Timeout = defaultTimeout
	}
	if spec.MaxOutputBytes <= 0 || spec.MaxOutputBytes > r.outputCap {
		spec.MaxOutputBytes = r.outputCap
	}
	r.tools[spec.Name] = &boundedTool{tool: t, spec: spec}
	r.defs = append(r.defs, model.ToolDefinition{
		Name: spec.Name, Description: spec.Description, InputSchema: append(json.RawMessage(nil), spec.InputSchema...),
	})
	sort.Slice(r.defs, func(i, j int) bool { return r.defs[i].Name < r.defs[j].Name })
}

// Definitions returns a frozen copy of the tool schemas sent to the provider.
func (r *Registry) Definitions() []model.ToolDefinition {
	return append([]model.ToolDefinition(nil), r.defs...)
}

func (r *Registry) Spec(name string) (Spec, bool) {
	bt, ok := r.tools[name]
	if !ok {
		return Spec{}, false
	}
	return bt.spec, true
}

// Execute dispatches one call under its timeout and output cap.
func (r *Registry) Execute(ctx context.Context, call model.ToolCall) (Result, error) {
	bt, ok := r.tools[call.Name]
	if !ok {
		return Result{}, fmt.Errorf("unknown tool %q", call.Name)
	}
	start := time.Now()
	runCtx, cancel := context.WithTimeout(ctx, bt.spec.Timeout)
	defer cancel()
	out, err := bt.tool.Execute(runCtx, call.Input)
	if err != nil {
		return Result{Duration: time.Since(start)}, err
	}
	result := Result{Output: out, Duration: time.Since(start)}
	if len(out) > bt.spec.MaxOutputBytes {
		art, err := r.artifacts.Write(call.Name, out)
		if err != nil {
			return result, err
		}
		summary := summarize(out, 256)
		result.Output, result.Summary = summary, summary
		result.ArtifactID, result.ArtifactRef, result.Truncated = art.ID, art.Ref, true
		return result, nil
	}
	result.Summary = summarize(out, 256)
	return result, nil
}

func summarize(out string, max int) string {
	runes := []rune(out)
	if len(runes) <= max {
		return out
	}
	return string(runes[:max]) + " … [diringkas]"
}

// boundedTool fixes the registered limits so a tool cannot raise its own cap.
type boundedTool struct {
	tool Tool
	spec Spec
}

func (b *boundedTool) Spec() Spec { return b.spec }
func (b *boundedTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return b.tool.Execute(ctx, args)
}
