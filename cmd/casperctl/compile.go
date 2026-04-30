package main

import (
	"encoding/json"
	"os"

	"github.com/spf13/cobra"
)

var compileCmd = &cobra.Command{
	Use:   "compile <proposal.json>",
	Short: "Compile forward + rollback execution plans (no AWS calls)",
	Long: `Compiles a proposal into the typed ExecutionPlan that the interpreter
would walk. Emits both the forward plan (steps that achieve the action)
and the rollback plan (steps that undo it on failure), bound to the
proposal hash. Useful for inspecting exactly which AWS calls 'run' would
make before approving or executing.

Dispatches by action type — supports rds_resize and rds_create_snapshot.`,
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
		fwd, rb := r.Compile()
		out := map[string]any{
			"action_type":   r.ActionType,
			"proposal_hash": r.Hash,
			"forward":       fwd,
			"rollback":      rb,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	},
}

func init() { rootCmd.AddCommand(compileCmd) }
