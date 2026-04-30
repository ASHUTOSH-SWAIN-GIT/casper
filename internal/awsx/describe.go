package awsx

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
)

// DescribeDBInstance returns the typed RDS instance description, or
// a clean (nil, ErrDBInstanceNotFound) when the instance does not exist.
// Used by the snapshot fetcher (and any other read-only consumer) — kept
// separate from the interpreter's plan-driven Call() so callers don't
// have to fabricate plan.APICall structs for ad-hoc reads.
func (c *Client) DescribeDBInstance(ctx context.Context, id string) (*rdstypes.DBInstance, error) {
	out, err := c.rds.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: aws.String(id),
	})
	if err != nil {
		if isDBInstanceNotFound(err) {
			return nil, ErrDBInstanceNotFound
		}
		return nil, err
	}
	if len(out.DBInstances) == 0 {
		return nil, ErrDBInstanceNotFound
	}
	return &out.DBInstances[0], nil
}

// DescribeDBSnapshot returns the typed RDS snapshot description, or
// (nil, ErrDBSnapshotNotFound) when the snapshot does not exist.
func (c *Client) DescribeDBSnapshot(ctx context.Context, id string) (*rdstypes.DBSnapshot, error) {
	out, err := c.rds.DescribeDBSnapshots(ctx, &rds.DescribeDBSnapshotsInput{
		DBSnapshotIdentifier: aws.String(id),
	})
	if err != nil {
		if isSnapshotNotFound(err) {
			return nil, ErrDBSnapshotNotFound
		}
		return nil, err
	}
	if len(out.DBSnapshots) == 0 {
		return nil, ErrDBSnapshotNotFound
	}
	return &out.DBSnapshots[0], nil
}
