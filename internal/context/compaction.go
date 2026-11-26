package context

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/amaldevice/sasmita/internal/domain"
	"github.com/amaldevice/sasmita/internal/model"
)

// SummaryRequestPrompt is appended to the cached prefix so the summarizing
// call itself hits the cache. It asks for a fixed set of keys so the resulting
// active context is structurally deterministic even when the model is not.
const SummaryRequestPrompt = "TASK: compact-summary-request. Summarize the conversation for continuation. Reply as JSON with keys goal, constraints, decisions, facts, files, tools, plan, openQuestions (arrays of short strings)."

var ErrNothingToCompact = errors.New("nothing to compact")

// DefaultCompactionThreshold matches TAD §19 (0.75 of the input budget).
const DefaultCompactionThreshold = 0.75

// Compactor decides when to replace old active context with a structured
// summary. It owns no wire logic: the provider call reuses Builder and the
// same model as the conversation (ADR-0001, ADR-0002).
type Compactor struct {
	Provider model.ModelProvider
	Builder  Builder
	// SystemMessages are the Mode instructions that precede canonical history
	// in the cached request. They are included only in the summary request, not
	// in the kept tail or the compaction record.
	SystemMessages    []domain.Message
	InputBudgetTokens int
	Threshold         float64
	KeepUserTurns     int
	NewID             func() string
	Now               func() time.Time
}

// Summary is the structured result of compaction. Every section is rendered in
// a fixed order, including empty ones, so the new prefix is byte-stable.
type Summary struct {
	Goal          []string `json:"goal"`
	Constraints   []string `json:"constraints"`
	Decisions     []string `json:"decisions"`
	Facts         []string `json:"facts"`
	Files         []string `json:"files"`
	Tools         []string `json:"tools"`
	Plan          []string `json:"plan"`
	OpenQuestions []string `json:"openQuestions"`
	Notes         []string `json:"notes,omitempty"`
}

// ParseSummary reads the fixed keys from provider text. Unstructured text is
// retained verbatim in the notes section rather than silently discarded.
func ParseSummary(text string) Summary {
	var out Summary
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &out); err != nil {
		out = Summary{}
	}
	if strings.TrimSpace(text) != "" && allEmpty(out) {
		out.Notes = []string{strings.TrimSpace(text)}
	}
	return out
}

func allEmpty(s Summary) bool {
	for _, group := range [][]string{s.Goal, s.Constraints, s.Decisions, s.Facts, s.Files, s.Tools, s.Plan, s.OpenQuestions, s.Notes} {
		if len(group) > 0 {
			return false
		}
	}
	return true
}

// Render produces the structured summary message placed after the system
// instruction and before the kept user turns.
func (s Summary) Render() string {
	sections := []struct {
		name  string
		items []string
	}{
		{"goal", s.Goal}, {"constraints", s.Constraints}, {"decisions", s.Decisions},
		{"facts", s.Facts}, {"files", s.Files}, {"tools", s.Tools},
		{"plan", s.Plan}, {"openQuestions", s.OpenQuestions}, {"notes", s.Notes},
	}
	var b strings.Builder
	b.WriteString("compaction summary")
	for _, section := range sections {
		b.WriteString("\n" + section.name + ":")
		if len(section.items) == 0 {
			b.WriteString("\n- (none)")
			continue
		}
		for _, item := range section.items {
			b.WriteString("\n- " + item)
		}
	}
	return b.String()
}

// Compact asks the provider for a structured summary through the cached prefix
// with the same model and frozen tools, then returns the new active context.
// Canonical history is untouched; only the derived view changes.
func (c Compactor) Compact(ctx context.Context, conversationID, modelRef, runID string, history []domain.Message) (domain.Compaction, []domain.Message, error) {
	var zero domain.Compaction
	if c.Provider == nil {
		return zero, nil, errors.New("compaction requires a provider")
	}
	omitted, kept := Split(history, c.KeepUserTurns)
	if len(omitted) == 0 || len(kept) == 0 {
		return zero, nil, ErrNothingToCompact
	}
	requestHistory := append(append(append([]domain.Message(nil), c.SystemMessages...), history...), domain.Message{
		Role: domain.RoleUser, Content: domain.TextContent(SummaryRequestPrompt),
	})
	request := c.Builder.Build(conversationID, modelRef, runID, requestHistory)
	var text strings.Builder
	if err := c.Provider.Stream(ctx, request, func(ev model.StreamEvent) {
		if ev.Kind == model.EventTextDelta {
			text.WriteString(ev.Text)
		}
	}); err != nil {
		return zero, nil, err
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	newID := randomCompactionID
	if c.NewID != nil {
		newID = c.NewID
	}
	record := domain.Compaction{
		ID:                newID(),
		ConversationID:    conversationID,
		RunID:             runID,
		ModelRef:          modelRef,
		CreatedAt:         now(),
		Summary:           ParseSummary(text.String()).Render(),
		KeptMessageIDs:    ids(kept),
		OmittedMessageIDs: ids(omitted),
		EstimatedTokens:   c.EstimateTokens(history),
	}
	active := append([]domain.Message{record.SummaryMessage()}, kept...)
	return record, active, nil
}

// Apply rebuilds the compacted active context from canonical history and the
// durable record. Later messages (appended after compaction) stay in place.
func Apply(record domain.Compaction, history []domain.Message) []domain.Message {
	if record.ID == "" {
		return history
	}
	start := -1
	for i, m := range history {
		if record.HasKept(m.ID) {
			start = i
			break
		}
	}
	if start < 0 {
		return history
	}
	out := append([]domain.Message{record.SummaryMessage()}, history[start:]...)
	return out
}

func randomCompactionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return "cmp-" + hex.EncodeToString(b[:])
}

func ids(messages []domain.Message) []string {
	out := make([]string, 0, len(messages))
	for _, m := range messages {
		out = append(out, m.ID)
	}
	return out
}

// EstimateTokens is a conservative character-based preflight estimate; no
// tokenizer is assumed for every model (TAD §11.8). It is never reported as
// provider usage.
func (c Compactor) EstimateTokens(history []domain.Message) int64 {
	var chars int
	for _, m := range history {
		chars += 16 // message envelope overhead
		for _, part := range m.Content {
			chars += len(part.Text) + len(part.Raw)
		}
	}
	return int64((chars + 3) / 4)
}

// ShouldCompact reports whether history reached the configured fraction of the
// input budget. A zero budget disables automatic compaction.
func (c Compactor) ShouldCompact(history []domain.Message) bool {
	if c.InputBudgetTokens <= 0 {
		return false
	}
	threshold := c.Threshold
	if threshold <= 0 {
		threshold = DefaultCompactionThreshold
	}
	return float64(c.EstimateTokens(history)) >= threshold*float64(c.InputBudgetTokens)
}

// Split divides canonical history at a safe compaction boundary. The kept tail
// contains the last keepUserTurns user turns, their assistant replies, and any
// tool-call pair that would otherwise be cut in half. It never rewrites a
// message: both slices are copies in canonical order (TAD §11.5, ADR-0001).
func Split(history []domain.Message, keepUserTurns int) (omitted, kept []domain.Message) {
	if keepUserTurns <= 0 {
		keepUserTurns = 2
	}
	start := -1
	seen := 0
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role != domain.RoleUser {
			continue
		}
		seen++
		if seen == keepUserTurns {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, append([]domain.Message(nil), history...)
	}
	// A user tool_result must stay with the assistant tool_use it answers.
	if start > 0 && history[start-1].Role == domain.RoleAssistant && hasToolUse(history[start-1]) {
		start--
	}
	kept = append([]domain.Message(nil), history[start:]...)
	if start > 0 {
		omitted = append([]domain.Message(nil), history[:start]...)
	}
	return omitted, kept
}

func hasToolUse(m domain.Message) bool {
	for _, part := range m.Content {
		if part.Type == "tool_use" {
			return true
		}
		if len(part.Raw) == 0 {
			continue
		}
		var probe struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(part.Raw, &probe) == nil && probe.Type == "tool_use" {
			return true
		}
	}
	return false
}
