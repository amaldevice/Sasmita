package domain

import "time"

// Compaction is the durable record of one prefix reset. Canonical history is
// never rewritten: the omitted and kept message IDs refer to messages that
// remain stored unchanged, and the structured summary is kept separately for
// debugging and recovery (TAD §11.5, GLOSSARY "compaction").
type Compaction struct {
	ID                string    `json:"id"`
	ConversationID    string    `json:"conversationId"`
	RunID             string    `json:"runId"`
	ModelRef          string    `json:"modelRef"`
	CreatedAt         time.Time `json:"createdAt"`
	Summary           string    `json:"summary"`
	KeptMessageIDs    []string  `json:"keptMessageIds"`
	OmittedMessageIDs []string  `json:"omittedMessageIds"`
	EstimatedTokens   int64     `json:"estimatedTokens"`
}

// SummaryMessage returns the active-context message that replaces the omitted
// prefix. It is derived data, never a canonical message.
func (c Compaction) SummaryMessage() Message {
	return Message{
		ID:             c.ID + "-summary",
		ConversationID: c.ConversationID,
		Role:           RoleSystem,
		Content:        TextContent(c.Summary),
		CreatedAt:      c.CreatedAt,
	}
}

// HasKept reports whether the compaction kept the given canonical message ID.
func (c Compaction) HasKept(id string) bool {
	for _, kept := range c.KeptMessageIDs {
		if kept == id {
			return true
		}
	}
	return false
}
