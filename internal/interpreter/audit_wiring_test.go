package interpreter

import (
	"context"
	"testing"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/audit"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/plan"
)

func TestRun_WritesAuditChainOnHappyPath(t *testing.T) {
	p := sampleProposal()
	fwd, _ := plan.CompileRDSResize(p, "hash-abc")

	client := &fakeClient{queue: []fakeReply{
		{body: describeResp("db.r6g.large", "available", map[string]any{})},
		{body: map[string]any{}}, // modify
		{body: describeResp("db.r6g.large", "modifying", map[string]any{"DBInstanceClass": "db.r6g.xlarge"})},
		{body: describeResp("db.r6g.xlarge", "available", map[string]any{})},
		{body: describeResp("db.r6g.xlarge", "available", map[string]any{})},
		{body: map[string]any{"Datapoints": map[string]any{"avg": 35.0}}},
	}}

	store := audit.NewMemoryStore(nil)
	i := newTestInterpreter(client)
	i.Audit = store

	if _, err := i.Run(context.Background(), fwd); err != nil {
		t.Fatalf("run: %v", err)
	}
	events, err := store.List(context.Background(), "hash-abc")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// 8 step_started + 8 step_completed + 1 plan_completed = 17.
	if got, want := len(events), 17; got != want {
		t.Errorf("events: got %d want %d", got, want)
	}
	if err := store.Verify(context.Background()); err != nil {
		t.Errorf("verify chain: %v", err)
	}
	if events[len(events)-1].Kind != audit.KindPlanCompleted {
		t.Errorf("last event: got %q want plan_completed", events[len(events)-1].Kind)
	}
}

func TestRun_WritesPlanFailedOnStepFailure(t *testing.T) {
	p := sampleProposal()
	fwd, _ := plan.CompileRDSResize(p, "hash-xyz")

	// describe-pre returns wrong class — preconditions fail, plan aborts.
	client := &fakeClient{queue: []fakeReply{
		{body: describeResp("db.r6g.SOMETHING_ELSE", "available", map[string]any{})},
	}}

	store := audit.NewMemoryStore(nil)
	i := newTestInterpreter(client)
	i.Audit = store

	if _, err := i.Run(context.Background(), fwd); err == nil {
		t.Fatal("expected run to fail")
	}

	events, _ := store.List(context.Background(), "hash-xyz")
	last := events[len(events)-1]
	if last.Kind != audit.KindPlanFailed {
		t.Errorf("last event kind: got %q want plan_failed", last.Kind)
	}
	if err := store.Verify(context.Background()); err != nil {
		t.Errorf("verify chain: %v", err)
	}
}
