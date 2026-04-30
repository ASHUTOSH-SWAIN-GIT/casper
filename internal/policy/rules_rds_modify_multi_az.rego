package casper.rds_modify_multi_az

import rego.v1

# Multi-AZ toggle is operationally heavy (10–15 minutes) and roughly
# doubles cost when enabled. Enabling Multi-AZ is generally safer than
# disabling it. The policy:
#  - Auto-allows enabling (single-AZ → multi-AZ) — adds redundancy.
#  - Requires approval for disabling — removing redundancy is a step
#    backwards in availability and should be a deliberate human decision.
#  - Denies no-ops.

default result := {
	"decision": "needs_approval",
	"reason":   "default policy: Multi-AZ changes require human approval"
}

# Auto-allow: enabling Multi-AZ.
result := {
	"decision": "allow",
	"reason":   "enabling Multi-AZ adds redundancy (auto-allowed)"
} if {
	input.proposal.current_multi_az == false
	input.proposal.target_multi_az == true
}

# Deny: no-op (target == current).
result := {
	"decision": "deny",
	"reason":   "no-op: target_multi_az equals current_multi_az"
} if {
	input.proposal.target_multi_az == input.proposal.current_multi_az
}
