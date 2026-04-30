package identity

import (
	"fmt"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// BuildRDSCreateSnapshotPolicy returns the minimal session policy for
// taking one named snapshot. The mutating action (CreateDBSnapshot) is
// scoped to the source instance; the rollback action (DeleteDBSnapshot)
// is scoped to the specific snapshot the proposal names. Describes
// remain "*" because AWS doesn't support resource scoping for them.
//
// Statements:
//   - rds:CreateDBSnapshot   → only on the named source instance
//   - rds:DeleteDBSnapshot   → only on the named target snapshot ARN
//   - rds:DescribeDBInstances→ "*" (no resource scoping in the API)
//   - rds:DescribeDBSnapshots→ "*" (same)
func BuildRDSCreateSnapshotPolicy(p action.RDSCreateSnapshotProposal, accountID string) SessionPolicy {
	dbARN := fmt.Sprintf("arn:aws:rds:%s:%s:db:%s",
		p.Region, accountID, p.DBInstanceIdentifier)
	snapshotARN := fmt.Sprintf("arn:aws:rds:%s:%s:snapshot:%s",
		p.Region, accountID, p.SnapshotIdentifier)

	return SessionPolicy{
		Version: "2012-10-17",
		Statement: []Statement{
			{
				Sid:      "CreateSnapshotOnNamedInstance",
				Effect:   "Allow",
				Action:   []string{"rds:CreateDBSnapshot"},
				Resource: []string{dbARN, snapshotARN},
			},
			{
				Sid:      "DeleteOnlyTheCreatedSnapshot",
				Effect:   "Allow",
				Action:   []string{"rds:DeleteDBSnapshot"},
				Resource: []string{snapshotARN},
			},
			{
				Sid:      "DescribeDBInstances",
				Effect:   "Allow",
				Action:   []string{"rds:DescribeDBInstances"},
				Resource: []string{"*"},
			},
			{
				Sid:      "DescribeDBSnapshots",
				Effect:   "Allow",
				Action:   []string{"rds:DescribeDBSnapshots"},
				Resource: []string{"*"},
			},
		},
	}
}
