package identity

import (
	"fmt"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// BuildRDSDeleteSnapshotPolicy returns the minimal session policy for
// deleting one named snapshot. Note: there is no MintForRDSDeleteSnapshot
// helper on the broker yet — this function is wired through the
// runnable adapter and used when the broker supports per-action mint
// methods (today the CLI falls back to default credentials when this
// action runs with the broker enabled).
func BuildRDSDeleteSnapshotPolicy(p action.RDSDeleteSnapshotProposal, accountID string) SessionPolicy {
	snapshotARN := fmt.Sprintf("arn:aws:rds:%s:%s:snapshot:%s",
		p.Region, accountID, p.SnapshotIdentifier)

	return SessionPolicy{
		Version: "2012-10-17",
		Statement: []Statement{
			{
				Sid:      "DeleteOnlyThisSnapshot",
				Effect:   "Allow",
				Action:   []string{"rds:DeleteDBSnapshot"},
				Resource: []string{snapshotARN},
			},
			{
				Sid:      "DescribeAnySnapshot",
				Effect:   "Allow",
				Action:   []string{"rds:DescribeDBSnapshots"},
				Resource: []string{"*"},
			},
		},
	}
}
