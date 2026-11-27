package domain

import (
	"encoding/json"
	"errors"
	"time"
)

// Approval is a user decision about one exact tool call. It is bound to the
// arguments hash and policy snapshot, so a changed request cannot reuse it
// (TAD §15.8). Approval is not a general permission to run a tool.
type Approval struct {
	ID             string
	RunID          string // stable checkpoint ID, also across a retry attempt
	ConversationID string
	CallID         string
	ToolID         string
	Mode           string
	ProjectID      string
	Status         string
	Decision       string
	Arguments      json.RawMessage
	ArgsHash       string
	PolicyHash     string
	Action         string
	Resource       string
	Impact         string
	Message        string
	CreatedAt      time.Time
	ResolvedAt     *time.Time
}

const (
	ApprovalPending    = "pending"
	ApprovalResolved   = "resolved"
	ApprovalSuperseded = "superseded"

	ApprovalAllowOnce = "allow_once"
	ApprovalAlways    = "always"
	ApprovalDeny      = "deny"
)

// PolicyGrant is the project-local result of "always allow in this project".
// A different policy snapshot invalidates it.
type PolicyGrant struct {
	ID           string
	ProjectID    string
	ToolID       string
	ResourceHash string
	PolicyHash   string
	CreatedAt    time.Time
}

var (
	ErrApprovalNotFound = errors.New("approval not found")
	ErrApprovalResolved = errors.New("approval already resolved")
	ErrApprovalStale    = errors.New("approval no longer matches the request")
	ErrInvalidDecision  = errors.New("invalid approval decision")
)
