package fabricrunner_test

import (
	"context"
	"errors"
	fr "github.com/agincgit/fabricrunner"
	"strings"
	"testing"
)

type textCounter struct{}

func (textCounter) Count(_ context.Context, r fr.ModelRequest, _ fr.ModelDescriptor) (int64, error) {
	var n int64
	for _, m := range r.Messages {
		for _, p := range m.Content {
			n += int64(len(p.Text))
		}
	}
	return n, nil
}
func contextFixture(t *testing.T) (*fr.Engine, fr.EngineRequest, *engineProvider) {
	e, r, p := engineFixture(t)
	e.ContextCounter = textCounter{}
	r.Candidates[0].Model.Capabilities.ContextTokens = 10000
	r.Initial.Messages[0].Content[0].Text = strings.Repeat("x", 8100)
	r.Budget.MaxInputTokens = 50000
	e.SpendEstimator = fixedSpend{bound: fr.SpendBound{InputTokens: 10000, OutputTokens: 20, Cost: 20}}
	return e, r, p
}
func TestAutomaticCompactionUnsetFailsOnOverflow(t *testing.T) {
	e, r, p := contextFixture(t)
	r.Initial.Messages[0].Content[0].Text = strings.Repeat("x", 11000)
	_, err := e.Run(context.Background(), r)
	if !errors.Is(err, fr.ErrContextOverflow) || p.calls != 0 {
		t.Fatalf("%v calls=%d", err, p.calls)
	}
}
func TestAutomaticCompactionSetCompacts(t *testing.T) {
	e, r, p := contextFixture(t)
	r.Initial.AutomaticCompaction = true
	projection := runEngine(t, e, r)
	if p.calls != 2 {
		t.Fatalf("calls=%d", p.calls)
	}
	found := false
	for _, record := range projection.Execution {
		if record.Compaction != nil {
			found = true
			if record.Compaction.Summary.Content[0].Text != "done" {
				t.Fatal("wrong summary")
			}
		}
	}
	if !found {
		t.Fatal("no compaction")
	}
}
func TestCompactionEventRecordsReplacedRange(t *testing.T) {
	e, r, _ := contextFixture(t)
	r.Initial.AutomaticCompaction = true
	projection := runEngine(t, e, r)
	for _, record := range projection.Execution {
		if c := record.Compaction; c != nil {
			if c.FirstEvent != 1 || c.LastEvent < 6 || len(c.Inputs) == 0 || c.BeforeTokens < 800 || c.AfterTokens != 4 {
				t.Fatalf("%+v", c)
			}
			return
		}
	}
	t.Fatal("missing record")
}
func TestReplayAppliesRecordedSummary(t *testing.T) {
	e, r, p := contextFixture(t)
	r.Initial.AutomaticCompaction = true
	runEngine(t, e, r)
	calls := p.calls
	projection, err := e.Replay(context.Background(), r.WorkloadID)
	if err != nil || p.calls != calls {
		t.Fatalf("%v", err)
	}
	result := projection.Execution[len(projection.Execution)-1].Result
	if result.Messages[0].Content[0].Text != "done" {
		t.Fatal("summary not adopted")
	}
}
func TestCompactionBudgetExhaustionTerminates(t *testing.T) {
	e, r, p := contextFixture(t)
	r.Initial.AutomaticCompaction = true
	r.Budget.MaxModelCalls = 1
	_, err := e.Run(context.Background(), r)
	if !errors.Is(err, fr.ErrLoopBudgetExceeded) || p.calls > 1 {
		t.Fatalf("%v calls=%d", err, p.calls)
	}
}

func TestCompactionExcludesAboveClassification(t *testing.T) {
	e, r, p := contextFixture(t)
	r.Initial.AutomaticCompaction = true
	r.Initial.Messages = append(r.Initial.Messages, fr.Message{Role: fr.RoleUser, Content: []fr.ContentPart{{Type: fr.ContentText, Classification: fr.ClassSecret, Text: "never send this"}}})
	p.run = func(_ context.Context, request fr.ModelRequest) (fr.ModelStream, error) {
		for _, message := range request.Messages {
			for _, part := range message.Content {
				if strings.Contains(part.Text, "never send this") {
					t.Fatal("secret crossed compaction boundary")
				}
			}
		}
		return &engineStream{events: []fr.ModelEvent{{Type: fr.ModelEventStart, Sequence: 1}, {Type: fr.ModelEventTextDelta, Sequence: 2, Text: "summary"}, {Type: fr.ModelEventStop, Sequence: 3, Stop: fr.StopEndTurn}}}, nil
	}
	projection := runEngine(t, e, r)
	for _, record := range projection.Execution {
		if record.Compaction != nil {
			if len(record.Compaction.Excluded) != 1 || record.Compaction.Excluded[0] != 1 {
				t.Fatal(record.Compaction.Excluded)
			}
			return
		}
	}
	t.Fatal("missing compaction")
}
func TestContextAccountingMatchesEventStream(t *testing.T) {
	e, r, p := contextFixture(t)
	r.Initial.AutomaticCompaction = true
	p.run = func(context.Context, fr.ModelRequest) (fr.ModelStream, error) {
		return &engineStream{events: []fr.ModelEvent{{Type: fr.ModelEventStart, Sequence: 1}, {Type: fr.ModelEventTextDelta, Sequence: 2, Text: "summary"}, {Type: fr.ModelEventUsage, Sequence: 3, Usage: &fr.Usage{InputTokens: 3, OutputTokens: 1}}, {Type: fr.ModelEventStop, Sequence: 4, Stop: fr.StopEndTurn}}}, nil
	}
	projection := runEngine(t, e, r)
	var tokens int64
	for _, record := range projection.Execution {
		if record.Compaction != nil {
			tokens += record.Compaction.Usage.InputTokens
		}
		if record.Loop != nil && record.Loop.ModelEvent != nil && record.Loop.ModelEvent.Usage != nil {
			tokens += record.Loop.ModelEvent.Usage.InputTokens
		}
	}
	result := projection.Execution[len(projection.Execution)-1].Result
	if result.Usage.InputTokens != tokens || tokens != 6 || result.ModelCalls != 2 {
		t.Fatalf("%+v tokens=%d", result, tokens)
	}
}
