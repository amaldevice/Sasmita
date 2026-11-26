// Package domain holds the application-owned vocabulary from GLOSSARY.md.
// It has no dependency on any provider SDK or desktop framework.
package domain

import (
	"errors"
	"strings"
	"time"
)

// Project is the workspace root that conversations belong to.
type Project struct {
	ID        string
	Name      string
	RootURI   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Conversation owns the canonical history: an append-only sequence of messages
// and the runs started from it.
type Conversation struct {
	ID              string
	ProjectID       string
	Title           string
	ActiveHarnessID string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ConversationDetail is the reload payload used by GetConversation after a restart.
type ConversationDetail struct {
	Conversation Conversation
	Messages     []Message
}

// ContentPart keeps message content structured so multimodal references and
// tool-call IDs can be added without changing the wire shape.
type ContentPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// TextContent builds the only content shape needed by the v1 walking skeleton.
func TextContent(text string) []ContentPart {
	return []ContentPart{{Type: "text", Text: text}}
}

// Message is one entry in the canonical history.
type Message struct {
	ID             string
	ConversationID string
	Role           MessageRole
	Content        []ContentPart
	CreatedAt      time.Time
}

// Text returns the concatenation of all text parts.
func (m Message) Text() string {
	parts := make([]string, 0, len(m.Content))
	for _, p := range m.Content {
		if p.Type == "text" {
			parts = append(parts, p.Text)
		}
	}
	return strings.Join(parts, "")
}

type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
)

// RunStatus follows the state machine in TAD §8.3.
type RunStatus string

const (
	RunQueued    RunStatus = "queued"
	RunPreparing RunStatus = "preparing"
	RunRunning   RunStatus = "running"
	RunCompleted RunStatus = "completed"
	RunFailed    RunStatus = "failed"
	RunCancelled RunStatus = "cancelled"
)

// Terminal reports whether the status is a terminal state.
func (s RunStatus) Terminal() bool {
	switch s {
	case RunCompleted, RunFailed, RunCancelled:
		return true
	default:
		return false
	}
}

// Run is the durable lifecycle record for one execution.
type Run struct {
	ID             string
	ConversationID string
	Status         RunStatus
	ModelRef       string
	HarnessID      string
	StartedAt      time.Time
	FinishedAt     *time.Time
}

// Usage is a provider-reported (or explicitly estimated) token measurement.
type Usage struct {
	ProviderID       string
	ModelID          string
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	UncachedTokens   int64
	Estimated        bool
	CostEstimate     string
}

// StartRunRequest is the typed IPC command payload for StartRun.
type StartRunRequest struct {
	ConversationID string `json:"conversationId"`
	Text           string `json:"text"`
	ModelRef       string `json:"modelRef"`
	HarnessID      string `json:"harnessId"`
}

var (
	ErrNotFound        = errors.New("not found")
	ErrInvalidArgument = errors.New("invalid argument")
	ErrTerminalRun     = errors.New("run is already in a terminal state")
)
