package identity

import (
	"fmt"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// BuildRDSRestoreFromSnapshotPolicy returns the minimal session policy
// for restoring a snapshot to a new instance.
func BuildRDSRestoreFromSnapshotPolicy(p action.RDSRestoreFromSnapshotProposal, accountID string) SessionPolicy {
	snapshotARN := fmt.Sprintf("arn:aws:rds:%s:%s:snapshot:%s",
		p.Region, accountID, p.SnapshotIdentifier)
	targetARN := fmt.Sprintf("arn:aws:rds:%s:%s:db:%s",
		p.Region, accountID, p.TargetDBInstanceIdentifier)

	return SessionPolicy{
		Version: "2012-10-17",
		Statement: []Statement{
			{
				Sid:      "RestoreFromNamedSnapshotToNamedInstance",
				Effect:   "Allow",
				Action:   []string{"rds:RestoreDBInstanceFromDBSnapshot"},
				Resource: []string{snapshotARN, targetARN},
			},
			{
				Sid:      "DeleteOnlyTheRestoredInstance",
				Effect:   "Allow",
				Action:   []string{"rds:DeleteDBInstance"},
				Resource: []string{targetARN},
			},
			{
				Sid:      "DescribeAnyInstance",
				Effect:   "Allow",
				Action:   []string{"rds:DescribeDBInstances"},
				Resource: []string{"*"},
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
