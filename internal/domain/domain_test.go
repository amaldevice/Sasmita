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
	for _, s := range []domain.RunStatus{
		domain.RunQueued, domain.RunPreparing, domain.RunRunning,
		domain.RunAwaitingApproval, domain.RunAwaitingUser, domain.RunPaused,
		domain.RunCancelling, domain.RunInterrupted,
	} {
		if s.Terminal() {
			t.Errorf("status %q must not be terminal", s)
		}
	}
}

// TAD §8.3: the full state machine is representable, transitions are explicit,
// and terminal/interrupted states have no outgoing edges.
func TestRunTransitions(t *testing.T) {
	legal := []struct{ from, to domain.RunStatus }{
		{domain.RunQueued, domain.RunPreparing},
		{domain.RunQueued, domain.RunCancelled},
		{domain.RunPreparing, domain.RunRunning},
		{domain.RunPreparing, domain.RunFailed},
		{domain.RunRunning, domain.RunAwaitingApproval},
		{domain.RunAwaitingApproval, domain.RunRunning},
		{domain.RunRunning, domain.RunAwaitingUser},
		{domain.RunAwaitingUser, domain.RunRunning},
		{domain.RunRunning, domain.RunPaused},
		{domain.RunPaused, domain.RunRunning},
		{domain.RunRunning, domain.RunCancelling},
		{domain.RunCancelling, domain.RunCancelled},
		{domain.RunRunning, domain.RunCompleted},
		{domain.RunRunning, domain.RunFailed},
		{domain.RunRunning, domain.RunCancelled},
	}
	for _, tc := range legal {
		if !tc.from.CanTransitionTo(tc.to) {
			t.Errorf("%q -> %q must be legal", tc.from, tc.to)
		}
	}
	illegal := []struct{ from, to domain.RunStatus }{
		{domain.RunQueued, domain.RunCompleted},
		{domain.RunQueued, domain.RunRunning},
		{domain.RunPreparing, domain.RunCompleted},
		{domain.RunCompleted, domain.RunRunning},
		{domain.RunFailed, domain.RunCompleted},
		{domain.RunCancelled, domain.RunCompleted},
		{domain.RunInterrupted, domain.RunRunning},
		{domain.RunCancelling, domain.RunCompleted},
		{domain.RunAwaitingApproval, domain.RunCompleted},
	}
	for _, tc := range illegal {
		if tc.from.CanTransitionTo(tc.to) {
			t.Errorf("%q -> %q must be rejected", tc.from, tc.to)
		}
	}
	for _, s := range []domain.RunStatus{
		domain.RunCompleted, domain.RunFailed, domain.RunCancelled, domain.RunInterrupted,
	} {
		if s.Active() {
			t.Errorf("status %q must not be active", s)
		}
	}
	if !domain.RunRunning.Active() || !domain.RunAwaitingApproval.Active() {
		t.Error("running and awaiting_approval must be active states")
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
