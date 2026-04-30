package identity

import (
	"fmt"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// BuildRDSRebootInstancePolicy returns the minimal session policy for
// rebooting one named instance.
func BuildRDSRebootInstancePolicy(p action.RDSRebootInstanceProposal, accountID string) SessionPolicy {
	dbARN := fmt.Sprintf("arn:aws:rds:%s:%s:db:%s",
		p.Region, accountID, p.DBInstanceIdentifier)

	return SessionPolicy{
		Version: "2012-10-17",
		Statement: []Statement{
			{
				Sid:      "RebootOnlyThisInstance",
				Effect:   "Allow",
				Action:   []string{"rds:RebootDBInstance"},
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
