package interpreter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/plan"
)

// Interpreter is the only code that touches AWS via Client.Call.
// It walks an ExecutionPlan one step at a time, captures every call
// verbatim, and stops on the first fatal failure.
//
// Sleep and Now are injectable for tests. In production they default
// to time.Sleep and time.Now.
type Interpreter struct {
	Client Client
	Sleep  func(time.Duration)
	Now    func() time.Time
}

// Run executes the plan against Client and returns the per-step results
// in execution order. The error is non-nil iff the plan terminated
// before completing all steps (a step failed with on_failure=abort or
// on_failure=rollback). The caller decides whether to invoke a rollback.
func (i *Interpreter) Run(ctx context.Context, p plan.ExecutionPlan) ([]StepResult, error) {
	captures := map[string]Response{}
	results := make([]StepResult, 0, len(p.Steps))

	for _, step := range p.Steps {
		r := i.runStep(ctx, step, captures)
		results = append(results, r)

		if r.Status == StepStatusFailed {
			return results, fmt.Errorf("step %q failed: %s", step.ID, r.Error)
		}
		// If the step produced a response, store it for downstream verifies.
		if len(r.Calls) > 0 {
			captures[step.ID] = r.Calls[len(r.Calls)-1].Response
		}
	}
	return results, nil
}

func (i *Interpreter) runStep(ctx context.Context, s plan.Step, captures map[string]Response) StepResult {
	r := StepResult{StepID: s.ID, Kind: s.Kind, StartedAt: i.now()}
	defer func() { r.FinishedAt = i.now() }()

	switch s.Kind {
	case plan.StepAWSAPICall:
		if s.APICall == nil {
			return failed(r, "aws_api_call step has no APICall")
		}
		resp, err := i.Client.Call(ctx, *s.APICall)
		r.Calls = append(r.Calls, CallRecord{Request: *s.APICall, Response: resp, Error: errStr(err)})
		if err != nil {
			return failed(r, err.Error())
		}
		r.Status = StepStatusDone
		return r

	case plan.StepPoll:
		if s.Poll == nil {
			return failed(r, "poll step has no Poll")
		}
		return i.runPoll(ctx, s, r)

	case plan.StepVerify:
		if s.Verify == nil {
			return failed(r, "verify step has no Verify")
		}
		return i.runVerify(ctx, s, r, captures)

	case plan.StepWait:
		if s.Wait == nil {
			return failed(r, "wait step has no Wait")
		}
		d, err := time.ParseDuration(s.Wait.Duration)
		if err != nil {
			return failed(r, fmt.Sprintf("invalid wait duration %q: %v", s.Wait.Duration, err))
		}
		i.sleep(d)
		r.Status = StepStatusDone
		return r

	default:
		return failed(r, fmt.Sprintf("unknown step kind %q", s.Kind))
	}
}

func (i *Interpreter) runPoll(ctx context.Context, s plan.Step, r StepResult) StepResult {
	timeout, err := time.ParseDuration(stringOrDefault(s.Timeout, "30m"))
	if err != nil {
		return failed(r, fmt.Sprintf("invalid timeout %q: %v", s.Timeout, err))
	}
	interval, err := time.ParseDuration(stringOrDefault(s.Poll.Interval, "5s"))
	if err != nil {
		return failed(r, fmt.Sprintf("invalid poll interval %q: %v", s.Poll.Interval, err))
	}

	deadline := i.now().Add(timeout)
	for {
		if ctx.Err() != nil {
			return failed(r, ctx.Err().Error())
		}
		resp, callErr := i.Client.Call(ctx, s.Poll.APICall)
		r.Calls = append(r.Calls, CallRecord{Request: s.Poll.APICall, Response: resp, Error: errStr(callErr)})
		if callErr != nil {
			return failed(r, callErr.Error())
		}
		if err := evalPredicate(resp.Body, s.Poll.Predicate); err == nil {
			r.Status = StepStatusDone
			return r
		}
		if !i.now().Before(deadline) {
			return failed(r, fmt.Sprintf("poll predicate not satisfied within %s", timeout))
		}
		i.sleep(interval)
	}
}

func (i *Interpreter) runVerify(ctx context.Context, s plan.Step, r StepResult, captures map[string]Response) StepResult {
	if s.Verify.APICall != nil && s.Verify.SourceStepID != "" {
		return failed(r, "verify step has both APICall and SourceStepID")
	}

	var body map[string]any
	switch {
	case s.Verify.APICall != nil:
		resp, err := i.Client.Call(ctx, *s.Verify.APICall)
		r.Calls = append(r.Calls, CallRecord{Request: *s.Verify.APICall, Response: resp, Error: errStr(err)})
		if err != nil {
			return failed(r, err.Error())
		}
		body = resp.Body
	case s.Verify.SourceStepID != "":
		src, ok := captures[s.Verify.SourceStepID]
		if !ok {
			return failed(r, fmt.Sprintf("source step %q has no captured response", s.Verify.SourceStepID))
		}
		body = src.Body
	default:
		return failed(r, "verify step has neither APICall nor SourceStepID")
	}

	var failures []string
	for _, p := range s.Verify.Assertions {
		if err := evalPredicate(body, p); err != nil {
			failures = append(failures, err.Error())
		}
	}
	if len(failures) > 0 {
		return failed(r, "assertion(s) failed: "+strings.Join(failures, "; "))
	}
	r.Status = StepStatusDone
	return r
}

func (i *Interpreter) sleep(d time.Duration) {
	if i.Sleep != nil {
		i.Sleep(d)
		return
	}
	time.Sleep(d)
}

func (i *Interpreter) now() time.Time {
	if i.Now != nil {
		return i.Now()
	}
	return time.Now()
}

func failed(r StepResult, msg string) StepResult {
	r.Status = StepStatusFailed
	r.Error = msg
	return r
}

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func stringOrDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// ErrPlanIncomplete is returned when Run terminates before completing
// all steps. Callers receive a wrapped version with the failing step's
// error as the cause.
var ErrPlanIncomplete = errors.New("plan did not complete")
