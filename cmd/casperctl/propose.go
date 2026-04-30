package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/jerkeyray/starling/eventlog"
	"github.com/spf13/cobra"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/llmcfg"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/proposer"
)

var (
	proposeIntent     string
	proposeInstance   string
	proposeRegion     string
	proposeCurrent    string
	proposeRetention  int
	proposeMultiAZ    bool
	proposeEngine     string
	proposeEngineVer  string
)

var proposeCmd = &cobra.Command{
	Use:   "propose [request.json]",
	Short: "Run the LLM proposer (file mode or NL mode); emits a typed proposal JSON",
	Long: `Two modes:

  FILE MODE (legacy):
    casperctl propose request.json > proposal.json

    Reads { intent, snapshot } from a request file and runs the proposer
    for the rds_resize action. Snapshot fields are taken verbatim from
    the file.

  NL MODE (new):
    casperctl propose --intent "scale up orders-prod, it's at 90% CPU" \
                      --instance orders-prod \
                      --region us-east-1 \
                      --current-class db.t4g.micro

    Routes the intent to the right action via a tiny Haiku-based
    classifier, then runs the matching per-action proposer with the
    snapshot fields you provide as flags.

  Backends:
    Default — Anthropic API direct.
      Required: ANTHROPIC_API_KEY.

    AWS Bedrock — set CASPER_LLM_BACKEND=bedrock.
      Required: AWS credentials in the standard SDK chain (AWS_PROFILE
      / AWS_ACCESS_KEY_ID etc.), AWS_REGION, and explicit Bedrock
      model IDs via CASPER_BEDROCK_PROPOSER_MODEL and
      CASPER_BEDROCK_ROUTER_MODEL (Bedrock IDs are version-pinned and
      account-specific; e.g. "us.anthropic.claude-sonnet-4-5-20250929-v1:0").

  Optional model overrides (Anthropic backend):
    CASPER_PROPOSER_MODEL  — default: claude-sonnet-4-6
    CASPER_ROUTER_MODEL    — default: claude-haiku-4-5

  The Starling event log lives at $CASPER_PROPOSER_LOG (default
  ./casper_proposer.db) for replay and inspection.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPropose,
}

func init() {
	proposeCmd.Flags().StringVar(&proposeIntent, "intent", "",
		"Natural-language operator intent (NL mode). When set, the request file is ignored.")
	proposeCmd.Flags().StringVar(&proposeInstance, "instance", "",
		"DB instance identifier (NL mode). Required if --intent is set.")
	proposeCmd.Flags().StringVar(&proposeRegion, "region", "",
		"AWS region (NL mode). Required if --intent is set.")
	proposeCmd.Flags().StringVar(&proposeCurrent, "current-class", "",
		"Current DB instance class, e.g. db.t4g.micro (NL mode, used when classifier picks rds_resize).")
	proposeCmd.Flags().StringVar(&proposeEngine, "engine", "postgres",
		"Engine name (NL mode). Defaults to postgres.")
	proposeCmd.Flags().StringVar(&proposeEngineVer, "engine-version", "",
		"Engine version (NL mode). Optional.")
	proposeCmd.Flags().IntVar(&proposeRetention, "current-backup-retention", 7,
		"Current backup retention in days (NL mode, used by rds_modify_backup_retention).")
	proposeCmd.Flags().BoolVar(&proposeMultiAZ, "multi-az", false,
		"Whether the instance is Multi-AZ (NL mode).")
	rootCmd.AddCommand(proposeCmd)
}

func runPropose(cmd *cobra.Command, args []string) error {
	cfg, err := llmConfigFromEnv()
	if err != nil {
		return err
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

	ctx := context.Background()

	// NL mode if --intent is set; otherwise file mode.
	if proposeIntent != "" {
		return runProposeNL(ctx, cfg, starLog)
	}
	if len(args) != 1 {
		return fmt.Errorf("file mode requires exactly one positional argument: <request.json>\n  (or use --intent for NL mode)")
	}
	return runProposeFile(ctx, cfg, starLog, args[0])
}

// llmCfg is an alias of llmcfg.Config kept so the rest of the CLI's
// internal helpers don't need to change names.
type llmCfg = llmcfg.Config

// llmConfigFromEnv defers to llmcfg.FromEnv. The CLI and casperd share
// the same env-derived configuration so behavior is consistent across
// both surfaces.
func llmConfigFromEnv() (llmCfg, error) {
	return llmcfg.FromEnv()
}

// runProposeFile is the legacy file-based path: reads { intent, snapshot }
// from a JSON file, runs the rds_resize proposer.
func runProposeFile(ctx context.Context, cfg llmCfg, starLog eventlog.EventLog, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read request: %w", err)
	}
	var req proposer.Request
	if err := json.Unmarshal(raw, &req); err != nil {
		return fmt.Errorf("parse request: %w", err)
	}

	prop, err := proposer.NewRDSResize(proposer.Config{
		Backend: cfg.Backend,
		APIKey:  cfg.APIKey,
		Region:  cfg.Region,
		Model:   cfg.ProposerModel,
		Log:     starLog,
	})
	if err != nil {
		return fmt.Errorf("build proposer: %w", err)
	}
	return runAndPrint(ctx, prop, req)
}

// runProposeNL is the NL-routing path: uses the cheap classifier to
// pick an action type, then runs the matching per-action proposer
// with the snapshot fields supplied as flags.
func runProposeNL(ctx context.Context, cfg llmCfg, starLog eventlog.EventLog) error {
	router, err := proposer.NewRouter(proposer.RouterConfig{
		Backend: cfg.Backend,
		APIKey:  cfg.APIKey,
		Region:  cfg.Region,
		Model:   cfg.RouterModel,
		Log:     starLog,
	})
	if err != nil {
		return fmt.Errorf("build router: %w", err)
	}
	batchRouting, err := router.Route(ctx, proposeIntent)
	if err != nil {
		return fmt.Errorf("route intent: %w", err)
	}
	if len(batchRouting.Actions) == 0 {
		return fmt.Errorf("router returned no actions")
	}
	routing := batchRouting.Actions[0]
	fmt.Fprintf(os.Stderr, "router: action=%s instance=%s confidence=%s reasoning=%s\n",
		routing.ActionType, routing.DBInstanceIdentifier, routing.Confidence, routing.Reasoning)

	// Apply user-provided fallbacks. The classifier extracts the
	// instance and region when explicit; if absent, fall back to flags.
	instance := routing.DBInstanceIdentifier
	if proposeInstance != "" {
		instance = proposeInstance
	}
	region := routing.Region
	if proposeRegion != "" {
		region = proposeRegion
	}
	if instance == "" {
		return fmt.Errorf("could not determine target instance — pass --instance explicitly")
	}
	if region == "" {
		return fmt.Errorf("could not determine region — pass --region explicitly")
	}

	snapshot := proposer.Snapshot{
		DBInstanceIdentifier: instance,
		Region:               region,
		CurrentInstanceClass: proposeCurrent,
		Engine:               proposeEngine,
		EngineVersion:        proposeEngineVer,
		Status:               "available",
		MultiAZ:              proposeMultiAZ,
	}
	req := proposer.Request{Intent: proposeIntent, Snapshot: snapshot}

	prop, err := proposer.NewForAction(routing.ActionType, proposer.Config{
		Backend: cfg.Backend,
		APIKey:  cfg.APIKey,
		Region:  cfg.Region,
		Model:   cfg.ProposerModel,
		Log:     starLog,
	})
	if err != nil {
		return fmt.Errorf("build proposer: %w", err)
	}
	return runAndPrint(ctx, prop, req)
}

func runAndPrint(ctx context.Context, prop *proposer.Proposer, req proposer.Request) error {
	res, err := prop.Propose(ctx, req)
	if err != nil {
		return fmt.Errorf("propose: %w", err)
	}

	fmt.Fprintf(os.Stderr,
		"proposer: action=%s model=%s run_id=%s tokens=in:%d/out:%d cost=$%.4f duration=%s\n",
		res.ActionType, res.Model, res.RunID, res.InputTokens, res.OutputTokens, res.CostUSD, res.Duration,
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
}
