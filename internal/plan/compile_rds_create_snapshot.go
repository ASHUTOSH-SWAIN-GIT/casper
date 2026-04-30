package plan

import (
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// CompileRDSCreateSnapshot produces forward + rollback plans for the
// rds_create_snapshot action. The action is additive — the source
// instance is unchanged, a new snapshot is produced. Rollback (if
// triggered) deletes the just-created snapshot.
//
// Forward plan (6 steps):
//  1. describe-pre — confirm source instance exists and is available
//  2. preconditions — verify state matches the proposal
//  3. create — call CreateDBSnapshot
//  4. poll-creating — wait until snapshot enters creating state
//  5. poll-available — wait until snapshot reaches available state
//  6. verify — confirm snapshot exists with the correct identifier
//
// Rollback plan (3 steps):
//  1. rollback-delete — call DeleteDBSnapshot on the just-created snapshot
//  2. rollback-poll-deleted — wait for the snapshot to disappear
//  3. rollback-verify — confirm the snapshot is gone
func CompileRDSCreateSnapshot(p action.RDSCreateSnapshotProposal, h action.ProposalHash) (forward, rollback ExecutionPlan) {
	return rdsCreateSnapshotForward(p, h), rdsCreateSnapshotRollback(p, h)
}

func rdsCreateSnapshotForward(p action.RDSCreateSnapshotProposal, h action.ProposalHash) ExecutionPlan {
	describeDBParams := map[string]any{
		"DBInstanceIdentifier": p.DBInstanceIdentifier,
	}
	createSnapshotParams := map[string]any{
		"DBInstanceIdentifier": p.DBInstanceIdentifier,
		"DBSnapshotIdentifier": p.SnapshotIdentifier,
	}
	describeSnapshotParams := map[string]any{
		"DBSnapshotIdentifier": p.SnapshotIdentifier,
	}

	describeSnapshotsExistingParams := map[string]any{
		"DBInstanceIdentifier": p.DBInstanceIdentifier,
	}

	steps := []Step{
		// describe-pre and check-existing-snapshots are independent reads — run
		// in parallel to shorten the pre-check phase.
		{
			ID:            "describe-pre",
			Kind:          StepAWSAPICall,
			Description:   "Describe instance to capture pre-state",
			OnFailure:     OnFailureAbort,
			ParallelGroup: "pre-checks",
			APICall: &APICall{
				Service:   "rds",
				Operation: "DescribeDBInstances",
				Params:    describeDBParams,
			},
		},
		{
			ID:            "check-existing-snapshots",
			Kind:          StepAWSAPICall,
			Description:   "List existing snapshots for the instance to detect identifier collisions",
			OnFailure:     OnFailureAbort,
			ParallelGroup: "pre-checks",
			APICall: &APICall{
				Service:   "rds",
				Operation: "DescribeDBSnapshots",
				Params:    describeSnapshotsExistingParams,
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
			ID:          "create",
			Kind:        StepAWSAPICall,
			Description: "Create the manual snapshot",
			OnFailure:   OnFailureRollback,
			APICall: &APICall{
				Service:   "rds",
				Operation: "CreateDBSnapshot",
				Params:    createSnapshotParams,
			},
		},
		{
			ID:          "poll-creating",
			Kind:        StepPoll,
			Description: "Poll until snapshot status=creating",
			Timeout:     "2m",
			OnFailure:   OnFailureRollback,
			Poll: &Poll{
				APICall: APICall{
					Service:   "rds",
					Operation: "DescribeDBSnapshots",
					Params:    describeSnapshotParams,
				},
				Predicate: Predicate{
					Path:     "DBSnapshots[0].Status",
					Operator: "eq",
					Value:    "creating",
				},
				Interval: "5s",
			},
		},
		{
			ID:          "poll-available",
			Kind:        StepPoll,
			Description: "Poll until snapshot status=available",
			Timeout:     "30m",
			OnFailure:   OnFailureRollback,
			Poll: &Poll{
				APICall: APICall{
					Service:   "rds",
					Operation: "DescribeDBSnapshots",
					Params:    describeSnapshotParams,
				},
				Predicate: Predicate{
					Path:     "DBSnapshots[0].Status",
					Operator: "eq",
					Value:    "available",
				},
				Interval: "30s",
			},
		},
		{
			ID:          "verify",
			Kind:        StepVerify,
			Description: "Verify snapshot exists with expected identifier",
			OnFailure:   OnFailureRollback,
			Verify: &Verify{
				APICall: &APICall{
					Service:   "rds",
					Operation: "DescribeDBSnapshots",
					Params:    describeSnapshotParams,
				},
				Assertions: []Predicate{
					{Path: "DBSnapshots[0].DBSnapshotIdentifier", Operator: "eq", Value: p.SnapshotIdentifier},
					{Path: "DBSnapshots[0].Status", Operator: "eq", Value: "available"},
				},
			},
		},
	}

	return ExecutionPlan{
		Kind:         PlanForward,
		ActionType:   "rds_create_snapshot",
		ProposalHash: h,
		Steps:        steps,
	}
}

func rdsCreateSnapshotRollback(p action.RDSCreateSnapshotProposal, h action.ProposalHash) ExecutionPlan {
	describeSnapshotParams := map[string]any{
		"DBSnapshotIdentifier": p.SnapshotIdentifier,
	}
	deleteSnapshotParams := map[string]any{
		"DBSnapshotIdentifier": p.SnapshotIdentifier,
	}

	steps := []Step{
		{
			ID:          "rollback-delete",
			Kind:        StepAWSAPICall,
			Description: "Delete the just-created snapshot",
			OnFailure:   OnFailureAbort,
			APICall: &APICall{
				Service:   "rds",
				Operation: "DeleteDBSnapshot",
				Params:    deleteSnapshotParams,
			},
		},
		{
			ID:          "rollback-poll-deleting",
			Kind:        StepPoll,
			Description: "Poll until snapshot status=deleting",
			Timeout:     "2m",
			OnFailure:   OnFailureAbort,
			Poll: &Poll{
				APICall: APICall{
					Service:   "rds",
					Operation: "DescribeDBSnapshots",
					Params:    describeSnapshotParams,
				},
				Predicate: Predicate{
					Path:     "DBSnapshots[0].Status",
					Operator: "eq",
					Value:    "deleting",
				},
				Interval: "5s",
			},
		},
		{
			ID:          "rollback-verify-deleted",
			Kind:        StepVerify,
			Description: "Confirm snapshot has been deleted",
			OnFailure:   OnFailureAbort,
			Verify: &Verify{
				APICall: &APICall{
					Service:   "rds",
					Operation: "DescribeDBSnapshots",
					Params:    describeSnapshotParams,
				},
				Assertions: []Predicate{
					{Path: "DBSnapshots", Operator: "empty"},
				},
			},
		},
	}

	return ExecutionPlan{
		Kind:         PlanRollback,
		ActionType:   "rds_create_snapshot",
		ProposalHash: h,
		Steps:        steps,
	}
}
