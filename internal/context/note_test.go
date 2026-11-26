package context_test

import (
	"testing"
	"time"

	ctxlayout "github.com/amaldevice/sasmita/internal/context"
)

// A context note is attached to a user turn and carries only what changed
// since the previous note (GLOSSARY, TAD §11.3). Day precision means an
// unchanged day is never repeated.
func TestContextNoteBuilderEmitsOnlyDeltas(t *testing.T) {
	builder := ctxlayout.NewNoteBuilder()
	day1 := time.Date(2026, 11, 26, 8, 0, 0, 0, time.UTC)
	in := ctxlayout.NoteInput{
		Workspace:   []string{"root=/work"},
		Attachments: []string{"file:///a.png"},
		Memory:      []string{"pref-dark"},
	}

	first := builder.Next(day1, in)
	const wantFirst = "date: 2026-11-26\nworkspace: root=/work\nattachment: file:///a.png\nmemory: pref-dark"
	if first != wantFirst {
		t.Fatalf("first note = %q, want %q", first, wantFirst)
	}
	if again := builder.Next(day1.Add(9*time.Hour), in); again != "" {
		t.Fatalf("unchanged note = %q, want empty", again)
	}

	day2 := day1.AddDate(0, 0, 1)
	changed := ctxlayout.NoteInput{
		Workspace:   []string{"root=/work"},
		Attachments: []string{"file:///a.png", "file:///b.png"},
		Memory:      []string{"pref-dark"},
	}
	const wantSecond = "date: 2026-11-27\nattachment: file:///b.png"
	if got := builder.Next(day2, changed); got != wantSecond {
		t.Fatalf("day-change note = %q, want %q", got, wantSecond)
	}

	changed.Workspace = []string{"root=/other"}
	if got := builder.Next(day2, changed); got != "workspace: root=/other" {
		t.Fatalf("workspace-change note = %q", got)
	}
}
