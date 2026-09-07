package providertest_test

import (
	"context"
	"encoding/json"
	"io"
	"sync/atomic"
	"testing"

	"github.com/agincgit/fabricrunner"
	"github.com/agincgit/fabricrunner/providertest"
)

func TestRunAcceptsConformingProvider(t *testing.T) {
	t.Parallel()

	providertest.Run(t, func(t *testing.T) providertest.Fixture {
		t.Helper()
		events := []fabricrunner.ModelEvent{
			{Type: fabricrunner.ModelEventStart, Sequence: 1, Extension: map[string]json.RawMessage{"request_id": json.RawMessage(`"req-1"`)}},
			{Type: fabricrunner.ModelEventTextDelta, Sequence: 2, Text: "done"},
			{Type: fabricrunner.ModelEventUsage, Sequence: 3, Usage: &fabricrunner.Usage{InputTokens: 2, OutputTokens: 1}},
			{Type: fabricrunner.ModelEventStop, Sequence: 4, Stop: fabricrunner.StopEndTurn},
		}
		models := []fabricrunner.ModelDescriptor{{
			Ref: fabricrunner.ModelRef{Provider: "scripted", Model: "model"},
			Capabilities: fabricrunner.ModelCapabilities{
				ContextTokens: 4096, MaxOutputTokens: 1024,
				Modalities: []string{"text"}, ToolUse: fabricrunner.ToolUseNative, Streaming: true,
			},
			Labels: map[string]string{"zone": "test"},
		}}
		provider := &scriptedProvider{name: "scripted", models: models, events: events}
		return providertest.Fixture{
			Provider: provider,
			Request: fabricrunner.ModelRequest{
				Model: models[0].Ref,
				Messages: []fabricrunner.Message{{Role: fabricrunner.RoleUser, Content: []fabricrunner.ContentPart{{
					Type: fabricrunner.ContentText, Classification: fabricrunner.ClassInternal, Text: "hello",
				}}}},
				ToolChoice: fabricrunner.ToolChoice{Mode: fabricrunner.ToolChoiceNone},
			},
			WantName: "scripted", WantModels: models, WantEvents: events,
			StreamCloseCount: func() int { return int(provider.closeCount.Load()) },
		}
	})
}

type scriptedProvider struct {
	name       string
	models     []fabricrunner.ModelDescriptor
	events     []fabricrunner.ModelEvent
	closeCount atomic.Int32
}

func (provider *scriptedProvider) Name() string { return provider.name }

func (provider *scriptedProvider) Models(context.Context) ([]fabricrunner.ModelDescriptor, error) {
	return provider.models, nil
}

func (provider *scriptedProvider) Stream(
	_ context.Context,
	_ fabricrunner.ModelRequest,
) (fabricrunner.ModelStream, error) {
	return &scriptedStream{provider: provider}, nil
}

type scriptedStream struct {
	provider *scriptedProvider
	index    int
}

func (stream *scriptedStream) Recv(context.Context) (fabricrunner.ModelEvent, error) {
	if stream.index >= len(stream.provider.events) {
		return fabricrunner.ModelEvent{}, io.EOF
	}
	event := stream.provider.events[stream.index]
	stream.index++
	return event, nil
}

func (stream *scriptedStream) Close() error {
	stream.provider.closeCount.Add(1)
	return nil
}
