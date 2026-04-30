package server

import (
	"sync"
	"time"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/interpreter"
)

// runRecord is the server-side execution record. It mirrors what the
// dashboard's "Run detail" screen needs: status, the two compiled
// plans (forward + rollback), the actual step results from the
// interpreter, and a denormalized link back to the originating
// proposal.
type runRecord struct {
	ID            string                    `json:"id"`
	ProposalID    string                    `json:"proposal_id"`
	ProposalHash  string                    `json:"proposal_hash"`
	ActionType    string                    `json:"action_type"`
	Region        string                    `json:"region"`
	Status        string                    `json:"status"` // pending | running | succeeded | failed | rolled_back
	Error         string                    `json:"error,omitempty"`
	ForwardSteps  []interpreter.StepResult  `json:"forward_steps"`
	RollbackSteps []interpreter.StepResult  `json:"rollback_steps,omitempty"`
	RolledBack    bool                      `json:"rolled_back"`
	StartedAt     time.Time                 `json:"started_at"`
	FinishedAt    *time.Time                `json:"finished_at,omitempty"`
	DurationMs    int64                     `json:"duration_ms"`
}

// runsStore is the in-memory runs index. Like proposalsStore it'll be
// swapped for a Postgres-backed implementation; the interface stays
// narrow so the swap is mechanical.
type runsStore struct {
	mu sync.RWMutex
	m  map[string]*runRecord
}

func newRunsStore() *runsStore {
	return &runsStore{m: make(map[string]*runRecord)}
}

func (s *runsStore) put(r *runRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[r.ID] = r
}

func (s *runsStore) get(id string) (*runRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.m[id]
	return r, ok
}

func (s *runsStore) update(id string, fn func(*runRecord)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.m[id]; ok {
		fn(r)
	}
}

func (s *runsStore) list(status string) []*runRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*runRecord, 0, len(s.m))
	for _, r := range s.m {
		if status != "" && r.Status != status {
			continue
		}
		out = append(out, r)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].StartedAt.After(out[j-1].StartedAt); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
