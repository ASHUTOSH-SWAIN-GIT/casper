package audit

import (
	"context"
	"testing"
	"time"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

func fixedClock() func() time.Time {
	t := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	return func() time.Time {
		t = t.Add(time.Second)
		return t
	}
}

func TestMemoryStore_AppendAssignsIDAndChain(t *testing.T) {
	s := NewMemoryStore(fixedClock())
	ctx := context.Background()
	const ph action.ProposalHash = "abc"

	e1, err := s.Append(ctx, KindProposed, ph, map[string]any{"who": "user-1"})
	if err != nil {
		t.Fatalf("append #1: %v", err)
	}
	e2, err := s.Append(ctx, KindPlanCompiled, ph, map[string]any{"steps": 8})
	if err != nil {
		t.Fatalf("append #2: %v", err)
	}

	if e1.ID != 1 || e2.ID != 2 {
		t.Errorf("ids: e1=%d e2=%d", e1.ID, e2.ID)
	}
	if e1.PrevHash != "" {
		t.Errorf("first event must have empty prev_hash, got %q", e1.PrevHash)
	}
	if e2.PrevHash != e1.Hash {
		t.Errorf("e2.prev_hash %q should equal e1.hash %q", e2.PrevHash, e1.Hash)
	}
	if e1.Hash == "" || e2.Hash == "" {
		t.Errorf("hashes empty: e1=%q e2=%q", e1.Hash, e2.Hash)
	}
	if e1.Hash == e2.Hash {
		t.Error("different events should have different hashes")
	}
}

func TestMemoryStore_VerifyDetectsTampering(t *testing.T) {
	s := NewMemoryStore(fixedClock())
	ctx := context.Background()
	const ph action.ProposalHash = "abc"

	if _, err := s.Append(ctx, KindProposed, ph, map[string]any{"who": "user-1"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if _, err := s.Append(ctx, KindPlanCompiled, ph, map[string]any{"steps": 8}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if _, err := s.Append(ctx, KindStepStarted, ph, map[string]any{"step_id": "modify"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := s.Verify(ctx); err != nil {
		t.Fatalf("clean chain failed verify: %v", err)
	}

	// Tamper with event #2's payload — chain must break at #2.
	s.events[1].Payload = map[string]any{"steps": 999}
	err := s.Verify(ctx)
	if err == nil {
		t.Fatal("expected verify to detect tampering, got nil")
	}
}

func TestMemoryStore_ListFiltersByProposal(t *testing.T) {
	s := NewMemoryStore(fixedClock())
	ctx := context.Background()
	const a, b action.ProposalHash = "AAA", "BBB"

	for i := 0; i < 3; i++ {
		s.Append(ctx, KindStepStarted, a, map[string]any{"i": i})
		s.Append(ctx, KindStepStarted, b, map[string]any{"i": i})
	}

	listA, _ := s.List(ctx, a)
	listB, _ := s.List(ctx, b)
	if len(listA) != 3 || len(listB) != 3 {
		t.Errorf("list lengths: a=%d b=%d", len(listA), len(listB))
	}
	for _, e := range listA {
		if e.ProposalHash != a {
			t.Errorf("listA contained foreign hash %q", e.ProposalHash)
		}
	}
}

func TestMemoryStore_EmptyVerify(t *testing.T) {
	s := NewMemoryStore(nil)
	if err := s.Verify(context.Background()); err != nil {
		t.Fatalf("empty store should verify, got: %v", err)
	}
}

func TestComputeHash_DeterministicAcrossKeyOrder(t *testing.T) {
	at := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	e1 := Event{
		Kind:         KindStepStarted,
		ProposalHash: "abc",
		Payload:      map[string]any{"a": 1, "b": 2},
		At:           at,
	}
	e2 := Event{
		Kind:         KindStepStarted,
		ProposalHash: "abc",
		Payload:      map[string]any{"b": 2, "a": 1},
		At:           at,
	}
	h1, err := computeHash("", e1)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := computeHash("", e2)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Errorf("key order changed hash: %q vs %q", h1, h2)
	}
}
