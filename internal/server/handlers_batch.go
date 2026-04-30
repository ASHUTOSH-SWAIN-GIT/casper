package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/policy"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/runner"
)

type batchApproveResponse struct {
	BatchID        string   `json:"batch_id"`
	ExecutionOrder string   `json:"execution_order"`
	RunIDs         []string `json:"run_ids"`
}

// handleApproveBatch approves all proposals in a batch and starts their
// runs. If execution_order is "sequential" the runs execute one after
// another (stopping on the first failure). If "parallel" all runs start
// simultaneously.
func (s *Server) handleApproveBatch(w http.ResponseWriter, r *http.Request) {
	if s.deps == nil {
		writeError(w, http.StatusServiceUnavailable, "deps_missing", "")
		return
	}
	batchID := r.PathValue("batch_id")
	proposals := s.deps.Proposals().listByBatch(batchID)
	if len(proposals) == 0 {
		writeError(w, http.StatusNotFound, "batch_not_found", "no proposals for batch "+batchID)
		return
	}

	// Validate: none already decided, none policy-denied.
	for _, p := range proposals {
		if p.Status == "approved" || p.Status == "rejected" || p.Status == "denied" {
			writeError(w, http.StatusConflict, "proposal_decided",
				"proposal "+p.ID+" already in terminal status: "+p.Status)
			return
		}
		if p.Policy.Decision == string(policy.DecisionDeny) {
			writeError(w, http.StatusForbidden, "policy_denied",
				"proposal "+p.ID+" was denied by policy: "+p.Policy.Reason)
			return
		}
	}

	// Build runnables and create run records.
	type entry struct {
		runID string
		rb    *runner.Runnable
		propID string
	}
	entries := make([]entry, 0, len(proposals))
	for _, prop := range proposals {
		rb, err := runner.Build(prop.ProposalBytes)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "proposal_invalid",
				"proposal "+prop.ID+": "+err.Error())
			return
		}
		now := time.Now().UTC()
		run := &runRecord{
			ID:           "run_" + randID(),
			ProposalID:   prop.ID,
			ProposalHash: prop.ProposalHash,
			ActionType:   prop.ActionType,
			Region:       prop.Region,
			Status:       "running",
			StartedAt:    now,
		}
		s.deps.Runs().put(run)
		s.deps.Proposals().update(prop.ID, func(p *proposalRecord) {
			p.Status = "approved"
			p.RunID = run.ID
			t := time.Now().UTC()
			p.DecidedAt = &t
		})
		entries = append(entries, entry{runID: run.ID, rb: rb, propID: prop.ID})
	}

	runIDs := make([]string, len(entries))
	for i, e := range entries {
		runIDs[i] = e.runID
	}

	executionOrder := proposals[0].ExecutionOrder
	if executionOrder != "parallel" {
		executionOrder = "sequential"
	}

	if executionOrder == "parallel" {
		for _, e := range entries {
			e := e
			go s.executeRun(e.runID, e.rb)
		}
	} else {
		go func() {
			for _, e := range entries {
				s.executeRun(e.runID, e.rb) // blocks until complete
				run, ok := s.deps.Runs().get(e.runID)
				if !ok || run.Status != "succeeded" {
					break // stop remaining runs on failure
				}
			}
		}()
	}

	writeJSON(w, http.StatusAccepted, batchApproveResponse{
		BatchID:        batchID,
		ExecutionOrder: executionOrder,
		RunIDs:         runIDs,
	})
}

// handleRejectBatch rejects all pending proposals in a batch.
func (s *Server) handleRejectBatch(w http.ResponseWriter, r *http.Request) {
	if s.deps == nil {
		writeError(w, http.StatusServiceUnavailable, "deps_missing", "")
		return
	}
	batchID := r.PathValue("batch_id")
	proposals := s.deps.Proposals().listByBatch(batchID)
	if len(proposals) == 0 {
		writeError(w, http.StatusNotFound, "batch_not_found", "no proposals for batch "+batchID)
		return
	}

	var body rejectRequest
	decodeOptionalJSON(r, &body)

	now := time.Now().UTC()
	for _, p := range proposals {
		if p.Status == "approved" || p.Status == "rejected" {
			continue
		}
		s.deps.Proposals().update(p.ID, func(rec *proposalRecord) {
			rec.Status = "rejected"
			rec.RejectReason = body.Reason
			t := now
			rec.DecidedAt = &t
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"batch_id": batchID, "status": "rejected"})
}

func decodeOptionalJSON(r *http.Request, v any) {
	_ = json.NewDecoder(r.Body).Decode(v)
}
