package plan

import (
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// CompileRDSResize turns a validated RDSResizeProposal + its hash into
// the forward and rollback plans defined in docs/rds_resize.md §5–§6.
//
// This compiler is the *only* place the shape of an RDS resize plan is
// expressed in code. The interpreter never branches on action type —
// it just executes whatever steps the compiler emitted. To change what
// an RDS resize does, change this function.
func CompileRDSResize(p action.RDSResizeProposal, h action.ProposalHash) (forward, rollback ExecutionPlan) {
	return rdsResizeForward(p, h), rdsResizeRollback(p, h)
}

func rdsResizeForward(p action.RDSResizeProposal, h action.ProposalHash) ExecutionPlan {
	describeParams := map[string]any{
		"DBInstanceIdentifier": p.DBInstanceIdentifier,
	}
	modifyParams := map[string]any{
		"DBInstanceIdentifier": p.DBInstanceIdentifier,
		"DBInstanceClass":      p.TargetInstanceClass,
		"ApplyImmediately":     true,
	}

	metricsParams := map[string]any{
		"Namespace":  "AWS/RDS",
		"MetricName": p.SuccessCriteria.Metric,
		"Dimensions": []map[string]any{
			{"Name": "DBInstanceIdentifier", "Value": p.DBInstanceIdentifier},
		},
		"Period":     60,
		"Statistics": []string{"Average"},
		"Window":     "5m",
	}

	steps := []Step{
		// describe-pre and metrics-pre are independent read-only calls — run them
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
				Params:    describeParams,
			},
		},
		{
			ID:            "metrics-pre",
			Kind:          StepAWSAPICall,
			Description:   "Fetch pre-resize CPU baseline from CloudWatch",
			OnFailure:     OnFailureAbort,
			ParallelGroup: "pre-checks",
			APICall: &APICall{
				Service:   "cloudwatch",
				Operation: "GetMetricStatistics",
				Params:    metricsParams,
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
					{Path: "DBInstances[0].DBInstanceClass", Operator: "eq", Value: p.CurrentInstanceClass},
				},
			},
		},
		{
			ID:          "modify",
			Kind:        StepAWSAPICall,
			Description: "Resize instance to target class",
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
			Description: "Poll until status=available",
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
				Interval: "15s",
			},
		},
		{
			ID:          "verify-class",
			Kind:        StepVerify,
			Description: "Verify class equals target and no pending modifications",
			OnFailure:   OnFailureRollback,
			Verify: &Verify{
				APICall: &APICall{
					Service:   "rds",
					Operation: "DescribeDBInstances",
					Params:    describeParams,
				},
				Assertions: []Predicate{
					{Path: "DBInstances[0].DBInstanceClass", Operator: "eq", Value: p.TargetInstanceClass},
					{Path: "DBInstances[0].PendingModifiedValues", Operator: "empty"},
				},
			},
		},
		{
			ID:          "wait-verification-window",
			Kind:        StepWait,
			Description: "Wait verification window before sampling success metric",
			OnFailure:   OnFailureRollback,
			Wait:        &Wait{Duration: p.SuccessCriteria.VerificationWindow},
		},
		{
			ID:          "verify-metric",
			Kind:        StepVerify,
			Description: "Assert success metric meets threshold over verification window",
			OnFailure:   OnFailureRollback,
			Verify: &Verify{
				APICall: &APICall{
					Service:   "cloudwatch",
					Operation: "GetMetricStatistics",
					Params: map[string]any{
						"Namespace":  "AWS/RDS",
						"MetricName": p.SuccessCriteria.Metric,
						"Dimensions": []map[string]any{
							{"Name": "DBInstanceIdentifier", "Value": p.DBInstanceIdentifier},
						},
						"Period":     60,
						"Statistics": []string{"Average"},
						"Window":     p.SuccessCriteria.VerificationWindow,
					},
				},
				Assertions: []Predicate{
					{Path: "Datapoints.avg", Operator: "lte", Value: p.SuccessCriteria.ThresholdPercent},
				},
			},
		},
	}

	return ExecutionPlan{
		Kind:         PlanForward,
		ActionType:   "rds_resize",
		ProposalHash: h,
		Steps:        steps,
	}
}

func rdsResizeRollback(p action.RDSResizeProposal, h action.ProposalHash) ExecutionPlan {
	describeParams := map[string]any{
		"DBInstanceIdentifier": p.DBInstanceIdentifier,
	}
	modifyParams := map[string]any{
		"DBInstanceIdentifier": p.DBInstanceIdentifier,
		"DBInstanceClass":      p.CurrentInstanceClass,
		"ApplyImmediately":     true,
	}

	steps := []Step{
		{
			ID:          "rollback-modify",
			Kind:        StepAWSAPICall,
			Description: "Resize instance back to original class",
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
			Description: "Poll until status=available",
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
				Interval: "15s",
			},
		},
		{
			ID:          "rollback-verify-class",
			Kind:        StepVerify,
			Description: "Verify class is back to original and no pending modifications",
			OnFailure:   OnFailureAbort,
			Verify: &Verify{
				APICall: &APICall{
					Service:   "rds",
					Operation: "DescribeDBInstances",
					Params:    describeParams,
				},
				Assertions: []Predicate{
					{Path: "DBInstances[0].DBInstanceClass", Operator: "eq", Value: p.CurrentInstanceClass},
					{Path: "DBInstances[0].PendingModifiedValues", Operator: "empty"},
				},
			},
		},
	}

	return ExecutionPlan{
		Kind:         PlanRollback,
		ActionType:   "rds_resize",
		ProposalHash: h,
		Steps:        steps,
	}
}
