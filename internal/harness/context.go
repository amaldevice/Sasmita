package harness

import (
	"encoding/json"

	"github.com/amaldevice/sasmita/internal/domain"
)

// ActiveHistory derives the active-context view for one run from canonical
// history. Canonical messages are never mutated: a Mode contributes a static
// instruction message, and blocks belonging to a different provider are left
// out so they cannot corrupt the new provider's byte-stable wire request
// (TAD §9.3, §10.1; GLOSSARY "active context").
//
// targetProvider is a domain.ProviderKind; an empty value preserves every raw
// block for adapters that do not declare their wire format.
func ActiveHistory(history []domain.Message, mode Mode, targetProvider domain.ProviderKind) []domain.Message {
	out := make([]domain.Message, 0, len(history)+1)
	if mode.Instructions != "" {
		out = append(out, domain.Message{
			Role: domain.RoleSystem, Content: domain.TextContent(mode.Instructions),
		})
	}
	for _, m := range history {
		view := m
		view.Content = make([]domain.ContentPart, 0, len(m.Content))
		for _, part := range m.Content {
			if len(part.Raw) > 0 && isForeignBlock(part.Raw, targetProvider) {
				continue
			}
			part.Raw = append(json.RawMessage(nil), part.Raw...)
			view.Content = append(view.Content, part)
		}
		if len(view.Content) == 0 {
			view.Content = domain.TextContent("")
		}
		out = append(out, view)
	}
	return out
}

// isForeignBlock detects blocks the target provider cannot replay. Anthropic
// thinking/signature/tool_use blocks are not valid on an OpenAI-compatible
// wire request; they are never rewritten into text (which would leak reasoning).
func isForeignBlock(raw json.RawMessage, target domain.ProviderKind) bool {
	if target == "" {
		return false
	}
	var probe struct {
		Type      string `json:"type"`
		Signature string `json:"signature"`
	}
	if json.Unmarshal(raw, &probe) != nil {
		return false
	}
	if target == domain.ProviderOpenAICompatible {
		switch probe.Type {
		case "thinking", "redacted_thinking", "tool_use":
			return true
		}
		return probe.Signature != ""
	}
	return false
}
