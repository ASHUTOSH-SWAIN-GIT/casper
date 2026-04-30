package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

var validateCmd = &cobra.Command{
	Use:   "validate <proposal.json>",
	Short: "Validate a proposal against the action JSON Schema",
	Long: `Validates a proposal file against the embedded RDSResizeProposal schema.
Exits 0 with "ok" on success; non-zero with the schema error otherwise.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		raw, err := readProposal(args[0])
		if err != nil {
			return err
		}
		if err := action.Validate(raw); err != nil {
			return fmt.Errorf("invalid proposal: %w", err)
		}
		fmt.Println("ok")
		return nil
	},
}

func init() { rootCmd.AddCommand(validateCmd) }
