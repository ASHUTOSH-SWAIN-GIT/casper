package plan

import (
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// CompileRDSRebootInstance produces the forward plan for rebooting an
// RDS instance. Reboot has no useful rollback (a reboot can't be
// "undone"), so the rollback plan is empty — if the forward plan
// fails, the operator must investigate manually.
//
// Forward plan (5 steps):
//  1. describe-pre — confirm instance exists and is available
//  2. preconditions — verify state matches the proposal
//  3. reboot — call RebootDBInstance
//  4. poll-rebooting — wait for status to leave "available" (briefly enters "rebooting")
//  5. poll-available — wait for status to return to "available"
//  6. verify — final describe + assert status=available
func CompileRDSRebootInstance(p action.RDSRebootInstanceProposal, h action.ProposalHash) (forward, rollback ExecutionPlan) {
	return rdsRebootInstanceForward(p, h), rdsRebootInstanceRollback(p, h)
}

func rdsRebootInstanceForward(p action.RDSRebootInstanceProposal, h action.ProposalHash) ExecutionPlan {
	describeParams := map[string]any{"DBInstanceIdentifier": p.DBInstanceIdentifier}
	rebootParams := map[string]any{
		"DBInstanceIdentifier": p.DBInstanceIdentifier,
		"ForceFailover":        p.ForceFailover,
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
				},
			},
		},
		{
			ID:          "reboot",
			Kind:        StepAWSAPICall,
			Description: "Reboot the instance",
			// abort, not rollback — there's nothing to undo on a failed reboot.
			OnFailure: OnFailureAbort,
			APICall: &APICall{
				Service:   "rds",
				Operation: "RebootDBInstance",
				Params:    rebootParams,
			},
		},
		{
			ID:          "poll-rebooting",
			Kind:        StepPoll,
			Description: "Poll until status=rebooting",
			Timeout:     "2m",
			OnFailure:   OnFailureAbort,
			Poll: &Poll{
				APICall: APICall{
					Service:   "rds",
					Operation: "DescribeDBInstances",
					Params:    describeParams,
				},
				Predicate: Predicate{
					Path:     "DBInstances[0].DBInstanceStatus",
					Operator: "eq",
					Value:    "rebooting",
				},
				Interval: "5s",
			},
		},
		{
			ID:          "poll-available",
			Kind:        StepPoll,
			Description: "Poll until status returns to available",
			Timeout:     "10m",
			OnFailure:   OnFailureAbort,
			Poll: &Poll{
				APICall: APICall{
					Service:   "rds",
					Operation: "DescribeDBInstances",
					Params:    describeParams,
				},
				Predicate: Predicate{
					Path:     "DBInstances[0].DBInstanceStatus",
					Operator: "eq",
					Value:    "available",
				},
				Interval: "10s",
			},
		},
		{
			ID:          "verify",
			Kind:        StepVerify,
			Description: "Verify instance is back to available with no pending modifications",
			OnFailure:   OnFailureAbort,
			Verify: &Verify{
				APICall: &APICall{
					Service:   "rds",
					Operation: "DescribeDBInstances",
					Params:    describeParams,
				},
				Assertions: []Predicate{
					{Path: "DBInstances[0].DBInstanceStatus", Operator: "eq", Value: "available"},
					{Path: "DBInstances[0].PendingModifiedValues", Operator: "empty"},
				},
			},
		},
	}

	return ExecutionPlan{
		Kind:         PlanForward,
		ActionType:   "rds_reboot_instance",
		ProposalHash: h,
		Steps:        steps,
	}
}

// rdsRebootInstanceRollback returns an empty rollback plan — there is
// no meaningful undo for a reboot. The rollback exists structurally
// (the interpreter machinery still needs an ExecutionPlan to dispatch
// to) but contains no steps. The CLI's "no rollback when no mutating
// step succeeded" check ensures this empty plan is never invoked.
func rdsRebootInstanceRollback(p action.RDSRebootInstanceProposal, h action.ProposalHash) ExecutionPlan {
	return ExecutionPlan{
		Kind:         PlanRollback,
		ActionType:   "rds_reboot_instance",
		ProposalHash: h,
		Steps:        []Step{},
	}
}
