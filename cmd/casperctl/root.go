package main

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "casperctl",
	Short: "Casper — AI trust layer for cloud infrastructure",
	Long: `casperctl drives Casper's trust layer from the command line.

Typical flow:
  1. casperctl propose request.json > proposal.json    (LLM produces a typed proposal)
  2. casperctl validate proposal.json                  (schema enforcement)
  3. casperctl policy   proposal.json                  (allow / deny / needs_approval)
  4. casperctl compile  proposal.json                  (the AWS calls that would run)
  5. casperctl run      proposal.json                  (executes against AWS, writes audit)

Steps 1–4 are local; step 5 mints AWS calls. The LLM is involved only
in step 1 — every other step is deterministic.`,
	SilenceUsage: true,
}
