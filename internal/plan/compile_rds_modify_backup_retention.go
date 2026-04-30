package plan

import (
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// CompileRDSModifyBackupRetention produces forward + rollback plans for
// changing an RDS instance's backup retention period. The action is
// reversible — rollback simply sets the retention back to the original
// value. Note: shortening retention immediately deletes backups older
// than the new window, which is NOT recoverable by setting retention
// back. Policy should restrict large reductions to needs_approval.
//
// Forward plan (5 steps): describe → preconditions → modify → poll-modifying → verify.
// Rollback plan (5 steps): same shape, modifying back to current_retention_days.
func CompileRDSModifyBackupRetention(p action.RDSModifyBackupRetentionProposal, h action.ProposalHash) (forward, rollback ExecutionPlan) {
	return rdsModifyBackupRetentionForward(p, h), rdsModifyBackupRetentionRollback(p, h)
}

func rdsModifyBackupRetentionForward(p action.RDSModifyBackupRetentionProposal, h action.ProposalHash) ExecutionPlan {
	describeParams := map[string]any{"DBInstanceIdentifier": p.DBInstanceIdentifier}
	modifyParams := map[string]any{
		"DBInstanceIdentifier":  p.DBInstanceIdentifier,
		"BackupRetentionPeriod": p.TargetRetentionDays,
		"ApplyImmediately":      true,
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
					{Path: "DBInstances[0].BackupRetentionPeriod", Operator: "eq", Value: p.CurrentRetentionDays},
				},
			},
		},
		{
			ID:          "modify",
			Kind:        StepAWSAPICall,
			Description: "Modify backup retention period",
			OnFailure:   OnFailureRollback,
			APICall: &APICall{
				Service:   "rds",
				Operation: "ModifyDBInstance",
				Params:    modifyParams,
			},
		},
		{
			ID:          "poll-applied",
			Kind:        StepPoll,
			Description: "Poll until BackupRetentionPeriod equals target (modify takes effect quickly for retention-only changes)",
			Timeout:     "5m",
			OnFailure:   OnFailureRollback,
			Poll: &Poll{
				APICall: APICall{
					Service:   "rds",
					Operation: "DescribeDBInstances",
					Params:    describeParams,
				},
				Predicate: Predicate{
					Path:     "DBInstances[0].BackupRetentionPeriod",
					Operator: "eq",
					Value:    p.TargetRetentionDays,
				},
				Interval: "5s",
			},
		},
		{
			ID:          "verify",
			Kind:        StepVerify,
			Description: "Verify retention is the target value and no other modifications are pending",
			OnFailure:   OnFailureRollback,
			Verify: &Verify{
				APICall: &APICall{
					Service:   "rds",
					Operation: "DescribeDBInstances",
					Params:    describeParams,
				},
				Assertions: []Predicate{
					{Path: "DBInstances[0].BackupRetentionPeriod", Operator: "eq", Value: p.TargetRetentionDays},
					{Path: "DBInstances[0].PendingModifiedValues", Operator: "empty"},
				},
			},
		},
	}

	return ExecutionPlan{
		Kind:         PlanForward,
		ActionType:   "rds_modify_backup_retention",
		ProposalHash: h,
		Steps:        steps,
	}
}

func rdsModifyBackupRetentionRollback(p action.RDSModifyBackupRetentionProposal, h action.ProposalHash) ExecutionPlan {
	describeParams := map[string]any{"DBInstanceIdentifier": p.DBInstanceIdentifier}
	modifyParams := map[string]any{
		"DBInstanceIdentifier":  p.DBInstanceIdentifier,
		"BackupRetentionPeriod": p.CurrentRetentionDays,
		"ApplyImmediately":      true,
	}

	steps := []Step{
		{
			ID:          "rollback-modify",
			Kind:        StepAWSAPICall,
			Description: "Revert backup retention to original value",
			OnFailure:   OnFailureAbort,
			APICall: &APICall{
				Service:   "rds",
				Operation: "ModifyDBInstance",
				Params:    modifyParams,
			},
		},
		{
			ID:          "rollback-poll-applied",
			Kind:        StepPoll,
			Description: "Poll until retention is back at original value",
			Timeout:     "5m",
			OnFailure:   OnFailureAbort,
			Poll: &Poll{
				APICall: APICall{
					Service:   "rds",
					Operation: "DescribeDBInstances",
					Params:    describeParams,
				},
				Predicate: Predicate{
					Path:     "DBInstances[0].BackupRetentionPeriod",
					Operator: "eq",
					Value:    p.CurrentRetentionDays,
				},
				Interval: "5s",
			},
		},
		{
			ID:          "rollback-verify",
			Kind:        StepVerify,
			Description: "Confirm retention is back at original",
			OnFailure:   OnFailureAbort,
			Verify: &Verify{
				APICall: &APICall{
					Service:   "rds",
					Operation: "DescribeDBInstances",
					Params:    describeParams,
				},
				Assertions: []Predicate{
					{Path: "DBInstances[0].BackupRetentionPeriod", Operator: "eq", Value: p.CurrentRetentionDays},
					{Path: "DBInstances[0].PendingModifiedValues", Operator: "empty"},
				},
			},
		},
	}

	return ExecutionPlan{
		Kind:         PlanRollback,
		ActionType:   "rds_modify_backup_retention",
		ProposalHash: h,
		Steps:        steps,
	}
}
