package fabricrunner_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	fr "github.com/agincgit/fabricrunner"
	"github.com/agincgit/fabricrunner/loop"
	"github.com/agincgit/fabricrunner/policy/baseline"
	"github.com/agincgit/fabricrunner/router/deterministic"
	"github.com/agincgit/fabricrunner/store/memory"
	"github.com/agincgit/fabricrunner/store/sqlite"
)

type engineProvider struct {
	calls int
	run   func(context.Context, fr.ModelRequest) (fr.ModelStream, error)
}

func (*engineProvider) Name() string                                         { return "test" }
func (*engineProvider) Models(context.Context) ([]fr.ModelDescriptor, error) { return nil, nil }
func (p *engineProvider) Stream(ctx context.Context, r fr.ModelRequest) (fr.ModelStream, error) {
	p.calls++
	if p.run != nil {
		return p.run(ctx, r)
	}
	return &engineStream{events: []fr.ModelEvent{
		{Type: fr.ModelEventStart, Sequence: 1}, {Type: fr.ModelEventTextDelta, Sequence: 2, Text: "done"},
		{Type: fr.ModelEventStop, Sequence: 3, Stop: fr.StopEndTurn},
	}}, nil
}

type engineStream struct{ events []fr.ModelEvent }

func (s *engineStream) Recv(ctx context.Context) (fr.ModelEvent, error) {
	if err := ctx.Err(); err != nil {
		return fr.ModelEvent{}, err
	}
	if len(s.events) == 0 {
		return fr.ModelEvent{}, io.EOF
	}
	e := s.events[0]
	s.events = s.events[1:]
	return e, nil
}
func (*engineStream) Close() error { return nil }

type engineTool struct{ calls int }

func (h *engineTool) Execute(context.Context, fr.ToolInvocation) (fr.ToolOutput, error) {
	h.calls++
	return fr.ToolOutput{JSON: json.RawMessage(`{"ok":true}`), Classification: fr.ClassPublic}, nil
}
func (*engineTool) Close(context.Context) error { return nil }

type allowEnginePolicy struct{ baseline.Policy }

func (allowEnginePolicy) EvaluateExecution(ctx context.Context, r fr.ExecutionPolicyRequest) (fr.PolicyVerdict, error) {
	v, err := (baseline.Policy{}).EvaluateExecution(ctx, r)
	v.Action = fr.PolicyAllow
	return v, err
}
func engineFixture(t *testing.T) (*fr.Engine, fr.EngineRequest, *engineProvider) {
	t.Helper()
	id := func() fr.ID {
		v, err := fr.NewID()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	p := &engineProvider{}
	r := fr.EngineRequest{
		WorkloadID: id(), SessionID: id(), StepID: id(), AttemptID: id(), Goal: "bounded workload", SourceZone: fr.ZonePersonal,
		Classification: fr.ClassPublic, ModelOutputClassification: fr.ClassPublic,
		Budget:     fr.Budget{MaxSteps: 1, MaxInputTokens: 100, MaxOutputTokens: 100, MaxModelCalls: 3, MaxToolCalls: 3, MaxCost: 100, MaxWallTime: time.Second},
		Initial:    fr.ModelRequest{Model: fr.ModelRef{Provider: "test", Model: "model"}, ToolChoice: fr.ToolChoice{Mode: fr.ToolChoiceAuto}, Messages: []fr.Message{{Role: fr.RoleUser, Content: []fr.ContentPart{{Type: fr.ContentText, Classification: fr.ClassPublic, Text: "start"}}}}},
		Candidates: []fr.RoutingCandidate{{ID: id(), Zone: fr.ZonePersonal, Authenticated: true, Compatible: true, Healthy: true, AvailableSlots: 1, AvailableAt: time.Unix(1, 0), Model: fr.ModelDescriptor{Ref: fr.ModelRef{Provider: "test", Model: "model"}, Capabilities: fr.ModelCapabilities{ToolUse: fr.ToolUseNative, MaxOutputTokens: 1000}}}},
	}
	return &fr.Engine{SpendEstimator: fixedSpend{bound: fr.SpendBound{InputTokens: 20, OutputTokens: 20, Cost: 20}}, Store: memory.New(), Loop: loop.Engine{}, ExecutionPolicy: allowEnginePolicy{}, DataEgressPolicy: baseline.Policy{}, Router: deterministic.Router{}, Providers: map[string]fr.Provider{"test": p}}, r, p
}
func engineEvents(t *testing.T, e *fr.Engine, r fr.EngineRequest) []fr.Event {
	t.Helper()
	v, err := e.Store.Load(context.Background(), fr.AggregateRef{Type: fr.WorkloadAggregateType, ID: r.WorkloadID}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func runEngine(t *testing.T, e *fr.Engine, r fr.EngineRequest) *fr.WorkloadProjection {
	t.Helper()
	v, err := e.Run(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func TestEndToEndProjectionReconstructsTerminalState(t *testing.T) {
	e, r, p := engineFixture(t)
	path := filepath.Join(t.TempDir(), "engine.db")
	s, err := sqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	e.Store = s
	got := runEngine(t, e, r)
	if got.Workload.State != fr.WorkloadSucceeded || p.calls != 1 {
		t.Fatalf("state=%s calls=%d", got.Workload.State, p.calls)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = sqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rebuilt, err := fr.LoadWorkloadProjection(context.Background(), s, got.Aggregate)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, rebuilt) {
		t.Fatal("reopened database differs from returned projection")
	}
}
func TestPolicyDenyPreventsModelCall(t *testing.T) {
	e, r, p := engineFixture(t)
	r.Initial.Messages[0].Content[0].Classification = fr.ClassSecret
	got, err := e.Run(context.Background(), r)
	if err == nil || p.calls != 0 || got.Workload.State != fr.WorkloadFailed {
		t.Fatalf("projection=%v calls=%d err=%v", got, p.calls, err)
	}
	found := false
	for _, a := range got.Execution {
		if a.Verdict != nil && a.Verdict.Action == fr.PolicyDeny && a.Verdict.Reason != "" {
			found = true
		}
	}
	if !found {
		t.Fatal("denial absent from durable audit")
	}
}
func TestEgressDenyPreventsCrossing(t *testing.T) {
	for _, classification := range []fr.Classification{fr.ClassConfidential, ""} {
		t.Run(string(classification), func(t *testing.T) {
			e, r, p := engineFixture(t)
			r.Candidates[0].Zone = fr.ZoneManagedCloud
			r.Classification = classification
			got, err := e.Run(context.Background(), r)
			if err == nil || p.calls != 0 {
				t.Fatalf("calls=%d err=%v", p.calls, err)
			}
			if got == nil {
				t.Fatal("missing audit for undecidable classification")
			}
			found := false
			for _, a := range got.Execution {
				if a.Verdict != nil && a.Verdict.Scope.Kind == fr.PolicyScopeDataEgress && !a.Verdict.Allows() {
					found = true
				}
			}
			if !found {
				t.Fatal("egress decision absent")
			}
		})
	}
}
func TestRoutingDecisionBoundToTurn(t *testing.T) {
	e, r, _ := engineFixture(t)
	bad := r.Candidates[0].Clone()
	bad.ID, _ = fr.NewID()
	bad.Healthy = false
	r.Candidates = append(r.Candidates, bad)
	got := runEngine(t, e, r)
	found := false
	for _, a := range got.Execution {
		if a.Routing != nil {
			found = true
			if a.Turn != 1 || a.StepID != r.StepID || a.Routing.Decision.SelectedCandidateID != r.Candidates[0].ID || len(a.Routing.Decision.Assessments) != 2 || a.Routing.Decision.Assessments[1].Exclusion == nil {
				t.Fatal("routing is not bound to turn or lacks exclusions")
			}
		}
	}
	if !found {
		t.Fatal("missing routing")
	}
}
func TestReplayContactsNoProviderOrTool(t *testing.T) {
	e, r, p := engineFixture(t)
	got := runEngine(t, e, r)
	p.run = func(context.Context, fr.ModelRequest) (fr.ModelStream, error) {
		t.Fatal("provider during replay")
		return nil, nil
	}
	e.Loop = nil
	e.Providers = nil
	e.ExecutionPolicy = nil
	e.DataEgressPolicy = nil
	e.Router = nil
	replayed, err := e.Replay(context.Background(), r.WorkloadID)
	if err != nil || !reflect.DeepEqual(got, replayed) {
		t.Fatalf("replay differs: %v", err)
	}
}
func TestReplayReconstructsIdenticalState(t *testing.T) { TestReplayContactsNoProviderOrTool(t) }
func TestCancellationLeavesPermittedState(t *testing.T) {
	e, r, p := engineFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.run = func(context.Context, fr.ModelRequest) (fr.ModelStream, error) { cancel(); return nil, context.Canceled }
	got, err := e.Run(ctx, r)
	if !errors.Is(err, context.Canceled) || got == nil {
		t.Fatalf("%v %v", got, err)
	}
	if !fr.CanTransitionWorkload(fr.WorkloadRunning, got.Workload.State) || !fr.CanTransitionStep(fr.StepRunning, got.Steps[r.StepID].State) {
		t.Fatal("invalid cancellation transition")
	}
}

type heldStore struct {
	fr.EventStore
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *heldStore) Append(ctx context.Context, ref fr.AggregateRef, v uint64, d ...fr.EventDraft) ([]fr.Event, error) {
	s.once.Do(func() { close(s.entered); <-s.release })
	return s.EventStore.Append(ctx, ref, v, d...)
}
func TestNoPartialTransitionObservable(t *testing.T) {
	e, r, _ := engineFixture(t)
	s := &heldStore{EventStore: e.Store, entered: make(chan struct{}), release: make(chan struct{})}
	e.Store = s
	done := make(chan error, 1)
	go func() { _, err := e.Run(context.Background(), r); done <- err }()
	<-s.entered
	if _, err := e.Replay(context.Background(), r.WorkloadID); !errors.Is(err, fr.ErrAggregateNotFound) {
		t.Errorf("uncommitted state exposed: %v", err)
	}
	close(s.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
func TestBudgetExhaustionTerminates(t *testing.T) {
	for _, dimension := range []string{"input_tokens", "output_tokens", "cost", "model_calls", "tool_calls", "wall_time"} {
		t.Run(dimension, func(t *testing.T) {
			e, r, p := engineFixture(t)
			p.run = func(ctx context.Context, _ fr.ModelRequest) (fr.ModelStream, error) {
				if dimension == "wall_time" {
					<-ctx.Done()
					return nil, ctx.Err()
				}
				events := []fr.ModelEvent{{Type: fr.ModelEventStart, Sequence: 1}}
				if dimension == "tool_calls" || dimension == "model_calls" {
					events = append(events, fr.ModelEvent{Type: fr.ModelEventToolCall, Sequence: 2, ToolCall: &fr.ToolCall{ID: "call", Name: "test", Input: json.RawMessage(`{}`)}}, fr.ModelEvent{Type: fr.ModelEventStop, Sequence: 3, Stop: fr.StopToolUse})
				} else {
					u := &fr.Usage{}
					switch dimension {
					case "input_tokens":
						u.InputTokens = 101
					case "output_tokens":
						u.OutputTokens = 101
					case "cost":
						u.Cost = 101
					}
					events = append(events, fr.ModelEvent{Type: fr.ModelEventUsage, Sequence: 2, Usage: u}, fr.ModelEvent{Type: fr.ModelEventStop, Sequence: 3, Stop: fr.StopEndTurn})
				}
				return &engineStream{events: events}, nil
			}
			r.Tools = []fr.ToolBinding{{Definition: fr.ToolDefinition{Name: "test", InputSchema: json.RawMessage(`{"type":"object"}`)}, Handler: &engineTool{}}}
			if dimension == "tool_calls" {
				r.Budget.MaxToolCalls = 0
			}
			if dimension == "model_calls" {
				r.Budget.MaxModelCalls = 1
			}
			if dimension == "wall_time" {
				r.Budget.MaxWallTime = 20 * time.Millisecond
			}
			got, err := e.Run(context.Background(), r)
			if !errors.Is(err, fr.ErrLoopBudgetExceeded) || got == nil || got.Workload.State != fr.WorkloadFailed {
				t.Fatalf("state=%v err=%v", got, err)
			}
			found := false
			for _, a := range got.Execution {
				if a.Result != nil && a.Result.BudgetExceeded == dimension {
					found = true
				}
			}
			if !found {
				t.Fatal("exhausted dimension absent from audit")
			}
		})
	}
}

type engineObserver func(context.Context, fr.Record) error

func (f engineObserver) Observe(ctx context.Context, r fr.Record) error { return f(ctx, r) }
func TestEngineRunsWithoutObserver(t *testing.T) {
	e, r, _ := engineFixture(t)
	a := runEngine(t, e, r)
	e.Store = memory.New()
	e.Observer = engineObserver(func(context.Context, fr.Record) error { return nil })
	b := runEngine(t, e, r)
	// Store-assigned IDs/timestamps and routing observation times differ between runs.
	if a.Workload.State != b.Workload.State || len(a.Execution) != len(b.Execution) {
		t.Fatal("observer changed execution")
	}
	for i := range a.Execution {
		left, right := a.Execution[i], b.Execution[i]
		if left.Routing != nil {
			left.Routing.Request.ObservedAt = time.Time{}
			left.Routing.Request.Deadline = time.Time{}
			left.Routing.Decision.ObservedAt = time.Time{}
			left.Routing.Decision.Deadline = time.Time{}
			right.Routing.Request.ObservedAt = time.Time{}
			right.Routing.Request.Deadline = time.Time{}
			right.Routing.Decision.ObservedAt = time.Time{}
			right.Routing.Decision.Deadline = time.Time{}
		}
		if !reflect.DeepEqual(left, right) {
			t.Fatalf("observer changed audit record %d", i)
		}
	}
}
func TestNilObserverIsValid(t *testing.T) { TestEngineRunsWithoutObserver(t) }
func TestPanickingObserverDoesNotFailWorkload(t *testing.T) {
	e, r, _ := engineFixture(t)
	e.Observer = engineObserver(func(context.Context, fr.Record) error { panic("observer") })
	runEngine(t, e, r)
}
func TestErroringObserverIsIgnored(t *testing.T) {
	e, r, _ := engineFixture(t)
	e.Observer = engineObserver(func(context.Context, fr.Record) error { return errors.New("observer") })
	runEngine(t, e, r)
}
func TestAuditDerivableWithoutObserver(t *testing.T) {
	TestEndToEndProjectionReconstructsTerminalState(t)
}
func TestObserverCoversFullPath(t *testing.T) {
	e, r, p := engineFixture(t)
	seen := make(chan fr.RecordKind, 256)
	e.Observer = engineObserver(func(_ context.Context, r fr.Record) error { seen <- r.Kind; return nil })
	h := &engineTool{}
	r.Tools = []fr.ToolBinding{{Definition: fr.ToolDefinition{Name: "test", InputSchema: json.RawMessage(`{"type":"object"}`)}, Handler: h}}
	p.run = func(context.Context, fr.ModelRequest) (fr.ModelStream, error) {
		if p.calls == 1 {
			return &engineStream{events: []fr.ModelEvent{{Type: fr.ModelEventStart, Sequence: 1}, {Type: fr.ModelEventToolCall, Sequence: 2, ToolCall: &fr.ToolCall{ID: "call", Name: "test", Input: json.RawMessage(`{}`)}}, {Type: fr.ModelEventStop, Sequence: 3, Stop: fr.StopToolUse}}}, nil
		}
		return &engineStream{events: []fr.ModelEvent{{Type: fr.ModelEventStart, Sequence: 1}, {Type: fr.ModelEventStop, Sequence: 2, Stop: fr.StopEndTurn}}}, nil
	}
	runEngine(t, e, r)
	want := map[fr.RecordKind]bool{fr.RecordWorkload: false, fr.RecordStep: false, fr.RecordAttempt: false, fr.RecordRouting: false, fr.RecordPolicy: false, fr.RecordProvider: false, fr.RecordTool: false}
	timeout := time.After(time.Second)
	for {
		all := true
		for _, ok := range want {
			all = all && ok
		}
		if all {
			return
		}
		select {
		case k := <-seen:
			want[k] = true
		case <-timeout:
			t.Fatalf("missing observation kinds: %v", want)
		}
	}
}
func TestLoopSinkUnchanged(t *testing.T) {
	// The public loop remains independently usable; its own suite exercises sinks.
	if reflect.TypeFor[fr.LoopSink]().NumMethod() != 1 || reflect.TypeFor[fr.LoopEvent]().NumField() != 10 {
		t.Fatal("turn-scoped contract changed")
	}
}
