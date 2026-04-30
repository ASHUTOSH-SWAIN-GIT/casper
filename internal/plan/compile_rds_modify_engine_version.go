package plan

import (
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// CompileRDSModifyEngineVersion produces forward + (empty) rollback
// plans for upgrading an RDS instance's engine version. Like
// rds_storage_grow, the rollback is empty because the action is
// irreversible — AWS doesn't support engine downgrades.
//
// Forward plan (5 steps): describe → preconditions → modify →
// poll-available (long timeout — engine upgrades take 30+ minutes
// for major versions) → verify.
//
// Rollback plan: EMPTY. All forward steps are on_failure: abort.
func CompileRDSModifyEngineVersion(p action.RDSModifyEngineVersionProposal, h action.ProposalHash) (forward, rollback ExecutionPlan) {
	return rdsModifyEngineVersionForward(p, h), rdsModifyEngineVersionRollback(p, h)
}

func rdsModifyEngineVersionForward(p action.RDSModifyEngineVersionProposal, h action.ProposalHash) ExecutionPlan {
	describeParams := map[string]any{"DBInstanceIdentifier": p.DBInstanceIdentifier}
	modifyParams := map[string]any{
		"DBInstanceIdentifier":     p.DBInstanceIdentifier,
		"EngineVersion":            p.TargetEngineVersion,
		"AllowMajorVersionUpgrade": p.AllowMajorVersionUpgrade,
		"ApplyImmediately":         true,
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
					{Path: "DBInstances[0].EngineVersion", Operator: "eq", Value: p.CurrentEngineVersion},
				},
			},
		},
		{
			ID:          "modify",
			Kind:        StepAWSAPICall,
			Description: "Upgrade engine version (irreversible)",
			OnFailure:   OnFailureAbort,
			APICall: &APICall{
				Service:   "rds",
				Operation: "ModifyDBInstance",
				Params:    modifyParams,
			},
		},
		{
			ID:          "poll-available",
			Kind:        StepPoll,
			Description: "Poll until engine upgrade completes (major versions can take 30+ minutes)",
			Timeout:     "60m",
			OnFailure:   OnFailureAbort,
			Poll: &Poll{
				APICall: APICall{
					Service:   "rds",
					Operation: "DescribeDBInstances",
					Params:    describeParams,
				},
				Predicate: Predicate{
					Path:     "DBInstances[0].EngineVersion",
					Operator: "eq",
					Value:    p.TargetEngineVersion,
				},
				Interval: "60s",
			},
		},
		{
			ID:          "verify",
			Kind:        StepVerify,
			Description: "Verify engine version equals target and instance is available",
			OnFailure:   OnFailureAbort,
			Verify: &Verify{
				APICall: &APICall{
					Service:   "rds",
					Operation: "DescribeDBInstances",
					Params:    describeParams,
				},
				Assertions: []Predicate{
					{Path: "DBInstances[0].EngineVersion", Operator: "eq", Value: p.TargetEngineVersion},
					{Path: "DBInstances[0].DBInstanceStatus", Operator: "eq", Value: "available"},
				},
			},
		},
	}

	return ExecutionPlan{
		Kind:         PlanForward,
		ActionType:   "rds_modify_engine_version",
		ProposalHash: h,
		Steps:        steps,
	}
}

func rdsModifyEngineVersionRollback(p action.RDSModifyEngineVersionProposal, h action.ProposalHash) ExecutionPlan {
	// Empty rollback — engine version downgrade is not supported by RDS.
	return ExecutionPlan{
		Kind:         PlanRollback,
		ActionType:   "rds_modify_engine_version",
		ProposalHash: h,
		Steps:        []Step{},
	}
}
