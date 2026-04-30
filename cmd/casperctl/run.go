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
	Long: `End-to-end execution: validates the proposal, hashes it, runs the
policy gate, optionally mints scoped credentials via STS, compiles
forward + rollback plans, and walks the forward plan against AWS while
streaming a hash-chained audit log.

Required environment:
  ANTHROPIC_API_KEY               (only for 'propose', not 'run')
  AWS_REGION + AWS credentials    (the SDK default chain)

Optional environment:
  DATABASE_URL                    Postgres DSN for durable audit
  CASPER_ROLE_ARN                 STS role to assume per action
  CASPER_EXTERNAL_ID              External ID for the role's trust policy

If both CASPER_ROLE_ARN and CASPER_EXTERNAL_ID are set, casperctl mints
per-action 15-minute credentials scoped to the proposal's resource
before invoking AWS. Without them it falls back to the default
credential chain (and prints a stderr warning).`,
	Args: cobra.ExactArgs(1),
	RunE: runProposal,
}

func init() {
	runCmd.Flags().BoolVar(&runInMemoryAudit, "in-memory-audit", false,
		"Use the in-memory audit store regardless of DATABASE_URL. Useful when a stale DATABASE_URL is set in your env but you don't want this run to require Postgres.")
	rootCmd.AddCommand(runCmd)
}

func runProposal(cmd *cobra.Command, args []string) error {
	raw, err := readProposal(args[0])
	if err != nil {
		return err
	}
	p, h, err := decodeProposal(raw)
	if err != nil {
		return err
	}

	ctx := context.Background()

	store, closeStore, err := openAuditStore(ctx, runInMemoryAudit)
	if err != nil {
		return fmt.Errorf("open audit store: %w", err)
	}
	defer closeStore()

	if _, err := store.Append(ctx, audit.KindProposed, h, map[string]any{
		"action_type":            "rds_resize",
		"db_instance_identifier": p.DBInstanceIdentifier,
		"region":                 p.Region,
		"current_instance_class": p.CurrentInstanceClass,
		"target_instance_class":  p.TargetInstanceClass,
	}); err != nil {
		return fmt.Errorf("audit proposed: %w", err)
	}

	// Policy gate.
	engine, err := policy.NewEngine(ctx)
	if err != nil {
		return fmt.Errorf("policy engine: %w", err)
	}
	verdict, err := engine.EvaluateRDSResize(ctx, p)
	if err != nil {
		return fmt.Errorf("policy evaluate: %w", err)
	}
	if _, err := store.Append(ctx, audit.KindPolicyEvaluated, h, map[string]any{
		"decision": string(verdict.Decision),
		"reason":   verdict.Reason,
	}); err != nil {
		return fmt.Errorf("audit policy_evaluated: %w", err)
	}
	fmt.Fprintf(os.Stderr, "policy: %s — %s\n", verdict.Decision, verdict.Reason)
	if verdict.Decision != policy.DecisionAllow {
		if err := dumpAudit(store, h); err != nil {
			return err
		}
		return fmt.Errorf("policy %s: %s", verdict.Decision, verdict.Reason)
	}

	// Compile plans.
	fwd, rb := plan.CompileRDSResize(p, h)
	if _, err := store.Append(ctx, audit.KindPlanCompiled, h, map[string]any{
		"forward_steps":  len(fwd.Steps),
		"rollback_steps": len(rb.Steps),
	}); err != nil {
		return fmt.Errorf("audit plan_compiled: %w", err)
	}

	// AWS config.
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(p.Region))
	if err != nil {
		return fmt.Errorf("load aws config: %w", err)
	}

	// Optional identity broker for bounded authority.
	if roleARN := os.Getenv("CASPER_ROLE_ARN"); roleARN != "" {
		extID := os.Getenv("CASPER_EXTERNAL_ID")
		if extID == "" {
			return fmt.Errorf("CASPER_EXTERNAL_ID is required when CASPER_ROLE_ARN is set")
		}
		broker, err := identity.New(cfg, identity.Config{RoleARN: roleARN, ExternalID: extID})
		if err != nil {
			return fmt.Errorf("identity broker: %w", err)
		}
		sess, err := broker.MintForRDSResize(ctx, p)
		if err != nil {
			return fmt.Errorf("mint credentials: %w", err)
		}
		cfg = sess.Cfg
		if _, err := store.Append(ctx, audit.KindCredentialsMinted, h, map[string]any{
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

	fmt.Fprintf(os.Stderr, "running forward plan (%d steps) on %s/%s...\n",
		len(fwd.Steps), p.Region, p.DBInstanceIdentifier)
	results, runErr := interp.Run(ctx, fwd)

	if runErr != nil {
		// Decide whether rollback is appropriate. Two conditions must
		// both hold for rollback to run:
		//
		//   1. The failed step's on_failure is "rollback" (not "abort").
		//   2. At least one AWS-mutating step in the forward plan
		//      completed successfully — i.e. some state actually
		//      changed. If no mutating step succeeded, there is
		//      nothing to undo; running rollback is a no-op modify
		//      that will silently time out polling for "modifying"
		//      status that will never come.
		//
		// Mutating steps are identified by convention as aws_api_call
		// steps whose on_failure is "rollback" — by design, only
		// state-changing calls in our plans carry the rollback flag.
		policy := failureMode(fwd, results)
		mutated := anyMutatingStepCompleted(fwd, results)

		if policy != plan.OnFailureRollback || !mutated {
			reason := fmt.Sprintf("on_failure=%s", policy)
			if policy == plan.OnFailureRollback && !mutated {
				reason = "no AWS state was changed before the failure"
			}
			fmt.Fprintf(os.Stderr, "forward plan failed: %v (no rollback — %s)\n",
				runErr, reason)
			if err := dumpAudit(store, h); err != nil {
				return err
			}
			return fmt.Errorf("forward failed: %w", runErr)
		}

		fmt.Fprintf(os.Stderr, "forward plan failed: %v\nrunning rollback (%d steps)...\n",
			runErr, len(rb.Steps))
		_, _ = store.Append(ctx, audit.KindRollbackBegun, h, map[string]any{"reason": runErr.Error()})
		_, rbErr := interp.Run(ctx, rb)
		_, _ = store.Append(ctx, audit.KindRollbackEnded, h, map[string]any{
			"ok":    rbErr == nil,
			"error": errOrEmpty(rbErr),
		})
		if err := dumpAudit(store, h); err != nil {
			return err
		}
		if rbErr != nil {
			return fmt.Errorf("forward failed (%v); rollback also failed: %w", runErr, rbErr)
		}
		return fmt.Errorf("forward failed, rolled back successfully: %w", runErr)
	}

	if err := dumpAudit(store, h); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "forward plan completed successfully")
	return nil
}

// failureMode looks up the OnFailure value of the step that just failed.
// It assumes the last entry in results is the failure (the interpreter
// halts after the first failed step) and matches it back against the
// plan by step ID. If we can't find a match (shouldn't happen) we
// default to OnFailureAbort — the safer choice, since "abort" is the
// no-op-on-error path.
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
// at least one state-changing AWS call. By convention, mutating steps in
// our plans are aws_api_call steps with on_failure=rollback (read-only
// describes / preconditions are aws_api_call/verify with on_failure=
// abort). If no such step ever reached "Done" status, no AWS state was
// changed, and a rollback would have nothing to undo.
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
