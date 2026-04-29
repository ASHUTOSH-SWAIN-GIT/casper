package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// PostgresStore is the durable Store. It implements the same hash-chain
// contract as MemoryStore, so callers (interpreter, CLI) don't care
// which one they got.
//
// Append serializes via a transaction-scoped advisory lock so concurrent
// processes can't race the chain. Time is truncated to microseconds —
// Postgres' TIMESTAMPTZ precision — so a value written and read back
// hashes identically.
type PostgresStore struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// NewPostgresStore connects to dsn, runs the schema migration, and
// returns a ready store. Caller must Close() when done.
func NewPostgresStore(ctx context.Context, dsn string, now func() time.Time) (*PostgresStore, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	s := &PostgresStore{pool: pool, now: now}
	if s.now == nil {
		s.now = time.Now
	}
	if err := s.migrate(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *PostgresStore) Close() {
	s.pool.Close()
}

const schemaSQL = `
CREATE TABLE IF NOT EXISTS audit_events (
    id            BIGSERIAL PRIMARY KEY,
    proposal_hash TEXT        NOT NULL,
    kind          TEXT        NOT NULL,
    payload       JSONB       NOT NULL,
    prev_hash     TEXT        NOT NULL DEFAULT '',
    hash          TEXT        NOT NULL,
    at            TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_events_proposal_hash
    ON audit_events(proposal_hash);
`

func (s *PostgresStore) migrate(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, schemaSQL)
	return err
}

func (s *PostgresStore) Append(ctx context.Context, kind Kind, ph action.ProposalHash, payload map[string]any) (Event, error) {
	var event Event
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// Serialize all chain appends. Constant key 0 means "the global
		// audit chain". Released automatically at transaction end.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(0)"); err != nil {
			return fmt.Errorf("advisory lock: %w", err)
		}

		var prev string
		err := tx.QueryRow(ctx,
			`SELECT hash FROM audit_events ORDER BY id DESC LIMIT 1`,
		).Scan(&prev)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read prev hash: %w", err)
		}

		event = Event{
			ProposalHash: ph,
			Kind:         kind,
			Payload:      payload,
			PrevHash:     prev,
			At:           s.now().UTC().Truncate(time.Microsecond),
		}
		h, err := computeHash(prev, event)
		if err != nil {
			return err
		}
		event.Hash = h

		payloadJSON, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal payload: %w", err)
		}

		return tx.QueryRow(ctx, `
            INSERT INTO audit_events (proposal_hash, kind, payload, prev_hash, hash, at)
            VALUES ($1, $2, $3, $4, $5, $6)
            RETURNING id`,
			string(ph), string(kind), payloadJSON, prev, h, event.At,
		).Scan(&event.ID)
	})
	if err != nil {
		return Event{}, err
	}
	return event, nil
}

func (s *PostgresStore) List(ctx context.Context, ph action.ProposalHash) ([]Event, error) {
	rows, err := s.pool.Query(ctx, `
        SELECT id, proposal_hash, kind, payload, prev_hash, hash, at
        FROM audit_events
        WHERE proposal_hash = $1
        ORDER BY id`,
		string(ph),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *PostgresStore) Verify(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `
        SELECT id, proposal_hash, kind, payload, prev_hash, hash, at
        FROM audit_events ORDER BY id`,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	prev := ""
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return err
		}
		if e.PrevHash != prev {
			return fmt.Errorf("event #%d (%s): prev_hash=%q want %q", e.ID, e.Kind, e.PrevHash, prev)
		}
		want, err := computeHash(prev, e)
		if err != nil {
			return fmt.Errorf("event #%d: hash compute: %w", e.ID, err)
		}
		if e.Hash != want {
			return fmt.Errorf("event #%d (%s): hash mismatch (chain broken)", e.ID, e.Kind)
		}
		prev = e.Hash
	}
	return rows.Err()
}

func scanEvent(rows pgx.Rows) (Event, error) {
	var e Event
	var ph string
	var kind string
	var payload []byte
	if err := rows.Scan(&e.ID, &ph, &kind, &payload, &e.PrevHash, &e.Hash, &e.At); err != nil {
		return Event{}, err
	}
	e.ProposalHash = action.ProposalHash(ph)
	e.Kind = Kind(kind)
	if err := json.Unmarshal(payload, &e.Payload); err != nil {
		return Event{}, fmt.Errorf("unmarshal payload: %w", err)
	}
	e.At = e.At.UTC()
	return e, nil
}
