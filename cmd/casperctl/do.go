package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/jerkeyray/starling/eventlog"
	"github.com/spf13/cobra"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/proposer"
)

var (
	doYes            bool
	doInMemoryAudit  bool
)

var doCmd = &cobra.Command{
	Use:   "do",
	Short: "End-to-end: route intent → propose → policy → execute against AWS",
	Long: `One-shot pipeline. Takes a free-form operator intent, routes it to
the right action, asks the proposer to fill in target values, prints
the proposal for review, and (after confirmation) runs the full
trust-layer pipeline: validate → policy gate → compile forward+rollback
plans → optional STS mint → execute against AWS → hash-chained audit log.

Example:
  casperctl do --intent "scale up casper-testt, CPU is at 90%" \
               --instance casper-testt --region ap-south-1 \
               --current-class db.t4g.micro

Flags mirror 'propose' (NL mode). Use --yes to skip the confirmation
prompt; use --in-memory-audit to bypass DATABASE_URL.

Backends and credentials work the same as 'propose' and 'run' — see
their --help for the full env-var list.`,
	Args: cobra.NoArgs,
	RunE: runDo,
}

func init() {
	doCmd.Flags().StringVar(&proposeIntent, "intent", "",
		"Natural-language operator intent. Required.")
	doCmd.Flags().StringVar(&proposeInstance, "instance", "",
		"DB instance identifier. Required if not extractable from intent.")
	doCmd.Flags().StringVar(&proposeRegion, "region", "",
		"AWS region. Required if not extractable from intent.")
	doCmd.Flags().StringVar(&proposeCurrent, "current-class", "",
		"Current DB instance class (used when classifier picks rds_resize).")
	doCmd.Flags().StringVar(&proposeEngine, "engine", "postgres",
		"Engine name. Defaults to postgres.")
	doCmd.Flags().StringVar(&proposeEngineVer, "engine-version", "",
		"Engine version. Optional.")
	doCmd.Flags().IntVar(&proposeRetention, "current-backup-retention", 7,
		"Current backup retention in days (used by rds_modify_backup_retention).")
	doCmd.Flags().BoolVar(&proposeMultiAZ, "multi-az", false,
		"Whether the instance is currently Multi-AZ.")
	doCmd.Flags().BoolVarP(&doYes, "yes", "y", false,
		"Skip the confirmation prompt and execute immediately.")
	doCmd.Flags().BoolVar(&doInMemoryAudit, "in-memory-audit", false,
		"Use the in-memory audit store regardless of DATABASE_URL.")
	rootCmd.AddCommand(doCmd)
}

func runDo(cmd *cobra.Command, args []string) error {
	if strings.TrimSpace(proposeIntent) == "" {
		return fmt.Errorf("--intent is required")
	}

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
	raw, err := generateProposal(ctx, cfg, starLog)
	if err != nil {
		return err
	}

	var pretty bytes.Buffer
	if err := json.Indent(&pretty, raw, "", "  "); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "\nproposal:")
	fmt.Fprintln(os.Stderr, pretty.String())

	if !doYes {
		fmt.Fprint(os.Stderr, "\nexecute against AWS? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
		default:
			fmt.Fprintln(os.Stderr, "aborted (no AWS calls were made)")
			return nil
		}
	}

	return executeProposal(ctx, raw, doInMemoryAudit)
}

// generateProposal runs the router + per-action proposer and returns the
// proposal JSON bytes — the same bytes 'casperctl propose' would emit to
// stdout in NL mode.
func generateProposal(ctx context.Context, cfg llmCfg, starLog eventlog.EventLog) ([]byte, error) {
	router, err := proposer.NewRouter(proposer.RouterConfig{
		Backend: cfg.Backend,
		APIKey:  cfg.APIKey,
		Region:  cfg.Region,
		Model:   cfg.RouterModel,
		Log:     starLog,
	})
	if err != nil {
		return nil, fmt.Errorf("build router: %w", err)
	}
	routing, err := router.Route(ctx, proposeIntent)
	if err != nil {
		return nil, fmt.Errorf("route intent: %w", err)
	}
	fmt.Fprintf(os.Stderr, "router: action=%s instance=%s confidence=%s reasoning=%s\n",
		routing.ActionType, routing.DBInstanceIdentifier, routing.Confidence, routing.Reasoning)

	instance := routing.DBInstanceIdentifier
	if proposeInstance != "" {
		instance = proposeInstance
	}
	region := routing.Region
	if proposeRegion != "" {
		region = proposeRegion
	}
	if instance == "" {
		return nil, fmt.Errorf("could not determine target instance — pass --instance explicitly")
	}
	if region == "" {
		return nil, fmt.Errorf("could not determine region — pass --region explicitly")
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
		return nil, fmt.Errorf("build proposer: %w", err)
	}
	res, err := prop.Propose(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("propose: %w", err)
	}
	fmt.Fprintf(os.Stderr,
		"proposer: action=%s model=%s run_id=%s tokens=in:%d/out:%d cost=$%.4f duration=%s\n",
		res.ActionType, res.Model, res.RunID, res.InputTokens, res.OutputTokens, res.CostUSD, res.Duration,
	)
	fmt.Fprintf(os.Stderr, "proposal hash: %s\n", res.ProposalHash)
	return res.ProposalRaw, nil
}
