package interpreter

import (
	"time"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/plan"
)

// StepStatus is the terminal status of a single step.
type StepStatus string

const (
	StepStatusDone    StepStatus = "done"
	StepStatusFailed  StepStatus = "failed"
	StepStatusSkipped StepStatus = "skipped"
)

// CallRecord is one AWS API call's verbatim request and response.
// A poll step produces many records; an aws_api_call step produces one.
type CallRecord struct {
	Request  plan.APICall
	Response Response
	Error    string
}

// StepResult is the audit-shaped record of one executed step.
// Persisted per-step in the real interpreter; returned in-memory for tests.
type StepResult struct {
	StepID     string
	Kind       plan.StepKind
	Status     StepStatus
	StartedAt  time.Time
	FinishedAt time.Time
	Calls      []CallRecord
	Error      string
}
