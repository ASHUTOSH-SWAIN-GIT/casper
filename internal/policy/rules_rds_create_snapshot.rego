package casper.rds_create_snapshot

import rego.v1

# Snapshots are additive — taking one doesn't change the source instance,
# and rollback simply deletes the just-created snapshot. The risk profile
# is mostly about identifier collisions and snapshot-storage cost. Default
# policy still requires human approval, with one auto-allow case below.

default result := {
	"decision": "needs_approval",
	"reason":   "default policy: snapshot creation requires human approval"
}

# Auto-allow: snapshots whose identifier follows the casper-managed
# convention "casper-<instance>-<timestamp-or-tag>". This signals the
# snapshot was created by the trust layer (not a manual ad-hoc one) and
# is safe to take.
result := {
	"decision": "allow",
	"reason":   "snapshot identifier matches casper-managed convention"
} if {
	regex.match(`^casper-[a-zA-Z0-9-]+$`, input.proposal.snapshot_identifier)
	startswith(input.proposal.snapshot_identifier, concat("-", ["casper", input.proposal.db_instance_identifier]))
}

# Deny: snapshot identifier conflicts with the source instance's name
# (would shadow the instance in some AWS UIs and is almost certainly a typo).
result := {
	"decision": "deny",
	"reason":   "snapshot identifier equals source instance identifier"
} if {
	input.proposal.snapshot_identifier == input.proposal.db_instance_identifier
}
