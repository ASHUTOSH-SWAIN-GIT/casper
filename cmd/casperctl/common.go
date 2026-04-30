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
