package fabricrunner_test

import (
	"context"
	"encoding/json"
	fr "github.com/agincgit/fabricrunner"
	"testing"
	"time"
)

type sandboxEngineTool struct{ engineTool }

func (*sandboxEngineTool) PrepareSandbox(ctx context.Context) (fr.SandboxCapability, error) {
	return fr.SandboxCapability{Established: true, Backend: "test"}, fr.RecordSandboxEvent(ctx, fr.SandboxEvent{Phase: "established", Backend: "test"})
}
func (*sandboxEngineTool) Close(ctx context.Context) error {
	return fr.RecordSandboxEvent(ctx, fr.SandboxEvent{Phase: "teardown", Backend: "test"})
}

type sandboxPolicy struct {
	allowEnginePolicy
	seen bool
}

func (p *sandboxPolicy) EvaluateExecution(ctx context.Context, r fr.ExecutionPolicyRequest) (fr.PolicyVerdict, error) {
	if r.Target.Kind == fr.ExecutionTargetTool {
		p.seen = r.Target.Sandbox.Established
	}
	return p.allowEnginePolicy.EvaluateExecution(ctx, r)
}
func TestPolicyCanEvaluateSandboxCapability(t *testing.T) { testSandboxLifecycle(t) }
func TestSandboxLifecycleRecorded(t *testing.T)           { testSandboxLifecycle(t) }
func testSandboxLifecycle(t *testing.T) {
	e, r, p := engineFixture(t)
	observations := make(chan fr.Record, 64)
	e.Observer = engineObserver(func(_ context.Context, record fr.Record) error { observations <- record; return nil })
	policy := &sandboxPolicy{}
	e.ExecutionPolicy = policy
	h := &sandboxEngineTool{}
	r.Tools = []fr.ToolBinding{{Definition: fr.ToolDefinition{Name: "write", SideEffecting: true, InputSchema: json.RawMessage(`{"type":"object"}`)}, Handler: h}}
	p.run = func(context.Context, fr.ModelRequest) (fr.ModelStream, error) {
		if p.calls == 1 {
			return &engineStream{events: []fr.ModelEvent{{Type: fr.ModelEventStart, Sequence: 1}, {Type: fr.ModelEventToolCall, Sequence: 2, ToolCall: &fr.ToolCall{ID: "call", Name: "write", Input: json.RawMessage(`{}`)}}, {Type: fr.ModelEventStop, Sequence: 3, Stop: fr.StopToolUse}}}, nil
		}
		return &engineStream{events: []fr.ModelEvent{{Type: fr.ModelEventStart, Sequence: 1}, {Type: fr.ModelEventStop, Sequence: 2, Stop: fr.StopEndTurn}}}, nil
	}
	projection := runEngine(t, e, r)
	if !policy.seen || h.calls != 1 {
		t.Fatalf("policy=%v calls=%d", policy.seen, h.calls)
	}
	phases := map[string]bool{}
	for _, record := range projection.Execution {
		if record.Sandbox != nil {
			phases[record.Sandbox.Phase] = true
		}
	}
	if !phases["established"] || !phases["teardown"] {
		t.Fatal(phases)
	}
	seen := map[string]bool{}
	deadline := time.After(time.Second)
	for !seen["sandbox_established"] || !seen["sandbox_teardown"] {
		select {
		case record := <-observations:
			seen[record.Attributes[fr.AttrOutcome]] = true
		case <-deadline:
			t.Fatal("sandbox observer lifecycle incomplete")
		}
	}
}
