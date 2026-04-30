package server

import (
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/llmcfg"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/proposer"
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
}

// NewRuntimeDeps wires the standard dependency graph used by casperd.
// llmcfg is read lazily so a missing API key only fails the proposals
// endpoint — not server startup.
func NewRuntimeDeps() Dependencies {
	return &runtimeDeps{
		cfgFn:     llmcfg.FromEnv,
		proposals: newProposalsStore(),
	}
}

func (d *runtimeDeps) LLMConfig() (llmConfig, error) { return d.cfgFn() }
func (d *runtimeDeps) Proposals() *proposalsStore    { return d.proposals }

// silence unused-import warning if proposer isn't referenced elsewhere
// in this file later.
var _ = proposer.BackendAnthropic
