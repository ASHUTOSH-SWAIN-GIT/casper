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
	Long: `Detects the proposal's action type, runs the matching Rego rules,
and prints the verdict (allow / deny / needs_approval) plus the reason.

Read-only — never makes AWS calls or writes audit events.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		raw, err := readProposal(args[0])
		if err != nil {
			return err
		}
		r, err := buildRunnable(raw)
		if err != nil {
			return err
		}

		ctx := context.Background()
		engine, err := policy.NewEngine(ctx)
		if err != nil {
			return err
		}
		v, err := r.EvaluatePolicy(ctx, engine)
		if err != nil {
			return err
		}
		out := map[string]any{
			"action_type": r.ActionType,
			"decision":    v.Decision,
			"reason":      v.Reason,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	},
}

func init() { rootCmd.AddCommand(policyCmd) }
