package context

import (
	"sort"
	"sync"
	"time"
)

// NoteInput is the dynamic information observed for the next user turn. It is
// a snapshot, not a diff: the builder decides what changed since the last note
// (GLOSSARY "context note", TAD §11.3).
type NoteInput struct {
	Workspace   []string
	Attachments []string
	Memory      []string
}

// NoteBuilder emits context-note deltas. The date has day precision and is
// emitted only when the day changes; workspace, attachment and memory entries
// are emitted only when they were not present in the previous snapshot. Once
// sent the note becomes canonical history and is never rewritten.
type NoteBuilder struct {
	mu           sync.Mutex
	previousDate string
	previous     NoteInput
	seenLegacy   map[string]bool
	hasPrevious  bool
}

func NewNoteBuilder() *NoteBuilder { return &NoteBuilder{} }

// Next returns the delta for this turn, or "" when nothing changed.
func (b *NoteBuilder) Next(now time.Time, in NoteInput) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	lines := b.deltas(now, in.Workspace, in.Attachments, in.Memory)
	b.previousDate, b.previous, b.hasPrevious = now.Format("2006-01-02"), cloneNote(in), true
	return join(lines)
}

// Observe folds a plain dynamic string into the note as a delta. It keeps older
// callers (which supply a single string) on the same delta-only path.
func (b *NoteBuilder) Observe(now time.Time, note string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if note == "" {
		return ""
	}
	if b.seenLegacy[note] {
		return ""
	}
	if b.seenLegacy == nil {
		b.seenLegacy = make(map[string]bool)
	}
	b.seenLegacy[note] = true
	b.hasPrevious = true
	return note
}

// deltas is called with the mutex held.
func (b *NoteBuilder) deltas(now time.Time, workspace, attachments, memory []string) []string {
	date := now.Format("2006-01-02")
	var lines []string
	if !b.hasPrevious || b.previousDate != date {
		lines = append(lines, "date: "+date)
	}
	lines = append(lines, changed("workspace", b.previous.Workspace, workspace)...)
	lines = append(lines, changed("attachment", b.previous.Attachments, attachments)...)
	lines = append(lines, changed("memory", b.previous.Memory, memory)...)
	return lines
}

func join(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	out := lines[0]
	for _, line := range lines[1:] {
		out += "\n" + line
	}
	return out
}

func changed(label string, before, after []string) []string {
	seen := make(map[string]bool, len(before))
	for _, item := range before {
		seen[item] = true
	}
	var fresh []string
	for _, item := range after {
		if item == "" || seen[item] {
			continue
		}
		fresh = append(fresh, label+": "+item)
	}
	sort.Strings(fresh)
	return fresh
}

func cloneNote(in NoteInput) NoteInput {
	return NoteInput{
		Workspace:   append([]string(nil), in.Workspace...),
		Attachments: append([]string(nil), in.Attachments...),
		Memory:      append([]string(nil), in.Memory...),
	}
}
