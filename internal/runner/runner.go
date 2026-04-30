// Package runner builds the per-action operations the trust-layer
// pipeline executes against a proposal: schema validation, typed
// decoding, policy evaluation, plan compilation, and IAM session
// policy generation.
//
// The single entry point is Build, which detects the action type from
// proposal shape, validates against the corresponding schema, and
// returns a Runnable bound to the typed proposal. Both casperctl and
// casperd consume this — keeping action dispatch in one place means
// new actions are added once, not twice.
package runner

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/identity"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/plan"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/policy"
)

// Runnable bundles the per-action closures the trust-layer pipeline
// needs. The dispatch happens once, in Build; everything downstream
// (CLI run, HTTP execute) calls through the same closures.
type Runnable struct {
	ActionType           string
	Hash                 action.ProposalHash
	Region               string
	ProposalAuditPayload map[string]any

	EvaluatePolicy func(ctx context.Context, e *policy.Engine) (policy.Verdict, error)
	Compile        func() (forward, rollback plan.ExecutionPlan)
	SessionPolicy  func(accountID string) identity.SessionPolicy
}

// Build detects the action type from the raw JSON, validates against
// the matching schema, and constructs a Runnable.
func Build(raw []byte) (*Runnable, error) {
	actionType, err := DetectActionType(raw)
	if err != nil {
		return nil, err
	}
	if err := ValidateForActionType(raw, actionType); err != nil {
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
		return &Runnable{
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
		return &Runnable{
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
		return &Runnable{
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
		return &Runnable{
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
		return &Runnable{
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
		return &Runnable{
			ActionType: "rds_storage_grow",
			Hash:       h,
			Region:     p.Region,
			ProposalAuditPayload: map[string]any{
				"action_type":                  "rds_storage_grow",
				"db_instance_identifier":       p.DBInstanceIdentifier,
				"region":                       p.Region,
				"current_allocated_storage_gb": p.CurrentAllocatedStorageGB,
				"target_allocated_storage_gb":  p.TargetAllocatedStorageGB,
				"reversibility":                "irreversible",
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
		return &Runnable{
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
		return &Runnable{
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

	case "rds_modify_engine_version":
		var p action.RDSModifyEngineVersionProposal
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("decode proposal: %w", err)
		}
		return &Runnable{
			ActionType: "rds_modify_engine_version",
			Hash:       h,
			Region:     p.Region,
			ProposalAuditPayload: map[string]any{
				"action_type":                 "rds_modify_engine_version",
				"db_instance_identifier":      p.DBInstanceIdentifier,
				"region":                      p.Region,
				"current_engine_version":      p.CurrentEngineVersion,
				"target_engine_version":       p.TargetEngineVersion,
				"allow_major_version_upgrade": p.AllowMajorVersionUpgrade,
				"reversibility":               "irreversible",
			},
			EvaluatePolicy: func(ctx context.Context, e *policy.Engine) (policy.Verdict, error) {
				return e.EvaluateRDSModifyEngineVersion(ctx, p)
			},
			Compile: func() (plan.ExecutionPlan, plan.ExecutionPlan) {
				return plan.CompileRDSModifyEngineVersion(p, h)
			},
			SessionPolicy: func(accountID string) identity.SessionPolicy {
				return identity.BuildRDSModifyEngineVersionPolicy(p, accountID)
			},
		}, nil

	case "rds_restore_from_snapshot":
		var p action.RDSRestoreFromSnapshotProposal
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("decode proposal: %w", err)
		}
		return &Runnable{
			ActionType: "rds_restore_from_snapshot",
			Hash:       h,
			Region:     p.Region,
			ProposalAuditPayload: map[string]any{
				"action_type":                   "rds_restore_from_snapshot",
				"snapshot_identifier":           p.SnapshotIdentifier,
				"target_db_instance_identifier": p.TargetDBInstanceIdentifier,
				"region":                        p.Region,
				"target_instance_class":         p.TargetInstanceClass,
			},
			EvaluatePolicy: func(ctx context.Context, e *policy.Engine) (policy.Verdict, error) {
				return e.EvaluateRDSRestoreFromSnapshot(ctx, p)
			},
			Compile: func() (plan.ExecutionPlan, plan.ExecutionPlan) {
				return plan.CompileRDSRestoreFromSnapshot(p, h)
			},
			SessionPolicy: func(accountID string) identity.SessionPolicy {
				return identity.BuildRDSRestoreFromSnapshotPolicy(p, accountID)
			},
		}, nil
	}

	return nil, fmt.Errorf("no runner registered for action type %q", actionType)
}

// DetectActionType picks the action type from a proposal's shape. If
// the proposal carries an explicit "action_type" field that wins;
// otherwise we discriminate by which unique field is present. Adding
// a new action means adding a case here.
func DetectActionType(raw []byte) (string, error) {
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return "", fmt.Errorf("parse proposal: %w", err)
	}
	if v, ok := probe["action_type"].(string); ok && v != "" {
		return v, nil
	}
	switch {
	case probe["snapshot_identifier"] != nil && probe["target_db_instance_identifier"] != nil:
		return "rds_restore_from_snapshot", nil
	case probe["target_instance_class"] != nil && probe["snapshot_identifier"] == nil:
		return "rds_resize", nil
	case probe["snapshot_identifier"] != nil && probe["db_instance_identifier"] == nil:
		return "rds_delete_snapshot", nil
	case probe["snapshot_identifier"] != nil:
		return "rds_create_snapshot", nil
	case probe["target_retention_days"] != nil:
		return "rds_modify_backup_retention", nil
	case probe["force_failover"] != nil:
		return "rds_reboot_instance", nil
	case probe["target_multi_az"] != nil:
		return "rds_modify_multi_az", nil
	case probe["target_allocated_storage_gb"] != nil:
		return "rds_storage_grow", nil
	case probe["replica_db_instance_identifier"] != nil:
		return "rds_create_read_replica", nil
	case probe["target_engine_version"] != nil:
		return "rds_modify_engine_version", nil
	}
	return "", fmt.Errorf("could not detect action type from proposal shape (no recognizable discriminator field)")
}

// ValidateForActionType dispatches to the correct schema validator
// based on the detected action type.
func ValidateForActionType(raw []byte, actionType string) error {
	switch actionType {
	case "rds_resize":
		return action.Validate(raw)
	case "rds_create_snapshot":
		return action.ValidateRDSCreateSnapshot(raw)
	case "rds_modify_backup_retention":
		return action.ValidateRDSModifyBackupRetention(raw)
	case "rds_reboot_instance":
		return action.ValidateRDSRebootInstance(raw)
	case "rds_modify_multi_az":
		return action.ValidateRDSModifyMultiAZ(raw)
	case "rds_storage_grow":
		return action.ValidateRDSStorageGrow(raw)
	case "rds_delete_snapshot":
		return action.ValidateRDSDeleteSnapshot(raw)
	case "rds_create_read_replica":
		return action.ValidateRDSCreateReadReplica(raw)
	case "rds_modify_engine_version":
		return action.ValidateRDSModifyEngineVersion(raw)
	case "rds_restore_from_snapshot":
		return action.ValidateRDSRestoreFromSnapshot(raw)
	default:
		return fmt.Errorf("no validator registered for action type %q", actionType)
	}
}
