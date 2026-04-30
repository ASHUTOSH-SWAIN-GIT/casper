package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/spf13/cobra"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/policy"
)

var policyCmd = &cobra.Command{
	Use:   "policy <proposal.json>",
	Short: "Evaluate a proposal against the policy engine",
	Long: `Runs the embedded Rego policy against a proposal and prints the
verdict (allow / deny / needs_approval) plus the reason that fired.

This subcommand is read-only — it never makes AWS calls and never writes
audit events. Use it to check whether a proposal would pass the policy
gate before invoking 'run'.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		raw, err := readProposal(args[0])
		if err != nil {
			return err
		}
		p, _, err := decodeProposal(raw)
		if err != nil {
			return err
		}

		ctx := context.Background()
		engine, err := policy.NewEngine(ctx)
		if err != nil {
			return err
		}
		v, err := engine.EvaluateRDSResize(ctx, p)
		if err != nil {
			return err
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	},
}

func init() { rootCmd.AddCommand(policyCmd) }
