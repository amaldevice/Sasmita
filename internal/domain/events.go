package domain

import (
	"encoding/json"
	"time"
)

// RunEventSchemaVersion is carried inside payloads. The top-level RunEvent wire
// shape is fixed by TAD §14.2 and must not gain extra fields.
const RunEventSchemaVersion = 1

// Event types used by the v1 walking skeleton. Further tickets extend this list.
const (
	EventRunQueued            = "run.queued"
	EventRunPreparing         = "run.preparing"
	EventRunStarted           = "run.started"
	EventRunCancelling        = "run.cancelling"
	EventRunCompleted         = "run.completed"
	EventRunFailed            = "run.failed"
	EventRunCancelled         = "run.cancelled"
	EventRunInterrupted       = "run.interrupted"
	EventApprovalRequired     = "tool.approval_required"
	EventModelRequestStarted  = "model.request_started"
	EventModelTextDelta       = "model.text_delta"
	EventModelUsage           = "model.usage"
	EventContextCompacted     = "context.compacted"
	EventContextBudgetWarning = "context.budget_warning"
)

// CompactionPayload carries the structured summary and its kept/omitted message
// IDs. The original summary request is not part of canonical history.
type CompactionPayload struct {
	Envelope
	Compaction
}

// IsImportantEvent reports events that must never be dropped for UI
// backpressure: terminal states and approval transitions (TAD §14.3).
func IsImportantEvent(eventType string) bool {
	switch eventType {
	case EventRunCompleted, EventRunFailed, EventRunCancelled, EventApprovalRequired:
		return true
	default:
		return false
	}
}

// RunEvent is the versioned stream item crossing IPC. JSON tags are verbatim
// from TAD §14.2; the Playwright seam replays these objects unchanged.
type RunEvent struct {
	RunID     string          `json:"runId"`
	Seq       uint64          `json:"seq"`
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

// Envelope is embedded in every event payload so consumers can reject unknown
// schema versions without guessing at payload fields.
type Envelope struct {
	Version int `json:"version"`
}

type StatusPayload struct {
	Envelope
	Status string `json:"status,omitempty"`
}

type TextDeltaPayload struct {
	Envelope
	Text string `json:"text"`
}

type UsagePayload struct {
	Envelope
	Usage `json:"usage"`
}

type ErrorPayload struct {
	Envelope
	Message string `json:"message"`
}

// ApprovalPayload is the inline approval card in the run stream. It carries
// plain-language detail straight from the policy decision; the frontend never
// derives authorization from it.
type ApprovalPayload struct {
	Envelope
	ApprovalID string          `json:"approvalId"`
	CallID     string          `json:"callId,omitempty"`
	ToolID     string          `json:"toolId,omitempty"`
	Status     string          `json:"status,omitempty"`
	Action     string          `json:"action"`
	Resource   string          `json:"resource"`
	Impact     string          `json:"impact"`
	Message    string          `json:"message"`
	Arguments  json.RawMessage `json:"arguments,omitempty"`
	ArgsHash   string          `json:"argsHash,omitempty"`
	PolicyHash string          `json:"policyHash,omitempty"`
}

// RunSnapshot is the reconnect recovery payload (TAD §14.3).
type RunSnapshot struct {
	Run    Run
	Events []RunEvent
	Answer string
}
