package plan

import (
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// CompileRDSCreateReadReplica produces forward + rollback plans for
// creating a read replica. The action is additive (the source instance
// is unchanged), so rollback is just deleting the new replica.
//
// Forward plan (5 steps):
//  1. describe-source — confirm source instance exists and supports replication
//  2. preconditions — verify state matches the proposal
//  3. create-replica — call CreateDBInstanceReadReplica
//  4. poll-available — wait for the replica to come up (~10–20 min)
//  5. verify — confirm the replica exists and is replicating from the source
//
// Rollback plan (3 steps):
//  1. rollback-delete-replica — call DeleteDBInstance on the replica
//  2. rollback-poll-deleting — wait for status=deleting
//  3. rollback-verify-deleted — confirm the replica is gone
func CompileRDSCreateReadReplica(p action.RDSCreateReadReplicaProposal, h action.ProposalHash) (forward, rollback ExecutionPlan) {
	return rdsCreateReadReplicaForward(p, h), rdsCreateReadReplicaRollback(p, h)
}

func rdsCreateReadReplicaForward(p action.RDSCreateReadReplicaProposal, h action.ProposalHash) ExecutionPlan {
	describeSourceParams := map[string]any{"DBInstanceIdentifier": p.SourceDBInstanceIdentifier}
	describeReplicaParams := map[string]any{"DBInstanceIdentifier": p.ReplicaDBInstanceIdentifier}
	createReplicaParams := map[string]any{
		"DBInstanceIdentifier":       p.ReplicaDBInstanceIdentifier,
		"SourceDBInstanceIdentifier": p.SourceDBInstanceIdentifier,
		"DBInstanceClass":            p.ReplicaInstanceClass,
	}

	steps := []Step{
		{
			ID:          "describe-source",
			Kind:        StepAWSAPICall,
			Description: "Describe source instance to capture pre-state",
			OnFailure:   OnFailureAbort,
			APICall: &APICall{
				Service:   "rds",
				Operation: "DescribeDBInstances",
				Params:    describeSourceParams,
			},
		},
		{
			ID:          "preconditions",
			Kind:        StepVerify,
			Description: "Verify source is available and not itself a read replica (RDS cannot chain replicas in v1)",
			OnFailure:   OnFailureAbort,
			Verify: &Verify{
				SourceStepID: "describe-source",
				Assertions: []Predicate{
					{Path: "DBInstances[0].DBInstanceIdentifier", Operator: "eq", Value: p.SourceDBInstanceIdentifier},
					{Path: "DBInstances[0].DBInstanceStatus", Operator: "eq", Value: "available"},
					{Path: "DBInstances[0].PendingModifiedValues", Operator: "empty"},
					{Path: "DBInstances[0].ReadReplicaSourceDBInstanceIdentifier", Operator: "empty"},
				},
			},
		},
		{
			ID:          "create-replica",
			Kind:        StepAWSAPICall,
			Description: "Create the read replica",
			OnFailure:   OnFailureRollback,
			APICall: &APICall{
				Service:   "rds",
				Operation: "CreateDBInstanceReadReplica",
				Params:    createReplicaParams,
			},
		},
		{
			ID:          "poll-available",
			Kind:        StepPoll,
			Description: "Poll until the replica reaches status=available (typically 10–20 min)",
			Timeout:     "45m",
			OnFailure:   OnFailureRollback,
			Poll: &Poll{
				APICall: APICall{
					Service:   "rds",
					Operation: "DescribeDBInstances",
					Params:    describeReplicaParams,
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
			Description: "Confirm the replica is replicating from the named source",
			OnFailure:   OnFailureRollback,
			Verify: &Verify{
				APICall: &APICall{
					Service:   "rds",
					Operation: "DescribeDBInstances",
					Params:    describeReplicaParams,
				},
				Assertions: []Predicate{
					{Path: "DBInstances[0].DBInstanceIdentifier", Operator: "eq", Value: p.ReplicaDBInstanceIdentifier},
					{Path: "DBInstances[0].DBInstanceStatus", Operator: "eq", Value: "available"},
					{Path: "DBInstances[0].ReadReplicaSourceDBInstanceIdentifier", Operator: "eq", Value: p.SourceDBInstanceIdentifier},
				},
			},
		},
	}

	return ExecutionPlan{
		Kind:         PlanForward,
		ActionType:   "rds_create_read_replica",
		ProposalHash: h,
		Steps:        steps,
	}
}

func rdsCreateReadReplicaRollback(p action.RDSCreateReadReplicaProposal, h action.ProposalHash) ExecutionPlan {
	describeReplicaParams := map[string]any{"DBInstanceIdentifier": p.ReplicaDBInstanceIdentifier}
	deleteReplicaParams := map[string]any{
		"DBInstanceIdentifier":   p.ReplicaDBInstanceIdentifier,
		"SkipFinalSnapshot":      true, // a freshly-created replica has no production data
		"DeleteAutomatedBackups": true,
	}

	steps := []Step{
		{
			ID:          "rollback-delete-replica",
			Kind:        StepAWSAPICall,
			Description: "Delete the just-created replica",
			OnFailure:   OnFailureAbort,
			APICall: &APICall{
				Service:   "rds",
				Operation: "DeleteDBInstance",
				Params:    deleteReplicaParams,
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
					Params:    describeReplicaParams,
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
			Description: "Confirm the replica is gone",
			OnFailure:   OnFailureAbort,
			Verify: &Verify{
				APICall: &APICall{
					Service:   "rds",
					Operation: "DescribeDBInstances",
					Params:    describeReplicaParams,
				},
				Assertions: []Predicate{
					{Path: "DBInstances", Operator: "empty"},
				},
			},
		},
	}

	return ExecutionPlan{
		Kind:         PlanRollback,
		ActionType:   "rds_create_read_replica",
		ProposalHash: h,
		Steps:        steps,
	}
}
