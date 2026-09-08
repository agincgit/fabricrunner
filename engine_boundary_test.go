package fabricrunner_test

import (
	"context"
	"encoding/json"
	"errors"
	fr "github.com/agincgit/fabricrunner"
	"strings"
	"testing"
)

type brokenEngineStore struct {
	fr.EventStore
	failType string
	failLoop fr.LoopEventType
}

func (s *brokenEngineStore) Append(ctx context.Context, ref fr.AggregateRef, version uint64, drafts ...fr.EventDraft) ([]fr.Event, error) {
	for _, d := range drafts {
		if d.Type == s.failType {
			return nil, errors.New("storage unavailable")
		}
		if d.Type == fr.EventTypeExecutionRecorded && s.failLoop != "" {
			var r fr.ExecutionRecord
			_ = json.Unmarshal(d.Payload, &r)
			if r.Loop != nil && r.Loop.Type == s.failLoop {
				return nil, errors.New("storage unavailable")
			}
		}
	}
	return s.EventStore.Append(ctx, ref, version, drafts...)
}
func TestStoreFailurePreventsProviderCall(t *testing.T) {
	for _, stage := range []string{"create", "policy", "turn"} {
		t.Run(stage, func(t *testing.T) {
			e, r, p := engineFixture(t)
			s := &brokenEngineStore{EventStore: e.Store}
			switch stage {
			case "create":
				s.failType = fr.EventTypeWorkloadCreated
			case "policy":
				s.failType = fr.EventTypeExecutionRecorded
			case "turn":
				s.failLoop = fr.LoopEventTurnStarted
			}
			e.Store = s
			if _, err := e.Run(context.Background(), r); err == nil || p.calls != 0 {
				t.Fatalf("calls=%d err=%v", p.calls, err)
			}
		})
	}
}
func TestDuplicateRunNeverRepeatsEffects(t *testing.T) {
	e, r, p := engineFixture(t)
	runEngine(t, e, r)
	_, err := e.Run(context.Background(), r)
	if !errors.Is(err, fr.ErrVersionConflict) || p.calls != 1 {
		t.Fatalf("calls=%d err=%v", p.calls, err)
	}
}

func TestProviderCancellationIsNotSuccess(t *testing.T) {
	e, r, p := engineFixture(t)
	p.run = func(context.Context, fr.ModelRequest) (fr.ModelStream, error) {
		return &engineStream{events: []fr.ModelEvent{{Type: fr.ModelEventStart, Sequence: 1}, {Type: fr.ModelEventStop, Sequence: 2, Stop: fr.StopCancelled}}}, nil
	}
	got, err := e.Run(context.Background(), r)
	if !errors.Is(err, context.Canceled) || got == nil || got.Workload.State != fr.WorkloadCancelled {
		t.Fatalf("projection=%v err=%v", got, err)
	}
}

type classifiedEngineTool struct {
	engineTool
	class fr.Classification
}

func (h *classifiedEngineTool) Execute(context.Context, fr.ToolInvocation) (fr.ToolOutput, error) {
	h.calls++
	return fr.ToolOutput{JSON: json.RawMessage(`{"value":"private-result"}`), Classification: h.class}, nil
}
func TestToolResultEgressReevaluatedBeforeNextTurn(t *testing.T) {
	e, r, p := engineFixture(t)
	r.Candidates[0].Zone = fr.ZoneManagedCloud
	h := &classifiedEngineTool{class: fr.ClassConfidential}
	r.Tools = []fr.ToolBinding{{Definition: fr.ToolDefinition{Name: "test", InputSchema: json.RawMessage(`{"type":"object"}`)}, Handler: h}}
	p.run = func(context.Context, fr.ModelRequest) (fr.ModelStream, error) {
		return &engineStream{events: []fr.ModelEvent{{Type: fr.ModelEventStart, Sequence: 1}, {Type: fr.ModelEventToolCall, Sequence: 2, ToolCall: &fr.ToolCall{ID: "call", Name: "test", Input: json.RawMessage(`{}`)}}, {Type: fr.ModelEventStop, Sequence: 3, Stop: fr.StopToolUse}}}, nil
	}
	got, err := e.Run(context.Background(), r)
	if err == nil || p.calls != 1 || h.calls != 1 || got.Workload.State != fr.WorkloadFailed {
		t.Fatalf("calls=%d tools=%d err=%v", p.calls, h.calls, err)
	}
}
func TestDeniedModelResultNeverReachesContentLog(t *testing.T) {
	e, r, _ := engineFixture(t)
	r.Candidates[0].Zone = fr.ZoneManagedCloud
	r.ModelOutputClassification = fr.ClassConfidential
	got, err := e.Run(context.Background(), r)
	if !errors.Is(err, fr.ErrExecutionDenied) || got == nil {
		t.Fatalf("%v", err)
	}
	for _, event := range engineEvents(t, e, r) {
		if strings.Contains(string(event.Payload), `"Text":"done"`) {
			t.Fatal("unapproved model content persisted")
		}
	}
}
func TestStoreFailurePreventsToolCall(t *testing.T) {
	e, r, p := engineFixture(t)
	h := &engineTool{}
	r.Tools = []fr.ToolBinding{{Definition: fr.ToolDefinition{Name: "test", InputSchema: json.RawMessage(`{"type":"object"}`)}, Handler: h}}
	e.Store = &brokenEngineStore{EventStore: e.Store, failLoop: fr.LoopEventToolStarted}
	p.run = func(context.Context, fr.ModelRequest) (fr.ModelStream, error) {
		return &engineStream{events: []fr.ModelEvent{{Type: fr.ModelEventStart, Sequence: 1}, {Type: fr.ModelEventToolCall, Sequence: 2, ToolCall: &fr.ToolCall{ID: "call", Name: "test", Input: json.RawMessage(`{}`)}}, {Type: fr.ModelEventStop, Sequence: 3, Stop: fr.StopToolUse}}}, nil
	}
	if _, err := e.Run(context.Background(), r); err == nil || h.calls != 0 {
		t.Fatalf("tool calls=%d err=%v", h.calls, err)
	}
}
