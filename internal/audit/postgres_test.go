package audit

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// openTestStore connects using CASPER_TEST_DATABASE_URL. If unset, the
// test is skipped — Postgres tests run only when a local DB is available.
// The store is wiped (TRUNCATE) before being returned.
func openTestStore(t *testing.T) *PostgresStore {
	t.Helper()
	dsn := os.Getenv("CASPER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set CASPER_TEST_DATABASE_URL to run postgres audit tests (e.g. postgres://casper:casper@localhost:5432/casper?sslmode=disable)")
	}
	ctx := context.Background()
	clock := fixedClock()
	s, err := NewPostgresStore(ctx, dsn, clock)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(s.Close)
	if _, err := s.pool.Exec(ctx, "TRUNCATE audit_events RESTART IDENTITY"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return s
}

func TestPostgres_AppendAssignsIDAndChain(t *testing.T) {
	s := openTestStore(t)
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
		t.Errorf("first event prev_hash should be empty, got %q", e1.PrevHash)
	}
	if e2.PrevHash != e1.Hash {
		t.Errorf("e2.prev_hash %q != e1.hash %q", e2.PrevHash, e1.Hash)
	}
}

func TestPostgres_VerifyOnCleanChain(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	const ph action.ProposalHash = "abc"

	for i := 0; i < 5; i++ {
		if _, err := s.Append(ctx, KindStepStarted, ph, map[string]any{"i": i}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	if err := s.Verify(ctx); err != nil {
		t.Errorf("verify clean chain: %v", err)
	}
}

func TestPostgres_VerifyDetectsTampering(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	const ph action.ProposalHash = "abc"

	for i := 0; i < 3; i++ {
		if _, err := s.Append(ctx, KindStepStarted, ph, map[string]any{"i": i}); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	// Tamper with the middle event's payload via direct SQL.
	if _, err := s.pool.Exec(ctx,
		`UPDATE audit_events SET payload = $1 WHERE id = 2`,
		[]byte(`{"i": 999}`),
	); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	if err := s.Verify(ctx); err == nil {
		t.Fatal("expected verify to detect tampering, got nil")
	}
}

func TestPostgres_ListFiltersByProposal(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	const a, b action.ProposalHash = "AAA", "BBB"

	for i := 0; i < 3; i++ {
		s.Append(ctx, KindStepStarted, a, map[string]any{"i": i})
		s.Append(ctx, KindStepStarted, b, map[string]any{"i": i})
	}
	listA, err := s.List(ctx, a)
	if err != nil {
		t.Fatalf("list A: %v", err)
	}
	listB, err := s.List(ctx, b)
	if err != nil {
		t.Fatalf("list B: %v", err)
	}
	if len(listA) != 3 || len(listB) != 3 {
		t.Errorf("lengths: a=%d b=%d", len(listA), len(listB))
	}
	for _, e := range listA {
		if e.ProposalHash != a {
			t.Errorf("listA contained foreign hash %q", e.ProposalHash)
		}
	}
}

func TestPostgres_TimeRoundTripsAtMicrosecond(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	const ph action.ProposalHash = "abc"

	e, err := s.Append(ctx, KindProposed, ph, map[string]any{})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	list, err := s.List(ctx, ph)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	got := list[0].At
	if !got.Equal(e.At) {
		t.Errorf("time mismatch: append=%v list=%v", e.At, got)
	}
	if got.Nanosecond()%1000 != 0 {
		t.Errorf("expected microsecond precision, got nanos=%d", got.Nanosecond())
	}
	if got.Sub(time.Time{}) == 0 {
		t.Error("zero time")
	}
}
