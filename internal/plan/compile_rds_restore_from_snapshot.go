package plan

import (
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// CompileRDSRestoreFromSnapshot produces forward + rollback plans for
// restoring a snapshot to a new RDS instance. The action is additive
// — the snapshot is unchanged. Rollback deletes the just-created
// instance; the snapshot remains intact.
//
// Forward plan (5 steps):
//  1. describe-snapshot — confirm the snapshot exists and is available
//  2. preconditions — verify state
//  3. restore — call RestoreDBInstanceFromDBSnapshot
//  4. poll-available — wait for the new instance (~10–30 min)
//  5. verify — confirm the new instance exists with the right class
//
// Rollback plan (3 steps): delete the new instance, poll until it's
// gone. The snapshot is never touched.
func CompileRDSRestoreFromSnapshot(p action.RDSRestoreFromSnapshotProposal, h action.ProposalHash) (forward, rollback ExecutionPlan) {
	return rdsRestoreFromSnapshotForward(p, h), rdsRestoreFromSnapshotRollback(p, h)
}

func rdsRestoreFromSnapshotForward(p action.RDSRestoreFromSnapshotProposal, h action.ProposalHash) ExecutionPlan {
	describeSnapshotParams := map[string]any{"DBSnapshotIdentifier": p.SnapshotIdentifier}
	describeInstanceParams := map[string]any{"DBInstanceIdentifier": p.TargetDBInstanceIdentifier}
	restoreParams := map[string]any{
		"DBSnapshotIdentifier": p.SnapshotIdentifier,
		"DBInstanceIdentifier": p.TargetDBInstanceIdentifier,
		"DBInstanceClass":      p.TargetInstanceClass,
	}

	steps := []Step{
		{
			ID:          "describe-snapshot",
			Kind:        StepAWSAPICall,
			Description: "Describe source snapshot to capture pre-state",
			OnFailure:   OnFailureAbort,
			APICall: &APICall{
				Service:   "rds",
				Operation: "DescribeDBSnapshots",
				Params:    describeSnapshotParams,
			},
		},
		{
			ID:          "preconditions",
			Kind:        StepVerify,
			Description: "Verify the snapshot exists and is available",
			OnFailure:   OnFailureAbort,
			Verify: &Verify{
				SourceStepID: "describe-snapshot",
				Assertions: []Predicate{
					{Path: "DBSnapshots[0].DBSnapshotIdentifier", Operator: "eq", Value: p.SnapshotIdentifier},
					{Path: "DBSnapshots[0].Status", Operator: "eq", Value: "available"},
				},
			},
		},
		{
			ID:          "restore",
			Kind:        StepAWSAPICall,
			Description: "Restore the snapshot to a new instance",
			OnFailure:   OnFailureRollback,
			APICall: &APICall{
				Service:   "rds",
				Operation: "RestoreDBInstanceFromDBSnapshot",
				Params:    restoreParams,
			},
		},
		{
			ID:          "poll-available",
			Kind:        StepPoll,
			Description: "Poll until the restored instance reaches available (10–30 min)",
			Timeout:     "60m",
			OnFailure:   OnFailureRollback,
			Poll: &Poll{
				APICall: APICall{
					Service:   "rds",
					Operation: "DescribeDBInstances",
					Params:    describeInstanceParams,
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
			Description: "Verify the restored instance has the correct identifier and class",
			OnFailure:   OnFailureRollback,
			Verify: &Verify{
				APICall: &APICall{
					Service:   "rds",
					Operation: "DescribeDBInstances",
					Params:    describeInstanceParams,
				},
				Assertions: []Predicate{
					{Path: "DBInstances[0].DBInstanceIdentifier", Operator: "eq", Value: p.TargetDBInstanceIdentifier},
					{Path: "DBInstances[0].DBInstanceClass", Operator: "eq", Value: p.TargetInstanceClass},
					{Path: "DBInstances[0].DBInstanceStatus", Operator: "eq", Value: "available"},
				},
			},
		},
	}

	return ExecutionPlan{
		Kind:         PlanForward,
		ActionType:   "rds_restore_from_snapshot",
		ProposalHash: h,
		Steps:        steps,
	}
}

func rdsRestoreFromSnapshotRollback(p action.RDSRestoreFromSnapshotProposal, h action.ProposalHash) ExecutionPlan {
	describeInstanceParams := map[string]any{"DBInstanceIdentifier": p.TargetDBInstanceIdentifier}
	deleteInstanceParams := map[string]any{
		"DBInstanceIdentifier":   p.TargetDBInstanceIdentifier,
		"SkipFinalSnapshot":      true, // it's a fresh restore — the source snapshot is the canonical artifact
		"DeleteAutomatedBackups": true,
	}

	steps := []Step{
		{
			ID:          "rollback-delete-instance",
			Kind:        StepAWSAPICall,
			Description: "Delete the just-restored instance",
			OnFailure:   OnFailureAbort,
			APICall: &APICall{
				Service:   "rds",
				Operation: "DeleteDBInstance",
				Params:    deleteInstanceParams,
			},
		},
		{
			ID:          "rollback-poll-deleting",
			Kind:        StepPoll,
			Description: "Poll until status=deleting",
			Timeout:     "5m",
			OnFailure:   OnFailureAbort,
			Poll: &Poll{
				APICall: APICall{
					Service:   "rds",
					Operation: "DescribeDBInstances",
					Params:    describeInstanceParams,
				},
				Predicate: Predicate{
					Path:     "DBInstances[0].DBInstanceStatus",
					Operator: "eq",
					Value:    "deleting",
				},
				Interval: "10s",
			},
		},
		{
			ID:          "rollback-verify-deleted",
			Kind:        StepVerify,
			Description: "Confirm the restored instance is gone",
			OnFailure:   OnFailureAbort,
			Verify: &Verify{
				APICall: &APICall{
					Service:   "rds",
					Operation: "DescribeDBInstances",
					Params:    describeInstanceParams,
				},
				Assertions: []Predicate{
					{Path: "DBInstances", Operator: "empty"},
				},
			},
		},
	}

	return ExecutionPlan{
		Kind:         PlanRollback,
		ActionType:   "rds_restore_from_snapshot",
		ProposalHash: h,
		Steps:        steps,
	}
}
