// Package snapshot fetches the deterministic infra state ("snapshot")
// that the proposer reasons against, given an action's resource type.
//
// Architecture: action_type → resource_type (from action.Spec) →
// Fetcher (from this package's registry) → proposer.Snapshot.
// The dispatch is two map lookups; no LLM is involved in deciding
// which describe API to call.
//
// Adding a new AWS resource type: write one Fetcher, register it from
// the package's init(), and tag the relevant action specs with the
// matching Resource string. All actions on that resource share the
// fetcher.
package snapshot

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/awsx"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/proposer"
)

// Fetcher reads live state for a single resource and packs it into
// a proposer.Snapshot. The target string is the resource identifier
// (DB instance ID for rds_instance, snapshot ID for rds_snapshot).
// Region is supplied separately because the awsx.Client is already
// region-bound at construction time; the Fetcher just stamps it onto
// the returned snapshot.
type Fetcher func(ctx context.Context, c *awsx.Client, target, region string) (proposer.Snapshot, error)

var registry = map[string]Fetcher{}

// Register binds a Fetcher to a resource type. Called from init() in
// per-resource files.
func Register(resourceType string, f Fetcher) {
	registry[resourceType] = f
}

// For looks up the Fetcher for a resource type. Callers receive
// (nil, false) for unregistered resources — the caller chooses whether
// to fall back to user-supplied flags or fail hard.
func For(resourceType string) (Fetcher, bool) {
	f, ok := registry[resourceType]
	return f, ok
}

// Fetch is the convenience wrapper. Returns a clear error when the
// resource type has no registered fetcher — useful for callers that
// want to fall back without re-checking the boolean.
func Fetch(ctx context.Context, c *awsx.Client, resourceType, target, region string) (proposer.Snapshot, error) {
	f, ok := For(resourceType)
	if !ok {
		return proposer.Snapshot{}, fmt.Errorf("no fetcher registered for resource type %q", resourceType)
	}
	return f(ctx, c, target, region)
}

func init() {
	Register("rds_instance", fetchRDSInstance)
	Register("rds_snapshot", fetchRDSSnapshot)
}

func fetchRDSInstance(ctx context.Context, c *awsx.Client, target, region string) (proposer.Snapshot, error) {
	db, err := c.DescribeDBInstance(ctx, target)
	if err != nil {
		return proposer.Snapshot{}, err
	}
	return proposer.Snapshot{
		DBInstanceIdentifier: aws.ToString(db.DBInstanceIdentifier),
		Region:               region,
		CurrentInstanceClass: aws.ToString(db.DBInstanceClass),
		Engine:               aws.ToString(db.Engine),
		EngineVersion:        aws.ToString(db.EngineVersion),
		Status:               aws.ToString(db.DBInstanceStatus),
		MultiAZ:              aws.ToBool(db.MultiAZ),
		AllocatedStorageGB:   aws.ToInt32(db.AllocatedStorage),
		BackupRetentionDays:  aws.ToInt32(db.BackupRetentionPeriod),
	}, nil
}

func fetchRDSSnapshot(ctx context.Context, c *awsx.Client, target, region string) (proposer.Snapshot, error) {
	s, err := c.DescribeDBSnapshot(ctx, target)
	if err != nil {
		return proposer.Snapshot{}, err
	}
	return proposer.Snapshot{
		Region:                     region,
		SnapshotIdentifier:         aws.ToString(s.DBSnapshotIdentifier),
		SourceDBInstanceIdentifier: aws.ToString(s.DBInstanceIdentifier),
		SnapshotStatus:             aws.ToString(s.Status),
		Engine:                     aws.ToString(s.Engine),
		EngineVersion:              aws.ToString(s.EngineVersion),
		AllocatedStorageGB:         aws.ToInt32(s.AllocatedStorage),
	}, nil
}
