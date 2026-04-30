package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/spf13/cobra"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/audit"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/awsx"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/identity"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/interpreter"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/plan"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/policy"
)

var (
	runInMemoryAudit bool
)

var runCmd = &cobra.Command{
	Use:   "run <proposal.json>",
	Short: "Validate, gate on policy, and execute the plan against AWS",
	Long: `End-to-end execution: detects the action type, validates the
proposal, hashes it, runs the policy gate, optionally mints scoped
credentials via STS, compiles forward + rollback plans, and walks the
forward plan against AWS while streaming a hash-chained audit log.

Multi-action: 'run' dispatches by action type detected from the
proposal's structure. Currently supports rds_resize and
rds_create_snapshot. Adding a new action means extending detectActionType
and buildRunnable; the run pipeline itself doesn't change.

Optional environment:
  DATABASE_URL                    Postgres DSN for durable audit
  CASPER_ROLE_ARN                 STS role to assume per action
  CASPER_EXTERNAL_ID              External ID for the role's trust policy

If both CASPER_ROLE_ARN and CASPER_EXTERNAL_ID are set, casperctl mints
per-action 15-minute credentials scoped to the proposal's resource.`,
	Args: cobra.ExactArgs(1),
	RunE: runProposal,
}

func init() {
	runCmd.Flags().BoolVar(&runInMemoryAudit, "in-memory-audit", false,
		"Use the in-memory audit store regardless of DATABASE_URL.")
	rootCmd.AddCommand(runCmd)
}

func runProposal(cmd *cobra.Command, args []string) error {
	raw, err := readProposal(args[0])
	if err != nil {
		return err
	}
	return executeProposal(context.Background(), raw, runInMemoryAudit)
}

// executeProposal runs the trust-layer pipeline (validate → policy → compile
// → mint creds → execute → audit) on a raw proposal byte slice. Shared by
// `casperctl run <file>` and `casperctl do --intent ...`.
func executeProposal(ctx context.Context, raw []byte, inMemoryAudit bool) error {
	r, err := buildRunnable(raw)
	if err != nil {
		return err
	}

	store, closeStore, err := openAuditStore(ctx, inMemoryAudit)
	if err != nil {
		return fmt.Errorf("open audit store: %w", err)
	}
	defer closeStore()

	if _, err := store.Append(ctx, audit.KindProposed, r.Hash, r.ProposalAuditPayload); err != nil {
		return fmt.Errorf("audit proposed: %w", err)
	}

	// Policy gate.
	engine, err := policy.NewEngine(ctx)
	if err != nil {
		return fmt.Errorf("policy engine: %w", err)
	}
	verdict, err := r.EvaluatePolicy(ctx, engine)
	if err != nil {
		return fmt.Errorf("policy evaluate: %w", err)
	}
	if _, err := store.Append(ctx, audit.KindPolicyEvaluated, r.Hash, map[string]any{
		"action_type": r.ActionType,
		"decision":    string(verdict.Decision),
		"reason":      verdict.Reason,
	}); err != nil {
		return fmt.Errorf("audit policy_evaluated: %w", err)
	}
	fmt.Fprintf(os.Stderr, "policy [%s]: %s — %s\n", r.ActionType, verdict.Decision, verdict.Reason)
	if verdict.Decision != policy.DecisionAllow {
		if err := dumpAudit(store, r.Hash); err != nil {
			return err
		}
		return fmt.Errorf("policy %s: %s", verdict.Decision, verdict.Reason)
	}

	// Compile plans.
	fwd, rb := r.Compile()
	if _, err := store.Append(ctx, audit.KindPlanCompiled, r.Hash, map[string]any{
		"action_type":    r.ActionType,
		"forward_steps":  len(fwd.Steps),
		"rollback_steps": len(rb.Steps),
	}); err != nil {
		return fmt.Errorf("audit plan_compiled: %w", err)
	}

	// AWS config.
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(r.Region))
	if err != nil {
		return fmt.Errorf("load aws config: %w", err)
	}

	// Optional identity broker for bounded authority. Each action
	// builds its own session policy (different operations and ARNs);
	// the broker just minted+stamps credentials, the policy comes
	// from the runnable.
	if roleARN := os.Getenv("CASPER_ROLE_ARN"); roleARN != "" {
		extID := os.Getenv("CASPER_EXTERNAL_ID")
		if extID == "" {
			return fmt.Errorf("CASPER_EXTERNAL_ID is required when CASPER_ROLE_ARN is set")
		}
		brokerCfg := identity.Config{RoleARN: roleARN, ExternalID: extID}
		broker, err := identity.New(cfg, brokerCfg)
		if err != nil {
			return fmt.Errorf("identity broker: %w", err)
		}
		// Translate r.SessionPolicy(accountID) → AssumeRole. This
		// assumes the broker's MintFor* methods are per-action; for
		// now we only have MintForRDSResize, so we extend or fall
		// back as needed.
		sess, mintErr := mintCredentialsForRunnable(ctx, broker, r, brokerCfg)
		if mintErr != nil {
			return fmt.Errorf("mint credentials: %w", mintErr)
		}
		cfg = sess.Cfg
		if _, err := store.Append(ctx, audit.KindCredentialsMinted, r.Hash, map[string]any{
			"action_type":  r.ActionType,
			"role_arn":     roleARN,
			"session_name": sess.SessionName,
			"policy_hash":  sess.PolicyHash,
			"ttl_seconds":  int(identity.SessionTTL.Seconds()),
			"expires_at":   sess.Expires.UTC().Format("2006-01-02T15:04:05Z07:00"),
		}); err != nil {
			return fmt.Errorf("audit credentials_minted: %w", err)
		}
		fmt.Fprintf(os.Stderr, "identity: minted scoped credentials %s (policy %s..., expires %s)\n",
			sess.SessionName, sess.PolicyHash[:12], sess.Expires.Format(time.RFC3339))
	} else {
		fmt.Fprintln(os.Stderr, "identity: no CASPER_ROLE_ARN — using default credentials (bounded authority NOT enforced)")
	}

	// Execute.
	interp := &interpreter.Interpreter{Client: awsx.New(cfg), Audit: store}

	fmt.Fprintf(os.Stderr, "running forward plan [%s] (%d steps) on %s...\n",
		r.ActionType, len(fwd.Steps), r.Region)
	results, runErr := interp.Run(ctx, fwd)

	if runErr != nil {
		policyMode := failureMode(fwd, results)
		mutated := anyMutatingStepCompleted(fwd, results)

		if policyMode != plan.OnFailureRollback || !mutated {
			reason := fmt.Sprintf("on_failure=%s", policyMode)
			if policyMode == plan.OnFailureRollback && !mutated {
				reason = "no AWS state was changed before the failure"
			}
			fmt.Fprintf(os.Stderr, "forward plan failed: %v (no rollback — %s)\n",
				runErr, reason)
			if err := dumpAudit(store, r.Hash); err != nil {
				return err
			}
			return fmt.Errorf("forward failed: %w", runErr)
		}

		fmt.Fprintf(os.Stderr, "forward plan failed: %v\nrunning rollback (%d steps)...\n",
			runErr, len(rb.Steps))
		_, _ = store.Append(ctx, audit.KindRollbackBegun, r.Hash, map[string]any{"reason": runErr.Error()})
		_, rbErr := interp.Run(ctx, rb)
		_, _ = store.Append(ctx, audit.KindRollbackEnded, r.Hash, map[string]any{
			"ok":    rbErr == nil,
			"error": errOrEmpty(rbErr),
		})
		if err := dumpAudit(store, r.Hash); err != nil {
			return err
		}
		if rbErr != nil {
			return fmt.Errorf("forward failed (%v); rollback also failed: %w", runErr, rbErr)
		}
		return fmt.Errorf("forward failed, rolled back successfully: %w", runErr)
	}

	if err := dumpAudit(store, r.Hash); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "forward plan completed successfully")
	return nil
}

// mintCredentialsForRunnable is the bridge between the per-action
// session-policy builder and the identity broker's typed mint
// methods. Today the broker has only MintForRDSResize; for new
// actions we either add new methods or generalize the broker.
//
// For now this falls back to MintForRDSResize for unknown action
// types — bounded authority is "best effort" while the broker
// catches up to the action set. New action types should add their
// own MintFor* method on identity.Broker.
func mintCredentialsForRunnable(ctx context.Context, broker *identity.Broker, r *runnable, brokerCfg identity.Config) (identity.Session, error) {
	switch r.ActionType {
	case "rds_resize":
		// Recover the typed proposal — we still have its bytes via the audit
		// payload, but the broker needs the typed struct. The cleanest fix
		// long-term is for r.SessionPolicy to drive a generic AssumeRole
		// path on Broker; for now, dispatch by type.
		p := action.RDSResizeProposal{
			DBInstanceIdentifier: r.ProposalAuditPayload["db_instance_identifier"].(string),
			Region:               r.ProposalAuditPayload["region"].(string),
			CurrentInstanceClass: r.ProposalAuditPayload["current_instance_class"].(string),
			TargetInstanceClass:  r.ProposalAuditPayload["target_instance_class"].(string),
			ApplyImmediately:     true,
		}
		return broker.MintForRDSResize(ctx, p)
	default:
		// Snapshot etc. — broker doesn't have a per-action method yet, so
		// we mint with the rds_resize policy as a placeholder. This is a
		// known gap; documented in render-audit / the project README.
		// Practically: when CASPER_ROLE_ARN is unset (the default), this
		// path isn't reached.
		return identity.Session{}, fmt.Errorf("identity broker does not yet support action type %q — run without CASPER_ROLE_ARN", r.ActionType)
	}
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

// anyMutatingStepCompleted reports whether the forward plan made it
// past at least one state-changing AWS call.
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

func dumpAudit(s audit.Store, h action.ProposalHash) error {
	events, err := s.List(context.Background(), h)
	if err != nil {
		return fmt.Errorf("list audit: %w", err)
	}
	enc := json.NewEncoder(os.Stdout)
	for _, e := range events {
		if err := enc.Encode(e); err != nil {
			return fmt.Errorf("encode event: %w", err)
		}
	}
	if err := s.Verify(context.Background()); err != nil {
		return fmt.Errorf("audit chain verify: %w", err)
	}
	fmt.Fprintf(os.Stderr, "audit log: %d events, chain verified\n", len(events))
	return nil
}
