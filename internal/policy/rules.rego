package casper.rds_resize

import rego.v1

# result is the verdict returned to the trust layer.
#
# Defaults to needs_approval — a human must sign before anything runs.
# An allow decision is reached only when the proposal matches a narrow,
# well-known shape ("safe in-family one-step upsize on a known family").
# Everything else falls through to the default.
#
# Adding a new auto-allow case is a deliberate edit to this file.
# Adding a new auto-deny case is also a deliberate edit. Anything we
# can't classify stays at needs_approval — fail-safe by default.

default result := {
	"decision": "needs_approval",
	"reason":   "default policy: action shape not in any auto-allow rule"
}

# Defense in depth: the schema also rejects no-ops, but if a malformed
# proposal slipped through, the policy denies it explicitly.
result := {
	"decision": "deny",
	"reason":   "no-op: target equals current instance class"
} if {
	input.proposal.current_instance_class == input.proposal.target_instance_class
}

# Auto-allow: same family, one step up, on an approved family.
result := {
	"decision": "allow",
	"reason":   "safe in-family one-step upsize"
} if {
	not no_op
	family(input.proposal.current_instance_class) == family(input.proposal.target_instance_class)
	family(input.proposal.current_instance_class) in approved_families
	one_step_upsize
}

approved_families := {"t4g", "r6g", "m6g", "r7g"}

no_op if {
	input.proposal.current_instance_class == input.proposal.target_instance_class
}

family(class) := parts[1] if {
	parts := split(class, ".")
}

size_name(class) := parts[2] if {
	parts := split(class, ".")
}

# size_order maps RDS size suffixes to integers so we can compare neighbours.
# Values are dense — "one step" means a delta of exactly 1.
size_order := {
	"micro":    1,
	"small":    2,
	"medium":   3,
	"large":    4,
	"xlarge":   5,
	"2xlarge":  6,
	"4xlarge":  7,
	"8xlarge":  8,
	"12xlarge": 9,
	"16xlarge": 10,
	"24xlarge": 11,
	"32xlarge": 12
}

one_step_upsize if {
	curr := size_order[size_name(input.proposal.current_instance_class)]
	targ := size_order[size_name(input.proposal.target_instance_class)]
	targ == curr + 1
}
