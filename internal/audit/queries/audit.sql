-- name: GetLatestHash :one
SELECT hash FROM audit_events ORDER BY id DESC LIMIT 1;

-- name: InsertEvent :one
INSERT INTO audit_events (proposal_hash, kind, payload, prev_hash, hash, at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id;

-- name: ListEventsByProposal :many
SELECT id, proposal_hash, kind, payload, prev_hash, hash, at
FROM audit_events
WHERE proposal_hash = $1
ORDER BY id;

-- name: ListAllEvents :many
SELECT id, proposal_hash, kind, payload, prev_hash, hash, at
FROM audit_events
ORDER BY id;

-- name: AdvisoryLockChain :exec
SELECT pg_advisory_xact_lock(0);

-- name: TruncateEvents :exec
TRUNCATE audit_events RESTART IDENTITY;
