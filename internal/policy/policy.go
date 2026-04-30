// Package policy is Casper's allow / deny / needs-approval engine.
//
// The policy engine evaluates a proposal (and, in future, simulator
// output) against a Rego policy module, then returns a Verdict the
// trust layer can act on. The Rego modules are embedded at compile
// time — there is no runtime "load policy from path" code path.
// Updating the policy is a code change.
//
// Each registered action type has its own Rego module; the engine
// holds a prepared query per type. Adding a new action means:
//
//  1. Drop a `rules_<action>.rego` file in this package.
//  2. Embed it via //go:embed in this file's `init()`.
//  3. Register it under the same `data.casper.<action>.result` query
//     path that the action's `Spec.PolicyQuery` claims.
//
// Importantly: a verdict only describes what *should* happen. The
// caller decides whether to honor it. The CLI honors verdicts
// strictly today (deny + needs_approval both abort); a future
// approval flow will turn needs_approval into "wait for a signed
// approval".
package policy

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	"github.com/open-policy-agent/opa/rego"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// Embedded Rego modules — one per action type. The compiler is fed
// every module so cross-action references would also work, though
// today each action's rules live in their own package.
//go:embed rules.rego
var rulesRDSResize []byte

//go:embed rules_rds_create_snapshot.rego
var rulesRDSCreateSnapshot []byte

//go:embed rules_rds_modify_backup_retention.rego
var rulesRDSModifyBackupRetention []byte

// Decision is the verdict triple. A future v2 may add "irreversible"
// or "needs_multi_party_approval" — for v1 the three values below are
// sufficient.
type Decision string

const (
	DecisionAllow         Decision = "allow"
	DecisionDeny          Decision = "deny"
	DecisionNeedsApproval Decision = "needs_approval"
)

// Verdict is what the engine returns. Reason is a single human-
// readable string from the Rego policy (the policy chooses the most
// specific reason that applies).
type Verdict struct {
	Decision Decision `json:"decision"`
	Reason   string   `json:"reason"`
}

// Engine wraps prepared OPA queries — one per action type. It is
// safe to share across goroutines; PreparedEvalQuery is goroutine-safe.
type Engine struct {
	queries map[string]rego.PreparedEvalQuery // keyed by action type
}

// NewEngine compiles every embedded policy module. Returns an error
// if any module is malformed (caught at startup, not at evaluation).
func NewEngine(ctx context.Context) (*Engine, error) {
	modules := map[string][]byte{
		"rds_resize":                  rulesRDSResize,
		"rds_create_snapshot":         rulesRDSCreateSnapshot,
		"rds_modify_backup_retention": rulesRDSModifyBackupRetention,
	}

	queries := make(map[string]rego.PreparedEvalQuery, len(modules))
	for actionType, src := range modules {
		spec, ok := action.Lookup(actionType)
		if !ok {
			return nil, fmt.Errorf("policy: action %q not in registry", actionType)
		}
		r := rego.New(
			rego.Query(spec.PolicyQuery),
			rego.Module("rules_"+actionType+".rego", string(src)),
		)
		pq, err := r.PrepareForEval(ctx)
		if err != nil {
			return nil, fmt.Errorf("compile %s policy: %w", actionType, err)
		}
		queries[actionType] = pq
	}
	return &Engine{queries: queries}, nil
}

// evaluate is the generic dispatch. Every action-specific Evaluate*
// method funnels through here.
func (e *Engine) evaluate(ctx context.Context, actionType string, proposal any) (Verdict, error) {
	pq, ok := e.queries[actionType]
	if !ok {
		return Verdict{}, fmt.Errorf("policy: no rules registered for %q", actionType)
	}
	rs, err := pq.Eval(ctx, rego.EvalInput(map[string]any{"proposal": proposal}))
	if err != nil {
		return Verdict{}, fmt.Errorf("evaluate: %w", err)
	}
	if len(rs) == 0 || len(rs[0].Expressions) == 0 {
		return Verdict{}, errors.New("policy returned no result")
	}
	raw, ok := rs[0].Expressions[0].Value.(map[string]any)
	if !ok {
		return Verdict{}, fmt.Errorf("unexpected result shape: %T", rs[0].Expressions[0].Value)
	}
	decision, _ := raw["decision"].(string)
	reason, _ := raw["reason"].(string)
	if decision == "" {
		return Verdict{}, errors.New("policy result missing decision")
	}
	return Verdict{Decision: Decision(decision), Reason: reason}, nil
}

// EvaluateRDSResize runs the policy against an rds_resize proposal.
func (e *Engine) EvaluateRDSResize(ctx context.Context, p action.RDSResizeProposal) (Verdict, error) {
	return e.evaluate(ctx, "rds_resize", p)
}

// EvaluateRDSCreateSnapshot runs the policy against a snapshot proposal.
func (e *Engine) EvaluateRDSCreateSnapshot(ctx context.Context, p action.RDSCreateSnapshotProposal) (Verdict, error) {
	return e.evaluate(ctx, "rds_create_snapshot", p)
}

// EvaluateRDSModifyBackupRetention runs the policy against a backup-retention change proposal.
func (e *Engine) EvaluateRDSModifyBackupRetention(ctx context.Context, p action.RDSModifyBackupRetentionProposal) (Verdict, error) {
	return e.evaluate(ctx, "rds_modify_backup_retention", p)
}
