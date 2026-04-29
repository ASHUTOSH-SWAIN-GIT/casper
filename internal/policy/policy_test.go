package policy

import (
	"context"
	"testing"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

func proposal(current, target string) action.RDSResizeProposal {
	return action.RDSResizeProposal{
		DBInstanceIdentifier: "orders-prod",
		Region:               "us-east-1",
		CurrentInstanceClass: current,
		TargetInstanceClass:  target,
		ApplyImmediately:     true,
		SuccessCriteria: action.SuccessCriteria{
			Metric:             "CPUUtilization",
			ThresholdPercent:   60,
			VerificationWindow: "5m",
		},
		Reasoning: "test",
	}
}

func TestEvaluate_AllowsSafeInFamilyOneStepUpsize(t *testing.T) {
	e, err := NewEngine(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		current, target string
	}{
		{"db.r6g.large", "db.r6g.xlarge"},
		{"db.t4g.medium", "db.t4g.large"},
		{"db.m6g.xlarge", "db.m6g.2xlarge"},
		{"db.r6g.4xlarge", "db.r6g.8xlarge"},
	}
	for _, c := range cases {
		v, err := e.EvaluateRDSResize(context.Background(), proposal(c.current, c.target))
		if err != nil {
			t.Errorf("%s -> %s: %v", c.current, c.target, err)
			continue
		}
		if v.Decision != DecisionAllow {
			t.Errorf("%s -> %s: got %q, want allow (reason: %s)", c.current, c.target, v.Decision, v.Reason)
		}
	}
}

func TestEvaluate_NeedsApprovalForCrossFamily(t *testing.T) {
	e, err := NewEngine(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	v, err := e.EvaluateRDSResize(context.Background(), proposal("db.t4g.large", "db.r6g.xlarge"))
	if err != nil {
		t.Fatal(err)
	}
	if v.Decision != DecisionNeedsApproval {
		t.Errorf("got %q, want needs_approval (reason: %s)", v.Decision, v.Reason)
	}
}

func TestEvaluate_NeedsApprovalForBigJump(t *testing.T) {
	e, err := NewEngine(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Two steps up, same family — not allowed by the one-step rule.
	v, err := e.EvaluateRDSResize(context.Background(), proposal("db.r6g.large", "db.r6g.2xlarge"))
	if err != nil {
		t.Fatal(err)
	}
	if v.Decision != DecisionNeedsApproval {
		t.Errorf("got %q, want needs_approval (reason: %s)", v.Decision, v.Reason)
	}
}

func TestEvaluate_NeedsApprovalForUnknownFamily(t *testing.T) {
	e, err := NewEngine(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// db.x2g family is not in the approved set.
	v, err := e.EvaluateRDSResize(context.Background(), proposal("db.x2g.large", "db.x2g.xlarge"))
	if err != nil {
		t.Fatal(err)
	}
	if v.Decision != DecisionNeedsApproval {
		t.Errorf("got %q, want needs_approval (reason: %s)", v.Decision, v.Reason)
	}
}

func TestEvaluate_DeniesNoOp(t *testing.T) {
	e, err := NewEngine(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	v, err := e.EvaluateRDSResize(context.Background(), proposal("db.r6g.large", "db.r6g.large"))
	if err != nil {
		t.Fatal(err)
	}
	if v.Decision != DecisionDeny {
		t.Errorf("got %q, want deny (reason: %s)", v.Decision, v.Reason)
	}
}

func TestEvaluate_NeedsApprovalForDownsize(t *testing.T) {
	e, err := NewEngine(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	v, err := e.EvaluateRDSResize(context.Background(), proposal("db.r6g.xlarge", "db.r6g.large"))
	if err != nil {
		t.Fatal(err)
	}
	if v.Decision != DecisionNeedsApproval {
		t.Errorf("got %q, want needs_approval (reason: %s)", v.Decision, v.Reason)
	}
}
