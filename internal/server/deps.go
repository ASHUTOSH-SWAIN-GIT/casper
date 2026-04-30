package server

import (
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/audit"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/llmcfg"
)

// llmConfig is a server-internal alias for llmcfg.Config so handlers
// don't need to import llmcfg directly. Keeping the alias narrow makes
// the boundary between "what the server needs" and "where it comes
// from" explicit.
type llmConfig = llmcfg.Config

// runtimeDeps is the production Dependencies implementation. main.go
// constructs one and passes it to server.New. Tests may pass any other
// type implementing Dependencies.
type runtimeDeps struct {
	cfgFn     func() (llmConfig, error)
	proposals *proposalsStore
	runs      *runsStore
	bus       *runEventBus
	auditSt   audit.Store
}

// NewRuntimeDeps wires the standard dependency graph used by casperd.
// llmcfg is read lazily so a missing API key only fails the proposals
// endpoint — not server startup. The audit store is in-memory for the
// alpha; a Postgres-backed store can be passed via NewRuntimeDepsWith
// once we're ready to persist across restarts.
func NewRuntimeDeps() Dependencies {
	return &runtimeDeps{
		cfgFn:     llmcfg.FromEnv,
		proposals: newProposalsStore(),
		runs:      newRunsStore(),
		bus:       newRunEventBus(),
		auditSt:   audit.NewMemoryStore(nil),
	}
}

func (d *runtimeDeps) LLMConfig() (llmConfig, error) { return d.cfgFn() }
func (d *runtimeDeps) Proposals() *proposalsStore    { return d.proposals }
func (d *runtimeDeps) Runs() *runsStore              { return d.runs }
func (d *runtimeDeps) Bus() *runEventBus             { return d.bus }
func (d *runtimeDeps) Audit() audit.Store            { return d.auditSt }
