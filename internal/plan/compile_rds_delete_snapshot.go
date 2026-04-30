package plan

import (
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// CompileRDSDeleteSnapshot produces forward + (empty) rollback plans
// for permanently deleting a manual RDS snapshot. Like rds_storage_grow,
// the rollback is empty because the action is irreversible — there
// is no recovery path for a deleted snapshot.
//
// Forward plan (4 steps):
//  1. describe-pre — confirm snapshot exists and is available
//  2. preconditions — verify state
//  3. delete — call DeleteDBSnapshot
//  4. verify-deleted — confirm DescribeDBSnapshots returns empty
//
// Rollback plan: EMPTY.
func CompileRDSDeleteSnapshot(p action.RDSDeleteSnapshotProposal, h action.ProposalHash) (forward, rollback ExecutionPlan) {
	return rdsDeleteSnapshotForward(p, h), rdsDeleteSnapshotRollback(p, h)
}

func rdsDeleteSnapshotForward(p action.RDSDeleteSnapshotProposal, h action.ProposalHash) ExecutionPlan {
	describeParams := map[string]any{"DBSnapshotIdentifier": p.SnapshotIdentifier}
	deleteParams := map[string]any{"DBSnapshotIdentifier": p.SnapshotIdentifier}

	steps := []Step{
		{
			ID:          "describe-pre",
			Kind:        StepAWSAPICall,
			Description: "Describe snapshot to capture pre-state",
			OnFailure:   OnFailureAbort,
			APICall: &APICall{
				Service:   "rds",
				Operation: "DescribeDBSnapshots",
				Params:    describeParams,
			},
		},
		{
			ID:          "preconditions",
			Kind:        StepVerify,
			Description: "Re-check preconditions: snapshot exists, is available, and is a manual snapshot",
			OnFailure:   OnFailureAbort,
			Verify: &Verify{
				SourceStepID: "describe-pre",
				Assertions: []Predicate{
					{Path: "DBSnapshots[0].DBSnapshotIdentifier", Operator: "eq", Value: p.SnapshotIdentifier},
					{Path: "DBSnapshots[0].Status", Operator: "eq", Value: "available"},
					{Path: "DBSnapshots[0].SnapshotType", Operator: "eq", Value: "manual"},
				},
			},
		},
		{
			ID:          "delete",
			Kind:        StepAWSAPICall,
			Description: "Delete the snapshot (irreversible)",
			// abort, not rollback — there is no recovery once deleted.
			OnFailure: OnFailureAbort,
			APICall: &APICall{
				Service:   "rds",
				Operation: "DeleteDBSnapshot",
				Params:    deleteParams,
			},
		},
		{
			ID:          "verify-deleted",
			Kind:        StepVerify,
			Description: "Confirm snapshot has been deleted (DescribeDBSnapshots returns empty)",
			OnFailure:   OnFailureAbort,
			Verify: &Verify{
				APICall: &APICall{
					Service:   "rds",
					Operation: "DescribeDBSnapshots",
					Params:    describeParams,
				},
				Assertions: []Predicate{
					{Path: "DBSnapshots", Operator: "empty"},
				},
			},
		},
	}

	return ExecutionPlan{
		Kind:         PlanForward,
		ActionType:   "rds_delete_snapshot",
		ProposalHash: h,
		Steps:        steps,
	}
}

func rdsDeleteSnapshotRollback(p action.RDSDeleteSnapshotProposal, h action.ProposalHash) ExecutionPlan {
	// Empty rollback — a deleted snapshot cannot be undeleted.
	return ExecutionPlan{
		Kind:         PlanRollback,
		ActionType:   "rds_delete_snapshot",
		ProposalHash: h,
		Steps:        []Step{},
	}
}
