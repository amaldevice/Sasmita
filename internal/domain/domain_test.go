package domain_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/amaldevice/sasmita/internal/domain"
)

// The RunEvent wire shape is fixed by TAD §14.2. The frontend replay seam
// consumes these objects verbatim, so key names and order are part of the contract.
func TestRunEventJSONShape(t *testing.T) {
	ts := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	ev := domain.RunEvent{
		RunID:     "run-1",
		Seq:       7,
		Type:      domain.EventModelTextDelta,
		Timestamp: ts,
		Payload:   json.RawMessage(`{"version":1,"text":"hi"}`),
	}
	got, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"runId":"run-1","seq":7,"type":"model.text_delta","timestamp":"2026-11-26T10:00:00Z","payload":{"version":1,"text":"hi"}}`
	if string(got) != want {
		t.Fatalf("RunEvent JSON mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestRunStatusTerminal(t *testing.T) {
	for _, s := range []domain.RunStatus{domain.RunCompleted, domain.RunFailed, domain.RunCancelled} {
		if !s.Terminal() {
			t.Errorf("status %q must be terminal", s)
		}
	}
	for _, s := range []domain.RunStatus{domain.RunQueued, domain.RunPreparing, domain.RunRunning} {
		if s.Terminal() {
			t.Errorf("status %q must not be terminal", s)
		}
	}
}

func TestMessageText(t *testing.T) {
	m := domain.Message{Content: []domain.ContentPart{
		{Type: "text", Text: "Hello "},
		{Type: "text", Text: "world"},
	}}
	if got := m.Text(); got != "Hello world" {
		t.Fatalf("Message.Text() = %q, want %q", got, "Hello world")
	}
}
