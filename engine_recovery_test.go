package fabricrunner_test

import (
	"context"
	"encoding/json"
	"errors"
	fr "github.com/agincgit/fabricrunner"
	"github.com/agincgit/fabricrunner/store/sqlite"
	"path/filepath"
	"testing"
)

type checkpointCrashStore struct {
	fr.EventStore
	crashed bool
}

func (s *checkpointCrashStore) Append(ctx context.Context, ref fr.AggregateRef, version uint64, drafts ...fr.EventDraft) ([]fr.Event, error) {
	if s.crashed {
		return nil, errors.New("simulated process storage loss")
	}
	events, err := s.EventStore.Append(ctx, ref, version, drafts...)
	for _, draft := range drafts {
		var record fr.ExecutionRecord
		if draft.Type == fr.EventTypeExecutionRecorded && json.Unmarshal(draft.Payload, &record) == nil && record.Loop != nil && record.Loop.Type == fr.LoopEventCheckpoint {
			s.crashed = true
		}
	}
	return events, err
}
func TestResumeAfterSQLiteReopenDoesNotRepeatCommittedTool(t *testing.T) {
	e, r, p := engineFixture(t)
	path := filepath.Join(t.TempDir(), "resume.db")
	store, err := sqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	e.Store = &checkpointCrashStore{EventStore: store}
	h := &engineTool{}
	r.Tools = []fr.ToolBinding{{Definition: fr.ToolDefinition{Name: "effect", InputSchema: json.RawMessage(`{"type":"object"}`)}, Handler: h}}
	p.run = func(context.Context, fr.ModelRequest) (fr.ModelStream, error) {
		if p.calls == 1 {
			return &engineStream{events: []fr.ModelEvent{{Type: fr.ModelEventStart, Sequence: 1}, {Type: fr.ModelEventToolCall, Sequence: 2, ToolCall: &fr.ToolCall{ID: "effect-1", Name: "effect", Input: json.RawMessage(`{}`)}}, {Type: fr.ModelEventStop, Sequence: 3, Stop: fr.StopToolUse}}}, nil
		}
		return &engineStream{events: []fr.ModelEvent{{Type: fr.ModelEventStart, Sequence: 1}, {Type: fr.ModelEventTextDelta, Sequence: 2, Text: "continued"}, {Type: fr.ModelEventStop, Sequence: 3, Stop: fr.StopEndTurn}}}, nil
	}
	projection, err := e.Run(context.Background(), r)
	if err == nil || h.calls != 1 || projection.Workload.State != fr.WorkloadRunning {
		t.Fatalf("%v calls=%d", err, h.calls)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = sqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	e.Store = store
	projection, err = e.Resume(context.Background(), r)
	if err != nil || h.calls != 1 || p.calls != 2 || projection.Workload.State != fr.WorkloadSucceeded {
		t.Fatalf("%v tool=%d model=%d projection=%+v", err, h.calls, p.calls, projection)
	}
}
func TestResumeRefusesUnknownCallOutcome(t *testing.T) {
	e, r, p := engineFixture(t)
	e.Store = &brokenEngineStore{EventStore: e.Store, failLoop: fr.LoopEventModel}
	_, _ = e.Run(context.Background(), r)
	calls := p.calls
	e.Store = e.Store.(*brokenEngineStore).EventStore
	_, err := e.Resume(context.Background(), r)
	if !errors.Is(err, fr.ErrUncertainOutcome) || p.calls != calls {
		t.Fatalf("%v calls=%d", err, p.calls)
	}
}
