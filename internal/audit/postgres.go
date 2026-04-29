package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // registers "pgx" driver for goose's database/sql access
	"github.com/pressly/goose/v3"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
	auditdb "github.com/ASHUTOSH-SWAIN-GIT/casper/internal/audit/db"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/migrations"
)

// PostgresStore implements Store on top of pgxpool, with goose for
// migrations and sqlc-generated queries (internal/audit/db). The chain
// logic itself lives in chain.go and is shared with MemoryStore — only
// the persistence backend differs.
//
// Append serializes via pg_advisory_xact_lock(0) so concurrent writers
// can't race the chain. Time is truncated to microseconds — Postgres'
// TIMESTAMPTZ precision — so values written and read back hash identically.
type PostgresStore struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// NewPostgresStore migrates to the latest schema, opens a pool, and
// returns a ready store. Caller must Close() when done.
func NewPostgresStore(ctx context.Context, dsn string, now func() time.Time) (*PostgresStore, error) {
	if err := runMigrations(ctx, dsn); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
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
	return s, nil
}

func (s *PostgresStore) Close() {
	s.pool.Close()
}

// runMigrations opens a short-lived database/sql handle (goose's API
// signature) and applies every pending migration in migrations/.
func runMigrations(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open sql: %w", err)
	}
	defer db.Close()
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("dialect: %w", err)
	}
	// Silence goose's stdout chatter; we have our own audit log.
	goose.SetLogger(goose.NopLogger())
	return goose.UpContext(ctx, db, ".")
}

func (s *PostgresStore) Append(ctx context.Context, kind Kind, ph action.ProposalHash, payload map[string]any) (Event, error) {
	var event Event
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := auditdb.New(tx)
		if err := q.AdvisoryLockChain(ctx); err != nil {
			return fmt.Errorf("advisory lock: %w", err)
		}
		prev, err := q.GetLatestHash(ctx)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read prev hash: %w", err)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			prev = ""
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
		id, err := q.InsertEvent(ctx, auditdb.InsertEventParams{
			ProposalHash: string(ph),
			Kind:         string(kind),
			Payload:      payloadJSON,
			PrevHash:     prev,
			Hash:         h,
			At:           pgtype.Timestamptz{Time: event.At, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("insert event: %w", err)
		}
		event.ID = id
		return nil
	})
	if err != nil {
		return Event{}, err
	}
	return event, nil
}

func (s *PostgresStore) List(ctx context.Context, ph action.ProposalHash) ([]Event, error) {
	q := auditdb.New(s.pool)
	rows, err := q.ListEventsByProposal(ctx, string(ph))
	if err != nil {
		return nil, err
	}
	return convertRows(rows)
}

func (s *PostgresStore) Verify(ctx context.Context) error {
	q := auditdb.New(s.pool)
	rows, err := q.ListAllEvents(ctx)
	if err != nil {
		return err
	}
	events, err := convertRows(rows)
	if err != nil {
		return err
	}
	prev := ""
	for _, e := range events {
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
	return nil
}

func convertRows(rows []auditdb.AuditEvent) ([]Event, error) {
	out := make([]Event, 0, len(rows))
	for _, r := range rows {
		var payload map[string]any
		if err := json.Unmarshal(r.Payload, &payload); err != nil {
			return nil, fmt.Errorf("unmarshal payload (event #%d): %w", r.ID, err)
		}
		out = append(out, Event{
			ID:           r.ID,
			ProposalHash: action.ProposalHash(r.ProposalHash),
			Kind:         Kind(r.Kind),
			Payload:      payload,
			PrevHash:     r.PrevHash,
			Hash:         r.Hash,
			At:           r.At.Time.UTC(),
		})
	}
	return out, nil
}
