package casper.rds_create_read_replica

import rego.v1

# Read replicas are additive (the source instance is unchanged) and
# reversible (delete the replica). The risk profile is mostly cost:
# a replica is a full-priced second instance running indefinitely.
# Default to needs_approval; auto-allow for casper-managed naming.

default result := {
	"decision": "needs_approval",
	"reason":   "default policy: read replica creation requires human approval (a replica is a full second instance, billed per hour)"
}

# Auto-allow: replica named with the casper-managed convention.
result := {
	"decision": "allow",
	"reason":   "replica identifier matches casper-managed convention"
} if {
	regex.match(`^casper-[a-zA-Z0-9-]+$`, input.proposal.replica_db_instance_identifier)
	startswith(input.proposal.replica_db_instance_identifier, concat("-", ["casper", input.proposal.source_db_instance_identifier]))
}

# Hard deny: replica identifier collides with source.
result := {
	"decision": "deny",
	"reason":   "replica_db_instance_identifier equals source_db_instance_identifier — would shadow the source"
} if {
	input.proposal.replica_db_instance_identifier == input.proposal.source_db_instance_identifier
}
