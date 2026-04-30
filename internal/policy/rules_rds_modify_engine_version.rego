package casper.rds_modify_engine_version

import rego.v1

# Engine version upgrades are irreversible — once data files migrate
# to the new version's format, AWS does not support downgrading. The
# policy defaults to deny; even minor version upgrades require human
# approval. Major version upgrades (allow_major_version_upgrade=true)
# are denied outright unless explicitly carved out.

default result := {
	"decision": "deny",
	"reason":   "engine version upgrade is irreversible — explicit human approval required, default is to refuse"
}

# Override to needs_approval for minor version upgrades only.
result := {
	"decision": "needs_approval",
	"reason":   "minor engine version upgrade (irreversible — requires human approval)"
} if {
	input.proposal.allow_major_version_upgrade == false
	input.proposal.target_engine_version != input.proposal.current_engine_version
}

# Hard deny: no-op.
result := {
	"decision": "deny",
	"reason":   "no-op: target_engine_version equals current_engine_version"
} if {
	input.proposal.target_engine_version == input.proposal.current_engine_version
}

# Hard deny: major upgrades. Operator must reduce scope (do a chain
# of minor upgrades to the highest minor of the current major) or
# cut over to a new instance restored from snapshot — both are out
# of casperctl's scope today.
result := {
	"decision": "deny",
	"reason":   "major version upgrades are disallowed by casperctl — cut over via snapshot+restore in a new instance instead"
} if {
	input.proposal.allow_major_version_upgrade == true
}
