// Package proposer is Casper's only LLM-driven component.
//
// It turns a natural-language intent + a deterministic infra snapshot
// into a structured RDSResizeProposal — and nothing else. No multi-turn
// reasoning, no live AWS reads at agent-time, no tools other than the
// one that emits a proposal. The agent's output is upstream of every
// trust-layer check; downstream code treats it as data, not as a
// trusted artifact.
//
// The agent is built on Starling, which gives us a hash-chained event
// log per run plus byte-for-byte replay — so any proposal Casper acts
// on can be verified against the recorded model run that produced it.
package proposer

// Snapshot is the infra state passed to the proposer as prompt context.
// It is built deterministically *before* the agent runs (either by
// fetching from AWS or hand-authored for testing) — the agent itself
// has no live cloud access.
//
// Fields here are the minimal surface the proposer needs to reason
// about an RDS resize. Adding fields is a deliberate change; the
// proposer's prompt should be updated alongside.
type Snapshot struct {
	// Instance fields (rds_instance resource).
	DBInstanceIdentifier  string  `json:"db_instance_identifier,omitempty"`
	Region                string  `json:"region"`
	CurrentInstanceClass  string  `json:"current_instance_class,omitempty"`
	Engine                string  `json:"engine,omitempty"`
	EngineVersion         string  `json:"engine_version,omitempty"`
	Status                string  `json:"status,omitempty"`
	MultiAZ               bool    `json:"multi_az,omitempty"`
	AllocatedStorageGB    int32   `json:"allocated_storage_gb,omitempty"`
	BackupRetentionDays   int32   `json:"backup_retention_days,omitempty"`
	RecentCPUUtilization  float64 `json:"recent_cpu_utilization,omitempty"`

	// Snapshot fields (rds_snapshot resource).
	SnapshotIdentifier         string `json:"snapshot_identifier,omitempty"`
	SourceDBInstanceIdentifier string `json:"source_db_instance_identifier,omitempty"`
	SnapshotStatus             string `json:"snapshot_status,omitempty"`
}

// Request is the single-file input to `casperctl propose`. Intent is
// the natural-language ask (verbatim from the user, never the LLM);
// Snapshot is the infra state the proposer reasons against.
type Request struct {
	Intent   string   `json:"intent"`
	Snapshot Snapshot `json:"snapshot"`
}
