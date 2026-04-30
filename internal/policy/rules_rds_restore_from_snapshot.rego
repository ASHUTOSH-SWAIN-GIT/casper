package casper.rds_restore_from_snapshot

import rego.v1

# Restoring from a snapshot is additive (the snapshot is unchanged) and
# reversible (delete the new instance). The risk profile is mostly cost
# (a full new instance) and operational (cutting traffic over to the
# restored copy). Default to needs_approval; auto-allow casper-managed
# naming.

default result := {
	"decision": "needs_approval",
	"reason":   "default policy: snapshot restore creates a full new instance and requires human approval"
}

# Auto-allow: target identifier follows casper-managed naming.
result := {
	"decision": "allow",
	"reason":   "target identifier matches casper-managed convention"
} if {
	regex.match(`^casper-[a-zA-Z0-9-]+$`, input.proposal.target_db_instance_identifier)
}

# Hard deny: target identifier collides with the source snapshot identifier.
result := {
	"decision": "deny",
	"reason":   "target_db_instance_identifier equals snapshot_identifier — would overload naming"
} if {
	input.proposal.target_db_instance_identifier == input.proposal.snapshot_identifier
}
