// Package context builds the active context from canonical history. It owns no
// provider wire logic: adapters serialize the resulting StreamRequest (ADR-0002).
package context

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/harness"
	"github.com/amaldevice/sasmita/internal/model"
)

// StaticSystemPrompt contains no per-request state: no date, cwd, git state or
// workspace details. Dynamic information is a tail context note on a user turn
// (ADR-0001, GLOSSARY "context note").
const StaticSystemPrompt = "You are Sasmita. Answer clearly and concisely. " + harness.VisualFirstInstructions

// SessionKey pins provider cache routing to a stable hash of the conversation
// ID rather than a per-run or per-step value (research/prompt-caching.md #7).
func SessionKey(conversationID string) string {
	sum := sha256.Sum256([]byte(conversationID))
	return hex.EncodeToString(sum[:16])
}

// Builder freezes the tool list for a conversation. Tools are copied once at
// construction, so later tool discovery cannot change serialized request bytes.
type Builder struct {
	Tools []model.ToolDefinition
}

// Build returns the append-only request layout
// [frozen tools][static system][byte-identical history][new user turn].
// Stored raw content blocks are copied verbatim so provider-specific shapes
// (thinking, signatures, tool_use) survive reload unchanged.
func (b Builder) Build(conversationID, modelRef, runID string, history []domain.Message) model.StreamRequest {
	messages := []model.Message{{Role: "system", Content: StaticSystemPrompt}}
	for _, m := range history {
		out := model.Message{Role: string(m.Role)}
		rawParts := 0
		for _, p := range m.Content {
			if len(p.Raw) > 0 {
				rawParts++
			}
		}
		if rawParts == 0 && len(m.Content) == 1 {
			out.Content = m.Content[0].Text
		} else {
			for _, p := range m.Content {
				if len(p.Raw) > 0 {
					out.Blocks = append(out.Blocks, append(json.RawMessage(nil), p.Raw...))
					continue
				}
				block, _ := model.MarshalDeterministic(struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}{Type: "text", Text: p.Text})
				out.Blocks = append(out.Blocks, block)
			}
		}
		messages = append(messages, out)
	}
	return model.StreamRequest{
		Model:          modelRef,
		Messages:       messages,
		Tools:          append([]model.ToolDefinition(nil), b.Tools...),
		PromptCacheKey: SessionKey(conversationID),
		ConversationID: conversationID,
		RunID:          runID,
	}
}
