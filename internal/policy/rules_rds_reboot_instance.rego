package casper.rds_reboot_instance

import rego.v1

# Reboots are operationally disruptive (the instance is unavailable
# for ~30s to a few minutes) but mechanically simple. The default is
# needs_approval — a reboot during peak traffic is the kind of thing
# a human should sign off on.

default result := {
	"decision": "needs_approval",
	"reason":   "default policy: reboot must be approved by a human"
}

# Auto-allow: reboots without force_failover are minimally disruptive
# and are commonly part of routine maintenance (parameter group
# changes, etc.). The auto-allow case here is narrow: only when the
# reasoning explicitly invokes a known-safe trigger. This rule is
# deliberately conservative.
#
# In v2 the policy could read input.simulator output to allow reboots
# only outside business hours / on staging instances, etc.
result := {
	"decision": "deny",
	"reason":   "force_failover only makes sense on Multi-AZ instances; the simulator should confirm Multi-AZ before allowing"
} if {
	input.proposal.force_failover == true
	# Until we have simulator output, we can't tell if the instance
	# is Multi-AZ, so deny. The operator can re-propose without
	# force_failover or upgrade the policy to consult the simulator.
}
