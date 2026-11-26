// Package model defines the application-owned provider contract (ADR-0002).
// No application service imports a provider SDK.
package model

import "context"

// Message is an active-context item. Role and Content are wire-neutral.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// StreamRequest is the normalized request passed to any provider adapter.
// PromptCacheKey is the pinned session key for prompt caching.
type StreamRequest struct {
	Model          string
	Messages       []Message
	PromptCacheKey string
}

type StreamEventKind uint8

const (
	EventTextDelta StreamEventKind = iota + 1
	EventUsage
)

// StreamEvent is a normalized provider event. Errors are returned from Stream.
type StreamEvent struct {
	Kind  StreamEventKind
	Text  string
	Usage *Usage
}

// Usage holds provider-reported token counts.
type Usage struct {
	ProviderID       string
	ModelID          string
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
}

// ModelProvider is implemented only by adapters in internal/model/adapters.
type ModelProvider interface {
	Name() string
	Stream(ctx context.Context, req StreamRequest, emit func(StreamEvent)) error
}
