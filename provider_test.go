package fabricrunner

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestDiscoverModelsValidatesAndClonesCatalog(t *testing.T) {
	t.Parallel()

	provider := &catalogProvider{name: "local", models: []ModelDescriptor{{
		Ref: ModelRef{Provider: "local", Model: "reasoner"},
		Capabilities: ModelCapabilities{
			ContextTokens: 8192, MaxOutputTokens: 2048, Modalities: []string{"text"},
			ToolUse: ToolUseNative, ParallelToolCalls: true, Streaming: true,
		},
		Labels: map[string]string{"zone": "personal"},
	}}}
	models, err := DiscoverModels(context.Background(), provider)
	if err != nil {
		t.Fatal(err)
	}
	provider.models[0].Capabilities.Modalities[0] = "mutated"
	provider.models[0].Labels["zone"] = "mutated"
	if models[0].Capabilities.Modalities[0] != "text" || models[0].Labels["zone"] != "personal" {
		t.Fatalf("discovered catalog aliases provider memory: %#v", models)
	}
	models[0].Capabilities.Modalities[0] = "caller mutation"
	models[0].Labels["zone"] = "caller mutation"
	if provider.models[0].Capabilities.Modalities[0] != "mutated" || provider.models[0].Labels["zone"] != "mutated" {
		t.Fatal("provider catalog aliases returned memory")
	}
}

func TestDiscoverModelsRejectsInvalidCatalogs(t *testing.T) {
	t.Parallel()

	valid := ModelDescriptor{
		Ref:          ModelRef{Provider: "test", Model: "model"},
		Capabilities: ModelCapabilities{ToolUse: ToolUseNone},
	}
	tests := []struct {
		name     string
		provider *catalogProvider
	}{
		{name: "empty provider", provider: &catalogProvider{}},
		{name: "surrounding whitespace", provider: &catalogProvider{name: " test "}},
		{name: "mismatched owner", provider: &catalogProvider{name: "test", models: []ModelDescriptor{{
			Ref: ModelRef{Provider: "other", Model: "model"}, Capabilities: ModelCapabilities{ToolUse: ToolUseNone},
		}}}},
		{name: "duplicate model", provider: &catalogProvider{name: "test", models: []ModelDescriptor{valid, valid}}},
		{name: "invalid descriptor", provider: &catalogProvider{name: "test", models: []ModelDescriptor{{
			Ref:          ModelRef{Provider: "test", Model: "model"},
			Capabilities: ModelCapabilities{ToolUse: ToolUseNone, ParallelToolCalls: true},
		}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := DiscoverModels(context.Background(), test.provider); !errors.Is(err, ErrProviderCatalog) {
				t.Fatalf("error = %v, want ErrProviderCatalog", err)
			}
		})
	}
}

func TestDiscoverModelsReturnsCancellationUnchanged(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	provider := &catalogProvider{name: "test"}
	if _, err := DiscoverModels(ctx, provider); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if provider.modelCalls.Load() != 0 {
		t.Fatalf("model calls = %d, want 0", provider.modelCalls.Load())
	}
}

func TestModelDescriptorValidation(t *testing.T) {
	t.Parallel()

	valid := ModelDescriptor{
		Ref: ModelRef{Provider: "test", Model: "model"},
		Capabilities: ModelCapabilities{
			ContextTokens: 4096, MaxOutputTokens: 1024, Modalities: []string{"text"}, ToolUse: ToolUseNative,
		},
		Labels: map[string]string{"zone": "personal"},
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*ModelDescriptor)
	}{
		{name: "negative context", mutate: func(value *ModelDescriptor) { value.Capabilities.ContextTokens = -1 }},
		{name: "negative output", mutate: func(value *ModelDescriptor) { value.Capabilities.MaxOutputTokens = -1 }},
		{name: "output above context", mutate: func(value *ModelDescriptor) { value.Capabilities.MaxOutputTokens = 5000 }},
		{name: "unknown tool use", mutate: func(value *ModelDescriptor) { value.Capabilities.ToolUse = "unknown" }},
		{name: "tool flag without mode", mutate: func(value *ModelDescriptor) {
			value.Capabilities.ToolUse = ToolUseNone
			value.Capabilities.StrictToolSchemas = true
		}},
		{name: "empty modality", mutate: func(value *ModelDescriptor) { value.Capabilities.Modalities = []string{""} }},
		{name: "duplicate modality", mutate: func(value *ModelDescriptor) { value.Capabilities.Modalities = []string{"text", "text"} }},
		{name: "noncanonical modality", mutate: func(value *ModelDescriptor) { value.Capabilities.Modalities = []string{" text"} }},
		{name: "empty label", mutate: func(value *ModelDescriptor) { value.Labels = map[string]string{" ": "value"} }},
		{name: "noncanonical provider", mutate: func(value *ModelDescriptor) { value.Ref.Provider = " test" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			value := valid.Clone()
			test.mutate(&value)
			if err := value.Validate(); err == nil {
				t.Fatal("invalid model descriptor was accepted")
			}
		})
	}
}

func TestModelEventValidationRejectsForeignPayloadsAndInvalidDeltas(t *testing.T) {
	t.Parallel()

	tests := []ModelEvent{
		{Type: ModelEventStart, Sequence: 1, Text: "foreign"},
		{Type: ModelEventTextDelta, Sequence: 1},
		{Type: ModelEventTextDelta, Sequence: 1, Text: "ok", Usage: &Usage{}},
		{Type: ModelEventToolCallDelta, Sequence: 1},
		{Type: ModelEventToolCallDelta, Sequence: 1, ToolCallDelta: &ToolCallDelta{Index: -1, ID: "call"}},
		{Type: ModelEventToolCallDelta, Sequence: 1, ToolCallDelta: &ToolCallDelta{}},
		{Type: ModelEventToolCallDelta, Sequence: 1, ToolCallDelta: &ToolCallDelta{ID: " call"}},
		{Type: ModelEventToolCall, Sequence: 1, ToolCall: &ToolCall{ID: "call", Name: "tool", Input: json.RawMessage(`{}`)}, Stop: StopToolUse},
		{Type: ModelEventToolCall, Sequence: 1, ToolCall: &ToolCall{ID: " call", Name: "tool", Input: json.RawMessage(`{}`)}},
		{Type: ModelEventError, Sequence: 1, Error: &ModelError{Code: "failed"}},
		{Type: ModelEventError, Sequence: 1, Error: &ModelError{Code: " failed", Message: "failed"}},
	}
	for index, event := range tests {
		if err := event.Validate(); err == nil {
			t.Fatalf("event %d was accepted: %#v", index, event)
		}
	}
	validDelta := ModelEvent{
		Type: ModelEventToolCallDelta, Sequence: 1,
		ToolCallDelta: &ToolCallDelta{Index: 0, InputFragment: `{"query":`},
	}
	if err := validDelta.Validate(); err != nil {
		t.Fatalf("valid delta rejected: %v", err)
	}
}

func TestModelStreamValidatorRejectsProtocolViolations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		events []ModelEvent
		eof    bool
	}{
		{name: "missing start", events: []ModelEvent{{Type: ModelEventTextDelta, Sequence: 1, Text: "late"}}},
		{name: "repeated start", events: []ModelEvent{{Type: ModelEventStart, Sequence: 1}, {Type: ModelEventStart, Sequence: 2}}},
		{name: "sequence gap", events: []ModelEvent{{Type: ModelEventStart, Sequence: 1}, {Type: ModelEventStop, Sequence: 3, Stop: StopEndTurn}}},
		{name: "after terminal", events: []ModelEvent{{Type: ModelEventStart, Sequence: 1}, {Type: ModelEventStop, Sequence: 2, Stop: StopEndTurn}, {Type: ModelEventTextDelta, Sequence: 3, Text: "late"}}},
		{name: "empty stream", eof: true},
		{name: "unterminated", events: []ModelEvent{{Type: ModelEventStart, Sequence: 1}}, eof: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			validator := ModelStreamValidator{}
			var err error
			for _, event := range test.events {
				if err = validator.Accept(event); err != nil {
					break
				}
			}
			if err == nil && test.eof {
				err = validator.Complete()
			}
			if !errors.Is(err, ErrModelStreamProtocol) {
				t.Fatalf("error = %v, want ErrModelStreamProtocol", err)
			}
		})
	}
}

func TestDrainStreamJoinsReceiveAndCloseFailures(t *testing.T) {
	t.Parallel()

	receiveFailure := errors.New("receive failed")
	closeFailure := errors.New("close failed")
	stream := &failingProviderStream{receiveErr: receiveFailure, closeErr: closeFailure}
	_, err := DrainStream(context.Background(), stream)
	if !errors.Is(err, receiveFailure) || !errors.Is(err, closeFailure) || stream.closeCount.Load() != 1 {
		t.Fatalf("error = %v, closes = %d", err, stream.closeCount.Load())
	}
}

func TestDrainStreamReturnsValidatedPrefixOnFailure(t *testing.T) {
	t.Parallel()

	receiveFailure := errors.New("receive failed")
	stream := &ownedEventStream{
		events: []ModelEvent{{Type: ModelEventStart, Sequence: 1}},
		err:    receiveFailure,
	}
	events, err := DrainStream(context.Background(), stream)
	if !errors.Is(err, receiveFailure) || len(events) != 1 || events[0].Type != ModelEventStart {
		t.Fatalf("events = %#v, error = %v", events, err)
	}
}

func TestDrainStreamReturnsCancellationAndCloses(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stream := &failingProviderStream{}
	_, err := DrainStream(ctx, stream)
	if !errors.Is(err, context.Canceled) || stream.closeCount.Load() != 1 {
		t.Fatalf("error = %v, closes = %d", err, stream.closeCount.Load())
	}
}

func TestDrainStreamDeepClonesEvents(t *testing.T) {
	t.Parallel()

	source := []ModelEvent{
		{Type: ModelEventStart, Sequence: 1, Extension: map[string]json.RawMessage{"id": json.RawMessage(`"one"`)}},
		{Type: ModelEventToolCallDelta, Sequence: 2, ToolCallDelta: &ToolCallDelta{Index: 0, ID: "call"}},
		{Type: ModelEventToolCall, Sequence: 3, ToolCall: &ToolCall{ID: "call", Name: "lookup", Input: json.RawMessage(`{}`)}},
		{Type: ModelEventStop, Sequence: 4, Stop: StopToolUse},
	}
	stream := &ownedEventStream{events: source}
	events, err := DrainStream(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	want := make([]ModelEvent, len(source))
	for index, event := range source {
		want[index] = event.Clone()
	}
	stream.events[0].Extension["id"][1] = 'x'
	stream.events[1].ToolCallDelta.ID = "mutated"
	stream.events[2].ToolCall.Input[0] = '['
	if !reflect.DeepEqual(events, want) {
		t.Fatal("drained events alias stream-owned memory")
	}
}

type catalogProvider struct {
	name       string
	models     []ModelDescriptor
	modelCalls atomic.Int32
}

func (provider *catalogProvider) Name() string { return provider.name }

func (provider *catalogProvider) Models(context.Context) ([]ModelDescriptor, error) {
	provider.modelCalls.Add(1)
	return provider.models, nil
}

func (*catalogProvider) Stream(context.Context, ModelRequest) (ModelStream, error) {
	return nil, errors.New("not implemented")
}

type failingProviderStream struct {
	receiveErr error
	closeErr   error
	closeCount atomic.Int32
}

func (stream *failingProviderStream) Recv(ctx context.Context) (ModelEvent, error) {
	if err := ctx.Err(); err != nil {
		return ModelEvent{}, err
	}
	if stream.receiveErr != nil {
		return ModelEvent{}, stream.receiveErr
	}
	return ModelEvent{}, io.EOF
}

func (stream *failingProviderStream) Close() error {
	stream.closeCount.Add(1)
	return stream.closeErr
}

type ownedEventStream struct {
	events []ModelEvent
	index  int
	err    error
}

func (stream *ownedEventStream) Recv(context.Context) (ModelEvent, error) {
	if stream.index >= len(stream.events) {
		if stream.err != nil {
			err := stream.err
			stream.err = nil
			return ModelEvent{}, err
		}
		return ModelEvent{}, io.EOF
	}
	event := stream.events[stream.index]
	stream.index++
	return event, nil
}

func (*ownedEventStream) Close() error { return nil }
