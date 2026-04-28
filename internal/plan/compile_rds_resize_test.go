package plan

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

func sampleProposal() action.RDSResizeProposal {
	return action.RDSResizeProposal{
		DBInstanceIdentifier: "orders-prod",
		Region:               "us-east-1",
		CurrentInstanceClass: "db.r6g.large",
		TargetInstanceClass:  "db.r6g.xlarge",
		ApplyImmediately:     true,
		SuccessCriteria: action.SuccessCriteria{
			Metric:             "CPUUtilization",
			ThresholdPercent:   60,
			VerificationWindow: "5m",
		},
		Reasoning: "CPU sustained at 90% over the last 30 minutes",
	}
}

const sampleHash action.ProposalHash = "deadbeef"

func TestCompile_ForwardHasEightStepsInOrder(t *testing.T) {
	fwd, _ := CompileRDSResize(sampleProposal(), sampleHash)

	if got, want := len(fwd.Steps), 8; got != want {
		t.Fatalf("forward step count: got %d want %d", got, want)
	}

	wantKinds := []StepKind{
		StepAWSAPICall, // describe-pre
		StepDecision,   // preconditions
		StepAWSAPICall, // modify
		StepPoll,       // poll-modifying
		StepPoll,       // poll-available
		StepVerify,     // verify-class
		StepWait,       // wait-verification-window
		StepVerify,     // verify-metric
	}
	for i, want := range wantKinds {
		if got := fwd.Steps[i].Kind; got != want {
			t.Errorf("step %d (%s): kind got %q want %q", i, fwd.Steps[i].ID, got, want)
		}
	}
}

func TestCompile_ForwardModifyUsesTargetClass(t *testing.T) {
	p := sampleProposal()
	fwd, _ := CompileRDSResize(p, sampleHash)

	modify := fwd.Steps[2]
	if modify.ID != "modify" {
		t.Fatalf("expected step 2 to be modify, got %q", modify.ID)
	}
	if modify.APICall == nil {
		t.Fatal("modify step has no APICall")
	}
	if got, want := modify.APICall.Operation, "ModifyDBInstance"; got != want {
		t.Errorf("operation: got %q want %q", got, want)
	}
	if got, want := modify.APICall.Params["DBInstanceClass"], p.TargetInstanceClass; got != want {
		t.Errorf("DBInstanceClass: got %v want %v", got, want)
	}
	if got := modify.APICall.Params["ApplyImmediately"]; got != true {
		t.Errorf("ApplyImmediately: got %v want true", got)
	}
	if got, want := modify.OnFailure, OnFailureRollback; got != want {
		t.Errorf("on_failure: got %q want %q", got, want)
	}
}

func TestCompile_RollbackHasFourStepsAndUsesCurrentClass(t *testing.T) {
	p := sampleProposal()
	_, rb := CompileRDSResize(p, sampleHash)

	if got, want := len(rb.Steps), 4; got != want {
		t.Fatalf("rollback step count: got %d want %d", got, want)
	}
	if rb.Kind != PlanRollback {
		t.Errorf("rollback kind: got %q want %q", rb.Kind, PlanRollback)
	}

	modify := rb.Steps[0]
	if modify.APICall == nil || modify.APICall.Operation != "ModifyDBInstance" {
		t.Fatalf("rollback step 0 must be ModifyDBInstance, got %+v", modify)
	}
	if got, want := modify.APICall.Params["DBInstanceClass"], p.CurrentInstanceClass; got != want {
		t.Errorf("rollback DBInstanceClass: got %v want %v", got, want)
	}
}

func TestCompile_BothPlansBindToProposalHash(t *testing.T) {
	fwd, rb := CompileRDSResize(sampleProposal(), sampleHash)
	if fwd.ProposalHash != sampleHash {
		t.Errorf("forward proposal hash: got %q want %q", fwd.ProposalHash, sampleHash)
	}
	if rb.ProposalHash != sampleHash {
		t.Errorf("rollback proposal hash: got %q want %q", rb.ProposalHash, sampleHash)
	}
}

func TestCompile_IsDeterministic(t *testing.T) {
	a, ar := CompileRDSResize(sampleProposal(), sampleHash)
	b, br := CompileRDSResize(sampleProposal(), sampleHash)

	if !reflect.DeepEqual(a, b) {
		t.Error("forward plan compilation is not deterministic")
	}
	if !reflect.DeepEqual(ar, br) {
		t.Error("rollback plan compilation is not deterministic")
	}
}

func TestCompile_PlanIsJSONSerializable(t *testing.T) {
	fwd, rb := CompileRDSResize(sampleProposal(), sampleHash)
	if _, err := json.Marshal(fwd); err != nil {
		t.Errorf("forward plan json: %v", err)
	}
	if _, err := json.Marshal(rb); err != nil {
		t.Errorf("rollback plan json: %v", err)
	}
}
