// Package server implements the casperd HTTP API.
//
// Layout:
//   server.go            — lifecycle, routing, middleware
//   handlers_actions.go  — GET /v1/actions, /v1/actions/{type}
//   handlers_proposals.go — POST /v1/proposals, GET /v1/proposals[/{id}], approve/reject
//   handlers_runs.go     — GET /v1/runs[/{id}], GET /v1/runs/{id}/events (SSE)
//   handlers_audit.go    — GET /v1/audit, POST /v1/audit/verify
//   sse.go               — SSE writer plumbing
//
// All endpoints emit JSON unless otherwise noted. Errors follow
// {"error": "...", "code": "..."} with appropriate HTTP status.
package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/audit"
)

// Options configures Server. Fields here will grow as we wire dependencies
// (audit store, identity broker, LLM config) but the public surface is
// kept narrow on purpose.
type Options struct {
	Addr string
	Deps Dependencies
}

// Dependencies are the runtime collaborators the handlers need. Pulled
// behind an interface so tests can swap in stubs and so main.go is the
// only place the full graph is wired.
type Dependencies interface {
	LLMConfig() (llmConfig, error)
	Proposals() *proposalsStore
	Runs() *runsStore
	Bus() *runEventBus
	Audit() audit.Store
}

// Server is the casperd HTTP application. Construct with New, mount
// with Handler.
type Server struct {
	opts Options
	mux  *http.ServeMux
	deps Dependencies
}

// New constructs a Server with all routes registered. Dependencies that
// require config (LLM keys, audit store) are wired here as we add them.
func New(opts Options) *Server {
	s := &Server{opts: opts, mux: http.NewServeMux(), deps: opts.Deps}
	s.routes()
	return s
}

// Handler returns the HTTP handler with middleware applied.
func (s *Server) Handler() http.Handler {
	return s.withMiddleware(s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)

	// Action catalog (read-only, derived from the action registry).
	s.mux.HandleFunc("GET /v1/actions", s.handleListActions)
	s.mux.HandleFunc("GET /v1/actions/{type}", s.handleGetAction)

	// Proposals (router + fetcher + proposer pipeline; persistence in-memory).
	s.mux.HandleFunc("POST /v1/proposals", s.handleCreateProposal)
	s.mux.HandleFunc("GET /v1/proposals", s.handleListProposals)
	s.mux.HandleFunc("GET /v1/proposals/{id}", s.handleGetProposal)
	s.mux.HandleFunc("POST /v1/proposals/{id}/approve", s.handleApproveProposal)
	s.mux.HandleFunc("POST /v1/proposals/{id}/reject", s.handleRejectProposal)

	// Runs (execution records + live SSE event stream).
	s.mux.HandleFunc("GET /v1/runs", s.handleListRuns)
	s.mux.HandleFunc("GET /v1/runs/{id}", s.handleGetRun)
	s.mux.HandleFunc("GET /v1/runs/{id}/events", s.handleRunEvents)

	// Audit (chain inspection + verification).
	s.mux.HandleFunc("GET /v1/audit", s.handleListAudit)
	s.mux.HandleFunc("POST /v1/audit/verify", s.handleVerifyAudit)

	// Workspace (environment summary + credential management).
	s.mux.HandleFunc("GET /v1/workspace", s.handleGetWorkspace)
	s.mux.HandleFunc("PUT /v1/workspace", s.handleUpdateWorkspace)
}

// withMiddleware wraps the mux with cross-cutting concerns:
//   - request logging
//   - permissive CORS for the local dashboard (dev only)
//   - JSON content-type default for non-streaming endpoints
func (s *Server) withMiddleware(h http.Handler) http.Handler {
	return logRequests(corsLocal(h))
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// writeJSON serializes payload to w with the given status. Failures to
// encode are logged and ignored — the client will see a truncated body,
// which is preferable to writing partial JSON twice.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// writeError emits {"error": message, "code": code} with the given HTTP
// status. Use writeJSON directly for non-error envelopes.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"error": message, "code": code})
}
