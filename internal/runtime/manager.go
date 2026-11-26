// Package runtime owns in-flight run state: the cancellation contexts handed
// to the provider and the local work, plus the live event sink for a run.
// It holds no SQL and no provider wire logic (TAD §18).
package runtime

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/amaldevice/sasmita/internal/domain"
)

// EmitFunc sends an event to the live UI stream for one run. important events
// (terminal and approval) block instead of being dropped under backpressure.
type EmitFunc func(eventType string, payload json.RawMessage, important bool) error

type activeRun struct {
	cancel context.CancelFunc
	emit   EmitFunc
}

// Manager tracks runs in this process. It is the cancellation seam between a
// UI command and the provider context (TAD §14.3).
type Manager struct {
	mu     sync.Mutex
	active map[string]*activeRun
}

func NewManager() *Manager {
	return &Manager{active: make(map[string]*activeRun)}
}

// Register makes a run cancellable. Calling it twice for the same run replaces
// the previous entry; StartRun only calls it once per run ID.
func (m *Manager) Register(runID string, cancel context.CancelFunc, emit EmitFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active[runID] = &activeRun{cancel: cancel, emit: emit}
}

// Cancel stops a run's provider stream and local work.
func (m *Manager) Cancel(runID string) error {
	m.mu.Lock()
	run, ok := m.active[runID]
	m.mu.Unlock()
	if !ok {
		return domain.ErrNotFound
	}
	run.cancel()
	return nil
}

// Emit delivers an event to a live run's UI stream.
func (m *Manager) Emit(runID, eventType string, payload json.RawMessage, important bool) error {
	m.mu.Lock()
	run, ok := m.active[runID]
	m.mu.Unlock()
	if !ok {
		return domain.ErrNotFound
	}
	return run.emit(eventType, payload, important)
}

// Finish removes a run once its terminal event has been persisted and sent.
func (m *Manager) Finish(runID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.active, runID)
}

// Has reports whether the run is still in flight in this process.
func (m *Manager) Has(runID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.active[runID]
	return ok
}
