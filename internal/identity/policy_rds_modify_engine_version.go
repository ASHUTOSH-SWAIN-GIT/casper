package identity

import (
	"fmt"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// BuildRDSModifyEngineVersionPolicy returns the minimal session policy
// for upgrading an instance's engine version.
func BuildRDSModifyEngineVersionPolicy(p action.RDSModifyEngineVersionProposal, accountID string) SessionPolicy {
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
