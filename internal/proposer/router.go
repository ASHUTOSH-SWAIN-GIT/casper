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
	"github.com/jerkeyray/starling/tool"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// DefaultRouterModel is the Anthropic-API model used for NL routing.
const DefaultRouterModel = "claude-haiku-4-5"

// Routing is the per-action result from the router.
type Routing struct {
	ActionType           string `json:"action_type"`
	DBInstanceIdentifier string `json:"db_instance_identifier"`
	Region               string `json:"region,omitempty"`
	Confidence           string `json:"confidence,omitempty"` // "high" | "medium" | "low"
	Reasoning            string `json:"reasoning,omitempty"`
}

// BatchRouting is what the router returns — one or more actions with an
// execution order hint.
type BatchRouting struct {
	Actions        []Routing `json:"actions"`
	ExecutionOrder string    `json:"execution_order"` // "sequential" | "parallel"
	Reasoning      string    `json:"reasoning,omitempty"`
}

// Router classifies a free-form intent into one or more Casper action
// types plus named resources and an execution order.
type Router struct {
	agent    *starling.Agent
	captured *routerCapture
}

type routerCapture struct {
	out *BatchRouting
}

func (c *routerCapture) reset()               { c.out = nil }
func (c *routerCapture) set(r BatchRouting)   { c.out = &r }
func (c *routerCapture) get() *BatchRouting   { return c.out }

// RouterConfig configures a Router.
type RouterConfig struct {
	Backend Backend
	APIKey  string
	Region  string
	Model   string
	Log     eventlog.EventLog
	Timeout time.Duration
}

// NewRouter constructs a Router.
func NewRouter(c RouterConfig) (*Router, error) {
	if c.Log == nil {
		return nil, errors.New("Log is required")
	}
	ttl := c.Timeout
	if ttl == 0 {
		ttl = 20 * time.Second
	}

	asProposerConfig := Config{
		Backend: c.Backend,
		APIKey:  c.APIKey,
		Region:  c.Region,
		Model:   c.Model,
	}
	if c.Model == "" && c.Backend == BackendAnthropic {
		asProposerConfig.Model = DefaultRouterModel
	}
	prov, model, err := buildProvider(asProposerConfig)
	if err != nil {
		return nil, err
	}

	cap := &routerCapture{}
	a := &starling.Agent{
		Provider: prov,
		Tools:    []tool.Tool{buildClassifyTool(cap)},
		Log:      c.Log,
		Config: starling.Config{
			Model:        model,
			SystemPrompt: routerSystemPrompt(),
			MaxTurns:     2,
		},
		Budget: &starling.Budget{
			MaxInputTokens:  5_000,
			MaxOutputTokens: 1_500,
			MaxUSD:          0.02,
			MaxWallClock:    ttl,
		},
	}
	return &Router{agent: a, captured: cap}, nil
}

// Route classifies a free-form intent and returns a BatchRouting. For
// single-action intents the batch will contain exactly one Routing.
func (r *Router) Route(ctx context.Context, intent string) (*BatchRouting, error) {
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

// classifyBatchInput is the schema the classify_actions tool accepts.
type classifyBatchInput struct {
	Actions []classifyItem `json:"actions" jsonschema:"description=One or more actions required to satisfy the operator's intent. Most intents require exactly one action."`
	ExecutionOrder string `json:"execution_order" jsonschema:"description=How to run multiple actions: 'sequential' (one after another; stop if one fails) or 'parallel' (all at once). Use 'sequential' when order matters (e.g. snapshot before resize). Ignored for single-action intents."`
	Reasoning string `json:"reasoning,omitempty" jsonschema:"description=Short rationale for the action choices and execution order."`
}

type classifyItem struct {
	ActionType           string `json:"action_type" jsonschema:"description=The Casper action type that best fits this part of the intent."`
	DBInstanceIdentifier string `json:"db_instance_identifier" jsonschema:"description=The RDS instance identifier this action targets."`
	Region               string `json:"region,omitempty" jsonschema:"description=AWS region if named explicitly in the intent."`
	Confidence           string `json:"confidence,omitempty" jsonschema:"description=One of: high, medium, low."`
}

func buildClassifyTool(c *routerCapture) tool.Tool {
	return tool.Typed(
		"classify_actions",
		"Classify a free-form operator intent into one or more Casper action types plus named resources. Call this exactly once.",
		func(ctx context.Context, in classifyBatchInput) (map[string]any, error) {
			if len(in.Actions) == 0 {
				return nil, errors.New("actions must have at least one item")
			}
			routings := make([]Routing, 0, len(in.Actions))
			for _, item := range in.Actions {
				if _, ok := action.Lookup(item.ActionType); !ok {
					return nil, fmt.Errorf("unknown action_type %q", item.ActionType)
				}
				if item.DBInstanceIdentifier == "" {
					return nil, errors.New("db_instance_identifier is required for each action")
				}
				routings = append(routings, Routing{
					ActionType:           item.ActionType,
					DBInstanceIdentifier: item.DBInstanceIdentifier,
					Region:               item.Region,
					Confidence:           item.Confidence,
					Reasoning:            in.Reasoning,
				})
			}
			order := in.ExecutionOrder
			if order != "parallel" {
				order = "sequential" // default to sequential for safety
			}
			c.set(BatchRouting{
				Actions:        routings,
				ExecutionOrder: order,
				Reasoning:      in.Reasoning,
			})
			return map[string]any{"ok": true, "action_count": len(routings)}, nil
		},
	)
}

func routerSystemPrompt() string {
	var b strings.Builder
	b.WriteString(`You are Casper's intent router.

Your job is to classify a free-form operator intent into one or more Casper action types and extract the named target resource(s).

Hard constraints:
- You must call classify_actions exactly ONCE.
- You must not write any free-form text outside the tool call. Reasoning belongs in the "reasoning" field.
- Most intents require exactly one action. Only emit multiple actions if the intent clearly asks for more than one distinct operation.

Available action types:
`)
	for _, t := range action.Types() {
		spec := action.MustLookup(t)
		fmt.Fprintf(&b, "  - %s — %s\n", t, spec.Description)
	}
	b.WriteString(`
Disambiguation hints:
- "scale up/down", "resize", "more CPU", "headroom" → rds_resize
- "snapshot", "backup once", "take a snapshot" → rds_create_snapshot
- "backup retention", "keep backups for" → rds_modify_backup_retention
- If the intent doesn't clearly fit any action, pick the closest match and set confidence to "low".

Multi-action hints:
- "snapshot AND resize X" → two actions: rds_create_snapshot + rds_resize, execution_order: "sequential"
- "resize X and Y simultaneously" → two rds_resize actions, execution_order: "parallel"
- "snapshot X before resizing it" → sequential (snapshot first, then resize)
- Default execution_order to "sequential" unless the operator explicitly asks for parallel/simultaneous.

Always extract the RDS instance identifier from the intent.`)

	jsonSampleSingle, _ := json.MarshalIndent(map[string]any{
		"actions": []any{map[string]any{
			"action_type":            "rds_resize",
			"db_instance_identifier": "orders-prod",
			"region":                 "us-east-1",
			"confidence":             "high",
		}},
		"execution_order": "sequential",
		"reasoning":       "Operator says CPU is sustained at 90%; that's a resize-up scenario.",
	}, "", "  ")
	b.WriteString("\n\nExample classify_actions input for \"orders-prod is at 90% CPU\":\n```json\n")
	b.Write(jsonSampleSingle)
	b.WriteString("\n```")

	jsonSampleMulti, _ := json.MarshalIndent(map[string]any{
		"actions": []any{
			map[string]any{"action_type": "rds_create_snapshot", "db_instance_identifier": "orders-prod", "confidence": "high"},
			map[string]any{"action_type": "rds_resize", "db_instance_identifier": "orders-prod", "confidence": "high"},
		},
		"execution_order": "sequential",
		"reasoning":       "Operator wants a safety snapshot before resizing; sequential is correct.",
	}, "", "  ")
	b.WriteString("\n\nExample for \"snapshot orders-prod then resize it\":\n```json\n")
	b.Write(jsonSampleMulti)
	b.WriteString("\n```")

	return b.String()
}
