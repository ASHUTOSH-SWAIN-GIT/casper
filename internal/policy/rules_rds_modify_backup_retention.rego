package casper.rds_modify_backup_retention

import rego.v1

# Backup retention is reversible (you can change it back), but reducing
# retention immediately deletes automated backups older than the new
# window — that data loss is NOT recovered by re-extending retention
# later. The policy treats reductions more cautiously than extensions.

default result := {
	"decision": "needs_approval",
	"reason":   "default policy: backup retention changes require human approval"
}

# Auto-allow: extending retention from a non-zero value to a higher
# non-zero value within sane bounds. No data loss possible — the
# instance simply keeps backups longer.
result := {
	"decision": "allow",
	"reason":   "extending backup retention by a small amount (no data loss)"
} if {
	input.proposal.target_retention_days > input.proposal.current_retention_days
	input.proposal.current_retention_days > 0
	input.proposal.target_retention_days <= 14
	input.proposal.target_retention_days - input.proposal.current_retention_days <= 7
}

# Deny: disabling automated backups (target=0) with a non-zero current
# value would delete all automated backups. This must require explicit
# human approval — never auto-allowed.
result := {
	"decision": "deny",
	"reason":   "setting backup retention to 0 disables automated backups and deletes existing ones (irreversible)"
} if {
	input.proposal.target_retention_days == 0
	input.proposal.current_retention_days > 0
}

# Deny: no-op (target == current).
result := {
	"decision": "deny",
	"reason":   "no-op: target_retention_days equals current_retention_days"
} if {
	input.proposal.target_retention_days == input.proposal.current_retention_days
}
