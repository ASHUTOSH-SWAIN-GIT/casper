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
	"sync"
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
	Instance string `json:"instance,omitempty"`
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

	starLog, err := starling.NewSQLite(":memory:")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "starling_init", err.Error())
		return
	}
	defer starLog.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	// 1. Route the intent → BatchRouting (1..N actions).
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
	batchRouting, err := router.Route(ctx, req.Intent)
	if err != nil {
		writeError(w, http.StatusBadGateway, "router_failed", "Proposal failed router run: "+err.Error())
		return
	}

	// 2. For each action, run snapshot fetch + proposer in parallel.
	type actionResult struct {
		routing     proposer.Routing
		snap        proposer.Snapshot
		res         *proposer.Result
		proposalDoc map[string]any
		rb          *runner.Runnable
		verdict     policy.Verdict
	}

	results := make([]actionResult, len(batchRouting.Actions))
	errs := make([]error, len(batchRouting.Actions))
	var wg sync.WaitGroup

	for i, routing := range batchRouting.Actions {
		i, routing := i, routing
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Resolve instance + region.
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
				errs[i] = writeErrorVal("instance_unresolved", "router did not identify a target — pass 'instance' explicitly")
				return
			}
			if region == "" {
				errs[i] = writeErrorVal("region_unresolved", "region not provided and AWS_REGION not set")
				return
			}
			routing.DBInstanceIdentifier = instance
			routing.Region = region

			// Fetch live state.
			spec, ok := action.Lookup(routing.ActionType)
			if !ok {
				errs[i] = writeErrorVal("action_unknown", "router returned unregistered action: "+routing.ActionType)
				return
			}
			snap := proposer.Snapshot{
				DBInstanceIdentifier: instance,
				Region:               region,
				Engine:               "postgres",
				Status:               "available",
			}
			if spec.Resource != "" {
				if awsCfg, awsErr := config.LoadDefaultConfig(ctx, config.WithRegion(region)); awsErr == nil {
					if fetched, fetchErr := snapshot.Fetch(ctx, awsx.New(awsCfg), spec.Resource, instance, region); fetchErr == nil {
						snap = fetched
					}
				}
			}

			// Per-action proposer — each action gets its own Starling log.
			actionLog, logErr := starling.NewSQLite(":memory:")
			if logErr != nil {
				errs[i] = logErr
				return
			}
			defer actionLog.Close()

			prop, propErr := proposer.NewForAction(routing.ActionType, proposer.Config{
				Backend: cfg.Backend,
				APIKey:  cfg.APIKey,
				Region:  cfg.Region,
				Model:   cfg.ProposerModel,
				Log:     actionLog,
			})
			if propErr != nil {
				errs[i] = propErr
				return
			}
			res, propRunErr := prop.Propose(ctx, proposer.Request{Intent: req.Intent, Snapshot: snap})
			if propRunErr != nil {
				errs[i] = propRunErr
				return
			}

			var proposalDoc map[string]any
			if decErr := json.Unmarshal(res.ProposalRaw, &proposalDoc); decErr != nil {
				errs[i] = decErr
				return
			}

			rb, buildErr := runner.Build(res.ProposalRaw)
			if buildErr != nil {
				errs[i] = buildErr
				return
			}

			engine, engineErr := policy.NewEngine(ctx)
			if engineErr != nil {
				errs[i] = engineErr
				return
			}
			verdict, polErr := rb.EvaluatePolicy(ctx, engine)
			if polErr != nil {
				errs[i] = polErr
				return
			}

			results[i] = actionResult{
				routing:     routing,
				snap:        snap,
				res:         res,
				proposalDoc: proposalDoc,
				rb:          rb,
				verdict:     verdict,
			}
		}()
	}
	wg.Wait()

	// Check for per-action errors — return the first one.
	for _, e := range errs {
		if e != nil {
			writeError(w, http.StatusBadGateway, "proposer_failed", e.Error())
			return
		}
	}

	// 3. Build batch + proposal records.
	batchID := "batch_" + randID()
	now := time.Now().UTC()
	proposals := make([]*proposalRecord, len(results))

	for i, ar := range results {
		spec, _ := action.Lookup(ar.routing.ActionType)
		rec := &proposalRecord{
			ID:             "prop_" + randID(),
			BatchID:        batchID,
			ExecutionOrder: batchRouting.ExecutionOrder,
			Intent:         req.Intent,
			ActionType:   ar.routing.ActionType,
			ResourceType: spec.Resource,
			Target:       ar.routing.DBInstanceIdentifier,
			Region:       ar.routing.Region,
			ProposalHash: string(ar.res.ProposalHash),
			Proposal:     ar.proposalDoc,
			Snapshot:     snapshotToMap(ar.snap),
			Policy: proposalPolicy{
				Decision: string(ar.verdict.Decision),
				Reason:   ar.verdict.Reason,
			},
			Router: proposalRouter{
				Model:      "router",
				Confidence: ar.routing.Confidence,
				Reasoning:  batchRouting.Reasoning,
			},
			Proposer: proposalMeta{
				Model:        ar.res.Model,
				InputTokens:  ar.res.InputTokens,
				OutputTokens: ar.res.OutputTokens,
				CostUSD:      ar.res.CostUSD,
				DurationMs:   ar.res.Duration.Milliseconds(),
				RunID:        ar.res.RunID,
			},
			Status:        statusForVerdict(ar.verdict),
			ProposalBytes: append([]byte(nil), ar.res.ProposalRaw...),
			CreatedAt:     now.Add(time.Duration(i) * time.Microsecond), // preserve order
		}
		s.deps.Proposals().put(rec)
		proposals[i] = rec
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"batch_id":        batchID,
		"execution_order": batchRouting.ExecutionOrder,
		"proposals":       proposals,
	})
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

// writeErrorVal returns an error value (used inside goroutines where we
// can't call writeError directly on the ResponseWriter).
func writeErrorVal(code, msg string) error {
	return fmt.Errorf("%s: %s", code, msg)
}
