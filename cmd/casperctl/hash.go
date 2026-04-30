package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

var hashCmd = &cobra.Command{
	Use:   "hash <proposal.json>",
	Short: "Print the canonical sha256 hash of a proposal",
	Long: `Computes the proposal hash — sha256 over the canonical (sorted-key,
normalized-number) JSON form. This is the predictability anchor: every
downstream artifact (policy verdict, plan, audit event) references this
hash, and the interpreter refuses to execute a plan whose source
proposal hash doesn't match an approved record.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		raw, err := readProposal(args[0])
		if err != nil {
			return err
		}
		h, err := action.Hash(raw)
		if err != nil {
			return err
		}
		fmt.Println(h)
		return nil
	},
}

func init() { rootCmd.AddCommand(hashCmd) }
