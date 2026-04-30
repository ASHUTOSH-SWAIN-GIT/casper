package identity

import (
	"fmt"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// BuildRDSModifyBackupRetentionPolicy returns the minimal session
// policy for changing backup retention. Like resize, the only mutating
// action is rds:ModifyDBInstance scoped to the specific instance ARN.
func BuildRDSModifyBackupRetentionPolicy(p action.RDSModifyBackupRetentionProposal, accountID string) SessionPolicy {
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
