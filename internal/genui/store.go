package genui

import (
	"encoding/json"
	"sync"
)

// storeSize bounds host-side memory: off-screen blocks are cheap, but a long
// conversation must not retain unlimited validated specs.
const storeSize = 64

// Store keeps validated specs on the host so the full UI payload never returns
// to the model as a tool result (TAD §12A.11).
type Store struct {
	mu    sync.Mutex
	specs map[string]json.RawMessage
	order []string
}

func NewStore() *Store {
	return &Store{specs: make(map[string]json.RawMessage)}
}

func (s *Store) Put(id string, spec json.RawMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.specs[id]; !exists {
		s.order = append(s.order, id)
	}
	s.specs[id] = append(json.RawMessage(nil), spec...)
	for len(s.order) > storeSize {
		oldest := s.order[0]
		s.order = s.order[1:]
		delete(s.specs, oldest)
	}
}

func (s *Store) Get(id string) (json.RawMessage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	spec, ok := s.specs[id]
	if !ok {
		return nil, false
	}
	return append(json.RawMessage(nil), spec...), true
}
