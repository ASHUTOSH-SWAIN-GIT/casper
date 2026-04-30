package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/audit"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/awsx"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/interpreter"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/plan"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/policy"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/runner"
)

type rejectRequest struct {
	Reason string `json:"reason,omitempty"`
}

// handleApproveProposal kicks off execution of a previously generated
// proposal. Returns immediately with the new run_id; clients subscribe
// to /v1/runs/{id}/events for live progress.
func (s *Server) handleApproveProposal(w http.ResponseWriter, r *http.Request) {
	if s.deps == nil {
		writeError(w, http.StatusServiceUnavailable, "deps_missing", "")
		return
	}
	id := r.PathValue("id")
	prop, ok := s.deps.Proposals().get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "proposal_not_found", "no proposal with id "+id)
		return
	}
	if prop.Status == "approved" || prop.Status == "rejected" || prop.Status == "denied" {
		writeError(w, http.StatusConflict, "proposal_decided",
			"proposal already in terminal status: "+prop.Status)
		return
	}
	if prop.Policy.Decision == string(policy.DecisionDeny) {
		writeError(w, http.StatusForbidden, "policy_denied",
			"proposal was denied by policy: "+prop.Policy.Reason)
		return
	}

	// Build the runnable from the original bytes — Build re-validates
	// the schema and decodes the typed proposal.
	rb, err := runner.Build(prop.ProposalBytes)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "proposal_invalid", err.Error())
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

	// Mark proposal as approved with run linkage.
	s.deps.Proposals().update(id, func(p *proposalRecord) {
		p.Status = "approved"
		p.RunID = run.ID
		t := time.Now().UTC()
		p.DecidedAt = &t
	})

	go s.executeRun(run.ID, rb)

	writeJSON(w, http.StatusAccepted, map[string]any{
		"run_id":      run.ID,
		"proposal_id": prop.ID,
		"status":      "running",
	})
}

func (s *Server) handleRejectProposal(w http.ResponseWriter, r *http.Request) {
	if s.deps == nil {
		writeError(w, http.StatusServiceUnavailable, "deps_missing", "")
		return
	}
	id := r.PathValue("id")
	prop, ok := s.deps.Proposals().get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "proposal_not_found", "no proposal with id "+id)
		return
	}
	if prop.Status == "approved" || prop.Status == "rejected" {
		writeError(w, http.StatusConflict, "proposal_decided",
			"proposal already in terminal status: "+prop.Status)
		return
	}

	var body rejectRequest
	_ = json.NewDecoder(r.Body).Decode(&body) // body is optional

	s.deps.Proposals().update(id, func(p *proposalRecord) {
		p.Status = "rejected"
		p.RejectReason = body.Reason
		t := time.Now().UTC()
		p.DecidedAt = &t
	})
	updated, _ := s.deps.Proposals().get(id)
	writeJSON(w, http.StatusOK, updated)
}

// executeRun runs the trust-layer pipeline against AWS. Mirrors the
// CLI's runProposal flow: audit "proposed" / "policy_evaluated" /
// "plan_compiled", then the interpreter walks the forward plan, with
// rollback on mutating-step failure when the plan opts in. Every audit
// Append also broadcasts to SSE subscribers via the auditTee.
func (s *Server) executeRun(runID string, rb *runner.Runnable) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	defer s.deps.Bus().finish(runID)

	store := teeForRun(s.deps.Audit(), s.deps.Bus(), runID)

	// audit "proposed"
	if _, err := store.Append(ctx, audit.KindProposed, rb.Hash, rb.ProposalAuditPayload); err != nil {
		s.failRun(runID, "audit_proposed", err)
		return
	}

	// policy gate (re-evaluated before execution as a safety check)
	engine, err := policy.NewEngine(ctx)
	if err != nil {
		s.failRun(runID, "policy_engine", err)
		return
	}
	verdict, err := rb.EvaluatePolicy(ctx, engine)
	if err != nil {
		s.failRun(runID, "policy_evaluate", err)
		return
	}
	_, _ = store.Append(ctx, audit.KindPolicyEvaluated, rb.Hash, map[string]any{
		"action_type": rb.ActionType,
		"decision":    string(verdict.Decision),
		"reason":      verdict.Reason,
	})
	// needs_approval is satisfied by the human approval that triggered this
	// run — only a hard deny blocks execution at this stage.
	if verdict.Decision == policy.DecisionDeny {
		s.failRun(runID, "policy_blocked", errors.New("policy decision: "+string(verdict.Decision)+" — "+verdict.Reason))
		return
	}

	// compile + audit
	fwd, rb2 := rb.Compile()
	_, _ = store.Append(ctx, audit.KindPlanCompiled, rb.Hash, map[string]any{
		"action_type":    rb.ActionType,
		"forward_steps":  len(fwd.Steps),
		"rollback_steps": len(rb2.Steps),
	})

	// AWS config
	region := rb.Region
	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		s.failRun(runID, "aws_config", err)
		return
	}

	// Execute forward plan
	interp := &interpreter.Interpreter{Client: awsx.New(awsCfg), Audit: store}
	results, runErr := interp.Run(ctx, fwd)
	s.deps.Runs().update(runID, func(r *runRecord) { r.ForwardSteps = results })

	if runErr == nil {
		s.completeRun(runID, "succeeded", "")
		return
	}

	mode := failureMode(fwd, results)
	mutated := anyMutatingStepCompleted(fwd, results)
	if mode != plan.OnFailureRollback || !mutated {
		log.Printf("run %s: forward failed (%v); no rollback (mode=%s mutated=%v)", runID, runErr, mode, mutated)
		s.completeRun(runID, "failed", runErr.Error())
		return
	}

	// Rollback path
	_, _ = store.Append(ctx, audit.KindRollbackBegun, rb.Hash, map[string]any{"reason": runErr.Error()})
	rbResults, rbErr := interp.Run(ctx, rb2)
	_, _ = store.Append(ctx, audit.KindRollbackEnded, rb.Hash, map[string]any{
		"ok":    rbErr == nil,
		"error": errStr(rbErr),
	})
	s.deps.Runs().update(runID, func(r *runRecord) {
		r.RollbackSteps = rbResults
		r.RolledBack = true
	})
	if rbErr != nil {
		s.completeRun(runID, "failed", "forward failed ("+runErr.Error()+"); rollback also failed: "+rbErr.Error())
		return
	}
	s.completeRun(runID, "rolled_back", runErr.Error())
}

func (s *Server) failRun(runID, code string, err error) {
	log.Printf("run %s: %s: %v", runID, code, err)
	s.completeRun(runID, "failed", code+": "+err.Error())
}

func (s *Server) completeRun(runID, status, errMsg string) {
	s.deps.Runs().update(runID, func(r *runRecord) {
		t := time.Now().UTC()
		r.Status = status
		r.Error = errMsg
		r.FinishedAt = &t
		r.DurationMs = t.Sub(r.StartedAt).Milliseconds()
	})
}

// failureMode looks up the OnFailure value of the step that just failed.
func failureMode(p plan.ExecutionPlan, results []interpreter.StepResult) plan.OnFailure {
	if len(results) == 0 {
		return plan.OnFailureAbort
	}
	failedID := results[len(results)-1].StepID
	for _, s := range p.Steps {
		if s.ID == failedID {
			return s.OnFailure
		}
	}
	return plan.OnFailureAbort
}

// anyMutatingStepCompleted reports whether the forward plan made it past
// at least one state-changing AWS call.
func anyMutatingStepCompleted(p plan.ExecutionPlan, results []interpreter.StepResult) bool {
	for _, r := range results {
		if r.Status != interpreter.StepStatusDone {
			continue
		}
		for _, s := range p.Steps {
			if s.ID == r.StepID && s.Kind == plan.StepAWSAPICall && s.OnFailure == plan.OnFailureRollback {
				return true
			}
		}
	}
	return false
}

func errStr(e error) string {
	if e == nil {
		return ""
	}
	return e.Error()
}
