// Package model defines the application-owned provider contract (ADR-0002).
// No application service imports a provider SDK.
package model

import (
	"context"
	"encoding/json"
)

// Message is an active-context item. Role and Content are wire-neutral.
// Blocks carries raw provider content blocks in their original byte order and
// key order; adapters replay them verbatim instead of re-serializing them.
type Message struct {
	Role    string            `json:"role"`
	Content string            `json:"content,omitempty"`
	Blocks  []json.RawMessage `json:"blocks,omitempty"`
}

// ToolDefinition is a provider-neutral tool schema. Adapters own the wire
// mapping; no provider SDK type crosses this boundary (ADR-0002).
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

// ToolCall is a provider-neutral tool invocation emitted during a stream.
type ToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input,omitempty"`
}

// StreamRequest is the normalized request passed to any provider adapter.
// PromptCacheKey is the pinned session key for prompt caching. Tools are
// optional and frozen for a conversation (ADR-0001).
type StreamRequest struct {
	Model          string
	Messages       []Message
	Tools          []ToolDefinition
	PromptCacheKey string
	ConversationID string
	RunID          string
}

type StreamEventKind uint8

const (
	EventTextDelta StreamEventKind = iota + 1
	EventUsage
	EventToolUse
	EventContentBlocks
)

// StreamEvent is a normalized provider event. Errors are returned from Stream.
type StreamEvent struct {
	Kind     StreamEventKind
	Text     string
	Usage    *Usage
	ToolCall *ToolCall
	// ContentBlocks is emitted once at the end of a turn with the assistant's
	// raw content blocks, including thinking and signatures.
	ContentBlocks []json.RawMessage
}

// Usage holds provider-reported token counts.
type Usage struct {
	ProviderID       string
	ModelID          string
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	UncachedTokens   int64
	Estimated        bool
}

// ModelProvider is implemented only by adapters in internal/model/adapters.
type ModelProvider interface {
	Name() string
	Stream(ctx context.Context, req StreamRequest, emit func(StreamEvent)) error
}

// ProviderKindAware lets the application service detect which wire format a
// configured adapter speaks. It is optional so test doubles and future
// providers are not forced to declare one; an undeclared kind keeps every
// stored content block (the pre-switch behaviour).
type ProviderKindAware interface {
	ProviderKind() string
}
