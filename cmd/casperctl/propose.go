package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/jerkeyray/starling/eventlog"
	"github.com/spf13/cobra"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/proposer"
)

var proposeCmd = &cobra.Command{
	Use:   "propose <request.json>",
	Short: "LLM intent + snapshot → typed proposal JSON (requires ANTHROPIC_API_KEY)",
	Long: `Runs the Starling-backed proposer agent against a request file
containing { intent, snapshot } and emits a validated proposal JSON to
stdout. Run metadata (model, run_id, tokens, cost) goes to stderr.

The agent is single-turn and single-tool: it must call propose_action
exactly once. The result is validated against the action JSON Schema
inside the tool body — what reaches stdout has already passed schema
enforcement and has a stable hash.

The Starling event log lives in ./casper_proposer.db (or
$CASPER_PROPOSER_LOG) for replay and inspection.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			return fmt.Errorf("ANTHROPIC_API_KEY is required")
		}
		raw, err := os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("read request: %w", err)
		}
		var req proposer.Request
		if err := json.Unmarshal(raw, &req); err != nil {
			return fmt.Errorf("parse request: %w", err)
		}

		logPath := os.Getenv("CASPER_PROPOSER_LOG")
		if logPath == "" {
			logPath = "casper_proposer.db"
		}
		starLog, err := eventlog.NewSQLite(logPath)
		if err != nil {
			return fmt.Errorf("open starling log: %w", err)
		}
		defer starLog.Close()

		prop, err := proposer.New(proposer.Config{
			APIKey: apiKey,
			Log:    starLog,
		})
		if err != nil {
			return fmt.Errorf("build proposer: %w", err)
		}

		ctx := context.Background()
		res, err := prop.Propose(ctx, req)
		if err != nil {
			return fmt.Errorf("propose: %w", err)
		}

		fmt.Fprintf(os.Stderr,
			"proposer: model=%s run_id=%s tokens=in:%d/out:%d cost=$%.4f duration=%s\n",
			res.Model, res.RunID, res.InputTokens, res.OutputTokens, res.CostUSD, res.Duration,
		)
		fmt.Fprintf(os.Stderr, "proposal hash: %s\n", res.ProposalHash)

		var pretty bytes.Buffer
		if err := json.Indent(&pretty, res.ProposalRaw, "", "  "); err != nil {
			return err
		}
		if _, err := os.Stdout.Write(pretty.Bytes()); err != nil {
			return err
		}
		_, err = os.Stdout.Write([]byte("\n"))
		return err
	},
}

func init() { rootCmd.AddCommand(proposeCmd) }
