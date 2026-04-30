package plan

import (
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// CompileRDSStorageGrow produces forward + (empty) rollback plans for
// growing an RDS instance's allocated storage. The rollback is empty
// because storage grow is **irreversible** — AWS does not support
// shrinking. If forward fails after the modify call succeeded, there
// is nothing the trust layer can do to undo it; the operator must
// either accept the partial state or perform a manual restore.
//
// Forward plan (5 steps): describe → preconditions → modify →
// poll-modifying-then-available → verify.
//
// Rollback plan: EMPTY. The CLI's "no rollback when no mutating step
// succeeded" check still applies, so this empty rollback is never
// reached on pre-mutation failures. Post-mutation failures
// (verify mismatch, etc.) terminate with rollback_failed because
// there is no recovery path.
func CompileRDSStorageGrow(p action.RDSStorageGrowProposal, h action.ProposalHash) (forward, rollback ExecutionPlan) {
	return rdsStorageGrowForward(p, h), rdsStorageGrowRollback(p, h)
}

func rdsStorageGrowForward(p action.RDSStorageGrowProposal, h action.ProposalHash) ExecutionPlan {
	describeParams := map[string]any{"DBInstanceIdentifier": p.DBInstanceIdentifier}
	modifyParams := map[string]any{
		"DBInstanceIdentifier": p.DBInstanceIdentifier,
		"AllocatedStorage":     p.TargetAllocatedStorageGB,
		"ApplyImmediately":     true,
	}

	steps := []Step{
		{
			ID:          "describe-pre",
			Kind:        StepAWSAPICall,
			Description: "Describe instance to capture pre-state",
			OnFailure:   OnFailureAbort,
			APICall: &APICall{
				Service:   "rds",
				Operation: "DescribeDBInstances",
				Params:    describeParams,
			},
		},
		{
			ID:          "preconditions",
			Kind:        StepVerify,
			Description: "Re-check preconditions against captured pre-state",
			OnFailure:   OnFailureAbort,
			Verify: &Verify{
				SourceStepID: "describe-pre",
				Assertions: []Predicate{
					{Path: "DBInstances[0].DBInstanceIdentifier", Operator: "eq", Value: p.DBInstanceIdentifier},
					{Path: "DBInstances[0].DBInstanceStatus", Operator: "eq", Value: "available"},
					{Path: "DBInstances[0].PendingModifiedValues", Operator: "empty"},
					{Path: "DBInstances[0].AllocatedStorage", Operator: "eq", Value: p.CurrentAllocatedStorageGB},
				},
			},
		},
		{
			ID:          "modify",
			Kind:        StepAWSAPICall,
			Description: "Increase allocated storage (irreversible)",
			// Note: on_failure=abort even though this is the mutating
			// step, because rollback is impossible. If verification
			// later fails, the operator gets rollback_failed and a
			// human must look — there is no automatic recovery.
			OnFailure: OnFailureAbort,
			APICall: &APICall{
				Service:   "rds",
				Operation: "ModifyDBInstance",
				Params:    modifyParams,
			},
		},
		{
			ID:          "poll-available",
			Kind:        StepPoll,
			Description: "Poll until instance returns to available with new storage (storage-optimization may continue afterwards in the background)",
			Timeout:     "30m",
			OnFailure:   OnFailureAbort,
			Poll: &Poll{
				APICall: APICall{
					Service:   "rds",
					Operation: "DescribeDBInstances",
					Params:    describeParams,
				},
				Predicate: Predicate{
					Path:     "DBInstances[0].AllocatedStorage",
					Operator: "eq",
					Value:    p.TargetAllocatedStorageGB,
				},
				Interval: "30s",
			},
		},
		{
			ID:          "verify",
			Kind:        StepVerify,
			Description: "Verify allocated storage equals target",
			OnFailure:   OnFailureAbort,
			Verify: &Verify{
				APICall: &APICall{
					Service:   "rds",
					Operation: "DescribeDBInstances",
					Params:    describeParams,
				},
				Assertions: []Predicate{
					{Path: "DBInstances[0].AllocatedStorage", Operator: "eq", Value: p.TargetAllocatedStorageGB},
				},
			},
		},
	}

	return ExecutionPlan{
		Kind:         PlanForward,
		ActionType:   "rds_storage_grow",
		ProposalHash: h,
		Steps:        steps,
	}
}

func rdsStorageGrowRollback(p action.RDSStorageGrowProposal, h action.ProposalHash) ExecutionPlan {
	// Empty rollback — storage cannot be shrunk. The structural
	// rollback plan exists so the audit log can record that one was
	// compiled, but it has no steps. With every forward step's
	// OnFailure set to "abort", this empty plan is never invoked.
	return ExecutionPlan{
		Kind:         PlanRollback,
		ActionType:   "rds_storage_grow",
		ProposalHash: h,
		Steps:        []Step{},
	}
}
