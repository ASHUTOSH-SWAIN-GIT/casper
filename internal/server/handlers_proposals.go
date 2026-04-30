package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

	// 6. Validate against the action's JSON Schema before policy.
	if err := validateForActionType(res.ProposalRaw, routing.ActionType); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "proposal_invalid", err.Error())
		return
	}

	// 7. Policy gate.
	verdict, polErr := s.evaluatePolicy(ctx, routing.ActionType, res.ProposalRaw)
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
		Status:    statusForVerdict(verdict),
		CreatedAt: now,
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

// evaluatePolicy runs the embedded Rego rules for the action type
// against the proposal JSON. Decoding the typed struct happens once
// per action — keep this in sync with cmd/casperctl/runnable.go's
// dispatch.
func (s *Server) evaluatePolicy(ctx context.Context, actionType string, raw []byte) (policy.Verdict, error) {
	engine, err := policy.NewEngine(ctx)
	if err != nil {
		return policy.Verdict{}, err
	}
	switch actionType {
	case "rds_resize":
		var p action.RDSResizeProposal
		if err := json.Unmarshal(raw, &p); err != nil {
			return policy.Verdict{}, err
		}
		return engine.EvaluateRDSResize(ctx, p)
	case "rds_create_snapshot":
		var p action.RDSCreateSnapshotProposal
		if err := json.Unmarshal(raw, &p); err != nil {
			return policy.Verdict{}, err
		}
		return engine.EvaluateRDSCreateSnapshot(ctx, p)
	case "rds_modify_backup_retention":
		var p action.RDSModifyBackupRetentionProposal
		if err := json.Unmarshal(raw, &p); err != nil {
			return policy.Verdict{}, err
		}
		return engine.EvaluateRDSModifyBackupRetention(ctx, p)
	case "rds_reboot_instance":
		var p action.RDSRebootInstanceProposal
		if err := json.Unmarshal(raw, &p); err != nil {
			return policy.Verdict{}, err
		}
		return engine.EvaluateRDSRebootInstance(ctx, p)
	case "rds_modify_multi_az":
		var p action.RDSModifyMultiAZProposal
		if err := json.Unmarshal(raw, &p); err != nil {
			return policy.Verdict{}, err
		}
		return engine.EvaluateRDSModifyMultiAZ(ctx, p)
	case "rds_storage_grow":
		var p action.RDSStorageGrowProposal
		if err := json.Unmarshal(raw, &p); err != nil {
			return policy.Verdict{}, err
		}
		return engine.EvaluateRDSStorageGrow(ctx, p)
	case "rds_delete_snapshot":
		var p action.RDSDeleteSnapshotProposal
		if err := json.Unmarshal(raw, &p); err != nil {
			return policy.Verdict{}, err
		}
		return engine.EvaluateRDSDeleteSnapshot(ctx, p)
	case "rds_create_read_replica":
		var p action.RDSCreateReadReplicaProposal
		if err := json.Unmarshal(raw, &p); err != nil {
			return policy.Verdict{}, err
		}
		return engine.EvaluateRDSCreateReadReplica(ctx, p)
	case "rds_modify_engine_version":
		var p action.RDSModifyEngineVersionProposal
		if err := json.Unmarshal(raw, &p); err != nil {
			return policy.Verdict{}, err
		}
		return engine.EvaluateRDSModifyEngineVersion(ctx, p)
	case "rds_restore_from_snapshot":
		var p action.RDSRestoreFromSnapshotProposal
		if err := json.Unmarshal(raw, &p); err != nil {
			return policy.Verdict{}, err
		}
		return engine.EvaluateRDSRestoreFromSnapshot(ctx, p)
	}
	return policy.Verdict{}, fmt.Errorf("no policy evaluator wired for %q", actionType)
}

// validateForActionType dispatches to the per-action JSON Schema validator.
func validateForActionType(raw []byte, actionType string) error {
	switch actionType {
	case "rds_resize":
		return action.Validate(raw)
	case "rds_create_snapshot":
		return action.ValidateRDSCreateSnapshot(raw)
	case "rds_modify_backup_retention":
		return action.ValidateRDSModifyBackupRetention(raw)
	case "rds_reboot_instance":
		return action.ValidateRDSRebootInstance(raw)
	case "rds_modify_multi_az":
		return action.ValidateRDSModifyMultiAZ(raw)
	case "rds_storage_grow":
		return action.ValidateRDSStorageGrow(raw)
	case "rds_delete_snapshot":
		return action.ValidateRDSDeleteSnapshot(raw)
	case "rds_create_read_replica":
		return action.ValidateRDSCreateReadReplica(raw)
	case "rds_modify_engine_version":
		return action.ValidateRDSModifyEngineVersion(raw)
	case "rds_restore_from_snapshot":
		return action.ValidateRDSRestoreFromSnapshot(raw)
	}
	return fmt.Errorf("no validator registered for %q", actionType)
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
