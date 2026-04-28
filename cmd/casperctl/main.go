package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/plan"
)

const usage = `casperctl — Casper trust layer CLI

Usage:
  casperctl validate <proposal.json>   Validate a proposal against the schema
  casperctl hash     <proposal.json>   Print the canonical proposal hash
  casperctl compile  <proposal.json>   Compile forward + rollback plans (JSON)
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
