package interpreter

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/action"
	"github.com/ASHUTOSH-SWAIN-GIT/casper/internal/plan"
)

// fakeClient returns scripted responses in call order. Each call matched
// against any APICall — tests assert on the recorded Calls slice instead.
type fakeClient struct {
	queue []fakeReply
	calls []plan.APICall
}

type fakeReply struct {
	body map[string]any
	err  error
}

func (f *fakeClient) Call(_ context.Context, c plan.APICall) (Response, error) {
	f.calls = append(f.calls, c)
	if len(f.queue) == 0 {
		return Response{}, fmt.Errorf("fakeClient: no scripted reply for call #%d (%s)", len(f.calls), c.Operation)
	}
	r := f.queue[0]
	f.queue = f.queue[1:]
	return Response{Body: r.body, RequestID: fmt.Sprintf("req-%d", len(f.calls))}, r.err
}

// newTestInterpreter wires a no-op sleep and a deterministic clock that
// advances by 1s per Now() call — enough to exercise poll timeouts.
func newTestInterpreter(client Client) *Interpreter {
	t0 := time.Unix(1_700_000_000, 0)
	tick := time.Duration(0)
	return &Interpreter{
		Client: client,
		Sleep:  func(time.Duration) {},
		Now: func() time.Time {
			tick += time.Second
			return t0.Add(tick)
		},
	}
}

func sampleProposal() action.RDSResizeProposal {
	return action.RDSResizeProposal{
		DBInstanceIdentifier: "orders-prod",
		Region:               "us-east-1",
		CurrentInstanceClass: "db.r6g.large",
		TargetInstanceClass:  "db.r6g.xlarge",
		ApplyImmediately:     true,
		SuccessCriteria: action.SuccessCriteria{
			Metric:             "CPUUtilization",
			ThresholdPercent:   60,
			VerificationWindow: "1s",
		},
		Reasoning: "test",
	}
}

func describeResp(class, status string, pending map[string]any) map[string]any {
	return map[string]any{
		"DBInstances": []any{
			map[string]any{
				"DBInstanceIdentifier":  "orders-prod",
				"DBInstanceStatus":      status,
				"DBInstanceClass":       class,
				"PendingModifiedValues": pending,
			},
		},
	}
}

func TestRun_HappyPathExecutesAllEightSteps(t *testing.T) {
	p := sampleProposal()
	fwd, _ := plan.CompileRDSResize(p, "hash-abc")

	client := &fakeClient{queue: []fakeReply{
		// 1. describe-pre
		{body: describeResp("db.r6g.large", "available", map[string]any{})},
		// 2. preconditions: SourceStepID, no AWS call
		// 3. modify
		{body: map[string]any{}},
		// 4. poll-modifying
		{body: describeResp("db.r6g.large", "modifying", map[string]any{"DBInstanceClass": "db.r6g.xlarge"})},
		// 5. poll-available
		{body: describeResp("db.r6g.xlarge", "available", map[string]any{})},
		// 6. verify-class (its own describe)
		{body: describeResp("db.r6g.xlarge", "available", map[string]any{})},
		// 7. wait — no AWS call
		// 8. verify-metric
		{body: map[string]any{
			"Datapoints": map[string]any{"avg": 35.0},
		}},
	}}

	i := newTestInterpreter(client)
	results, err := i.Run(context.Background(), fwd)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := len(results), 8; got != want {
		t.Fatalf("results: got %d want %d", got, want)
	}
	for n, r := range results {
		if r.Status != StepStatusDone {
			t.Errorf("step %d (%s): status %q error %q", n, r.StepID, r.Status, r.Error)
		}
	}
}

func TestRun_PreconditionsFailureAbortsBeforeModify(t *testing.T) {
	p := sampleProposal()
	fwd, _ := plan.CompileRDSResize(p, "hash-abc")

	client := &fakeClient{queue: []fakeReply{
		// describe-pre returns an unexpected current class — preconditions step fails.
		{body: describeResp("db.r6g.SOMETHING_ELSE", "available", map[string]any{})},
	}}

	i := newTestInterpreter(client)
	results, err := i.Run(context.Background(), fwd)
	if err == nil {
		t.Fatal("expected run to fail at preconditions, got nil")
	}
	if got, want := len(results), 2; got != want {
		t.Fatalf("results: got %d want %d (describe-pre + failed preconditions)", got, want)
	}
	if results[1].StepID != "preconditions" || results[1].Status != StepStatusFailed {
		t.Errorf("expected preconditions to be failed, got %+v", results[1])
	}
	// ModifyDBInstance must NOT have been called.
	for _, c := range client.calls {
		if c.Operation == "ModifyDBInstance" {
			t.Fatalf("ModifyDBInstance was called despite precondition failure")
		}
	}
}

func TestRun_PollWaitsForPredicate(t *testing.T) {
	step := plan.Step{
		ID:        "poll-test",
		Kind:      plan.StepPoll,
		Timeout:   "1m",
		OnFailure: plan.OnFailureAbort,
		Poll: &plan.Poll{
			APICall: plan.APICall{Service: "rds", Operation: "DescribeDBInstances"},
			Predicate: plan.Predicate{
				Path: "DBInstances[0].DBInstanceStatus", Operator: "eq", Value: "available",
			},
			Interval: "1s",
		},
	}
	pl := plan.ExecutionPlan{Steps: []plan.Step{step}}

	client := &fakeClient{queue: []fakeReply{
		{body: describeResp("db.r6g.xlarge", "modifying", nil)},
		{body: describeResp("db.r6g.xlarge", "modifying", nil)},
		{body: describeResp("db.r6g.xlarge", "available", nil)},
	}}
	i := newTestInterpreter(client)
	results, err := i.Run(context.Background(), pl)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := len(client.calls), 3; got != want {
		t.Errorf("poll calls: got %d want %d", got, want)
	}
	if results[0].Status != StepStatusDone {
		t.Errorf("expected done, got %+v", results[0])
	}
}

func TestRun_PollTimesOut(t *testing.T) {
	step := plan.Step{
		ID:        "poll-test",
		Kind:      plan.StepPoll,
		Timeout:   "3s",
		OnFailure: plan.OnFailureAbort,
		Poll: &plan.Poll{
			APICall: plan.APICall{Service: "rds", Operation: "DescribeDBInstances"},
			Predicate: plan.Predicate{
				Path: "DBInstances[0].DBInstanceStatus", Operator: "eq", Value: "available",
			},
			Interval: "1s",
		},
	}
	pl := plan.ExecutionPlan{Steps: []plan.Step{step}}

	// Always returns modifying — predicate never satisfied. Test clock
	// advances 1s per Now() call, so poll must time out.
	client := &fakeClient{queue: []fakeReply{
		{body: describeResp("db.r6g.xlarge", "modifying", nil)},
		{body: describeResp("db.r6g.xlarge", "modifying", nil)},
		{body: describeResp("db.r6g.xlarge", "modifying", nil)},
		{body: describeResp("db.r6g.xlarge", "modifying", nil)},
		{body: describeResp("db.r6g.xlarge", "modifying", nil)},
	}}
	i := newTestInterpreter(client)
	_, err := i.Run(context.Background(), pl)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestRun_AWSCallErrorAbortsStep(t *testing.T) {
	step := plan.Step{
		ID:        "describe",
		Kind:      plan.StepAWSAPICall,
		OnFailure: plan.OnFailureAbort,
		APICall:   &plan.APICall{Service: "rds", Operation: "DescribeDBInstances"},
	}
	pl := plan.ExecutionPlan{Steps: []plan.Step{step}}

	client := &fakeClient{queue: []fakeReply{
		{err: errors.New("AccessDenied")},
	}}
	i := newTestInterpreter(client)
	results, err := i.Run(context.Background(), pl)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if results[0].Status != StepStatusFailed || results[0].Error == "" {
		t.Errorf("expected failed step with error, got %+v", results[0])
	}
	if len(results[0].Calls) != 1 || results[0].Calls[0].Error == "" {
		t.Errorf("expected the failed call recorded with its error, got %+v", results[0].Calls)
	}
}

func TestRun_ParallelGroup_BothSucceed(t *testing.T) {
	// Two aws_api_call steps in the same parallel group — both should
	// execute concurrently and both results should appear in results slice.
	client := &fakeClient{
		queue: []fakeReply{
			{body: map[string]any{"ok": "a"}},
			{body: map[string]any{"ok": "b"}},
		},
	}
	interp := newTestInterpreter(client)

	p := plan.ExecutionPlan{
		Kind:         plan.PlanForward,
		ActionType:   "test",
		ProposalHash: "hash",
		Steps: []plan.Step{
			{
				ID: "step-a", Kind: plan.StepAWSAPICall,
				OnFailure: plan.OnFailureAbort, ParallelGroup: "pg1",
				APICall: &plan.APICall{Service: "rds", Operation: "OpA"},
			},
			{
				ID: "step-b", Kind: plan.StepAWSAPICall,
				OnFailure: plan.OnFailureAbort, ParallelGroup: "pg1",
				APICall: &plan.APICall{Service: "rds", Operation: "OpB"},
			},
		},
	}

	results, err := interp.Run(context.Background(), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Status != StepStatusDone {
			t.Errorf("step %q: want done, got %s", r.StepID, r.Status)
		}
	}
}

func TestRun_ParallelGroup_OneFailsAbortsGroup(t *testing.T) {
	client := &fakeClient{
		queue: []fakeReply{
			{body: map[string]any{"ok": "a"}},
			{err: errors.New("boom")},
		},
	}
	interp := newTestInterpreter(client)

	p := plan.ExecutionPlan{
		Kind:         plan.PlanForward,
		ActionType:   "test",
		ProposalHash: "hash",
		Steps: []plan.Step{
			{
				ID: "step-a", Kind: plan.StepAWSAPICall,
				OnFailure: plan.OnFailureAbort, ParallelGroup: "pg1",
				APICall: &plan.APICall{Service: "rds", Operation: "OpA"},
			},
			{
				ID: "step-b", Kind: plan.StepAWSAPICall,
				OnFailure: plan.OnFailureAbort, ParallelGroup: "pg1",
				APICall: &plan.APICall{Service: "rds", Operation: "OpB"},
			},
		},
	}

	results, err := interp.Run(context.Background(), p)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
	failed := 0
	for _, r := range results {
		if r.Status == StepStatusFailed {
			failed++
		}
	}
	if failed != 1 {
		t.Errorf("want 1 failed step, got %d", failed)
	}
}
