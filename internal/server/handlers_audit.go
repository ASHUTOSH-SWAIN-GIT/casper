package server

import (
	"net/http"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/audit"
)

// handleListAudit returns audit events filtered by proposal_hash and
// optionally by kind. Proposal hash is required — listing the entire
// chain in one shot is intentionally not supported (use the per-run
// view instead).
func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request) {
	if s.deps == nil {
		writeError(w, http.StatusServiceUnavailable, "deps_missing", "")
		return
	}
	hash := r.URL.Query().Get("proposal_hash")
	if hash == "" {
		writeError(w, http.StatusBadRequest, "proposal_hash_required",
			"query parameter 'proposal_hash' is required")
		return
	}
	kind := r.URL.Query().Get("kind")

	events, err := s.deps.Audit().List(r.Context(), action.ProposalHash(hash))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "audit_list_failed", err.Error())
		return
	}
	if kind != "" {
		filtered := events[:0]
		for _, e := range events {
			if string(e.Kind) == kind {
				filtered = append(filtered, e)
			}
		}
		events = filtered
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events": events,
		"total":  len(events),
	})
}

// handleVerifyAudit asks the audit store to walk the entire chain and
// confirm every PrevHash/Hash pair is intact. Returns 200 with ok=true
// on success; 422 with the inner error on any break.
func (s *Server) handleVerifyAudit(w http.ResponseWriter, r *http.Request) {
	if s.deps == nil {
		writeError(w, http.StatusServiceUnavailable, "deps_missing", "")
		return
	}
	if err := s.deps.Audit().Verify(r.Context()); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// silence unused-import warning if audit.Event drops out elsewhere.
var _ audit.Event
