package proposer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	starling "github.com/jerkeyray/starling"
	"github.com/jerkeyray/starling/eventlog"
	"github.com/jerkeyray/starling/provider/anthropic"
	"github.com/jerkeyray/starling/tool"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// DefaultRouterModel is the model used for NL routing. Haiku is ~5x
// cheaper than Sonnet and the routing task is simple enough that the
// extra capability of larger models doesn't justify the cost.
const DefaultRouterModel = "claude-haiku-4-5"

// Routing is what the router emits.
type Routing struct {
	ActionType         string `json:"action_type"`
	DBInstanceIdentifier string `json:"db_instance_identifier"`
	Region             string `json:"region,omitempty"`
	Confidence         string `json:"confidence,omitempty"` // "high" | "medium" | "low"
	Reasoning          string `json:"reasoning,omitempty"`
}

// Router classifies a free-form intent into a Casper action type plus
// the named resource. It is a tiny single-tool single-turn agent —
// same architecture as the per-action proposer, just smaller.
type Router struct {
	agent      *starling.Agent
	captured   *routerCapture
}

type routerCapture struct {
	out *Routing
}

func (c *routerCapture) reset()                    { c.out = nil }
func (c *routerCapture) set(r Routing)             { c.out = &r }
func (c *routerCapture) get() *Routing             { return c.out }

// RouterConfig configures a Router.
type RouterConfig struct {
	APIKey  string            // ANTHROPIC_API_KEY
	Model   string            // optional, defaults to DefaultRouterModel
	Log     eventlog.EventLog // required — Starling's run log
	Timeout time.Duration     // optional, defaults to 20s
}

// NewRouter constructs a Router. The router holds its own Starling
// agent — separate from the per-action proposer — because its prompt,
// tool, and model differ.
func NewRouter(c RouterConfig) (*Router, error) {
	if c.APIKey == "" {
		return nil, errors.New("APIKey is required")
	}
	if c.Log == nil {
		return nil, errors.New("Log is required")
	}
	if c.Model == "" {
		c.Model = DefaultRouterModel
	}
	ttl := c.Timeout
	if ttl == 0 {
		ttl = 20 * time.Second
	}

	prov, err := anthropic.New(anthropic.WithAPIKey(c.APIKey))
	if err != nil {
		return nil, fmt.Errorf("anthropic provider: %w", err)
	}

	cap := &routerCapture{}
	a := &starling.Agent{
		Provider: prov,
		Tools:    []tool.Tool{buildClassifyTool(cap)},
		Log:      c.Log,
		Config: starling.Config{
			Model:        c.Model,
			SystemPrompt: routerSystemPrompt(),
			MaxTurns:     2,
		},
		Budget: &starling.Budget{
			MaxInputTokens:  4_000,
			MaxOutputTokens: 1_000,
			MaxUSD:          0.01, // routing should be cheap
			MaxWallClock:    ttl,
		},
	}
	return &Router{agent: a, captured: cap}, nil
}

// Route classifies a free-form intent.
func (r *Router) Route(ctx context.Context, intent string) (*Routing, error) {
	if strings.TrimSpace(intent) == "" {
		return nil, errors.New("intent is empty")
	}
	r.captured.reset()
	_, runErr := r.agent.Run(ctx, intent)
	if got := r.captured.get(); got != nil {
		return got, nil
	}
	if runErr != nil {
		return nil, fmt.Errorf("router run: %w", runErr)
	}
	return nil, errors.New("router did not classify the intent")
}

// classifyInput is the schema the router's classify tool accepts.
type classifyInput struct {
	ActionType           string `json:"action_type" jsonschema:"description=The Casper action type that best fits the operator's intent. Must be one of the registered action types."`
	DBInstanceIdentifier string `json:"db_instance_identifier" jsonschema:"description=The RDS database instance identifier the action targets, extracted from the intent."`
	Region               string `json:"region,omitempty" jsonschema:"description=Optional AWS region if the operator named one explicitly."`
	Confidence           string `json:"confidence,omitempty" jsonschema:"description=One of: high, medium, low."`
	Reasoning            string `json:"reasoning,omitempty" jsonschema:"description=Short rationale for the action choice."`
}

func buildClassifyTool(c *routerCapture) tool.Tool {
	return tool.Typed(
		"classify_action",
		"Classify a free-form operator intent into a Casper action type plus the named resource. Call this exactly once.",
		func(ctx context.Context, in classifyInput) (map[string]any, error) {
			if _, ok := action.Lookup(in.ActionType); !ok {
				return nil, fmt.Errorf("unknown action_type %q", in.ActionType)
			}
			if in.DBInstanceIdentifier == "" {
				return nil, errors.New("db_instance_identifier is required")
			}
			c.set(Routing{
				ActionType:           in.ActionType,
				DBInstanceIdentifier: in.DBInstanceIdentifier,
				Region:               in.Region,
				Confidence:           in.Confidence,
				Reasoning:            in.Reasoning,
			})
			return map[string]any{"ok": true}, nil
		},
	)
}

// routerSystemPrompt is built dynamically from the action registry so
// adding a new action automatically informs the router.
func routerSystemPrompt() string {
	var b strings.Builder
	b.WriteString(`You are Casper's intent router.

Your only job is to classify a free-form operator intent into one of the registered Casper action types and to extract the named target resource (the RDS instance identifier).

Hard constraints:
- You must call classify_action exactly ONCE.
- You must not write any free-form text, explanation, or commentary outside the tool call. Reasoning belongs in the "reasoning" field.

Available action types:
`)
	for _, t := range action.Types() {
		spec := action.MustLookup(t)
		fmt.Fprintf(&b, "  - %s — %s\n", t, spec.Description)
	}
	b.WriteString(`
Disambiguation hints:
- "scale up", "scale down", "resize", "more CPU", "more headroom", "underprovisioned", "overprovisioned" → rds_resize
- "snapshot", "backup once", "take a snapshot", "create a snapshot" → rds_create_snapshot
- "backup retention", "keep backups for", "retention period" → rds_modify_backup_retention
- If the intent doesn't clearly fit any action, pick the closest match and set confidence to "low".

Always extract the RDS database instance identifier from the intent — operators usually name it directly ("orders-prod", "casper-test", etc.).`)

	jsonSampleOut, _ := json.MarshalIndent(map[string]any{
		"action_type":            "rds_resize",
		"db_instance_identifier": "orders-prod",
		"region":                 "us-east-1",
		"confidence":             "high",
		"reasoning":              "Operator says CPU is sustained at 90%; that's a resize-up scenario.",
	}, "", "  ")
	b.WriteString("\n\nExample classify_action input for the intent \"orders-prod is at 90% CPU\":\n```json\n")
	b.Write(jsonSampleOut)
	b.WriteString("\n```")
	return b.String()
}
