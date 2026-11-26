package domain

import "errors"

// RunStatus follows the state machine in TAD §8.3. Interrupted is the
// recovery state added by ticket #21: an active run whose process went away.
type RunStatus string

const (
	RunQueued           RunStatus = "queued"
	RunPreparing        RunStatus = "preparing"
	RunRunning          RunStatus = "running"
	RunAwaitingApproval RunStatus = "awaiting_approval"
	RunAwaitingUser     RunStatus = "awaiting_user"
	RunPaused           RunStatus = "paused"
	RunCancelling       RunStatus = "cancelling"
	RunCompleted        RunStatus = "completed"
	RunFailed           RunStatus = "failed"
	RunCancelled        RunStatus = "cancelled"
	RunInterrupted      RunStatus = "interrupted"
)

var (
	ErrIllegalTransition = errors.New("illegal run transition")

	// legalTransitions is the TAD §8.3 graph. Cancellation edges from waiting
	// states are included so a user can always stop a run; interrupted has no
	// outgoing edge because a continue is a new attempt tied to this run.
	// Startup recovery moves any active state to interrupted. This is not a
	// normal execution transition but a durable record that the process died.
	legalTransitions = map[RunStatus]map[RunStatus]bool{
		RunQueued:           {RunPreparing: true, RunCancelled: true, RunInterrupted: true},
		RunPreparing:        {RunRunning: true, RunFailed: true, RunCancelling: true, RunInterrupted: true},
		RunRunning:          {RunAwaitingApproval: true, RunAwaitingUser: true, RunPaused: true, RunCancelling: true, RunCompleted: true, RunFailed: true, RunCancelled: true, RunInterrupted: true},
		RunAwaitingApproval: {RunRunning: true, RunCancelling: true, RunInterrupted: true},
		RunAwaitingUser:     {RunRunning: true, RunCancelling: true, RunInterrupted: true},
		RunPaused:           {RunRunning: true, RunCancelling: true, RunInterrupted: true},
		RunCancelling:       {RunCancelled: true, RunInterrupted: true},
	}
)

// CanTransitionTo reports whether to is an allowed successor of s.
func (s RunStatus) CanTransitionTo(to RunStatus) bool {
	return legalTransitions[s][to]
}

// Terminal reports whether the status is one of TAD's terminal states.
func (s RunStatus) Terminal() bool {
	switch s {
	case RunCompleted, RunFailed, RunCancelled:
		return true
	default:
		return false
	}
}

// Active reports whether the run can still do work. Interrupted is not active:
// only a new attempt tied to the original run can continue it.
func (s RunStatus) Active() bool {
	switch s {
	case RunQueued, RunPreparing, RunRunning, RunAwaitingApproval, RunAwaitingUser, RunPaused, RunCancelling:
		return true
	default:
		return false
	}
}

// ActiveStatuses is the set of states that startup recovery may interrupt.
func ActiveStatuses() []RunStatus {
	return []RunStatus{RunQueued, RunPreparing, RunRunning, RunAwaitingApproval, RunAwaitingUser, RunPaused, RunCancelling}
}
