package plan

import (
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// CompileRDSModifyMultiAZ produces forward + rollback plans for
// toggling an RDS instance's Multi-AZ deployment. The shape is
// nearly identical to rds_resize: long polling cycle, verify the
// final topology, rollback flips the toggle back.
//
// Forward plan (7 steps): describe → preconditions → modify →
// poll-modifying → poll-available → verify → wait-settle.
//
// Rollback plan (4 steps): same shape, modifying back to current_multi_az.
func CompileRDSModifyMultiAZ(p action.RDSModifyMultiAZProposal, h action.ProposalHash) (forward, rollback ExecutionPlan) {
	return rdsModifyMultiAZForward(p, h), rdsModifyMultiAZRollback(p, h)
}

func rdsModifyMultiAZForward(p action.RDSModifyMultiAZProposal, h action.ProposalHash) ExecutionPlan {
	describeParams := map[string]any{"DBInstanceIdentifier": p.DBInstanceIdentifier}
	modifyParams := map[string]any{
		"DBInstanceIdentifier": p.DBInstanceIdentifier,
		"MultiAZ":              p.TargetMultiAZ,
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
					{Path: "DBInstances[0].MultiAZ", Operator: "eq", Value: p.CurrentMultiAZ},
				},
			},
		},
		{
			ID:          "modify",
			Kind:        StepAWSAPICall,
			Description: "Toggle Multi-AZ deployment",
			OnFailure:   OnFailureRollback,
			APICall: &APICall{
				Service:   "rds",
				Operation: "ModifyDBInstance",
				Params:    modifyParams,
			},
		},
		{
			ID:          "poll-modifying",
			Kind:        StepPoll,
			Description: "Poll until status=modifying",
			Timeout:     "2m",
			OnFailure:   OnFailureRollback,
			Poll: &Poll{
				APICall: APICall{
					Service:   "rds",
					Operation: "DescribeDBInstances",
					Params:    describeParams,
				},
				Predicate: Predicate{
					Path:     "DBInstances[0].DBInstanceStatus",
					Operator: "eq",
					Value:    "modifying",
				},
				Interval: "5s",
			},
		},
		{
			ID:          "poll-available",
			Kind:        StepPoll,
			Description: "Poll until status=available (Multi-AZ change can take 15+ minutes)",
			Timeout:     "30m",
			OnFailure:   OnFailureRollback,
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
				Interval: "30s",
			},
		},
		{
			ID:          "verify",
			Kind:        StepVerify,
			Description: "Verify Multi-AZ is now the target value and no pending modifications",
			OnFailure:   OnFailureRollback,
			Verify: &Verify{
				APICall: &APICall{
					Service:   "rds",
					Operation: "DescribeDBInstances",
					Params:    describeParams,
				},
				Assertions: []Predicate{
					{Path: "DBInstances[0].MultiAZ", Operator: "eq", Value: p.TargetMultiAZ},
					{Path: "DBInstances[0].PendingModifiedValues", Operator: "empty"},
				},
			},
		},
	}

	return ExecutionPlan{
		Kind:         PlanForward,
		ActionType:   "rds_modify_multi_az",
		ProposalHash: h,
		Steps:        steps,
	}
}

func rdsModifyMultiAZRollback(p action.RDSModifyMultiAZProposal, h action.ProposalHash) ExecutionPlan {
	describeParams := map[string]any{"DBInstanceIdentifier": p.DBInstanceIdentifier}
	modifyParams := map[string]any{
		"DBInstanceIdentifier": p.DBInstanceIdentifier,
		"MultiAZ":              p.CurrentMultiAZ,
		"ApplyImmediately":     true,
	}

	steps := []Step{
		{
			ID:          "rollback-modify",
			Kind:        StepAWSAPICall,
			Description: "Revert Multi-AZ to original value",
			OnFailure:   OnFailureAbort,
			APICall: &APICall{
				Service:   "rds",
				Operation: "ModifyDBInstance",
				Params:    modifyParams,
			},
		},
		{
			ID:          "rollback-poll-modifying",
			Kind:        StepPoll,
			Description: "Poll until status=modifying",
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
					Value:    "modifying",
				},
				Interval: "5s",
			},
		},
		{
			ID:          "rollback-poll-available",
			Kind:        StepPoll,
			Description: "Poll until status returns to available",
			Timeout:     "30m",
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
				Interval: "30s",
			},
		},
		{
			ID:          "rollback-verify",
			Kind:        StepVerify,
			Description: "Confirm Multi-AZ is back at original",
			OnFailure:   OnFailureAbort,
			Verify: &Verify{
				APICall: &APICall{
					Service:   "rds",
					Operation: "DescribeDBInstances",
					Params:    describeParams,
				},
				Assertions: []Predicate{
					{Path: "DBInstances[0].MultiAZ", Operator: "eq", Value: p.CurrentMultiAZ},
					{Path: "DBInstances[0].PendingModifiedValues", Operator: "empty"},
				},
			},
		},
	}

	return ExecutionPlan{
		Kind:         PlanRollback,
		ActionType:   "rds_modify_multi_az",
		ProposalHash: h,
		Steps:        steps,
	}
}
