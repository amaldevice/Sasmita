// Package domain holds the application-owned vocabulary from GLOSSARY.md.
// It has no dependency on any provider SDK or desktop framework.
package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/amaldevice/sasmita/internal/usage"
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

// NoProjectID is the project id of a conversation that is not in a project.
// Such a conversation has no folder to open (the sentinel project's root_uri
// is empty). See migrations/003_init.sql.
const NoProjectID = ""

// SearchResult is one hit from full-text search over titles and messages.
type SearchResult struct {
	ConversationID string
	Title          string
	ProjectID      string
	Snippet        string
}

// ContentPart keeps message content structured so multimodal references and
// tool-call IDs can be added without changing the wire shape.
type ContentPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	// Raw is the exact provider content block, preserved for byte-identical
	// replay in a later active context. Text mirrors its readable text so
	// Text() and FTS keep working on canonical history.
	Raw json.RawMessage `json:"raw,omitempty"`
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
	// RunID links an assistant message to the run that produced it. It is a
	// read-model aid, not part of canonical message content.
	RunID string
	// Usage is populated on read for answers whose run recorded usage.
	Usage *usage.AnswerUsage
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

// Run is the durable lifecycle record for one execution. OriginalRunID links a
// retry/continue attempt to the run that was interrupted (TAD §8.3).
type Run struct {
	ID             string
	ConversationID string
	Status         RunStatus
	ModelRef       string
	HarnessID      string
	StartedAt      time.Time
	FinishedAt     *time.Time
	OriginalRunID  string
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
