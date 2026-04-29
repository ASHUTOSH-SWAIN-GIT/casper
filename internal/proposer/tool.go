package proposer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/jerkeyray/starling/tool"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// proposeInput mirrors action.RDSResizeProposal. We don't reuse the
// action type directly because Starling generates the tool's JSON
// Schema from struct tags; this lets us add `jsonschema:"required"`
// hints that nudge the model toward emitting all fields. The
// authoritative validation still happens via action.Validate inside
// the tool — Starling's schema is advisory, ours is binding.
type proposeInput struct {
	DBInstanceIdentifier string             `json:"db_instance_identifier" jsonschema:"description=The RDS instance to modify."`
	Region               string             `json:"region" jsonschema:"description=AWS region the instance lives in (e.g. us-east-1)."`
	CurrentInstanceClass string             `json:"current_instance_class" jsonschema:"description=Instance class right now (must match the snapshot)."`
	TargetInstanceClass  string             `json:"target_instance_class" jsonschema:"description=Instance class to resize to."`
	ApplyImmediately     bool               `json:"apply_immediately" jsonschema:"description=Must be true for v1 — schedule-window resizes are not supported."`
	SuccessCriteria      proposeSuccess     `json:"success_criteria" jsonschema:"description=How we will verify the resize achieved its intent."`
	Reasoning            string             `json:"reasoning" jsonschema:"description=Short rationale for choosing this target class and success criteria."`
}

type proposeSuccess struct {
	Metric             string  `json:"metric" jsonschema:"description=CloudWatch metric name. v1 only supports CPUUtilization."`
	ThresholdPercent   float64 `json:"threshold_percent" jsonschema:"description=Pass condition: metric average over the verification window must be at or below this percent."`
	VerificationWindow string  `json:"verification_window" jsonschema:"description=Duration string (e.g. 5m) over which we sample the metric after the instance returns to available."`
}

type proposeOutput struct {
	Hash string `json:"proposal_hash"`
}

// captured holds the proposal raw bytes + hash that the tool emits
// during a run. Propose() reads it back after agent.Run returns to
// produce the final result. Per-Proposer (single-flight) state is
// fine for v1 — Propose calls are not concurrent on a single instance.
type captured struct {
	mu   sync.Mutex
	raw  []byte
	hash action.ProposalHash
}

func (c *captured) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.raw = nil
	c.hash = ""
}

func (c *captured) get() ([]byte, action.ProposalHash) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.raw, c.hash
}

func (c *captured) set(raw []byte, h action.ProposalHash) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.raw = raw
	c.hash = h
}

// buildProposeTool constructs the single tool the agent has access to.
// Validation against action.Validate is performed inside the tool —
// even if Starling's auto-generated schema accepted something extra,
// the authoritative schema will reject it and the tool returns an
// error to the model. The model has one turn (MaxTurns=1) so it cannot
// retry; a failed proposal terminates the run.
func buildProposeTool(c *captured) tool.Tool {
	return tool.Typed(
		"propose_action",
		"Emit exactly one structured RDS resize proposal. This is the only action you can take. Call it exactly once with all fields populated.",
		func(ctx context.Context, in proposeInput) (proposeOutput, error) {
			if in.DBInstanceIdentifier == "" {
				return proposeOutput{}, errors.New("db_instance_identifier required")
			}
			raw, err := json.Marshal(in)
			if err != nil {
				return proposeOutput{}, fmt.Errorf("marshal proposal: %w", err)
			}
			if err := action.Validate(raw); err != nil {
				return proposeOutput{}, fmt.Errorf("schema validation: %w", err)
			}
			h, err := action.Hash(raw)
			if err != nil {
				return proposeOutput{}, fmt.Errorf("hash proposal: %w", err)
			}
			c.set(raw, h)
			return proposeOutput{Hash: string(h)}, nil
		},
	)
}
