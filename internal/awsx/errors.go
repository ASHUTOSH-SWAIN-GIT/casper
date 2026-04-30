package awsx

import "errors"

// ErrDBInstanceNotFound is returned by Describe* helpers when the
// resource is missing — separate from a transport error so callers
// (notably the snapshot fetcher) can fall back gracefully.
var ErrDBInstanceNotFound = errors.New("db instance not found")

// ErrDBSnapshotNotFound mirrors ErrDBInstanceNotFound for snapshots.
var ErrDBSnapshotNotFound = errors.New("db snapshot not found")
