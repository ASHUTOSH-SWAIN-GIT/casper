// Package policy is Casper's allow / deny / needs-approval engine.
//
// The policy engine evaluates a proposal (and, in future, simulator
// output) against a Rego policy module, then returns a Verdict the
// trust layer can act on. The Rego module is embedded at compile time
// — there is no runtime "load policy from path" code path. Updating
// the policy is a code change.
//
// Importantly: a verdict only describes what *should* happen. The
// caller decides whether to honor it. The CLI honors verdicts
// strictly today (deny + needs_approval both abort); a future approval
// flow will turn needs_approval into "wait for a signed approval".
package policy

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	"github.com/open-policy-agent/opa/rego"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

//go:embed rules.rego
var rulesRego []byte

// Decision is the verdict triple. A future v2 may add "irreversible"
// or "needs_multi_party_approval" — for v1 the three values below are
// sufficient.
type Decision string

const (
	DecisionAllow          Decision = "allow"
	DecisionDeny           Decision = "deny"
	DecisionNeedsApproval  Decision = "needs_approval"
)

// Verdict is what the engine returns. Reason is a single human-
// readable string from the Rego policy (the policy chooses the most
// specific reason that applies).
type Verdict struct {
	Decision Decision `json:"decision"`
	Reason   string   `json:"reason"`
}

// Engine wraps an OPA prepared query. Construction compiles the
// embedded policy once; Evaluate is then cheap to call repeatedly.
type Engine struct {
	pq rego.PreparedEvalQuery
}

// NewEngine compiles the embedded policy. Returns an error if the
// Rego module is malformed (caught at startup, not at evaluation).
func NewEngine(ctx context.Context) (*Engine, error) {
	r := rego.New(
		rego.Query("data.casper.rds_resize.result"),
		rego.Module("rules.rego", string(rulesRego)),
	)
	pq, err := r.PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("compile policy: %w", err)
	}
	return &Engine{pq: pq}, nil
}

// EvaluateRDSResize runs the policy against a single proposal.
// The function name encodes the action type — different actions get
// different evaluators when more land in v2.
func (e *Engine) EvaluateRDSResize(ctx context.Context, p action.RDSResizeProposal) (Verdict, error) {
	rs, err := e.pq.Eval(ctx, rego.EvalInput(map[string]any{
		"proposal": p,
	}))
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
	return Verdict{
		Decision: Decision(decision),
		Reason:   reason,
	}, nil
}
