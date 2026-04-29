// Package audit implements Casper's append-only, hash-chained event log.
//
// Every meaningful state change in the system — proposal accepted, plan
// compiled, step started, AWS call made, verification verdict, rollback
// triggered — is written here as an Event. Events are linked by hash
// (each Hash includes the previous Event's Hash), so any tampering
// with a past row breaks the chain forward of that row.
//
// This package contains the Event type, the Store interface, and the
// hash chain logic. Concrete stores (memory, postgres) live in this
// package as well.
package audit

import (
	"context"
	"time"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// Kind tags what an Event records. Adding a kind is a deliberate
// extension; downstream consumers (audit UI, replay tools) switch on
// this string.
type Kind string

const (
	KindProposed       Kind = "proposed"
	KindPlanCompiled   Kind = "plan_compiled"
	KindStepStarted    Kind = "step_started"
	KindStepCompleted  Kind = "step_completed"
	KindStepFailed     Kind = "step_failed"
	KindPlanCompleted  Kind = "plan_completed"
	KindPlanFailed     Kind = "plan_failed"
	KindRollbackBegun  Kind = "rollback_begun"
	KindRollbackEnded  Kind = "rollback_ended"
)

// Event is one row in the audit log. PrevHash and Hash are filled in
// by the Store at Append time — callers should not set them.
type Event struct {
	ID           int64               `json:"id"`
	ProposalHash action.ProposalHash `json:"proposal_hash"`
	Kind         Kind                `json:"kind"`
	Payload      map[string]any      `json:"payload"`
	PrevHash     string              `json:"prev_hash"`
	Hash         string              `json:"hash"`
	At           time.Time           `json:"at"`
}

// Store is the durable home of audit events. The contract:
//
//   - Append assigns ID, At, PrevHash, and Hash; returns the completed Event.
//   - The hash chain is per-Store (one global chain), not per-proposal,
//     so tampering anywhere is detectable from anywhere downstream.
//   - List returns events in append order.
type Store interface {
	Append(ctx context.Context, kind Kind, proposalHash action.ProposalHash, payload map[string]any) (Event, error)
	List(ctx context.Context, proposalHash action.ProposalHash) ([]Event, error)
	Verify(ctx context.Context) error
}
