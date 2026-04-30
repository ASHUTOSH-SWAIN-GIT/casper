package server

import (
	"sync"
	"time"
)

// proposalRecord is the server-side proposal envelope. The wire shape
// is JSON-tagged for direct response use; internal-only fields stay
// untagged.
type proposalRecord struct {
	ID            string         `json:"id"`
	Intent        string         `json:"intent"`
	ActionType    string         `json:"action_type"`
	ResourceType  string         `json:"resource_type"`
	Target        string         `json:"target"`
	Region        string         `json:"region"`
	ProposalHash  string         `json:"proposal_hash"`
	Proposal      map[string]any `json:"proposal"`
	Snapshot      map[string]any `json:"snapshot,omitempty"`
	Policy        proposalPolicy `json:"policy"`
	Router        proposalRouter `json:"router"`
	Proposer      proposalMeta   `json:"proposer"`
	Status        string         `json:"status"` // pending | approved | rejected | auto_allowed | denied
	RejectReason  string         `json:"reject_reason,omitempty"`
	RunID         string         `json:"run_id,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	DecidedAt     *time.Time     `json:"decided_at,omitempty"`

	// ProposalBytes is the canonical proposal JSON used to compute the
	// hash and rebuild a runner.Runnable at execution time. Not exposed
	// on the wire — the decoded Proposal map is enough for clients.
	ProposalBytes []byte `json:"-"`
}

type proposalPolicy struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

type proposalRouter struct {
	Model      string `json:"model"`
	Confidence string `json:"confidence,omitempty"`
	Reasoning  string `json:"reasoning,omitempty"`
}

type proposalMeta struct {
	Model        string  `json:"model"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	DurationMs   int64   `json:"duration_ms"`
	RunID        string  `json:"agent_run_id"` // Starling agent run id (distinct from execution run)
}

// proposalsStore keeps proposal records in memory. Postgres-backed
// implementation will land in a follow-up; the interface is narrow on
// purpose so the swap is mechanical.
type proposalsStore struct {
	mu sync.RWMutex
	m  map[string]*proposalRecord
}

func newProposalsStore() *proposalsStore {
	return &proposalsStore{m: make(map[string]*proposalRecord)}
}

func (s *proposalsStore) put(p *proposalRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[p.ID] = p
}

func (s *proposalsStore) get(id string) (*proposalRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.m[id]
	return p, ok
}

// update applies fn to the proposal record under write lock. No-op if
// the id is unknown.
func (s *proposalsStore) update(id string, fn func(*proposalRecord)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.m[id]; ok {
		fn(p)
	}
}

// list returns proposals sorted newest-first, optionally filtered by
// status. Empty status string means "all".
func (s *proposalsStore) list(status string) []*proposalRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*proposalRecord, 0, len(s.m))
	for _, p := range s.m {
		if status != "" && p.Status != status {
			continue
		}
		out = append(out, p)
	}
	// newest first
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].CreatedAt.After(out[j-1].CreatedAt); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
