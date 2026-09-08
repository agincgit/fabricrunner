package providertest_test

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	fr "github.com/agincgit/fabricrunner"
	"github.com/agincgit/fabricrunner/loop"
)

type hintProvider struct {
	*scriptedProvider
	requests []fr.ModelRequest
}

func (p *hintProvider) Stream(ctx context.Context, request fr.ModelRequest) (fr.ModelStream, error) {
	p.requests = append(p.requests, fr.CloneModelRequest(request))
	return p.scriptedProvider.Stream(ctx, request)
}

type hintSelector struct{ provider *hintProvider }

func (s hintSelector) SelectTurn(ctx context.Context, _ fr.TurnContext) (fr.TurnSelection, error) {
	models, err := fr.DiscoverModels(ctx, s.provider)
	if err != nil {
		return fr.TurnSelection{}, err
	}
	return fr.TurnSelection{Provider: s.provider, Model: models[0].Ref}, nil
}

func hintFixture(automaticCompaction bool, events []fr.ModelEvent) *hintProvider {
	return &hintProvider{scriptedProvider: &scriptedProvider{
		name: "scripted",
		models: []fr.ModelDescriptor{{Ref: fr.ModelRef{Provider: "scripted", Model: "model"}, Capabilities: fr.ModelCapabilities{
			ContextTokens: 64, MaxOutputTokens: 32, ToolUse: fr.ToolUseNone, AutomaticCompaction: automaticCompaction,
		}}}, events: events,
	}}
}

func runHintLoop(t *testing.T, p *hintProvider) (fr.LoopResult, error) {
	t.Helper()
	newID := func() fr.ID {
		id, err := fr.NewID()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	return (loop.Engine{}).Run(context.Background(), fr.LoopRequest{
		WorkloadID: newID(), StepID: newID(), AttemptID: newID(), Selector: hintSelector{provider: p},
		Initial:                   fr.ModelRequest{Model: fr.ModelRef{Provider: "scripted", Model: "model"}, ToolChoice: fr.ToolChoice{Mode: fr.ToolChoiceNone}, Messages: []fr.Message{{Role: fr.RoleUser, Content: []fr.ContentPart{{Type: fr.ContentText, Classification: fr.ClassPublic, Text: strings.Repeat("original context ", 100)}}}}},
		Budget:                    fr.Budget{MaxModelCalls: 3, MaxInputTokens: 10000, MaxOutputTokens: 100, MaxWallTime: time.Second},
		ModelOutputClassification: fr.ClassPublic, ToolErrorClassification: fr.ClassPublic,
	})
}

func TestAutomaticCompactionCapabilityDoesNotEnableRunnerCompaction(t *testing.T) {
	var baseline fr.LoopResult
	var originalRequests []fr.ModelRequest
	for _, enabled := range []bool{false, true} {
		p := hintFixture(enabled, []fr.ModelEvent{{Type: fr.ModelEventStart, Sequence: 1}, {Type: fr.ModelEventTextDelta, Sequence: 2, Text: "done"}, {Type: fr.ModelEventStop, Sequence: 3, Stop: fr.StopEndTurn}})
		catalog, err := fr.DiscoverModels(context.Background(), p)
		if err != nil {
			t.Fatal(err)
		}
		if catalog[0].Capabilities.AutomaticCompaction != enabled {
			t.Fatal("capability metadata was lost")
		}
		result, err := runHintLoop(t, p)
		if err != nil {
			t.Fatal(err)
		}
		if result.ModelCalls != 1 || len(p.requests) != 1 || p.closeCount.Load() != 1 {
			t.Fatal("capability triggered extra work or leaked the stream")
		}
		if !enabled {
			baseline = result
			originalRequests = p.requests
			continue
		}
		if !reflect.DeepEqual(baseline, result) || !reflect.DeepEqual(originalRequests, p.requests) {
			t.Fatal("descriptive capability changed messages or execution")
		}
	}
}

func TestRetryableHintDoesNotRetryFailedTurn(t *testing.T) {
	var baseline fr.LoopResult
	var baselineError string
	for _, retryable := range []bool{false, true} {
		p := hintFixture(false, []fr.ModelEvent{{Type: fr.ModelEventStart, Sequence: 1}, {Type: fr.ModelEventError, Sequence: 2, Error: &fr.ModelError{Code: "transient", Message: "test provider failure", Retryable: retryable}}})
		result, err := runHintLoop(t, p)
		if err == nil {
			t.Fatal("failed turn unexpectedly succeeded")
		}
		if result.ModelCalls != 1 || len(p.requests) != 1 || p.closeCount.Load() != 1 {
			t.Fatal("retry hint caused another call or leaked the stream")
		}
		if !retryable {
			baseline = result
			baselineError = err.Error()
			continue
		}
		if !reflect.DeepEqual(baseline, result) || baselineError != err.Error() {
			t.Fatal("retry hint changed the terminal outcome")
		}
	}
}
