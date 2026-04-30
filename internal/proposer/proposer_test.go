package proposer

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// These tests exercise the parts of the proposer that don't require a
// live Anthropic API call: schema validation in the tool, request
// marshaling, system prompt invariants, and goal assembly. The
// end-to-end LLM run is exercised via a separate integration test
// gated on ANTHROPIC_API_KEY being set.

func TestBuildGoal_IncludesIntentAndSnapshot(t *testing.T) {
	req := Request{
		Intent: "orders-prod is at 90% CPU sustained",
		Snapshot: Snapshot{
			DBInstanceIdentifier: "orders-prod",
			Region:               "us-east-1",
			CurrentInstanceClass: "db.r6g.large",
			Engine:               "postgres",
			Status:               "available",
		},
	}
	goal, err := buildGoal(req)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"orders-prod is at 90% CPU sustained",
		"db.r6g.large",
		"the proposal tool",
		"Do not write anything outside the tool call",
	} {
		if !strings.Contains(goal, want) {
			t.Errorf("goal missing %q", want)
		}
	}
}

func TestProposeTool_AcceptsValidInput(t *testing.T) {
	cap := &captured{}
	tl := buildRDSResizeProposeTool(cap)

	in := rdsResizeProposeInput{
		DBInstanceIdentifier: "orders-prod",
		Region:               "us-east-1",
		CurrentInstanceClass: "db.r6g.large",
		TargetInstanceClass:  "db.r6g.xlarge",
		ApplyImmediately:     true,
		SuccessCriteria: rdsResizeProposeSuccess{
			Metric:             "CPUUtilization",
			ThresholdPercent:   60,
			VerificationWindow: "5m",
		},
		Reasoning: "CPU at 90% sustained — one-step upsize doubles compute headroom",
	}
	raw, _ := json.Marshal(in)
	out, err := tl.Execute(context.Background(), raw)
	if err != nil {
		t.Fatalf("tool: %v", err)
	}
	var resp proposeOutput
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Hash) != 64 {
		t.Errorf("hash length: got %d want 64", len(resp.Hash))
	}

	rawCap, hash := cap.get()
	if len(rawCap) == 0 {
		t.Error("captured.raw is empty after successful tool call")
	}
	if string(hash) != resp.Hash {
		t.Errorf("captured hash != returned hash: %q vs %q", hash, resp.Hash)
	}
	// Captured bytes must pass the authoritative validator.
	if err := action.Validate(rawCap); err != nil {
		t.Errorf("captured bytes failed action.Validate: %v", err)
	}
}

func TestProposeTool_RejectsApplyImmediatelyFalse(t *testing.T) {
	cap := &captured{}
	tl := buildRDSResizeProposeTool(cap)
	in := rdsResizeProposeInput{
		DBInstanceIdentifier: "orders-prod",
		Region:               "us-east-1",
		CurrentInstanceClass: "db.r6g.large",
		TargetInstanceClass:  "db.r6g.xlarge",
		ApplyImmediately:     false, // schema requires true
		SuccessCriteria: rdsResizeProposeSuccess{
			Metric:             "CPUUtilization",
			ThresholdPercent:   60,
			VerificationWindow: "5m",
		},
		Reasoning: "test",
	}
	raw, _ := json.Marshal(in)
	if _, err := tl.Execute(context.Background(), raw); err == nil {
		t.Fatal("expected tool to reject apply_immediately=false")
	}
}

func TestProposeTool_RejectsThresholdOutOfRange(t *testing.T) {
	cap := &captured{}
	tl := buildRDSResizeProposeTool(cap)
	in := rdsResizeProposeInput{
		DBInstanceIdentifier: "orders-prod",
		Region:               "us-east-1",
		CurrentInstanceClass: "db.r6g.large",
		TargetInstanceClass:  "db.r6g.xlarge",
		ApplyImmediately:     true,
		SuccessCriteria: rdsResizeProposeSuccess{
			Metric:             "CPUUtilization",
			ThresholdPercent:   200,
			VerificationWindow: "5m",
		},
		Reasoning: "test",
	}
	raw, _ := json.Marshal(in)
	if _, err := tl.Execute(context.Background(), raw); err == nil {
		t.Fatal("expected tool to reject threshold > 100")
	}
}

func TestProposeTool_RejectsEmptyReasoning(t *testing.T) {
	cap := &captured{}
	tl := buildRDSResizeProposeTool(cap)
	in := rdsResizeProposeInput{
		DBInstanceIdentifier: "orders-prod",
		Region:               "us-east-1",
		CurrentInstanceClass: "db.r6g.large",
		TargetInstanceClass:  "db.r6g.xlarge",
		ApplyImmediately:     true,
		SuccessCriteria: rdsResizeProposeSuccess{
			Metric:             "CPUUtilization",
			ThresholdPercent:   60,
			VerificationWindow: "5m",
		},
		Reasoning: "",
	}
	raw, _ := json.Marshal(in)
	if _, err := tl.Execute(context.Background(), raw); err == nil {
		t.Fatal("expected tool to reject empty reasoning")
	}
}

func TestSystemPrompt_InvariantsPresent(t *testing.T) {
	for _, want := range []string{
		"propose_rds_resize exactly ONCE",
		"apply_immediately\" must be true",
		"CPUUtilization",
	} {
		if !strings.Contains(rdsResizeSystemPrompt, want) {
			t.Errorf("system prompt missing constraint: %q", want)
		}
	}
}

func TestRequest_RoundTripsJSON(t *testing.T) {
	req := Request{
		Intent: "scale up",
		Snapshot: Snapshot{
			DBInstanceIdentifier: "x",
			Region:               "us-east-1",
			CurrentInstanceClass: "db.t4g.medium",
			Engine:               "mysql",
			Status:               "available",
			MultiAZ:              true,
			RecentCPUUtilization: 88.5,
		},
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var got Request
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got != req {
		t.Errorf("round-trip mismatch:\n got %+v\nwant %+v", got, req)
	}
}
