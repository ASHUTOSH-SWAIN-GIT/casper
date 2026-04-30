package proposer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/jerkeyray/starling/tool"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// captured holds the proposal raw bytes + hash that any per-action
// tool emits during a run. The Propose method reads it back after
// agent.Run returns. Per-Proposer (single-flight) state is fine for
// v1 — Propose calls are not concurrent on a single instance.
type captured struct {
	mu   sync.Mutex
	raw  []byte
	hash action.ProposalHash
}

func (c *captured) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.raw = nil
	c.hash = ""
}

func (c *captured) get() ([]byte, action.ProposalHash) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.raw, c.hash
}

func (c *captured) set(raw []byte, h action.ProposalHash) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.raw = raw
	c.hash = h
}

// ──────────────────────────────────────────────────────────────────
// rds_resize action
// ──────────────────────────────────────────────────────────────────

// rdsResizeProposeInput mirrors action.RDSResizeProposal. We don't
// reuse the action type directly because Starling generates the
// tool's JSON Schema from struct tags; this lets us add `jsonschema`
// hints that nudge the model toward emitting all fields. The
// authoritative validation still happens via action.Validate inside
// the tool — Starling's schema is advisory, ours is binding.
type rdsResizeProposeInput struct {
	DBInstanceIdentifier string                  `json:"db_instance_identifier" jsonschema:"description=The RDS instance to modify."`
	Region               string                  `json:"region" jsonschema:"description=AWS region the instance lives in (e.g. us-east-1)."`
	CurrentInstanceClass string                  `json:"current_instance_class" jsonschema:"description=Instance class right now (must match the snapshot)."`
	TargetInstanceClass  string                  `json:"target_instance_class" jsonschema:"description=Instance class to resize to."`
	ApplyImmediately     bool                    `json:"apply_immediately" jsonschema:"description=Must be true for v1 — schedule-window resizes are not supported."`
	SuccessCriteria      rdsResizeProposeSuccess `json:"success_criteria" jsonschema:"description=How we will verify the resize achieved its intent."`
	Reasoning            string                  `json:"reasoning" jsonschema:"description=Short rationale for choosing this target class and success criteria."`
}

type rdsResizeProposeSuccess struct {
	Metric             string  `json:"metric" jsonschema:"description=CloudWatch metric name. v1 only supports CPUUtilization."`
	ThresholdPercent   float64 `json:"threshold_percent" jsonschema:"description=Pass condition: metric average over the verification window must be at or below this percent."`
	VerificationWindow string  `json:"verification_window" jsonschema:"description=Duration string (e.g. 5m) over which we sample the metric after the instance returns to available."`
}

type proposeOutput struct {
	Hash string `json:"proposal_hash"`
}

func buildRDSResizeProposeTool(c *captured) tool.Tool {
	return tool.Typed(
		"propose_rds_resize",
		"Emit exactly one structured RDS resize proposal. This is the only action you can take. Call it exactly once with all fields populated.",
		func(ctx context.Context, in rdsResizeProposeInput) (proposeOutput, error) {
			if in.DBInstanceIdentifier == "" {
				return proposeOutput{}, errors.New("db_instance_identifier required")
			}
			raw, err := json.Marshal(in)
			if err != nil {
				return proposeOutput{}, fmt.Errorf("marshal proposal: %w", err)
			}
			if err := action.Validate(raw); err != nil {
				return proposeOutput{}, fmt.Errorf("schema validation: %w", err)
			}
			h, err := action.Hash(raw)
			if err != nil {
				return proposeOutput{}, fmt.Errorf("hash proposal: %w", err)
			}
			c.set(raw, h)
			return proposeOutput{Hash: string(h)}, nil
		},
	)
}

const rdsResizeSystemPrompt = `You are Casper's RDS resize proposer.

Your only job is to turn a natural-language intent and an infrastructure snapshot into exactly one structured proposal by calling the propose_rds_resize tool.

Hard constraints:
- You must call propose_rds_resize exactly ONCE.
- You must not write any free-form text, explanation, or commentary outside the tool call. Reasoning belongs in the proposal's "reasoning" field.
- Every field on propose_rds_resize is required. Populate them all.
- "current_instance_class" must equal the snapshot's current_instance_class verbatim.
- "apply_immediately" must be true.
- "success_criteria.metric" must be "CPUUtilization" for v1.
- "success_criteria.verification_window" must be a Go duration string (e.g. "5m").

How to choose a target_instance_class:
- For CPU pressure / "needs more headroom" intents: pick the next size up in the same family (e.g. db.r6g.large -> db.r6g.xlarge).
- For "right-size" / "downsize" / cost intents: pick the next size down in the same family.
- Do not change instance family unless the intent specifically asks for memory- or compute-optimized.
- Do not set target equal to current — that's a no-op and will be rejected.

Threshold guidance (success_criteria.threshold_percent):
- For an upsize meant to relieve CPU pressure: target threshold is around 60 (we expect post-resize CPU to be at most 60%).
- For a downsize: target threshold is around 80 (we tolerate moderate CPU).
- Default verification_window is "5m".

Your output is a proposal, not a recommendation. Casper will independently evaluate it against policy, simulate its impact, and gate it on human approval where appropriate. Be conservative when uncertain — a smaller upsize that gets auto-allowed is more useful than a larger one that needs approval.`

// ──────────────────────────────────────────────────────────────────
// rds_create_snapshot action
// ──────────────────────────────────────────────────────────────────

type rdsCreateSnapshotProposeInput struct {
	DBInstanceIdentifier string `json:"db_instance_identifier" jsonschema:"description=The RDS instance to take a snapshot of."`
	Region               string `json:"region" jsonschema:"description=AWS region the instance lives in (e.g. us-east-1)."`
	SnapshotIdentifier   string `json:"snapshot_identifier" jsonschema:"description=Identifier for the new snapshot. Use the convention 'casper-<instance>-<short-tag>' to qualify for auto-approval."`
	Reasoning            string `json:"reasoning" jsonschema:"description=Short rationale for taking the snapshot."`
}

func buildRDSCreateSnapshotProposeTool(c *captured) tool.Tool {
	return tool.Typed(
		"propose_rds_create_snapshot",
		"Emit exactly one structured RDS snapshot-creation proposal. Call this exactly once with all fields populated.",
		func(ctx context.Context, in rdsCreateSnapshotProposeInput) (proposeOutput, error) {
			if in.DBInstanceIdentifier == "" {
				return proposeOutput{}, errors.New("db_instance_identifier required")
			}
			if in.SnapshotIdentifier == "" {
				return proposeOutput{}, errors.New("snapshot_identifier required")
			}
			raw, err := json.Marshal(in)
			if err != nil {
				return proposeOutput{}, fmt.Errorf("marshal proposal: %w", err)
			}
			if err := action.ValidateRDSCreateSnapshot(raw); err != nil {
				return proposeOutput{}, fmt.Errorf("schema validation: %w", err)
			}
			h, err := action.Hash(raw)
			if err != nil {
				return proposeOutput{}, fmt.Errorf("hash proposal: %w", err)
			}
			c.set(raw, h)
			return proposeOutput{Hash: string(h)}, nil
		},
	)
}

// ──────────────────────────────────────────────────────────────────
// rds_modify_backup_retention action
// ──────────────────────────────────────────────────────────────────

type rdsModifyBackupRetentionProposeInput struct {
	DBInstanceIdentifier string `json:"db_instance_identifier" jsonschema:"description=The RDS instance to modify."`
	Region               string `json:"region" jsonschema:"description=AWS region the instance lives in (e.g. us-east-1)."`
	CurrentRetentionDays int    `json:"current_retention_days" jsonschema:"description=Current backup retention period in days (must match the snapshot)."`
	TargetRetentionDays  int    `json:"target_retention_days" jsonschema:"description=Target backup retention period in days. Range 0-35. Setting to 0 disables automated backups."`
	ApplyImmediately     bool   `json:"apply_immediately" jsonschema:"description=Must be true for v1."`
	Reasoning            string `json:"reasoning" jsonschema:"description=Short rationale for the retention change."`
}

func buildRDSModifyBackupRetentionProposeTool(c *captured) tool.Tool {
	return tool.Typed(
		"propose_rds_modify_backup_retention",
		"Emit exactly one structured RDS backup retention change proposal. Call this exactly once with all fields populated.",
		func(ctx context.Context, in rdsModifyBackupRetentionProposeInput) (proposeOutput, error) {
			if in.DBInstanceIdentifier == "" {
				return proposeOutput{}, errors.New("db_instance_identifier required")
			}
			raw, err := json.Marshal(in)
			if err != nil {
				return proposeOutput{}, fmt.Errorf("marshal proposal: %w", err)
			}
			if err := action.ValidateRDSModifyBackupRetention(raw); err != nil {
				return proposeOutput{}, fmt.Errorf("schema validation: %w", err)
			}
			h, err := action.Hash(raw)
			if err != nil {
				return proposeOutput{}, fmt.Errorf("hash proposal: %w", err)
			}
			c.set(raw, h)
			return proposeOutput{Hash: string(h)}, nil
		},
	)
}

const rdsModifyBackupRetentionSystemPrompt = `You are Casper's RDS backup-retention proposer.

Your only job is to turn a natural-language intent and an infrastructure snapshot into exactly one structured proposal by calling the propose_rds_modify_backup_retention tool.

Hard constraints:
- You must call propose_rds_modify_backup_retention exactly ONCE.
- You must not write any free-form text outside the tool call.
- "current_retention_days" must equal the snapshot's current retention verbatim (the operator provides this — typically 1, 7, 14, etc.).
- "target_retention_days" must be in the range 0–35.
- "apply_immediately" must be true.

How to choose target_retention_days:
- If the operator asks to "increase" / "extend" retention without specifying a number, default to 14 (a common safer-side value).
- If the operator names a specific number, use it.
- Setting target to 0 disables automated backups — only do this if the operator explicitly asks to disable backups, and explain in reasoning. Casper's policy will likely deny this and require human approval.
- Do not set target equal to current — that's a no-op and will be rejected.

Casper will independently evaluate the proposal against policy and gate it on human approval where appropriate. Reductions in retention are treated more cautiously than extensions because backups older than the new window are immediately deleted.`

// ──────────────────────────────────────────────────────────────────
// rds_reboot_instance action
// ──────────────────────────────────────────────────────────────────

type rdsRebootInstanceProposeInput struct {
	DBInstanceIdentifier string `json:"db_instance_identifier" jsonschema:"description=The RDS instance to reboot."`
	Region               string `json:"region" jsonschema:"description=AWS region the instance lives in (e.g. us-east-1)."`
	ForceFailover        bool   `json:"force_failover" jsonschema:"description=Only valid for Multi-AZ instances. When true, fails over to the standby; when false, reboots in place. Default false."`
	Reasoning            string `json:"reasoning" jsonschema:"description=Short rationale for the reboot."`
}

func buildRDSRebootInstanceProposeTool(c *captured) tool.Tool {
	return tool.Typed(
		"propose_rds_reboot_instance",
		"Emit exactly one structured RDS reboot proposal. Call this exactly once with all fields populated.",
		func(ctx context.Context, in rdsRebootInstanceProposeInput) (proposeOutput, error) {
			if in.DBInstanceIdentifier == "" {
				return proposeOutput{}, errors.New("db_instance_identifier required")
			}
			raw, err := json.Marshal(in)
			if err != nil {
				return proposeOutput{}, fmt.Errorf("marshal proposal: %w", err)
			}
			if err := action.ValidateRDSRebootInstance(raw); err != nil {
				return proposeOutput{}, fmt.Errorf("schema validation: %w", err)
			}
			h, err := action.Hash(raw)
			if err != nil {
				return proposeOutput{}, fmt.Errorf("hash proposal: %w", err)
			}
			c.set(raw, h)
			return proposeOutput{Hash: string(h)}, nil
		},
	)
}

// ──────────────────────────────────────────────────────────────────
// rds_modify_multi_az action
// ──────────────────────────────────────────────────────────────────

type rdsModifyMultiAZProposeInput struct {
	DBInstanceIdentifier string `json:"db_instance_identifier" jsonschema:"description=The RDS instance to modify."`
	Region               string `json:"region" jsonschema:"description=AWS region the instance lives in (e.g. us-east-1)."`
	CurrentMultiAZ       bool   `json:"current_multi_az" jsonschema:"description=Current Multi-AZ deployment state (must match the snapshot)."`
	TargetMultiAZ        bool   `json:"target_multi_az" jsonschema:"description=Target Multi-AZ deployment state."`
	ApplyImmediately     bool   `json:"apply_immediately" jsonschema:"description=Must be true for v1."`
	Reasoning            string `json:"reasoning" jsonschema:"description=Short rationale for the Multi-AZ change."`
}

func buildRDSModifyMultiAZProposeTool(c *captured) tool.Tool {
	return tool.Typed(
		"propose_rds_modify_multi_az",
		"Emit exactly one structured RDS Multi-AZ toggle proposal. Call this exactly once with all fields populated.",
		func(ctx context.Context, in rdsModifyMultiAZProposeInput) (proposeOutput, error) {
			if in.DBInstanceIdentifier == "" {
				return proposeOutput{}, errors.New("db_instance_identifier required")
			}
			raw, err := json.Marshal(in)
			if err != nil {
				return proposeOutput{}, fmt.Errorf("marshal proposal: %w", err)
			}
			if err := action.ValidateRDSModifyMultiAZ(raw); err != nil {
				return proposeOutput{}, fmt.Errorf("schema validation: %w", err)
			}
			h, err := action.Hash(raw)
			if err != nil {
				return proposeOutput{}, fmt.Errorf("hash proposal: %w", err)
			}
			c.set(raw, h)
			return proposeOutput{Hash: string(h)}, nil
		},
	)
}

// ──────────────────────────────────────────────────────────────────
// rds_storage_grow action (IRREVERSIBLE)
// ──────────────────────────────────────────────────────────────────

type rdsStorageGrowProposeInput struct {
	DBInstanceIdentifier      string `json:"db_instance_identifier" jsonschema:"description=The RDS instance to grow."`
	Region                    string `json:"region" jsonschema:"description=AWS region the instance lives in (e.g. us-east-1)."`
	CurrentAllocatedStorageGB int    `json:"current_allocated_storage_gb" jsonschema:"description=Current allocated storage in GB (must match the snapshot)."`
	TargetAllocatedStorageGB  int    `json:"target_allocated_storage_gb" jsonschema:"description=Target allocated storage in GB. Must be greater than current. THIS IS IRREVERSIBLE — RDS cannot shrink storage."`
	ApplyImmediately          bool   `json:"apply_immediately" jsonschema:"description=Must be true."`
	Reasoning                 string `json:"reasoning" jsonschema:"description=Short rationale. The reasoning should acknowledge that this action is irreversible."`
}

func buildRDSStorageGrowProposeTool(c *captured) tool.Tool {
	return tool.Typed(
		"propose_rds_storage_grow",
		"Emit exactly one structured RDS storage-grow proposal. This action is IRREVERSIBLE. Call this exactly once with all fields populated.",
		func(ctx context.Context, in rdsStorageGrowProposeInput) (proposeOutput, error) {
			if in.DBInstanceIdentifier == "" {
				return proposeOutput{}, errors.New("db_instance_identifier required")
			}
			raw, err := json.Marshal(in)
			if err != nil {
				return proposeOutput{}, fmt.Errorf("marshal proposal: %w", err)
			}
			if err := action.ValidateRDSStorageGrow(raw); err != nil {
				return proposeOutput{}, fmt.Errorf("schema validation: %w", err)
			}
			h, err := action.Hash(raw)
			if err != nil {
				return proposeOutput{}, fmt.Errorf("hash proposal: %w", err)
			}
			c.set(raw, h)
			return proposeOutput{Hash: string(h)}, nil
		},
	)
}

// ──────────────────────────────────────────────────────────────────
// rds_delete_snapshot action (IRREVERSIBLE)
// ──────────────────────────────────────────────────────────────────

type rdsDeleteSnapshotProposeInput struct {
	SnapshotIdentifier string `json:"snapshot_identifier" jsonschema:"description=The snapshot to delete. THIS IS IRREVERSIBLE — once deleted, the data is gone."`
	Region             string `json:"region" jsonschema:"description=AWS region the snapshot lives in (e.g. us-east-1)."`
	Reasoning          string `json:"reasoning" jsonschema:"description=Short rationale. The reasoning should acknowledge that deletion is irreversible."`
}

func buildRDSDeleteSnapshotProposeTool(c *captured) tool.Tool {
	return tool.Typed(
		"propose_rds_delete_snapshot",
		"Emit exactly one structured RDS snapshot deletion proposal. THIS ACTION IS IRREVERSIBLE. Call this exactly once with all fields populated.",
		func(ctx context.Context, in rdsDeleteSnapshotProposeInput) (proposeOutput, error) {
			if in.SnapshotIdentifier == "" {
				return proposeOutput{}, errors.New("snapshot_identifier required")
			}
			raw, err := json.Marshal(in)
			if err != nil {
				return proposeOutput{}, fmt.Errorf("marshal proposal: %w", err)
			}
			if err := action.ValidateRDSDeleteSnapshot(raw); err != nil {
				return proposeOutput{}, fmt.Errorf("schema validation: %w", err)
			}
			h, err := action.Hash(raw)
			if err != nil {
				return proposeOutput{}, fmt.Errorf("hash proposal: %w", err)
			}
			c.set(raw, h)
			return proposeOutput{Hash: string(h)}, nil
		},
	)
}

// ──────────────────────────────────────────────────────────────────
// rds_create_read_replica action
// ──────────────────────────────────────────────────────────────────

type rdsCreateReadReplicaProposeInput struct {
	SourceDBInstanceIdentifier  string `json:"source_db_instance_identifier" jsonschema:"description=The primary RDS instance to replicate from."`
	ReplicaDBInstanceIdentifier string `json:"replica_db_instance_identifier" jsonschema:"description=Identifier for the new replica. Use 'casper-<source>-<short-tag>' to qualify for auto-approval."`
	Region                      string `json:"region" jsonschema:"description=AWS region the source instance lives in."`
	ReplicaInstanceClass        string `json:"replica_instance_class" jsonschema:"description=Instance class for the replica (typically equal to or smaller than the source's class)."`
	Reasoning                   string `json:"reasoning" jsonschema:"description=Short rationale. Note that a replica is a full second instance, billed per hour."`
}

func buildRDSCreateReadReplicaProposeTool(c *captured) tool.Tool {
	return tool.Typed(
		"propose_rds_create_read_replica",
		"Emit exactly one structured RDS read-replica creation proposal. Call this exactly once with all fields populated.",
		func(ctx context.Context, in rdsCreateReadReplicaProposeInput) (proposeOutput, error) {
			if in.SourceDBInstanceIdentifier == "" {
				return proposeOutput{}, errors.New("source_db_instance_identifier required")
			}
			if in.ReplicaDBInstanceIdentifier == "" {
				return proposeOutput{}, errors.New("replica_db_instance_identifier required")
			}
			raw, err := json.Marshal(in)
			if err != nil {
				return proposeOutput{}, fmt.Errorf("marshal proposal: %w", err)
			}
			if err := action.ValidateRDSCreateReadReplica(raw); err != nil {
				return proposeOutput{}, fmt.Errorf("schema validation: %w", err)
			}
			h, err := action.Hash(raw)
			if err != nil {
				return proposeOutput{}, fmt.Errorf("hash proposal: %w", err)
			}
			c.set(raw, h)
			return proposeOutput{Hash: string(h)}, nil
		},
	)
}

const rdsCreateReadReplicaSystemPrompt = `You are Casper's RDS read-replica proposer.

Your only job is to turn a natural-language intent and an infrastructure snapshot into exactly one structured proposal by calling the propose_rds_create_read_replica tool.

Hard constraints:
- You must call propose_rds_create_read_replica exactly ONCE.
- "source_db_instance_identifier" must match the snapshot's instance identifier verbatim.
- "replica_db_instance_identifier" must NOT equal the source — that would be a name collision.
- "replica_instance_class" must start with "db." (it's an RDS instance class).

How to choose replica_db_instance_identifier:
- Use "casper-<source>-replica" or "casper-<source>-<short-tag>" to qualify for auto-approval.
- Pick a tag that conveys purpose: e.g. "casper-orders-prod-analytics" or "casper-orders-prod-reports".

How to choose replica_instance_class:
- Default to the source instance's class (matches read capacity).
- For analytics replicas, a smaller class can be reasonable to save cost.

Be aware: replicas are full-priced second instances. Mention this in reasoning so the human reviewer can weigh the cost.`

const rdsDeleteSnapshotSystemPrompt = `You are Casper's RDS snapshot-deletion proposer.

This action is IRREVERSIBLE: once a snapshot is deleted, AWS does not retain it — the data is permanently gone. Casper's policy defaults to deny and never auto-allows. Even valid proposals always require human approval.

Hard constraints:
- You must call propose_rds_delete_snapshot exactly ONCE.
- The snapshot must exist and have status "available".
- Do not propose deleting a snapshot whose identifier contains "prod" — Casper's policy hard-denies these.
- Acknowledge the irreversibility in "reasoning". Example: "Snapshot is 90 days old, predates the migration; manual cleanup. Deletion is irreversible — verified the snapshot is not referenced by any active restore."

Be conservative — when in doubt, prefer NOT to propose deletion. The cost of leaving an unused snapshot is small ($0.095/GB-month); the cost of deleting one that's still needed is unbounded.`

const rdsStorageGrowSystemPrompt = `You are Casper's RDS storage-grow proposer.

This action is IRREVERSIBLE: AWS does not support shrinking allocated storage on an existing RDS instance. Your proposal will be evaluated by a policy that defaults to deny — even valid proposals require explicit human approval.

Hard constraints:
- You must call propose_rds_storage_grow exactly ONCE.
- "current_allocated_storage_gb" must equal the snapshot's current_allocated_storage_gb verbatim.
- "target_allocated_storage_gb" must be GREATER than current (no shrink).
- "apply_immediately" must be true.

Guidance for choosing target:
- Default to a modest, deliberate increase (e.g. +20% or +50GB, whichever is larger). The policy denies anything more than 10x current.
- The policy auto-decreases to needs_approval (rather than deny) when the increase is ≤100GB AND ≤2x current. Stay within those bounds for the smoothest path.
- Acknowledge the irreversibility in "reasoning". Example: "Disk usage at 95% on a 100GB volume; growing to 200GB to provide ~6 months of headroom. Storage growth is irreversible; recovery to a smaller size requires dump+restore."

Be conservative — over-allocating is harder to fix than under-allocating because you can always grow again later but you cannot shrink.`

const rdsModifyMultiAZSystemPrompt = `You are Casper's RDS Multi-AZ toggle proposer.

Your only job is to turn a natural-language intent and an infrastructure snapshot into exactly one structured proposal by calling the propose_rds_modify_multi_az tool.

Hard constraints:
- You must call propose_rds_modify_multi_az exactly ONCE.
- You must not write any free-form text outside the tool call.
- "current_multi_az" must equal the snapshot's multi_az verbatim.
- "apply_immediately" must be true.
- Do not set target equal to current — that's a no-op and will be rejected.

Guidance:
- "enable HA / multi-AZ" / "make it highly available" → target_multi_az=true
- "save money on this DB" / "downgrade to single-AZ" / "non-critical instance" → target_multi_az=false (will need approval)
- Note: enabling Multi-AZ roughly doubles instance cost; mention this in reasoning.

Casper's policy auto-allows enabling Multi-AZ (adds redundancy) but requires approval for disabling it (removes redundancy).`

const rdsRebootInstanceSystemPrompt = `You are Casper's RDS reboot proposer.

Your only job is to turn a natural-language intent and an infrastructure snapshot into exactly one structured proposal by calling the propose_rds_reboot_instance tool.

Hard constraints:
- You must call propose_rds_reboot_instance exactly ONCE.
- You must not write any free-form text outside the tool call.
- Set "force_failover" to true ONLY if the operator explicitly asks to test failover. Default to false.
- "force_failover" is meaningless on Single-AZ instances; if the snapshot shows multi_az=false, set force_failover=false unconditionally.

Casper's policy currently denies force_failover requests until the simulator can confirm the instance is Multi-AZ. Set force_failover=false unless the operator is specifically requesting a failover test.`

const rdsCreateSnapshotSystemPrompt = `You are Casper's RDS snapshot-creation proposer.

Your only job is to turn a natural-language intent and an infrastructure snapshot into exactly one structured proposal by calling the propose_rds_create_snapshot tool.

Hard constraints:
- You must call propose_rds_create_snapshot exactly ONCE.
- You must not write any free-form text outside the tool call. Reasoning belongs in the proposal's "reasoning" field.
- Every field on propose_rds_create_snapshot is required. Populate them all.

How to choose a snapshot_identifier:
- Use the convention "casper-<instance-id>-<short-tag>" so the policy engine recognizes it as casper-managed and auto-allows the action.
- The <short-tag> should be a brief, lowercase descriptor (e.g. "preupgrade", "manual-2026-04-30", "before-resize").
- Do NOT set snapshot_identifier equal to db_instance_identifier — that's an obvious typo and will be denied by policy.
- Snapshot identifiers must start with a letter and contain only letters, digits, and hyphens.

Casper will independently evaluate the proposal against policy and gate it on human approval where appropriate. Be conservative — pick descriptive identifiers, briefly explain the reasoning.`
