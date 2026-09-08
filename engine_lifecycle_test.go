package fabricrunner_test

import (
	"context"
	"encoding/json"
	"errors"
	fr "github.com/agincgit/fabricrunner"
	"strings"
	"testing"
	"time"
)

type fixedSpend struct{ bound fr.SpendBound }

func (s fixedSpend) Estimate(context.Context, fr.ModelRequest, fr.ModelDescriptor) (fr.SpendBound, error) {
	return s.bound, nil
}

type inspectingApprover struct{ t *testing.T }

func (a inspectingApprover) Approve(_ context.Context, r fr.ApprovalRequest) (fr.ApprovalDecision, error) {
	data, _ := json.Marshal(r)
	if strings.Contains(string(data), "private prompt") {
		a.t.Fatal("approval carried raw content")
	}
	return fr.ApprovalDecision{Approved: true, Actor: "test"}, nil
}
func TestApprovalRequestExcludesConfidentialContent(t *testing.T) {
	e, r, _ := engineFixture(t)
	r.Initial.Messages[0].Content[0].Text = "private prompt"
	r.Initial.Messages[0].Content[0].Classification = fr.ClassConfidential
	e.Approver = inspectingApprover{t: t}
	runEngine(t, e, r)
}

type waitingApprover struct{}

func (waitingApprover) Approve(ctx context.Context, _ fr.ApprovalRequest) (fr.ApprovalDecision, error) {
	<-ctx.Done()
	return fr.ApprovalDecision{}, ctx.Err()
}
func TestApprovalTimeoutRecorded(t *testing.T) {
	e, r, p := engineFixture(t)
	e.Approver = waitingApprover{}
	e.ApprovalTimeout = time.Millisecond
	projection, err := e.Run(context.Background(), r)
	if !errors.Is(err, fr.ErrApprovalDenied) || p.calls != 0 {
		t.Fatalf("%v calls=%d", err, p.calls)
	}
	found := false
	for _, record := range projection.Execution {
		found = found || record.Approval != nil && record.Approval.Decision.Reason == "approval_timeout"
	}
	if !found {
		t.Fatal("missing timeout record")
	}
}

type cleanupTool struct {
	closed int
	fail   bool
}

func (*cleanupTool) Execute(context.Context, fr.ToolInvocation) (fr.ToolOutput, error) {
	return fr.ToolOutput{JSON: json.RawMessage(`{}`), Classification: fr.ClassPublic}, nil
}
func (h *cleanupTool) Close(context.Context) error {
	h.closed++
	if h.fail {
		return errors.New("cleanup broke")
	}
	return nil
}
func TestCleanupRunsOnEveryTerminationPath(t *testing.T) {
	for _, path := range []string{"success", "failure", "cancellation", "budget", "approval"} {
		t.Run(path, func(t *testing.T) {
			e, r, p := engineFixture(t)
			h := &cleanupTool{}
			r.Tools = []fr.ToolBinding{{Definition: fr.ToolDefinition{Name: "noop", InputSchema: json.RawMessage(`{"type":"object"}`)}, Handler: h}}
			switch path {
			case "failure":
				p.run = func(context.Context, fr.ModelRequest) (fr.ModelStream, error) {
					return nil, errors.New("provider broke")
				}
			case "cancellation":
				p.run = func(context.Context, fr.ModelRequest) (fr.ModelStream, error) { return nil, context.Canceled }
			case "budget":
				r.Budget.MaxSteps = 0
			case "approval":
				e.Approver = &testApprover{}
			}
			_, _ = e.Run(context.Background(), r)
			if h.closed != 1 {
				t.Fatalf("closed %d times", h.closed)
			}
		})
	}
}
func TestCleanupFailureDoesNotMaskCause(t *testing.T) {
	e, r, _ := engineFixture(t)
	h := &cleanupTool{fail: true}
	e.Approver = &testApprover{}
	r.Tools = []fr.ToolBinding{{Definition: fr.ToolDefinition{Name: "noop", InputSchema: json.RawMessage(`{"type":"object"}`)}, Handler: h}}
	projection, err := e.Run(context.Background(), r)
	if !errors.Is(err, fr.ErrApprovalDenied) || !strings.Contains(err.Error(), "cleanup broke") {
		t.Fatal(err)
	}
	found := false
	for _, record := range projection.Execution {
		found = found || record.Cleanup != nil && record.Cleanup.Failed
	}
	if !found {
		t.Fatal("cleanup failure not recorded")
	}
}

type testApprover struct {
	calls int
	allow bool
}

func (a *testApprover) Approve(_ context.Context, r fr.ApprovalRequest) (fr.ApprovalDecision, error) {
	a.calls++
	return fr.ApprovalDecision{Approved: a.allow, Actor: "test"}, nil
}
func TestUnconfiguredApproverDoesNotPause(t *testing.T) {
	e, r, _ := engineFixture(t)
	runEngine(t, e, r)
}
func TestApprovalDenialTerminatesInPermittedState(t *testing.T) {
	e, r, p := engineFixture(t)
	e.Approver = &testApprover{}
	projection, err := e.Run(context.Background(), r)
	if !errors.Is(err, fr.ErrApprovalDenied) || p.calls != 0 || projection.Workload.State != fr.WorkloadFailed {
		t.Fatalf("%v calls=%d projection=%+v", err, p.calls, projection)
	}
}
func TestReplayDoesNotReRequestApproval(t *testing.T) {
	e, r, _ := engineFixture(t)
	a := &testApprover{allow: true}
	e.Approver = a
	runEngine(t, e, r)
	before := a.calls
	if _, err := e.Replay(context.Background(), r.WorkloadID); err != nil {
		t.Fatal(err)
	}
	if a.calls != before {
		t.Fatal("approval repeated")
	}
}
func TestBudgetCheckedBeforeSpend(t *testing.T) {
	e, r, p := engineFixture(t)
	e.SpendEstimator = fixedSpend{bound: fr.SpendBound{InputTokens: r.Budget.MaxInputTokens + 1}}
	projection, err := e.Run(context.Background(), r)
	if !errors.Is(err, fr.ErrLoopBudgetExceeded) || p.calls != 0 {
		t.Fatalf("%v calls=%d", err, p.calls)
	}
	result := projection.Execution[len(projection.Execution)-1].Result
	if result.BudgetExceeded != "input_tokens" {
		t.Fatalf("%+v", result)
	}
}
func TestMissingSpendBoundDeniesBeforeProvider(t *testing.T) {
	e, r, p := engineFixture(t)
	e.SpendEstimator = nil
	if _, err := e.Run(context.Background(), r); err == nil || p.calls != 0 {
		t.Fatalf("%v calls=%d", err, p.calls)
	}
}
func TestEachBudgetDimensionTerminates(t *testing.T) {
	for _, dimension := range []string{"input_tokens", "output_tokens", "cost", "steps"} {
		t.Run(dimension, func(t *testing.T) {
			e, r, p := engineFixture(t)
			bound := fr.SpendBound{}
			switch dimension {
			case "input_tokens":
				bound.InputTokens = r.Budget.MaxInputTokens + 1
			case "output_tokens":
				bound.OutputTokens = r.Budget.MaxOutputTokens + 1
			case "cost":
				bound.Cost = r.Budget.MaxCost + 1
			case "steps":
				r.Budget.MaxSteps = 0
			}
			e.SpendEstimator = fixedSpend{bound: bound}
			projection, err := e.Run(context.Background(), r)
			if !errors.Is(err, fr.ErrLoopBudgetExceeded) || p.calls != 0 {
				t.Fatalf("%v calls=%d", err, p.calls)
			}
			result := projection.Execution[len(projection.Execution)-1].Result
			if result.BudgetExceeded != dimension {
				t.Fatalf("%+v", result)
			}
		})
	}
}
