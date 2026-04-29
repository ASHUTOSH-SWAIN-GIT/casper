package audit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// MemoryStore is an in-process Store. Use it for tests, and for the
// CLI when no DATABASE_URL is configured. It is goroutine-safe.
type MemoryStore struct {
	mu     sync.Mutex
	events []Event
	now    func() time.Time
}

// NewMemoryStore returns an empty MemoryStore. now is overridable
// for deterministic tests; pass nil for time.Now.
func NewMemoryStore(now func() time.Time) *MemoryStore {
	if now == nil {
		now = time.Now
	}
	return &MemoryStore{now: now}
}

func (s *MemoryStore) Append(_ context.Context, kind Kind, proposalHash action.ProposalHash, payload map[string]any) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e := Event{
		ID:           int64(len(s.events) + 1),
		ProposalHash: proposalHash,
		Kind:         kind,
		Payload:      payload,
		At:           s.now(),
	}
	prev := ""
	if n := len(s.events); n > 0 {
		prev = s.events[n-1].Hash
	}
	e.PrevHash = prev
	h, err := computeHash(prev, e)
	if err != nil {
		return Event{}, err
	}
	e.Hash = h
	s.events = append(s.events, e)
	return e, nil
}

func (s *MemoryStore) List(_ context.Context, proposalHash action.ProposalHash) ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Event, 0, len(s.events))
	for _, e := range s.events {
		if e.ProposalHash == proposalHash {
			out = append(out, e)
		}
	}
	return out, nil
}

// Verify walks the chain front-to-back and returns an error at the
// first event whose hash does not match its computed value or whose
// PrevHash does not match the prior event's Hash.
func (s *MemoryStore) Verify(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev := ""
	for i, e := range s.events {
		if e.PrevHash != prev {
			return fmt.Errorf("event #%d (%s): prev_hash=%q want %q", e.ID, e.Kind, e.PrevHash, prev)
		}
		want, err := computeHash(prev, e)
		if err != nil {
			return fmt.Errorf("event #%d: hash compute: %w", e.ID, err)
		}
		if e.Hash != want {
			return fmt.Errorf("event #%d (%s): hash=%q want %q (chain broken at index %d)", e.ID, e.Kind, e.Hash, want, i)
		}
		prev = e.Hash
	}
	return nil
}
