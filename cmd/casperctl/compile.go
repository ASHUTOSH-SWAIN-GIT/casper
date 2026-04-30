package main

import (
	"encoding/json"
	"os"

	"github.com/spf13/cobra"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/plan"
)

var compileCmd = &cobra.Command{
	Use:   "compile <proposal.json>",
	Short: "Compile forward + rollback execution plans (no AWS calls)",
	Long: `Compiles a proposal into the typed ExecutionPlan that the interpreter
would walk. Emits both the forward plan (steps that achieve the action)
and the rollback plan (steps that undo it on failure), bound to the
proposal hash. Useful for inspecting exactly which AWS calls 'run' would
make before approving or executing.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		raw, err := readProposal(args[0])
		if err != nil {
			return err
		}
		p, h, err := decodeProposal(raw)
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
	},
}

func init() { rootCmd.AddCommand(compileCmd) }
