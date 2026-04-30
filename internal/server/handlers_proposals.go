package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	starling "github.com/jerkeyray/starling/eventlog"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/awsx"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/policy"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/proposer"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/runner"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/snapshot"
)

// createProposalRequest is the wire shape for POST /v1/proposals.
type createProposalRequest struct {
	Intent   string `json:"intent"`
	Region   string `json:"region,omitempty"`
	Instance string `json:"instance,omitempty"` // override target if router fails to extract one
}

func (s *Server) handleCreateProposal(w http.ResponseWriter, r *http.Request) {
	if s.deps == nil {
		writeError(w, http.StatusServiceUnavailable, "deps_missing",
			"server constructed without runtime dependencies")
		return
	}

	var req createProposalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Intent) == "" {
		writeError(w, http.StatusBadRequest, "intent_required", "field 'intent' is required")
		return
	}

	cfg, err := s.deps.LLMConfig()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "llm_unavailable", err.Error())
		return
	}

	// Per-request Starling event log. Using in-memory SQLite keeps
	// agent-run replay possible without polluting the user's disk; for
	// production we'd persist it alongside the audit DB.
	starLog, err := starling.NewSQLite(":memory:")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "starling_init", err.Error())
		return
	}
	defer starLog.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	// 1. Route the intent.
	router, err := proposer.NewRouter(proposer.RouterConfig{
		Backend: cfg.Backend,
		APIKey:  cfg.APIKey,
		Region:  cfg.Region,
		Model:   cfg.RouterModel,
		Log:     starLog,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "router_build", err.Error())
		return
	}
	routing, err := router.Route(ctx, req.Intent)
	if err != nil {
		writeError(w, http.StatusBadGateway, "router_failed", err.Error())
		return
	}

	// 2. Resolve target + region with fallbacks.
	instance := routing.DBInstanceIdentifier
	if req.Instance != "" {
		instance = req.Instance
	}
	region := routing.Region
	if req.Region != "" {
		region = req.Region
	}
	if region == "" {
		if v := os.Getenv("AWS_REGION"); v != "" {
			region = v
		} else if v := os.Getenv("AWS_DEFAULT_REGION"); v != "" {
			region = v
		}
	}
	if instance == "" {
		writeError(w, http.StatusUnprocessableEntity, "instance_unresolved",
			"router did not identify a target — pass 'instance' explicitly")
		return
	}
	if region == "" {
		writeError(w, http.StatusUnprocessableEntity, "region_unresolved",
			"region not provided and AWS_REGION not set")
		return
	}

	// 3. Lookup resource type and fetch live state.
	spec, ok := action.Lookup(routing.ActionType)
	if !ok {
		writeError(w, http.StatusInternalServerError, "action_unknown",
			"router returned unregistered action: "+routing.ActionType)
		return
	}

	snap := proposer.Snapshot{
		DBInstanceIdentifier: instance,
		Region:               region,
		Engine:               "postgres",
		Status:               "available",
	}
	if spec.Resource != "" {
		awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
		if err == nil {
			fetched, fetchErr := snapshot.Fetch(ctx, awsx.New(awsCfg), spec.Resource, instance, region)
			if fetchErr == nil {
				snap = fetched
			}
		}
	}

	// 4. Run the per-action proposer.
	prop, err := proposer.NewForAction(routing.ActionType, proposer.Config{
		Backend: cfg.Backend,
		APIKey:  cfg.APIKey,
		Region:  cfg.Region,
		Model:   cfg.ProposerModel,
		Log:     starLog,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "proposer_build", err.Error())
		return
	}
	res, err := prop.Propose(ctx, proposer.Request{Intent: req.Intent, Snapshot: snap})
	if err != nil {
		writeError(w, http.StatusBadGateway, "proposer_failed", err.Error())
		return
	}

	// 5. Decode the proposal JSON for response shaping.
	var proposalDoc map[string]any
	if err := json.Unmarshal(res.ProposalRaw, &proposalDoc); err != nil {
		writeError(w, http.StatusInternalServerError, "proposal_decode", err.Error())
		return
	}

	// 6 & 7. Build runnable (validates against schema, decodes typed
	// proposal, prepares per-action closures), then evaluate policy.
	rb, err := runner.Build(res.ProposalRaw)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "proposal_invalid", err.Error())
		return
	}
	engine, err := policy.NewEngine(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "policy_engine", err.Error())
		return
	}
	verdict, polErr := rb.EvaluatePolicy(ctx, engine)
	if polErr != nil {
		writeError(w, http.StatusInternalServerError, "policy_failed", polErr.Error())
		return
	}

	// 8. Persist + return.
	now := time.Now().UTC()
	rec := &proposalRecord{
		ID:           "prop_" + randID(),
		Intent:       req.Intent,
		ActionType:   routing.ActionType,
		ResourceType: spec.Resource,
		Target:       instance,
		Region:       region,
		ProposalHash: string(res.ProposalHash),
		Proposal:     proposalDoc,
		Snapshot:     snapshotToMap(snap),
		Policy: proposalPolicy{
			Decision: string(verdict.Decision),
			Reason:   verdict.Reason,
		},
		Router: proposalRouter{
			Model:      "router",
			Confidence: routing.Confidence,
			Reasoning:  routing.Reasoning,
		},
		Proposer: proposalMeta{
			Model:        res.Model,
			InputTokens:  res.InputTokens,
			OutputTokens: res.OutputTokens,
			CostUSD:      res.CostUSD,
			DurationMs:   res.Duration.Milliseconds(),
			RunID:        res.RunID,
		},
		Status:        statusForVerdict(verdict),
		ProposalBytes: append([]byte(nil), res.ProposalRaw...),
		CreatedAt:     now,
	}
	s.deps.Proposals().put(rec)
	writeJSON(w, http.StatusCreated, rec)
}

func (s *Server) handleListProposals(w http.ResponseWriter, r *http.Request) {
	if s.deps == nil {
		writeError(w, http.StatusServiceUnavailable, "deps_missing",
			"server constructed without runtime dependencies")
		return
	}
	status := r.URL.Query().Get("status")
	out := s.deps.Proposals().list(status)
	writeJSON(w, http.StatusOK, map[string]any{
		"proposals": out,
		"total":     len(out),
	})
}

func (s *Server) handleGetProposal(w http.ResponseWriter, r *http.Request) {
	if s.deps == nil {
		writeError(w, http.StatusServiceUnavailable, "deps_missing",
			"server constructed without runtime dependencies")
		return
	}
	id := r.PathValue("id")
	p, ok := s.deps.Proposals().get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "proposal_not_found", "no proposal with id "+id)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// statusForVerdict maps the policy decision to the proposal's lifecycle
// status: "allow" auto-allows, "needs_approval" stays pending, "deny"
// is terminal.
func statusForVerdict(v policy.Verdict) string {
	switch v.Decision {
	case policy.DecisionAllow:
		return "auto_allowed"
	case policy.DecisionDeny:
		return "denied"
	default:
		return "pending"
	}
}

// snapshotToMap renders the typed snapshot via JSON round-trip so the
// response embeds it as a generic object — easier for the dashboard
// than mirroring the Go struct shape.
func snapshotToMap(s proposer.Snapshot) map[string]any {
	b, _ := json.Marshal(s)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

func randID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
