package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/amaldevice/sasmita/internal/model"
	"github.com/amaldevice/sasmita/internal/tools"
)

const maxResultBytes = 8192

// Service owns the meta-tools, the local index and per-conversation promotion
// state. It is created before the provider tool list is frozen (ADR-0003).
type Service struct {
	registry   *tools.Registry
	index      *Index
	dispatcher *Dispatcher
	base       []model.ToolDefinition

	mu         sync.RWMutex
	discovered map[string]map[string]bool
	promoted   map[string]map[string]bool
}

// New registers the meta-tools, then freezes the native definition list that
// every new conversation starts with. v1 has no discovery-only tools: the only
// candidate, query_table, belongs to the out-of-scope Phase 4 Mode Analis Data.
func New(registry *tools.Registry, cfg DispatcherConfig) *Service {
	s := &Service{
		registry:   registry,
		dispatcher: NewDispatcher(registry, cfg),
		discovered: make(map[string]map[string]bool),
		promoted:   make(map[string]map[string]bool),
	}
	registry.Register(&findToolsTool{service: s})
	registry.Register(&callToolTool{service: s})
	s.index = NewIndex(registry.EnabledSpecs())
	s.base = registry.Definitions()
	return s
}

// Definitions returns the frozen base list plus tools promoted at a previous
// compaction of this conversation. It is identical on every turn until then.
func (s *Service) Definitions(conversationID string) []model.ToolDefinition {
	s.mu.RLock()
	promoted := make([]string, 0, len(s.promoted[conversationID]))
	for name := range s.promoted[conversationID] {
		promoted = append(promoted, name)
	}
	s.mu.RUnlock()
	sort.Strings(promoted)
	out := append([]model.ToolDefinition(nil), s.base...)
	seen := make(map[string]bool, len(out))
	for _, def := range out {
		seen[def.Name] = true
	}
	for _, name := range promoted {
		if seen[name] {
			continue
		}
		spec, ok := s.registry.Spec(name)
		if !ok {
			continue
		}
		out = append(out, model.ToolDefinition{
			Name: spec.Name, Description: spec.Description,
			InputSchema: append(json.RawMessage(nil), spec.InputSchema...),
		})
		seen[name] = true
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Note records a tool used through call_tool so compaction can promote it.
func (s *Service) Note(conversationID, name string) {
	if conversationID == "" || name == "" {
		return
	}
	s.mu.Lock()
	if s.discovered[conversationID] == nil {
		s.discovered[conversationID] = make(map[string]bool)
	}
	s.discovered[conversationID][name] = true
	s.mu.Unlock()
}

// Promote is called only at compaction, the one allowed prefix reset point.
func (s *Service) Promote(conversationID string) []string {
	s.mu.Lock()
	names := make([]string, 0, len(s.discovered[conversationID]))
	for name := range s.discovered[conversationID] {
		names = append(names, name)
	}
	sort.Strings(names)
	if s.promoted[conversationID] == nil {
		s.promoted[conversationID] = make(map[string]bool)
	}
	for _, name := range names {
		s.promoted[conversationID][name] = true
	}
	delete(s.discovered, conversationID)
	s.mu.Unlock()
	return names
}

// Find returns the ranked result as a compact, bounded tool-result string.
func (s *Service) Find(query string) (string, error) {
	hits := s.index.Search(query, 5)
	for {
		encoded, err := model.MarshalDeterministic(struct {
			Count int  `json:"count"`
			Tools []hit `json:"tools"`
		}{Count: len(hits), Tools: hits})
		if err != nil {
			return "", err
		}
		if len(encoded) <= maxResultBytes || len(hits) == 0 {
			return string(encoded), nil
		}
		hits = hits[:len(hits)-1]
	}
}

// Resolve turns a call_tool envelope into the real call used for policy,
// approval and execution. The assistant tool_use block stays the original
// envelope in canonical history (ADR-0003).
func (s *Service) Resolve(call model.ToolCall) (model.ToolCall, error) {
	var in struct {
		Name string          `json:"name"`
		Args json.RawMessage `json:"args"`
	}
	if err := json.Unmarshal(call.Input, &in); err != nil {
		return model.ToolCall{}, fmt.Errorf("call_tool: invalid arguments: %w", err)
	}
	if in.Name == "" {
		return model.ToolCall{}, fmt.Errorf("call_tool: name is required")
	}
	if _, ok := s.registry.Spec(in.Name); !ok {
		return model.ToolCall{}, fmt.Errorf("call_tool: tool %q is not enabled", in.Name)
	}
	if len(in.Args) == 0 {
		in.Args = json.RawMessage(`{}`)
	}
	return model.ToolCall{ID: call.ID, Name: in.Name, Input: in.Args}, nil
}

// Call dispatches one resolved tool call through the bounded dispatcher.
func (s *Service) Call(ctx context.Context, call model.ToolCall) (tools.Result, error) {
	return s.dispatcher.Call(ctx, call.Name, call.Input)
}

var _ tools.Tool = (*findToolsTool)(nil)

type findToolsTool struct{ service *Service }

func (t *findToolsTool) Spec() tools.Spec {
	return tools.Spec{
		Name: "find_tools",
		Description: "Find enabled tools by keyword using the local index. Returns up to five tools with their full schemas as a tool result; does not change the tool list.",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"required":["query"],` +
			`"properties":{"query":{"type":"string"}}}`),
		Risk: tools.RiskReadOnly,
	}
}

func (t *findToolsTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(args, &in); err != nil || in.Query == "" {
		return `{"count":0,"tools":[]}`, nil
	}
	return t.service.Find(in.Query)
}

var _ tools.Tool = (*callToolTool)(nil)

type callToolTool struct{ service *Service }

func (t *callToolTool) Spec() tools.Spec {
	return tools.Spec{
		Name: "call_tool",
		Description: "Call any enabled tool by name with its arguments. The target schema is validated in Go; policy and approval apply to the target tool.",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"required":["name","args"],` +
			`"properties":{"name":{"type":"string"},"args":{"type":"object"}}}`),
		Risk: tools.RiskCompute,
	}
}

func (t *callToolTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	call, err := t.service.Resolve(model.ToolCall{Input: args})
	if err != nil {
		return "", err
	}
	result, err := t.service.Call(ctx, call)
	if err != nil {
		return "", err
	}
	return result.Output, nil
}
