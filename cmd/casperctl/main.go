package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/audit"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/awsx"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/interpreter"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/plan"
)

const usage = `casperctl — Casper trust layer CLI

Usage:
  casperctl validate <proposal.json>   Validate a proposal against the schema
  casperctl hash     <proposal.json>   Print the canonical proposal hash
  casperctl compile  <proposal.json>   Compile forward + rollback plans (JSON)
  casperctl run      <proposal.json>   Execute the plan against AWS
                                       (requires AWS credentials in env)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "validate":
		err = runValidate(args)
	case "hash":
		err = runHash(args)
	case "compile":
		err = runCompile(args)
	case "run":
		err = runRun(args)
	case "-h", "--help", "help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func readProposal(args []string) ([]byte, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("expected one argument: <proposal.json>")
	}
	return os.ReadFile(args[0])
}

func runValidate(args []string) error {
	raw, err := readProposal(args)
	if err != nil {
		return err
	}
	if err := action.Validate(raw); err != nil {
		return fmt.Errorf("invalid proposal: %w", err)
	}
	fmt.Println("ok")
	return nil
}

func runHash(args []string) error {
	raw, err := readProposal(args)
	if err != nil {
		return err
	}
	h, err := action.Hash(raw)
	if err != nil {
		return err
	}
	fmt.Println(h)
	return nil
}

func runCompile(args []string) error {
	raw, err := readProposal(args)
	if err != nil {
		return err
	}
	if err := action.Validate(raw); err != nil {
		return fmt.Errorf("invalid proposal: %w", err)
	}
	var p action.RDSResizeProposal
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("decode proposal: %w", err)
	}
	h, err := action.Hash(raw)
	if err != nil {
		return err
	}

	fwd, rb := plan.CompileRDSResize(p, h)
	out := map[string]any{
		"proposal_hash": h,
		"forward":       fwd,
		"rollback":      rb,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func runRun(args []string) error {
	raw, err := readProposal(args)
	if err != nil {
		return err
	}
	if err := action.Validate(raw); err != nil {
		return fmt.Errorf("invalid proposal: %w", err)
	}
	var p action.RDSResizeProposal
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("decode proposal: %w", err)
	}
	h, err := action.Hash(raw)
	if err != nil {
		return err
	}
	fwd, rb := plan.CompileRDSResize(p, h)

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(p.Region))
	if err != nil {
		return fmt.Errorf("load aws config: %w", err)
	}

	store := audit.NewMemoryStore(nil)
	if _, err := store.Append(ctx, audit.KindProposed, h, map[string]any{
		"action_type":            "rds_resize",
		"db_instance_identifier": p.DBInstanceIdentifier,
		"region":                 p.Region,
		"current_instance_class": p.CurrentInstanceClass,
		"target_instance_class":  p.TargetInstanceClass,
	}); err != nil {
		return fmt.Errorf("audit proposed: %w", err)
	}
	if _, err := store.Append(ctx, audit.KindPlanCompiled, h, map[string]any{
		"forward_steps":  len(fwd.Steps),
		"rollback_steps": len(rb.Steps),
	}); err != nil {
		return fmt.Errorf("audit plan_compiled: %w", err)
	}

	interp := &interpreter.Interpreter{Client: awsx.New(cfg), Audit: store}

	fmt.Fprintf(os.Stderr, "running forward plan (%d steps) on %s/%s...\n",
		len(fwd.Steps), p.Region, p.DBInstanceIdentifier)
	_, runErr := interp.Run(ctx, fwd)

	if runErr != nil {
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

func dumpAudit(s *audit.MemoryStore, h action.ProposalHash) error {
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

func errOrEmpty(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
