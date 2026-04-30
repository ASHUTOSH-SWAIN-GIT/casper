package casper.rds_delete_snapshot

import rego.v1

# Deleting a snapshot is irreversible — the snapshot data is permanently
# discarded with no AWS-side grace period or tombstone. The default is
# DENY (fail-closed). The only override is needs_approval — never auto-allow.
#
# v2 should consult the simulator for "snapshot age" and "snapshot is
# referenced by an active restore" — until then, the policy makes a
# narrow override based on naming convention only.

default result := {
	"decision": "deny",
	"reason":   "snapshot deletion is irreversible — explicit human approval required, default is to refuse"
}

# Override to needs_approval (still requires a human) for snapshots
# whose identifier matches the casper-managed naming convention. The
# rationale: snapshots created by Casper itself are more likely to be
# safe to delete than ad-hoc snapshots an operator made deliberately.
result := {
	"decision": "needs_approval",
	"reason":   "snapshot identifier matches casper-managed convention (irreversible — requires human approval)"
} if {
	regex.match(`^casper-[a-zA-Z0-9-]+$`, input.proposal.snapshot_identifier)
}

# Hard deny: deleting snapshots that look like production-marker names.
# This is a defensive heuristic — anything containing "prod" in the
# identifier is too dangerous to delete via casperctl. Operator must
# rename the snapshot or do this through a different channel.
result := {
	"decision": "deny",
	"reason":   "snapshot identifier contains 'prod' — too risky for automated deletion, even with approval"
} if {
	regex.match(`(?i)prod`, input.proposal.snapshot_identifier)
}
