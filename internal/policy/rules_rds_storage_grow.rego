package casper.rds_storage_grow

import rego.v1

# Storage growth is IRREVERSIBLE on RDS — there is no AWS-supported way
# to shrink allocated storage on an existing instance. The recovery
# path (dump → create-new-with-smaller-storage → restore → cutover)
# is out of Casper's scope. This makes storage-grow a category of
# action where:
#
#  1. The default verdict is DENY (not needs_approval) — irreversibility
#     warrants stronger fail-closed behavior.
#  2. There is no auto-allow path — even small growth requires a human
#     signing off, because they have to acknowledge the irreversibility.
#  3. Catastrophic-jump cases get an even stricter denial reason.
#
# This is the canonical "irreversibility-aware policy" pattern in
# Casper: the action's reversibility class drives the default, not just
# the auto-allow rules.

default result := {
	"decision": "deny",
	"reason":   "RDS storage growth is irreversible (cannot shrink) — explicit human approval required, but the default is to refuse"
}

# Override default to needs_approval (instead of deny) for modest,
# clearly-bounded growth. The human still has to sign — the override
# just signals "this isn't catastrophic, you can reasonably approve."
result := {
	"decision": "needs_approval",
	"reason":   "modest storage growth (irreversible — requires human approval)"
} if {
	input.proposal.target_allocated_storage_gb > input.proposal.current_allocated_storage_gb
	input.proposal.target_allocated_storage_gb - input.proposal.current_allocated_storage_gb <= 100
	input.proposal.target_allocated_storage_gb <= 2 * input.proposal.current_allocated_storage_gb
}

# Hard deny: no-op or shrink (which AWS would reject anyway, but we
# catch it at the policy layer for a clearer error).
result := {
	"decision": "deny",
	"reason":   "no-op or shrink: target_allocated_storage_gb must be greater than current"
} if {
	input.proposal.target_allocated_storage_gb <= input.proposal.current_allocated_storage_gb
}

# Hard deny: catastrophic jump (>10x current). Signals likely operator
# error or runaway agent rather than a deliberate request.
result := {
	"decision": "deny",
	"reason":   "catastrophic storage jump (>10x current) — likely error; if intentional, propose smaller increments"
} if {
	input.proposal.target_allocated_storage_gb > 10 * input.proposal.current_allocated_storage_gb
}
