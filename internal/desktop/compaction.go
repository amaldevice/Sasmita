package desktop

import (
	"context"
	"time"
)

// CompactionDTO is the typed IPC projection of a context compaction. It carries
// the summary and the IDs it kept/omitted for debugging and recovery; canonical
// history is never rewritten (TAD §11.5, GLOSSARY "compaction").
type CompactionDTO struct {
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

// CompactConversation is the user-requested compaction command.
func (s *Service) CompactConversation(ctx context.Context, conversationID string) (CompactionDTO, error) {
	record, err := s.svc.CompactConversation(ctx, conversationID)
	if err != nil {
		return CompactionDTO{}, err
	}
	return CompactionDTO{
		ID:                record.ID,
		ConversationID:    record.ConversationID,
		RunID:             record.RunID,
		ModelRef:          record.ModelRef,
		CreatedAt:         record.CreatedAt,
		Summary:           record.Summary,
		KeptMessageIDs:    append([]string(nil), record.KeptMessageIDs...),
		OmittedMessageIDs: append([]string(nil), record.OmittedMessageIDs...),
		EstimatedTokens:   record.EstimatedTokens,
	}, nil
}
