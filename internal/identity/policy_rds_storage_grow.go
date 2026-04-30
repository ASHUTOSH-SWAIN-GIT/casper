package identity

import (
	"fmt"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// BuildRDSStorageGrowPolicy returns the minimal session policy for
// growing storage on the named instance.
func BuildRDSStorageGrowPolicy(p action.RDSStorageGrowProposal, accountID string) SessionPolicy {
	dbARN := fmt.Sprintf("arn:aws:rds:%s:%s:db:%s",
		p.Region, accountID, p.DBInstanceIdentifier)

	return SessionPolicy{
		Version: "2012-10-17",
		Statement: []Statement{
			{
				Sid:      "ModifyOnlyThisInstance",
				Effect:   "Allow",
				Action:   []string{"rds:ModifyDBInstance"},
				Resource: []string{dbARN},
			},
			{
				Sid:      "DescribeAnyInstance",
				Effect:   "Allow",
				Action:   []string{"rds:DescribeDBInstances"},
				Resource: []string{"*"},
			},
		},
	}
}
