package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/audit"
)

// readProposal reads raw JSON from disk for any subcommand that takes
// a proposal path argument. Centralized so the path-handling and error
// messages are consistent across validate / hash / policy / compile / run.
func readProposal(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read proposal: %w", err)
	}
	return raw, nil
}

// decodeProposal validates and decodes a proposal in one step. Used by
// subcommands that need the typed struct (compile, run, policy).
//
// Today this only handles rds_resize — multi-action subcommands use
// detectActionType + per-action validators directly.
func decodeProposal(raw []byte) (action.RDSResizeProposal, action.ProposalHash, error) {
	if err := action.Validate(raw); err != nil {
		return action.RDSResizeProposal{}, "", fmt.Errorf("invalid proposal: %w", err)
	}
	var p action.RDSResizeProposal
	if err := json.Unmarshal(raw, &p); err != nil {
		return action.RDSResizeProposal{}, "", fmt.Errorf("decode proposal: %w", err)
	}
	h, err := action.Hash(raw)
	if err != nil {
		return action.RDSResizeProposal{}, "", err
	}
	return p, h, nil
}

// detectActionType picks the action type from a proposal's shape.
// Today proposals don't carry an explicit action_type field, so we
// detect by which unique field is present. If/when proposals start
// carrying an explicit action_type, we honor that first.
//
// Adding a new action: add a case here. Failing to detect returns an
// error — better than silently routing to the wrong action.
func detectActionType(raw []byte) (string, error) {
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return "", fmt.Errorf("parse proposal: %w", err)
	}
	if v, ok := probe["action_type"].(string); ok && v != "" {
		return v, nil // explicit field wins (forward-compat)
	}
	switch {
	case probe["target_instance_class"] != nil:
		return "rds_resize", nil
	case probe["snapshot_identifier"] != nil:
		return "rds_create_snapshot", nil
	}
	return "", fmt.Errorf("could not detect action type from proposal shape (no recognizable discriminator field)")
}

// validateForActionType dispatches to the correct schema validator
// based on the detected action type.
func validateForActionType(raw []byte, actionType string) error {
	switch actionType {
	case "rds_resize":
		return action.Validate(raw)
	case "rds_create_snapshot":
		return action.ValidateRDSCreateSnapshot(raw)
	default:
		return fmt.Errorf("no validator registered for action type %q", actionType)
	}
}

// openAuditStore picks an audit.Store based on flags + env. forceMemory
// (set by --in-memory-audit on `run`) trumps DATABASE_URL — useful when
// you have a stale DATABASE_URL in your environment but don't want to
// touch Postgres for this invocation.
func openAuditStore(ctx context.Context, forceMemory bool) (audit.Store, func(), error) {
	if forceMemory {
		fmt.Fprintln(os.Stderr, "audit: in-memory store (--in-memory-audit)")
		return audit.NewMemoryStore(nil), func() {}, nil
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "audit: in-memory store (set DATABASE_URL to persist)")
		return audit.NewMemoryStore(nil), func() {}, nil
	}
	s, err := audit.NewPostgresStore(ctx, dsn, nil)
	if err != nil {
		return nil, nil, err
	}
	fmt.Fprintln(os.Stderr, "audit: postgres store")
	return s, s.Close, nil
}

func errOrEmpty(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
