package proposer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	starling "github.com/jerkeyray/starling"
	"github.com/jerkeyray/starling/eventlog"
	"github.com/jerkeyray/starling/provider/anthropic"
	"github.com/jerkeyray/starling/tool"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// DefaultModel is the Anthropic model used when none is specified.
// Sonnet is the right tier for one-shot tool use: it follows schemas
// reliably and is meaningfully cheaper than Opus for this workload.
const DefaultModel = "claude-sonnet-4-6"

// systemPrompt is the agent's standing brief. It is hashed by
// Starling into the RunStarted event, so the prompt that produced any
// recorded proposal is reproducible from the event log.
//
// Hard constraints come first (must call the tool exactly once,
// must not write free-form text), guidance second (how to choose a
// target class given an intent), domain knowledge last.
const systemPrompt = `You are Casper's RDS resize proposer.

Your only job is to turn a natural-language intent and an infrastructure snapshot into exactly one structured proposal by calling the propose_action tool.

Hard constraints:
- You must call propose_action exactly ONCE.
- You must not write any free-form text, explanation, or commentary outside the tool call. Reasoning belongs in the proposal's "reasoning" field.
- Every field on propose_action is required. Populate them all.
- "current_instance_class" must equal the snapshot's current_instance_class verbatim.
- "apply_immediately" must be true.
- "success_criteria.metric" must be "CPUUtilization" for v1.
- "success_criteria.verification_window" must be a Go duration string (e.g. "5m").

How to choose a target_instance_class:
- For CPU pressure / "needs more headroom" intents: pick the next size up in the same family (e.g. db.r6g.large -> db.r6g.xlarge).
- For "right-size" / "downsize" / cost intents: pick the next size down in the same family.
- Do not change instance family unless the intent specifically asks for memory- or compute-optimized.
- Do not set target equal to current — that's a no-op and will be rejected.

Threshold guidance (success_criteria.threshold_percent):
- For an upsize meant to relieve CPU pressure: target threshold is around 60 (we expect post-resize CPU to be at most 60%).
- For a downsize: target threshold is around 80 (we tolerate moderate CPU).
- Default verification_window is "5m".

Your output is a proposal, not a recommendation. Casper will independently evaluate it against policy, simulate its impact, and gate it on human approval where appropriate. Be conservative when uncertain — a smaller upsize that gets auto-allowed is more useful than a larger one that needs approval.`

// Proposer wraps a Starling agent. New constructs the agent once;
// Propose can be called repeatedly (single-flight per instance).
type Proposer struct {
	agent    *starling.Agent
	captured *captured
}

// Config configures a Proposer at construction time.
type Config struct {
	APIKey  string             // ANTHROPIC_API_KEY
	Model   string             // optional, defaults to DefaultModel
	Log     eventlog.EventLog  // required — Starling's run log
	Budget  *starling.Budget   // optional, sane defaults applied if nil
	Timeout time.Duration      // optional, applied as MaxWallClock when Budget is nil
}

// New constructs a Proposer. The Anthropic provider is built here;
// the tool closure captures the per-Proposer state used to read back
// the emitted proposal after Run returns.
func New(c Config) (*Proposer, error) {
	if c.APIKey == "" {
		return nil, errors.New("APIKey is required (ANTHROPIC_API_KEY)")
	}
	if c.Log == nil {
		return nil, errors.New("Log is required")
	}
	if c.Model == "" {
		c.Model = DefaultModel
	}
	if c.Budget == nil {
		ttl := c.Timeout
		if ttl == 0 {
			ttl = 45 * time.Second
		}
		// Tight default — a real proposer run is ~1.5K in + ~500 out
		// for the tool call, plus a short wrap-up. $0.05 is ~5x the
		// expected cost; if we trip it, something's wrong (runaway
		// retries, a stuck stream) and we want to fail closed.
		c.Budget = &starling.Budget{
			MaxInputTokens:  10_000,
			MaxOutputTokens: 2_000,
			MaxUSD:          0.05,
			MaxWallClock:    ttl,
		}
	}

	prov, err := anthropic.New(anthropic.WithAPIKey(c.APIKey))
	if err != nil {
		return nil, fmt.Errorf("anthropic provider: %w", err)
	}

	cap := &captured{}
	a := &starling.Agent{
		Provider: prov,
		Tools:    []tool.Tool{buildProposeTool(cap)},
		Log:      c.Log,
		Config: starling.Config{
			Model:        c.Model,
			SystemPrompt: systemPrompt,
			// 2 turns: turn 1 is the tool_use call (which is what we want),
			// turn 2 is the model's brief acknowledgment after seeing the
			// tool result. The proposal is captured during turn 1, so even
			// if turn 2 ran arbitrarily we'd already have what we need —
			// but Starling's ReAct loop wants to render that final turn,
			// so we let it happen and tightly bound the budget instead.
			MaxTurns: 2,
		},
		Budget: c.Budget,
	}
	return &Proposer{agent: a, captured: cap}, nil
}

// Result is what Propose returns. The proposal Raw bytes have already
// been validated by action.Validate; Hash is canonical. The Starling
// metadata (RunID, MerkleRoot) lets anyone replay the run later to
// verify the model produced exactly these bytes.
type Result struct {
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

// Propose runs the agent against intent + snapshot. Exactly one tool
// call is expected; if the agent terminates without calling
// propose_action (refused, budget exhausted, etc.), Propose returns
// an error and the partial run is still in the event log for audit.
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

	// The proposal is captured during the tool call (turn 1). If the
	// agent errors *after* that — most commonly hitting max_turns on
	// the wrap-up turn — we still have a valid proposal to return.
	// Only treat the run error as fatal if no proposal was captured.
	raw, hash := p.captured.get()
	if len(raw) == 0 {
		if runErr != nil {
			return nil, fmt.Errorf("agent run: %w", runErr)
		}
		return nil, fmt.Errorf("agent did not call propose_action (terminal: %s, run_id: %s)",
			runRes.TerminalKind, runRes.RunID)
	}

	return &Result{
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

// buildGoal turns a Request into the agent's user-message body. The
// snapshot is serialized to JSON so the model sees exactly the bytes
// the rest of the system has — no paraphrasing.
func buildGoal(req Request) (string, error) {
	snapJSON, err := json.MarshalIndent(req.Snapshot, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal snapshot: %w", err)
	}
	return fmt.Sprintf(`Intent (verbatim from operator):
%s

Current infrastructure snapshot:
%s

Call propose_action with a complete, schema-conforming proposal that addresses this intent. Do not write anything outside the tool call.`,
		req.Intent, string(snapJSON),
	), nil
}
