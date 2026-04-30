package identity

import (
	"fmt"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// BuildRDSCreateReadReplicaPolicy returns the minimal session policy
// for creating a replica off the named source. Permissions span the
// source instance (must be readable) and the new replica's ARN
// (CreateDBInstanceReadReplica + the rollback DeleteDBInstance both
// scope to the replica).
func BuildRDSCreateReadReplicaPolicy(p action.RDSCreateReadReplicaProposal, accountID string) SessionPolicy {
	sourceARN := fmt.Sprintf("arn:aws:rds:%s:%s:db:%s",
		p.Region, accountID, p.SourceDBInstanceIdentifier)
	replicaARN := fmt.Sprintf("arn:aws:rds:%s:%s:db:%s",
		p.Region, accountID, p.ReplicaDBInstanceIdentifier)

	return SessionPolicy{
		Version: "2012-10-17",
		Statement: []Statement{
			{
				Sid:      "CreateReplicaOffSource",
				Effect:   "Allow",
				Action:   []string{"rds:CreateDBInstanceReadReplica"},
				Resource: []string{sourceARN, replicaARN},
			},
			{
				Sid:      "DeleteOnlyTheCreatedReplica",
				Effect:   "Allow",
				Action:   []string{"rds:DeleteDBInstance"},
				Resource: []string{replicaARN},
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
