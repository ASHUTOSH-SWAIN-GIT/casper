package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/identity"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/plan"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/policy"
)

// runnable bundles the per-action operations the trust-layer pipeline
// needs into a single struct. It exists so runProposal can stay
// action-agnostic — the dispatch happens once, in buildRunnable, and
// every step after that calls through closures.
//
// Adding a new action means adding a case to buildRunnable that returns
// a fully-populated runnable. Nothing else in the run pipeline changes.
type runnable struct {
	// ActionType is the canonical type string ("rds_resize", etc.).
	ActionType string

	// Hash is the proposal hash (canonical sha256 over the proposal
	// bytes). Used everywhere as the predictability anchor.
	Hash action.ProposalHash

	// Region is the AWS region this action targets. Used to load AWS
	// config and to namespace resources.
	Region string

	// ProposalAuditPayload is what gets written to the KindProposed
	// audit event. Must include action_type so the audit chain is
	// self-describing.
	ProposalAuditPayload map[string]any

	// EvaluatePolicy runs the action's Rego rules against the proposal
	// and returns the verdict.
	EvaluatePolicy func(ctx context.Context, e *policy.Engine) (policy.Verdict, error)

	// Compile turns the proposal into forward + rollback ExecutionPlans.
	Compile func() (forward, rollback plan.ExecutionPlan)

	// SessionPolicy returns the per-action IAM policy that should be
	// passed to STS:AssumeRole when the identity broker is configured.
	SessionPolicy func(accountID string) identity.SessionPolicy
}

// buildRunnable detects the action type, validates against the right
// schema, decodes into the typed struct, and constructs a runnable
// bound to that proposal.
func buildRunnable(raw []byte) (*runnable, error) {
	actionType, err := detectActionType(raw)
	if err != nil {
		return nil, err
	}
	if err := validateForActionType(raw, actionType); err != nil {
		return nil, fmt.Errorf("invalid proposal: %w", err)
	}
	h, err := action.Hash(raw)
	if err != nil {
		return nil, err
	}

	switch actionType {
	case "rds_resize":
		var p action.RDSResizeProposal
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("decode proposal: %w", err)
		}
		return &runnable{
			ActionType: "rds_resize",
			Hash:       h,
			Region:     p.Region,
			ProposalAuditPayload: map[string]any{
				"action_type":            "rds_resize",
				"db_instance_identifier": p.DBInstanceIdentifier,
				"region":                 p.Region,
				"current_instance_class": p.CurrentInstanceClass,
				"target_instance_class":  p.TargetInstanceClass,
			},
			EvaluatePolicy: func(ctx context.Context, e *policy.Engine) (policy.Verdict, error) {
				return e.EvaluateRDSResize(ctx, p)
			},
			Compile: func() (plan.ExecutionPlan, plan.ExecutionPlan) {
				return plan.CompileRDSResize(p, h)
			},
			SessionPolicy: func(accountID string) identity.SessionPolicy {
				return identity.BuildRDSResizePolicy(p, accountID)
			},
		}, nil

	case "rds_create_snapshot":
		var p action.RDSCreateSnapshotProposal
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("decode proposal: %w", err)
		}
		return &runnable{
			ActionType: "rds_create_snapshot",
			Hash:       h,
			Region:     p.Region,
			ProposalAuditPayload: map[string]any{
				"action_type":            "rds_create_snapshot",
				"db_instance_identifier": p.DBInstanceIdentifier,
				"region":                 p.Region,
				"snapshot_identifier":    p.SnapshotIdentifier,
			},
			EvaluatePolicy: func(ctx context.Context, e *policy.Engine) (policy.Verdict, error) {
				return e.EvaluateRDSCreateSnapshot(ctx, p)
			},
			Compile: func() (plan.ExecutionPlan, plan.ExecutionPlan) {
				return plan.CompileRDSCreateSnapshot(p, h)
			},
			SessionPolicy: func(accountID string) identity.SessionPolicy {
				return identity.BuildRDSCreateSnapshotPolicy(p, accountID)
			},
		}, nil

	case "rds_modify_backup_retention":
		var p action.RDSModifyBackupRetentionProposal
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("decode proposal: %w", err)
		}
		return &runnable{
			ActionType: "rds_modify_backup_retention",
			Hash:       h,
			Region:     p.Region,
			ProposalAuditPayload: map[string]any{
				"action_type":            "rds_modify_backup_retention",
				"db_instance_identifier": p.DBInstanceIdentifier,
				"region":                 p.Region,
				"current_retention_days": p.CurrentRetentionDays,
				"target_retention_days":  p.TargetRetentionDays,
			},
			EvaluatePolicy: func(ctx context.Context, e *policy.Engine) (policy.Verdict, error) {
				return e.EvaluateRDSModifyBackupRetention(ctx, p)
			},
			Compile: func() (plan.ExecutionPlan, plan.ExecutionPlan) {
				return plan.CompileRDSModifyBackupRetention(p, h)
			},
			SessionPolicy: func(accountID string) identity.SessionPolicy {
				return identity.BuildRDSModifyBackupRetentionPolicy(p, accountID)
			},
		}, nil

	case "rds_reboot_instance":
		var p action.RDSRebootInstanceProposal
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("decode proposal: %w", err)
		}
		return &runnable{
			ActionType: "rds_reboot_instance",
			Hash:       h,
			Region:     p.Region,
			ProposalAuditPayload: map[string]any{
				"action_type":            "rds_reboot_instance",
				"db_instance_identifier": p.DBInstanceIdentifier,
				"region":                 p.Region,
				"force_failover":         p.ForceFailover,
			},
			EvaluatePolicy: func(ctx context.Context, e *policy.Engine) (policy.Verdict, error) {
				return e.EvaluateRDSRebootInstance(ctx, p)
			},
			Compile: func() (plan.ExecutionPlan, plan.ExecutionPlan) {
				return plan.CompileRDSRebootInstance(p, h)
			},
			SessionPolicy: func(accountID string) identity.SessionPolicy {
				return identity.BuildRDSRebootInstancePolicy(p, accountID)
			},
		}, nil

	case "rds_modify_multi_az":
		var p action.RDSModifyMultiAZProposal
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("decode proposal: %w", err)
		}
		return &runnable{
			ActionType: "rds_modify_multi_az",
			Hash:       h,
			Region:     p.Region,
			ProposalAuditPayload: map[string]any{
				"action_type":            "rds_modify_multi_az",
				"db_instance_identifier": p.DBInstanceIdentifier,
				"region":                 p.Region,
				"current_multi_az":       p.CurrentMultiAZ,
				"target_multi_az":        p.TargetMultiAZ,
			},
			EvaluatePolicy: func(ctx context.Context, e *policy.Engine) (policy.Verdict, error) {
				return e.EvaluateRDSModifyMultiAZ(ctx, p)
			},
			Compile: func() (plan.ExecutionPlan, plan.ExecutionPlan) {
				return plan.CompileRDSModifyMultiAZ(p, h)
			},
			SessionPolicy: func(accountID string) identity.SessionPolicy {
				return identity.BuildRDSModifyMultiAZPolicy(p, accountID)
			},
		}, nil

	case "rds_storage_grow":
		var p action.RDSStorageGrowProposal
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("decode proposal: %w", err)
		}
		return &runnable{
			ActionType: "rds_storage_grow",
			Hash:       h,
			Region:     p.Region,
			ProposalAuditPayload: map[string]any{
				"action_type":                 "rds_storage_grow",
				"db_instance_identifier":      p.DBInstanceIdentifier,
				"region":                      p.Region,
				"current_allocated_storage_gb": p.CurrentAllocatedStorageGB,
				"target_allocated_storage_gb":  p.TargetAllocatedStorageGB,
				"reversibility":               "irreversible", // surfaced for auditors
			},
			EvaluatePolicy: func(ctx context.Context, e *policy.Engine) (policy.Verdict, error) {
				return e.EvaluateRDSStorageGrow(ctx, p)
			},
			Compile: func() (plan.ExecutionPlan, plan.ExecutionPlan) {
				return plan.CompileRDSStorageGrow(p, h)
			},
			SessionPolicy: func(accountID string) identity.SessionPolicy {
				return identity.BuildRDSStorageGrowPolicy(p, accountID)
			},
		}, nil

	case "rds_delete_snapshot":
		var p action.RDSDeleteSnapshotProposal
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("decode proposal: %w", err)
		}
		return &runnable{
			ActionType: "rds_delete_snapshot",
			Hash:       h,
			Region:     p.Region,
			ProposalAuditPayload: map[string]any{
				"action_type":         "rds_delete_snapshot",
				"snapshot_identifier": p.SnapshotIdentifier,
				"region":              p.Region,
				"reversibility":       "irreversible",
			},
			EvaluatePolicy: func(ctx context.Context, e *policy.Engine) (policy.Verdict, error) {
				return e.EvaluateRDSDeleteSnapshot(ctx, p)
			},
			Compile: func() (plan.ExecutionPlan, plan.ExecutionPlan) {
				return plan.CompileRDSDeleteSnapshot(p, h)
			},
			SessionPolicy: func(accountID string) identity.SessionPolicy {
				return identity.BuildRDSDeleteSnapshotPolicy(p, accountID)
			},
		}, nil

	case "rds_create_read_replica":
		var p action.RDSCreateReadReplicaProposal
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("decode proposal: %w", err)
		}
		return &runnable{
			ActionType: "rds_create_read_replica",
			Hash:       h,
			Region:     p.Region,
			ProposalAuditPayload: map[string]any{
				"action_type":                    "rds_create_read_replica",
				"source_db_instance_identifier":  p.SourceDBInstanceIdentifier,
				"replica_db_instance_identifier": p.ReplicaDBInstanceIdentifier,
				"region":                         p.Region,
				"replica_instance_class":         p.ReplicaInstanceClass,
			},
			EvaluatePolicy: func(ctx context.Context, e *policy.Engine) (policy.Verdict, error) {
				return e.EvaluateRDSCreateReadReplica(ctx, p)
			},
			Compile: func() (plan.ExecutionPlan, plan.ExecutionPlan) {
				return plan.CompileRDSCreateReadReplica(p, h)
			},
			SessionPolicy: func(accountID string) identity.SessionPolicy {
				return identity.BuildRDSCreateReadReplicaPolicy(p, accountID)
			},
		}, nil

	default:
		return nil, fmt.Errorf("no runner registered for action type %q", actionType)
	}
}
