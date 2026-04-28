package plan

import (
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
)

// StepKind enumerates the things the interpreter knows how to do.
// Adding a new kind is a deliberate change — the interpreter has a
// single switch over StepKind and no other action-specific code.
type StepKind string

const (
	StepAWSAPICall StepKind = "aws_api_call"
	StepPoll       StepKind = "poll"
	StepVerify     StepKind = "verify"
	StepWait       StepKind = "wait"
)

// OnFailure is what the interpreter does when a step fails.
type OnFailure string

const (
	OnFailureAbort    OnFailure = "abort"
	OnFailureRollback OnFailure = "rollback"
	OnFailureContinue OnFailure = "continue"
)

// PlanKind distinguishes a forward plan from its paired rollback.
type PlanKind string

const (
	PlanForward  PlanKind = "forward"
	PlanRollback PlanKind = "rollback"
)

// APICall is the parameters of a single AWS SDK call.
// Service + Operation map to aws-sdk-go-v2; Params are the input shape
// for that operation, marshalled by the interpreter at execution time.
type APICall struct {
	Service   string         `json:"service"`
	Operation string         `json:"operation"`
	Params    map[string]any `json:"params"`
}

// Predicate is a structured assertion the interpreter can evaluate
// against an AWS response payload. Path is a JSON-pointer-ish dotted
// path, Operator is one of {"eq","ne","empty","not_empty"}, and Value
// is the expected literal where applicable.
type Predicate struct {
	Path     string `json:"path"`
	Operator string `json:"operator"`
	Value    any    `json:"value,omitempty"`
}

// Poll repeats APICall until Predicate holds, bounded by the parent
// Step's Timeout.
type Poll struct {
	APICall   APICall   `json:"api_call"`
	Predicate Predicate `json:"predicate"`
	Interval  string    `json:"interval"`
}

// Verify asserts a list of predicates. Exactly one of APICall or
// SourceStepID is set: APICall makes a fresh AWS call, SourceStepID
// evaluates against the captured response of a prior step (used for
// re-checking preconditions against pre-state without re-fetching).
type Verify struct {
	APICall      *APICall    `json:"api_call,omitempty"`
	SourceStepID string      `json:"source_step_id,omitempty"`
	Assertions   []Predicate `json:"assertions"`
}

// Wait pauses for Duration. The step records the wall-clock time it
// observed before and after, for determinism in replay.
type Wait struct {
	Duration string `json:"duration"`
}

// Step is one unit of work in an ExecutionPlan. Exactly one of the
// kind-specific fields (APICall, Poll, Verify, Wait) is populated,
// determined by Kind.
type Step struct {
	ID          string    `json:"id"`
	Kind        StepKind  `json:"kind"`
	Description string    `json:"description"`
	Timeout     string    `json:"timeout,omitempty"`
	OnFailure   OnFailure `json:"on_failure"`

	APICall *APICall `json:"api_call,omitempty"`
	Poll    *Poll    `json:"poll,omitempty"`
	Verify  *Verify  `json:"verify,omitempty"`
	Wait    *Wait    `json:"wait,omitempty"`
}

// ExecutionPlan is the data the interpreter consumes. Every plan is
// bound to a single proposal hash; the interpreter refuses to run a
// plan whose ProposalHash does not match an approved proposal.
type ExecutionPlan struct {
	Kind         PlanKind            `json:"kind"`
	ActionType   string              `json:"action_type"`
	ProposalHash action.ProposalHash `json:"proposal_hash"`
	Steps        []Step              `json:"steps"`
}
