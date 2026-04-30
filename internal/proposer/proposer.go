package proposer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	starling "github.com/jerkeyray/starling"
	"github.com/jerkeyray/starling/eventlog"
	provider_iface "github.com/jerkeyray/starling/provider"
	"github.com/jerkeyray/starling/provider/anthropic"
	"github.com/jerkeyray/starling/provider/bedrock"
	"github.com/jerkeyray/starling/tool"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// Backend selects which LLM service the agent talks to. Anthropic API
// is the default — direct calls to api.anthropic.com authenticated
// with ANTHROPIC_API_KEY. Bedrock routes through AWS Bedrock and
// authenticates with the AWS SDK credential chain (AWS_PROFILE /
// IAM role / etc.).
type Backend int

const (
	BackendAnthropic Backend = iota
	BackendBedrock
)

// DefaultModel is the Anthropic model used by per-action proposers
// when none is specified. Sonnet is the right tier for one-shot tool
// use: it follows schemas reliably and is meaningfully cheaper than
// Opus for this workload. The smaller Haiku model is used for the
// upstream NL router (see router.go).
//
// Bedrock model IDs are version-pinned and region-prefixed (e.g.
// "us.anthropic.claude-sonnet-4-5-20250929-v1:0"). Casper does NOT
// supply a default Bedrock ID — different users have access to
// different inference profiles. Pass Model explicitly when
// Backend=BackendBedrock, or set CASPER_BEDROCK_PROPOSER_MODEL.
const DefaultModel = "claude-sonnet-4-6"

// Proposer wraps a Starling agent for a single action type. New<Action>
// constructors set up the appropriate tool, system prompt, and budget;
// Propose runs the agent and returns the captured proposal bytes.
type Proposer struct {
	agent      *starling.Agent
	captured   *captured
	actionType string
}

// Config is shared across all per-action proposer constructors.
type Config struct {
	// Backend picks the LLM service. Defaults to BackendAnthropic.
	Backend Backend

	// APIKey is required when Backend=BackendAnthropic.
	APIKey string

	// Region is used when Backend=BackendBedrock. If empty, the
	// Bedrock provider falls back to AWS_REGION / AWS_DEFAULT_REGION
	// from the AWS SDK default chain.
	Region string

	// Model is the model identifier. Required when Backend=BackendBedrock
	// (Bedrock IDs are version-pinned and account-specific). Optional
	// for Anthropic — defaults to DefaultModel.
	Model string

	Log     eventlog.EventLog // required — Starling's run log
	Budget  *starling.Budget  // optional, sane defaults applied if nil
	Timeout time.Duration     // optional, applied as MaxWallClock when Budget is nil
}

// Result is the per-call return shape — same across all action types.
type Result struct {
	ActionType   string              `json:"action_type"`
	ProposalRaw  []byte              `json:"proposal"`
	ProposalHash action.ProposalHash `json:"proposal_hash"`
	RunID        string              `json:"starling_run_id"`
	MerkleRoot   []byte              `json:"merkle_root"`
	Model        string              `json:"model"`
	InputTokens  int64               `json:"input_tokens"`
	OutputTokens int64               `json:"output_tokens"`
	CostUSD      float64             `json:"cost_usd"`
	Duration     time.Duration       `json:"duration"`
}

// NewRDSResize constructs an rds_resize proposer.
func NewRDSResize(c Config) (*Proposer, error) {
	cap := &captured{}
	a, err := buildAgent(c, rdsResizeSystemPrompt, []tool.Tool{buildRDSResizeProposeTool(cap)})
	if err != nil {
		return nil, err
	}
	return &Proposer{agent: a, captured: cap, actionType: "rds_resize"}, nil
}

// NewRDSCreateSnapshot constructs an rds_create_snapshot proposer.
func NewRDSCreateSnapshot(c Config) (*Proposer, error) {
	cap := &captured{}
	a, err := buildAgent(c, rdsCreateSnapshotSystemPrompt, []tool.Tool{buildRDSCreateSnapshotProposeTool(cap)})
	if err != nil {
		return nil, err
	}
	return &Proposer{agent: a, captured: cap, actionType: "rds_create_snapshot"}, nil
}

// NewRDSModifyBackupRetention constructs an rds_modify_backup_retention proposer.
func NewRDSModifyBackupRetention(c Config) (*Proposer, error) {
	cap := &captured{}
	a, err := buildAgent(c, rdsModifyBackupRetentionSystemPrompt, []tool.Tool{buildRDSModifyBackupRetentionProposeTool(cap)})
	if err != nil {
		return nil, err
	}
	return &Proposer{agent: a, captured: cap, actionType: "rds_modify_backup_retention"}, nil
}

// NewRDSRebootInstance constructs an rds_reboot_instance proposer.
func NewRDSRebootInstance(c Config) (*Proposer, error) {
	cap := &captured{}
	a, err := buildAgent(c, rdsRebootInstanceSystemPrompt, []tool.Tool{buildRDSRebootInstanceProposeTool(cap)})
	if err != nil {
		return nil, err
	}
	return &Proposer{agent: a, captured: cap, actionType: "rds_reboot_instance"}, nil
}

// NewRDSModifyMultiAZ constructs an rds_modify_multi_az proposer.
func NewRDSModifyMultiAZ(c Config) (*Proposer, error) {
	cap := &captured{}
	a, err := buildAgent(c, rdsModifyMultiAZSystemPrompt, []tool.Tool{buildRDSModifyMultiAZProposeTool(cap)})
	if err != nil {
		return nil, err
	}
	return &Proposer{agent: a, captured: cap, actionType: "rds_modify_multi_az"}, nil
}

// NewRDSStorageGrow constructs an rds_storage_grow proposer.
func NewRDSStorageGrow(c Config) (*Proposer, error) {
	cap := &captured{}
	a, err := buildAgent(c, rdsStorageGrowSystemPrompt, []tool.Tool{buildRDSStorageGrowProposeTool(cap)})
	if err != nil {
		return nil, err
	}
	return &Proposer{agent: a, captured: cap, actionType: "rds_storage_grow"}, nil
}

// NewRDSDeleteSnapshot constructs an rds_delete_snapshot proposer.
func NewRDSDeleteSnapshot(c Config) (*Proposer, error) {
	cap := &captured{}
	a, err := buildAgent(c, rdsDeleteSnapshotSystemPrompt, []tool.Tool{buildRDSDeleteSnapshotProposeTool(cap)})
	if err != nil {
		return nil, err
	}
	return &Proposer{agent: a, captured: cap, actionType: "rds_delete_snapshot"}, nil
}

// NewRDSCreateReadReplica constructs an rds_create_read_replica proposer.
func NewRDSCreateReadReplica(c Config) (*Proposer, error) {
	cap := &captured{}
	a, err := buildAgent(c, rdsCreateReadReplicaSystemPrompt, []tool.Tool{buildRDSCreateReadReplicaProposeTool(cap)})
	if err != nil {
		return nil, err
	}
	return &Proposer{agent: a, captured: cap, actionType: "rds_create_read_replica"}, nil
}

// NewRDSModifyEngineVersion constructs an rds_modify_engine_version proposer.
func NewRDSModifyEngineVersion(c Config) (*Proposer, error) {
	cap := &captured{}
	a, err := buildAgent(c, rdsModifyEngineVersionSystemPrompt, []tool.Tool{buildRDSModifyEngineVersionProposeTool(cap)})
	if err != nil {
		return nil, err
	}
	return &Proposer{agent: a, captured: cap, actionType: "rds_modify_engine_version"}, nil
}

// NewRDSRestoreFromSnapshot constructs an rds_restore_from_snapshot proposer.
func NewRDSRestoreFromSnapshot(c Config) (*Proposer, error) {
	cap := &captured{}
	a, err := buildAgent(c, rdsRestoreFromSnapshotSystemPrompt, []tool.Tool{buildRDSRestoreFromSnapshotProposeTool(cap)})
	if err != nil {
		return nil, err
	}
	return &Proposer{agent: a, captured: cap, actionType: "rds_restore_from_snapshot"}, nil
}

// NewForAction is the generic dispatcher used by the CLI's NL mode.
// It looks up the action type in the registry and returns the
// matching Proposer.
func NewForAction(actionType string, c Config) (*Proposer, error) {
	if _, ok := action.Lookup(actionType); !ok {
		return nil, fmt.Errorf("unknown action type %q", actionType)
	}
	switch actionType {
	case "rds_resize":
		return NewRDSResize(c)
	case "rds_create_snapshot":
		return NewRDSCreateSnapshot(c)
	case "rds_modify_backup_retention":
		return NewRDSModifyBackupRetention(c)
	case "rds_reboot_instance":
		return NewRDSRebootInstance(c)
	case "rds_modify_multi_az":
		return NewRDSModifyMultiAZ(c)
	case "rds_storage_grow":
		return NewRDSStorageGrow(c)
	case "rds_delete_snapshot":
		return NewRDSDeleteSnapshot(c)
	case "rds_create_read_replica":
		return NewRDSCreateReadReplica(c)
	case "rds_modify_engine_version":
		return NewRDSModifyEngineVersion(c)
	case "rds_restore_from_snapshot":
		return NewRDSRestoreFromSnapshot(c)
	default:
		return nil, fmt.Errorf("no proposer wired for action type %q (registered but not yet implemented)", actionType)
	}
}

// buildAgent is the shared agent-construction path. Each action's
// New<Action> constructor calls into here with its own system prompt
// and tool — the budget, MaxTurns, and provider plumbing are shared.
func buildAgent(c Config, systemPrompt string, tools []tool.Tool) (*starling.Agent, error) {
	if c.Log == nil {
		return nil, errors.New("Log is required")
	}

	prov, model, err := buildProvider(c)
	if err != nil {
		return nil, err
	}

	budget := c.Budget
	if budget == nil {
		ttl := c.Timeout
		if ttl == 0 {
			ttl = 45 * time.Second
		}
		budget = &starling.Budget{
			MaxInputTokens:  10_000,
			MaxOutputTokens: 2_000,
			MaxUSD:          0.05,
			MaxWallClock:    ttl,
		}
	}

	return &starling.Agent{
		Provider: prov,
		Tools:    tools,
		Log:      c.Log,
		Config: starling.Config{
			Model:        model,
			SystemPrompt: systemPrompt,
			MaxTurns:     2,
		},
		Budget: budget,
	}, nil
}

// buildProvider constructs the Starling provider for the configured
// backend and returns the resolved model ID. Centralized so both the
// per-action proposers and the NL router (router.go) share the same
// switch.
func buildProvider(c Config) (provider_iface.Provider, string, error) {
	switch c.Backend {
	case BackendBedrock:
		if c.Model == "" {
			return nil, "", errors.New("Model is required when Backend=BackendBedrock (no defaults — Bedrock IDs are version-pinned and account-specific; set CASPER_BEDROCK_PROPOSER_MODEL or pass explicitly)")
		}
		opts := []bedrock.Option{}
		if c.Region != "" {
			opts = append(opts, bedrock.WithRegion(c.Region))
		}
		prov, err := bedrock.New(opts...)
		if err != nil {
			return nil, "", fmt.Errorf("bedrock provider: %w", err)
		}
		return prov, c.Model, nil

	default: // BackendAnthropic
		if c.APIKey == "" {
			return nil, "", errors.New("APIKey is required for Backend=BackendAnthropic (ANTHROPIC_API_KEY)")
		}
		model := c.Model
		if model == "" {
			model = DefaultModel
		}
		prov, err := anthropic.New(anthropic.WithAPIKey(c.APIKey))
		if err != nil {
			return nil, "", fmt.Errorf("anthropic provider: %w", err)
		}
		return prov, model, nil
	}
}

// Propose runs the agent. The captured proposal is returned even when
// the agent terminates with max_turns after a successful tool call —
// see docs/starling-feedback.md §1 for context on this defensive read.
func (p *Proposer) Propose(ctx context.Context, req Request) (*Result, error) {
	if req.Intent == "" {
		return nil, errors.New("intent is required")
	}
	if req.Snapshot.DBInstanceIdentifier == "" {
		return nil, errors.New("snapshot.db_instance_identifier is required")
	}

	p.captured.reset()
	goal, err := buildGoal(req)
	if err != nil {
		return nil, err
	}

	runRes, runErr := p.agent.Run(ctx, goal)

	raw, hash := p.captured.get()
	if len(raw) == 0 {
		if runErr != nil {
			return nil, fmt.Errorf("agent run: %w", runErr)
		}
		return nil, fmt.Errorf("agent did not call the proposal tool (terminal: %s, run_id: %s)",
			runRes.TerminalKind, runRes.RunID)
	}

	return &Result{
		ActionType:   p.actionType,
		ProposalRaw:  raw,
		ProposalHash: hash,
		RunID:        runRes.RunID,
		MerkleRoot:   runRes.MerkleRoot,
		Model:        p.agent.Config.Model,
		InputTokens:  runRes.InputTokens,
		OutputTokens: runRes.OutputTokens,
		CostUSD:      runRes.TotalCostUSD,
		Duration:     runRes.Duration,
	}, nil
}

// buildGoal turns a Request into the agent's user-message body. Same
// shape across action types — the agent's per-action system prompt
// guides which fields to populate in the proposal.
func buildGoal(req Request) (string, error) {
	snapJSON, err := json.MarshalIndent(req.Snapshot, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal snapshot: %w", err)
	}
	return fmt.Sprintf(`Intent (verbatim from operator):
%s

Current infrastructure snapshot:
%s

Call the proposal tool with a complete, schema-conforming proposal that addresses this intent. Do not write anything outside the tool call.`,
		req.Intent, string(snapJSON),
	), nil
}
