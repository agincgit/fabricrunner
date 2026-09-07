package loop_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agincgit/fabricrunner"
	turnloop "github.com/agincgit/fabricrunner/loop"
)

func TestEngineCanChangeProviderBetweenTurns(t *testing.T) {
	t.Parallel()

	first := &fakeProvider{name: "personal", streams: [][]fabricrunner.ModelEvent{{
		{Type: fabricrunner.ModelEventStart, Sequence: 1},
		{Type: fabricrunner.ModelEventToolCall, Sequence: 2, ToolCall: &fabricrunner.ToolCall{ID: "call-1", Name: "lookup", Input: json.RawMessage(`{"query":"go"}`)}},
		{Type: fabricrunner.ModelEventStop, Sequence: 3, Stop: fabricrunner.StopToolUse},
	}}}
	second := &fakeProvider{name: "managed", streams: [][]fabricrunner.ModelEvent{{
		{Type: fabricrunner.ModelEventStart, Sequence: 1},
		{Type: fabricrunner.ModelEventTextDelta, Sequence: 2, Text: "answer"},
		{Type: fabricrunner.ModelEventUsage, Sequence: 3, Usage: &fabricrunner.Usage{InputTokens: 10, OutputTokens: 2, Cost: 5}},
		{Type: fabricrunner.ModelEventStop, Sequence: 4, Stop: fabricrunner.StopEndTurn},
	}}}
	selector := &fakeSelector{selections: []fabricrunner.TurnSelection{
		{Provider: first, Model: fabricrunner.ModelRef{Provider: "personal", Model: "local"}},
		{Provider: second, Model: fabricrunner.ModelRef{Provider: "managed", Model: "frontier"}},
	}}
	handler := &fakeTool{execute: func(_ context.Context, invocation fabricrunner.ToolInvocation) (fabricrunner.ToolOutput, error) {
		invocation.Call.Input[0] = ' '
		return fabricrunner.ToolOutput{JSON: json.RawMessage(`{"value":"result"}`), Classification: fabricrunner.ClassInternal}, nil
	}}
	sink := &recordingSink{mutate: true}
	request := loopRequest(t, selector, sink, []fabricrunner.ToolBinding{toolBinding(handler, true)})
	original := fabricrunner.CloneModelRequest(request.Initial)

	result, err := (turnloop.Engine{}).Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Stop != fabricrunner.StopEndTurn || result.ModelCalls != 2 || result.ToolCalls != 1 ||
		result.Usage.InputTokens != 10 || result.Usage.OutputTokens != 2 || result.Usage.Cost != 5 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(result.Messages) != 4 || result.Messages[3].Content[0].Text != "answer" {
		t.Fatalf("messages = %#v", result.Messages)
	}
	if len(first.requests) != 1 || first.requests[0].Model.Model != "local" ||
		len(second.requests) != 1 || second.requests[0].Model.Model != "frontier" {
		t.Fatalf("provider requests: first=%#v second=%#v", first.requests, second.requests)
	}
	if len(second.requests[0].Messages) != 3 || second.requests[0].Messages[2].Role != fabricrunner.RoleTool {
		t.Fatalf("second turn did not receive ordered tool result: %#v", second.requests[0].Messages)
	}
	if !reflect.DeepEqual(request.Initial, original) {
		t.Fatal("loop collaborators mutated caller-owned model request")
	}
	if handler.closeCount.Load() != 1 {
		t.Fatalf("tool close count = %d, want 1", handler.closeCount.Load())
	}
	for index, event := range sink.events {
		if event.Sequence != uint64(index+1) {
			t.Fatalf("event %d sequence = %d", index, event.Sequence)
		}
	}
}

func TestSchemaInvalidInputDoesNotExecuteTool(t *testing.T) {
	t.Parallel()

	provider := toolThenAnswerProvider(json.RawMessage(`{"query":7}`))
	selector := &fakeSelector{selections: []fabricrunner.TurnSelection{
		{Provider: provider, Model: modelRef()}, {Provider: provider, Model: modelRef()},
	}}
	handler := &fakeTool{}
	request := loopRequest(t, selector, nil, []fabricrunner.ToolBinding{toolBinding(handler, true)})
	result, err := (turnloop.Engine{}).Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if handler.executeCount.Load() != 0 || len(result.Failures) != 1 ||
		result.Failures[0].Code != "tool_input_invalid" ||
		!errors.Is(result.Failures[0].Cause, fabricrunner.ErrLoopToolInput) {
		t.Fatalf("schema failure = %#v, executions = %d", result.Failures, handler.executeCount.Load())
	}
}

func TestUndeclaredAndDuplicateCallsNeverExecuteTools(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		firstTurn  []fabricrunner.ModelEvent
		selections int
		wantError  error
		wantCode   string
	}{
		{
			name: "undeclared tool",
			firstTurn: []fabricrunner.ModelEvent{
				{Type: fabricrunner.ModelEventStart, Sequence: 1},
				{Type: fabricrunner.ModelEventToolCall, Sequence: 2, ToolCall: &fabricrunner.ToolCall{ID: "call-1", Name: "missing", Input: json.RawMessage(`{}`)}},
				{Type: fabricrunner.ModelEventStop, Sequence: 3, Stop: fabricrunner.StopToolUse},
			},
			selections: 2,
			wantCode:   "tool_not_found",
		},
		{
			name: "duplicate call ID",
			firstTurn: []fabricrunner.ModelEvent{
				{Type: fabricrunner.ModelEventStart, Sequence: 1},
				{Type: fabricrunner.ModelEventToolCall, Sequence: 2, ToolCall: &fabricrunner.ToolCall{ID: "same", Name: "lookup", Input: json.RawMessage(`{"query":"one"}`)}},
				{Type: fabricrunner.ModelEventToolCall, Sequence: 3, ToolCall: &fabricrunner.ToolCall{ID: "same", Name: "lookup", Input: json.RawMessage(`{"query":"two"}`)}},
				{Type: fabricrunner.ModelEventStop, Sequence: 4, Stop: fabricrunner.StopToolUse},
			},
			selections: 1,
			wantError:  fabricrunner.ErrLoopStream,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			provider := &fakeProvider{name: "test", streams: [][]fabricrunner.ModelEvent{
				test.firstTurn,
				{
					{Type: fabricrunner.ModelEventStart, Sequence: 1},
					{Type: fabricrunner.ModelEventStop, Sequence: 2, Stop: fabricrunner.StopEndTurn},
				},
			}}
			selections := make([]fabricrunner.TurnSelection, test.selections)
			for index := range selections {
				selections[index] = fabricrunner.TurnSelection{Provider: provider, Model: modelRef()}
			}
			selector := &fakeSelector{selections: selections}
			handler := &fakeTool{}
			request := loopRequest(t, selector, nil, []fabricrunner.ToolBinding{toolBinding(handler, true)})
			result, err := (turnloop.Engine{}).Run(context.Background(), request)
			if test.wantError != nil && !errors.Is(err, test.wantError) {
				t.Fatalf("error = %v, want %v", err, test.wantError)
			}
			if test.wantError == nil && err != nil {
				t.Fatal(err)
			}
			if handler.executeCount.Load() != 0 {
				t.Fatalf("tool executions = %d, want 0", handler.executeCount.Load())
			}
			if test.wantCode != "" && (len(result.Failures) != 1 || result.Failures[0].Code != test.wantCode) {
				t.Fatalf("failures = %#v", result.Failures)
			}
		})
	}
}

func TestPublicToolErrorIsNormalizedWithoutLosingCause(t *testing.T) {
	t.Parallel()

	provider := toolThenAnswerProvider(json.RawMessage(`{"query":"go"}`))
	selector := &fakeSelector{selections: []fabricrunner.TurnSelection{
		{Provider: provider, Model: modelRef()}, {Provider: provider, Model: modelRef()},
	}}
	cause := errors.New("upstream unavailable")
	handler := &fakeTool{execute: func(context.Context, fabricrunner.ToolInvocation) (fabricrunner.ToolOutput, error) {
		return fabricrunner.ToolOutput{}, &fabricrunner.ToolError{
			Code: "lookup_unavailable", Message: "lookup is temporarily unavailable", Cause: cause,
		}
	}}
	request := loopRequest(t, selector, nil, []fabricrunner.ToolBinding{toolBinding(handler, false)})
	result, err := (turnloop.Engine{}).Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Failures) != 1 || result.Failures[0].Code != "lookup_unavailable" ||
		result.Failures[0].Message != "lookup is temporarily unavailable" ||
		!errors.Is(result.Failures[0].Cause, cause) {
		t.Fatalf("normalized failure = %#v", result.Failures)
	}
	if part := result.Messages[2].Content[0]; !part.IsError ||
		string(part.JSON) != `{"error":{"code":"lookup_unavailable","message":"lookup is temporarily unavailable"}}` {
		t.Fatalf("model-visible error = %#v", part)
	}
}

func TestOutputSchemaAndMiddlewareFailuresBecomeExplicitErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		execute    func(context.Context, fabricrunner.ToolInvocation) (fabricrunner.ToolOutput, error)
		middleware []fabricrunner.ToolResultMiddleware
		wantCode   string
	}{
		{
			name: "output schema", wantCode: "tool_output_invalid",
			execute: func(context.Context, fabricrunner.ToolInvocation) (fabricrunner.ToolOutput, error) {
				return fabricrunner.ToolOutput{JSON: json.RawMessage(`{"wrong":true}`), Classification: fabricrunner.ClassInternal}, nil
			},
		},
		{
			name: "middleware", wantCode: "tool_result_middleware_failed", execute: successfulTool,
			middleware: []fabricrunner.ToolResultMiddleware{middlewareFunc(func(context.Context, fabricrunner.ToolInvocation, fabricrunner.ToolOutput) (fabricrunner.ToolOutput, error) {
				return fabricrunner.ToolOutput{}, errors.New("redaction failed")
			})},
		},
		{
			name: "panic", wantCode: "tool_panicked",
			execute: func(context.Context, fabricrunner.ToolInvocation) (fabricrunner.ToolOutput, error) {
				panic("tool panic")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			provider := toolThenAnswerProvider(json.RawMessage(`{"query":"go"}`))
			selector := &fakeSelector{selections: []fabricrunner.TurnSelection{
				{Provider: provider, Model: modelRef()}, {Provider: provider, Model: modelRef()},
			}}
			handler := &fakeTool{execute: test.execute}
			request := loopRequest(t, selector, nil, []fabricrunner.ToolBinding{toolBinding(handler, true)})
			request.Middleware = test.middleware
			result, err := (turnloop.Engine{}).Run(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Failures) != 1 || result.Failures[0].Code != test.wantCode {
				t.Fatalf("failures = %#v", result.Failures)
			}
			toolPart := result.Messages[2].Content[0]
			if !toolPart.IsError {
				t.Fatalf("failure was not marked as an error: %#v", toolPart)
			}
		})
	}
}

func TestMiddlewareRunsInDeclarationOrder(t *testing.T) {
	t.Parallel()

	provider := toolThenAnswerProvider(json.RawMessage(`{"query":"go"}`))
	selector := &fakeSelector{selections: []fabricrunner.TurnSelection{
		{Provider: provider, Model: modelRef()}, {Provider: provider, Model: modelRef()},
	}}
	var order []string
	request := loopRequest(t, selector, nil, []fabricrunner.ToolBinding{toolBinding(&fakeTool{execute: successfulTool}, true)})
	request.Middleware = []fabricrunner.ToolResultMiddleware{
		middlewareFunc(func(_ context.Context, _ fabricrunner.ToolInvocation, output fabricrunner.ToolOutput) (fabricrunner.ToolOutput, error) {
			order = append(order, "first")
			output.JSON = json.RawMessage(`{"value":"first"}`)
			return output, nil
		}),
		middlewareFunc(func(_ context.Context, _ fabricrunner.ToolInvocation, output fabricrunner.ToolOutput) (fabricrunner.ToolOutput, error) {
			order = append(order, "second")
			output.JSON = json.RawMessage(`{"value":"second"}`)
			return output, nil
		}),
	}
	result, err := (turnloop.Engine{}).Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"first", "second"}) ||
		string(result.Messages[2].Content[0].JSON) != `{"value":"second"}` {
		t.Fatalf("middleware order = %v, result = %s", order, result.Messages[2].Content[0].JSON)
	}
}

func TestParallelToolMiddlewareRunsSeriallyInCallOrder(t *testing.T) {
	t.Parallel()

	provider := twoToolsThenAnswerProvider()
	selector := &fakeSelector{selections: []fabricrunner.TurnSelection{
		{Provider: provider, Model: modelRef()}, {Provider: provider, Model: modelRef()},
	}}
	var order []string
	request := loopRequest(t, selector, nil, []fabricrunner.ToolBinding{toolBinding(&overlapTool{}, true)})
	request.Middleware = []fabricrunner.ToolResultMiddleware{middlewareFunc(func(
		_ context.Context,
		invocation fabricrunner.ToolInvocation,
		output fabricrunner.ToolOutput,
	) (fabricrunner.ToolOutput, error) {
		order = append(order, invocation.Call.ID)
		return output, nil
	})}
	if _, err := (turnloop.Engine{}).Run(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"call-1", "call-2"}) {
		t.Fatalf("middleware call order = %v", order)
	}
}

func TestParallelSafetyControlsOverlapAndPreservesOrder(t *testing.T) {
	t.Parallel()

	for _, parallelSafe := range []bool{false, true} {
		parallelSafe := parallelSafe
		t.Run(map[bool]string{false: "unsafe", true: "safe"}[parallelSafe], func(t *testing.T) {
			t.Parallel()
			provider := twoToolsThenAnswerProvider()
			selector := &fakeSelector{selections: []fabricrunner.TurnSelection{
				{Provider: provider, Model: modelRef()}, {Provider: provider, Model: modelRef()},
			}}
			handler := &overlapTool{}
			sink := &recordingSink{}
			request := loopRequest(t, selector, sink, []fabricrunner.ToolBinding{toolBinding(handler, parallelSafe)})
			request.ParallelTools = true
			result, err := (turnloop.Engine{}).Run(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			wantMax := int32(1)
			if parallelSafe {
				wantMax = 2
			}
			if handler.maximum.Load() != wantMax {
				t.Fatalf("maximum overlap = %d, want %d", handler.maximum.Load(), wantMax)
			}
			toolMessage := result.Messages[2]
			if toolMessage.Content[0].ToolCallID != "call-1" || toolMessage.Content[1].ToolCallID != "call-2" {
				t.Fatalf("tool result order = %#v", toolMessage.Content)
			}
			var completed []string
			for _, event := range sink.events {
				if event.Type == fabricrunner.LoopEventToolCompleted {
					completed = append(completed, event.ToolCall.ID)
				}
			}
			if !reflect.DeepEqual(completed, []string{"call-1", "call-2"}) {
				t.Fatalf("completion order = %v", completed)
			}
		})
	}
}

func TestModelCallBudgetStopsBeforeNextSelection(t *testing.T) {
	t.Parallel()

	provider := toolThenAnswerProvider(json.RawMessage(`{"query":"go"}`))
	selector := &fakeSelector{selections: []fabricrunner.TurnSelection{{Provider: provider, Model: modelRef()}}}
	handler := &fakeTool{execute: successfulTool}
	sink := &recordingSink{}
	request := loopRequest(t, selector, sink, []fabricrunner.ToolBinding{toolBinding(handler, false)})
	request.Budget.MaxModelCalls = 1
	result, err := (turnloop.Engine{}).Run(context.Background(), request)
	if !errors.Is(err, fabricrunner.ErrLoopBudgetExceeded) || result.BudgetExceeded != "model_calls" ||
		selector.calls.Load() != 1 || len(provider.requests) != 1 {
		t.Fatalf("budget result = %#v, error = %v, selections = %d", result, err, selector.calls.Load())
	}
	if last := sink.events[len(sink.events)-1]; last.Type != fabricrunner.LoopEventStopped ||
		last.Stop != fabricrunner.StopUnknown || last.FailureCode != "budget_model_calls" {
		t.Fatalf("terminal budget event = %#v", last)
	}
}

func TestToolCallBudgetStopsBeforeExecution(t *testing.T) {
	t.Parallel()

	provider := toolThenAnswerProvider(json.RawMessage(`{"query":"go"}`))
	selector := &fakeSelector{selections: []fabricrunner.TurnSelection{{Provider: provider, Model: modelRef()}}}
	handler := &fakeTool{execute: successfulTool}
	sink := &recordingSink{}
	request := loopRequest(t, selector, sink, []fabricrunner.ToolBinding{toolBinding(handler, false)})
	request.Budget.MaxToolCalls = 0
	result, err := (turnloop.Engine{}).Run(context.Background(), request)
	if !errors.Is(err, fabricrunner.ErrLoopBudgetExceeded) || result.BudgetExceeded != "tool_calls" ||
		handler.executeCount.Load() != 0 {
		t.Fatalf("tool budget result = %#v, error = %v, executions = %d", result, err, handler.executeCount.Load())
	}
	if last := sink.events[len(sink.events)-1]; last.Type != fabricrunner.LoopEventStopped || last.FailureCode != "budget_tool_calls" {
		t.Fatalf("terminal budget event = %#v", last)
	}
}

func TestUsageBudgetsStopOnReportedUsage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		usage  fabricrunner.Usage
		mutate func(*fabricrunner.Budget)
		want   string
	}{
		{name: "input", usage: fabricrunner.Usage{InputTokens: 2}, mutate: func(budget *fabricrunner.Budget) { budget.MaxInputTokens = 1 }, want: "input_tokens"},
		{name: "output", usage: fabricrunner.Usage{OutputTokens: 2}, mutate: func(budget *fabricrunner.Budget) { budget.MaxOutputTokens = 1 }, want: "output_tokens"},
		{name: "cost", usage: fabricrunner.Usage{Cost: 2}, mutate: func(budget *fabricrunner.Budget) { budget.MaxCost = 1 }, want: "cost"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			provider := &fakeProvider{name: "usage", streams: [][]fabricrunner.ModelEvent{{
				{Type: fabricrunner.ModelEventStart, Sequence: 1},
				{Type: fabricrunner.ModelEventUsage, Sequence: 2, Usage: &test.usage},
				{Type: fabricrunner.ModelEventStop, Sequence: 3, Stop: fabricrunner.StopEndTurn},
			}}}
			selector := &fakeSelector{selections: []fabricrunner.TurnSelection{{Provider: provider, Model: modelRef()}}}
			request := loopRequest(t, selector, nil, nil)
			test.mutate(&request.Budget)
			result, err := (turnloop.Engine{}).Run(context.Background(), request)
			if !errors.Is(err, fabricrunner.ErrLoopBudgetExceeded) || result.BudgetExceeded != test.want {
				t.Fatalf("usage budget result = %#v, error = %v", result, err)
			}
		})
	}
}

func TestWallTimeBudgetMapsProviderCancellation(t *testing.T) {
	t.Parallel()

	stream := &blockingStream{entered: make(chan struct{})}
	provider := &blockingProvider{stream: stream}
	selector := &fakeSelector{selections: []fabricrunner.TurnSelection{{Provider: provider, Model: modelRef()}}}
	request := loopRequest(t, selector, nil, nil)
	request.Budget.MaxWallTime = 20 * time.Millisecond
	result, err := (turnloop.Engine{}).Run(context.Background(), request)
	if !errors.Is(err, fabricrunner.ErrLoopBudgetExceeded) || result.BudgetExceeded != "wall_time" ||
		stream.closeCount.Load() != 1 {
		t.Fatalf("wall budget result = %#v, error = %v, closes = %d", result, err, stream.closeCount.Load())
	}
}

func TestMalformedStreamsFailClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		events []fabricrunner.ModelEvent
	}{
		{name: "missing start", events: []fabricrunner.ModelEvent{
			{Type: fabricrunner.ModelEventTextDelta, Sequence: 1, Text: "late"},
			{Type: fabricrunner.ModelEventStop, Sequence: 2, Stop: fabricrunner.StopEndTurn},
		}},
		{name: "repeated start", events: []fabricrunner.ModelEvent{
			{Type: fabricrunner.ModelEventStart, Sequence: 1},
			{Type: fabricrunner.ModelEventStart, Sequence: 2},
		}},
		{name: "out of order", events: []fabricrunner.ModelEvent{{Type: fabricrunner.ModelEventStart, Sequence: 2}}},
		{name: "unterminated", events: []fabricrunner.ModelEvent{{Type: fabricrunner.ModelEventStart, Sequence: 1}}},
		{name: "after terminal", events: []fabricrunner.ModelEvent{
			{Type: fabricrunner.ModelEventStart, Sequence: 1},
			{Type: fabricrunner.ModelEventStop, Sequence: 2, Stop: fabricrunner.StopEndTurn},
			{Type: fabricrunner.ModelEventTextDelta, Sequence: 3, Text: "late"},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			provider := &fakeProvider{name: "test", streams: [][]fabricrunner.ModelEvent{test.events}}
			selector := &fakeSelector{selections: []fabricrunner.TurnSelection{{Provider: provider, Model: modelRef()}}}
			request := loopRequest(t, selector, nil, nil)
			_, err := (turnloop.Engine{}).Run(context.Background(), request)
			if !errors.Is(err, fabricrunner.ErrLoopStream) {
				t.Fatalf("error = %v, want ErrLoopStream", err)
			}
		})
	}
}

func TestCancellationClosesActiveStreamAndTools(t *testing.T) {
	t.Parallel()

	stream := &blockingStream{entered: make(chan struct{})}
	provider := &blockingProvider{stream: stream}
	selector := &fakeSelector{selections: []fabricrunner.TurnSelection{{Provider: provider, Model: modelRef()}}}
	handler := &fakeTool{}
	request := loopRequest(t, selector, nil, []fabricrunner.ToolBinding{toolBinding(handler, false)})
	ctx, cancel := context.WithCancel(context.Background())
	resultChannel := make(chan error, 1)
	go func() {
		_, err := (turnloop.Engine{}).Run(ctx, request)
		resultChannel <- err
	}()
	select {
	case <-stream.entered:
	case <-time.After(time.Second):
		t.Fatal("stream Recv was not entered")
	}
	cancel()
	select {
	case err := <-resultChannel:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled loop did not return")
	}
	if stream.closeCount.Load() != 1 || handler.closeCount.Load() != 1 {
		t.Fatalf("stream closes = %d, tool closes = %d", stream.closeCount.Load(), handler.closeCount.Load())
	}
}

func TestPanicStillClosesToolsInReverseOrder(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var order []string
	first := &fakeTool{close: func(context.Context) error { mu.Lock(); defer mu.Unlock(); order = append(order, "first"); return nil }}
	second := &fakeTool{close: func(context.Context) error { mu.Lock(); defer mu.Unlock(); order = append(order, "second"); return nil }}
	selector := selectorFunc(func(context.Context, fabricrunner.TurnContext) (fabricrunner.TurnSelection, error) {
		panic("selector panic")
	})
	request := loopRequest(t, selector, nil, []fabricrunner.ToolBinding{
		toolBindingNamed("first", first, false), toolBindingNamed("second", second, false),
	})
	_, err := (turnloop.Engine{}).Run(context.Background(), request)
	if !errors.Is(err, fabricrunner.ErrLoopPanic) {
		t.Fatalf("error = %v, want ErrLoopPanic", err)
	}
	if !reflect.DeepEqual(order, []string{"second", "first"}) {
		t.Fatalf("cleanup order = %v", order)
	}
}

func TestCleanupErrorsAreJoinedAndReportedTerminally(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var order []string
	closeFailure := errors.New("close failed")
	first := &fakeTool{close: func(context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, "first")
		return closeFailure
	}}
	second := &fakeTool{close: func(context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, "second")
		panic("close panic")
	}}
	provider := &fakeProvider{name: "test", streams: [][]fabricrunner.ModelEvent{{
		{Type: fabricrunner.ModelEventStart, Sequence: 1},
		{Type: fabricrunner.ModelEventStop, Sequence: 2, Stop: fabricrunner.StopEndTurn},
	}}}
	selector := &fakeSelector{selections: []fabricrunner.TurnSelection{{Provider: provider, Model: modelRef()}}}
	sink := &recordingSink{}
	request := loopRequest(t, selector, sink, []fabricrunner.ToolBinding{
		toolBindingNamed("first", first, false), toolBindingNamed("second", second, false),
	})
	_, err := (turnloop.Engine{}).Run(context.Background(), request)
	if !errors.Is(err, fabricrunner.ErrLoopCleanup) || !errors.Is(err, closeFailure) {
		t.Fatalf("cleanup error = %v", err)
	}
	if !reflect.DeepEqual(order, []string{"second", "first"}) ||
		first.closeCount.Load() != 1 || second.closeCount.Load() != 1 {
		t.Fatalf("cleanup order = %v, counts = %d/%d", order, first.closeCount.Load(), second.closeCount.Load())
	}
	if last := sink.events[len(sink.events)-1]; last.Type != fabricrunner.LoopEventStopped ||
		last.Stop != fabricrunner.StopUnknown || last.FailureCode != "failed" {
		t.Fatalf("terminal cleanup event = %#v", last)
	}
}

func TestInvalidRequestInvokesNoCollaborator(t *testing.T) {
	t.Parallel()

	selector := &fakeSelector{}
	sink := &recordingSink{}
	handler := &fakeTool{}
	request := loopRequest(t, selector, sink, []fabricrunner.ToolBinding{toolBinding(handler, false)})
	request.Initial.ToolChoice.Mode = "invalid"
	if _, err := (turnloop.Engine{}).Run(context.Background(), request); err == nil {
		t.Fatal("invalid request unexpectedly succeeded")
	}
	if selector.calls.Load() != 0 || len(sink.events) != 0 ||
		handler.executeCount.Load() != 0 || handler.closeCount.Load() != 0 {
		t.Fatalf("collaborator calls: selector=%d sink=%d execute=%d close=%d",
			selector.calls.Load(), len(sink.events), handler.executeCount.Load(), handler.closeCount.Load())
	}
}

func TestSinkFailureStopsBeforeProviderSideEffect(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{name: "test", streams: [][]fabricrunner.ModelEvent{{
		{Type: fabricrunner.ModelEventStart, Sequence: 1},
		{Type: fabricrunner.ModelEventStop, Sequence: 2, Stop: fabricrunner.StopEndTurn},
	}}}
	selector := &fakeSelector{selections: []fabricrunner.TurnSelection{{Provider: provider, Model: modelRef()}}}
	sinkError := errors.New("sink unavailable")
	request := loopRequest(t, selector, &recordingSink{failAt: 1, failure: sinkError}, nil)
	_, err := (turnloop.Engine{}).Run(context.Background(), request)
	if !errors.Is(err, sinkError) || len(provider.requests) != 0 {
		t.Fatalf("error = %v, provider calls = %d", err, len(provider.requests))
	}
}

func loopRequest(
	t *testing.T,
	selector fabricrunner.TurnSelector,
	sink fabricrunner.LoopSink,
	tools []fabricrunner.ToolBinding,
) fabricrunner.LoopRequest {
	t.Helper()
	return fabricrunner.LoopRequest{
		WorkloadID: testID(t), StepID: testID(t), AttemptID: testID(t),
		Initial: fabricrunner.ModelRequest{
			Model: modelRef(),
			Messages: []fabricrunner.Message{{Role: fabricrunner.RoleUser, Content: []fabricrunner.ContentPart{{
				Type: fabricrunner.ContentText, Classification: fabricrunner.ClassInternal, Text: "question",
			}}}},
			ToolChoice:      fabricrunner.ToolChoice{Mode: fabricrunner.ToolChoiceAuto},
			MaxOutputTokens: 1000,
		},
		Tools: tools, Selector: selector, Sink: sink,
		Budget: fabricrunner.Budget{
			MaxInputTokens: 10000, MaxOutputTokens: 10000, MaxModelCalls: 4,
			MaxToolCalls: 4, MaxCost: 10000, MaxWallTime: 2 * time.Second,
		},
		ParallelTools: true, ModelOutputClassification: fabricrunner.ClassInternal,
		ToolErrorClassification: fabricrunner.ClassInternal,
	}
}

func toolBinding(handler fabricrunner.ToolHandler, parallel bool) fabricrunner.ToolBinding {
	return toolBindingNamed("lookup", handler, parallel)
}

func toolBindingNamed(name string, handler fabricrunner.ToolHandler, parallel bool) fabricrunner.ToolBinding {
	return fabricrunner.ToolBinding{
		Definition: fabricrunner.ToolDefinition{
			Name:         name,
			InputSchema:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"],"additionalProperties":false}`),
			OutputSchema: json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"],"additionalProperties":false}`),
		},
		Handler: handler, ParallelSafe: parallel,
	}
}

func toolThenAnswerProvider(input json.RawMessage) *fakeProvider {
	return &fakeProvider{name: "test", streams: [][]fabricrunner.ModelEvent{
		{
			{Type: fabricrunner.ModelEventStart, Sequence: 1},
			{Type: fabricrunner.ModelEventToolCall, Sequence: 2, ToolCall: &fabricrunner.ToolCall{ID: "call-1", Name: "lookup", Input: input}},
			{Type: fabricrunner.ModelEventStop, Sequence: 3, Stop: fabricrunner.StopToolUse},
		},
		{
			{Type: fabricrunner.ModelEventStart, Sequence: 1},
			{Type: fabricrunner.ModelEventTextDelta, Sequence: 2, Text: "done"},
			{Type: fabricrunner.ModelEventStop, Sequence: 3, Stop: fabricrunner.StopEndTurn},
		},
	}}
}

func twoToolsThenAnswerProvider() *fakeProvider {
	provider := toolThenAnswerProvider(json.RawMessage(`{"query":"one"}`))
	provider.streams[0] = []fabricrunner.ModelEvent{
		{Type: fabricrunner.ModelEventStart, Sequence: 1},
		{Type: fabricrunner.ModelEventToolCall, Sequence: 2, ToolCall: &fabricrunner.ToolCall{ID: "call-1", Name: "lookup", Input: json.RawMessage(`{"query":"one"}`)}},
		{Type: fabricrunner.ModelEventToolCall, Sequence: 3, ToolCall: &fabricrunner.ToolCall{ID: "call-2", Name: "lookup", Input: json.RawMessage(`{"query":"two"}`)}},
		{Type: fabricrunner.ModelEventStop, Sequence: 4, Stop: fabricrunner.StopToolUse},
	}
	return provider
}

func successfulTool(context.Context, fabricrunner.ToolInvocation) (fabricrunner.ToolOutput, error) {
	return fabricrunner.ToolOutput{JSON: json.RawMessage(`{"value":"ok"}`), Classification: fabricrunner.ClassInternal}, nil
}

func modelRef() fabricrunner.ModelRef { return fabricrunner.ModelRef{Provider: "test", Model: "model"} }

type fakeSelector struct {
	selections []fabricrunner.TurnSelection
	calls      atomic.Int32
}

func (selector *fakeSelector) SelectTurn(_ context.Context, turn fabricrunner.TurnContext) (fabricrunner.TurnSelection, error) {
	index := int(selector.calls.Add(1)) - 1
	if len(turn.Messages) > 0 {
		turn.Messages[0].Content[0].Text = "mutated"
	}
	if index >= len(selector.selections) {
		return fabricrunner.TurnSelection{}, errors.New("unexpected selection")
	}
	return selector.selections[index], nil
}

type selectorFunc func(context.Context, fabricrunner.TurnContext) (fabricrunner.TurnSelection, error)

func (function selectorFunc) SelectTurn(ctx context.Context, turn fabricrunner.TurnContext) (fabricrunner.TurnSelection, error) {
	return function(ctx, turn)
}

type middlewareFunc func(context.Context, fabricrunner.ToolInvocation, fabricrunner.ToolOutput) (fabricrunner.ToolOutput, error)

func (function middlewareFunc) ProcessToolResult(
	ctx context.Context,
	invocation fabricrunner.ToolInvocation,
	output fabricrunner.ToolOutput,
) (fabricrunner.ToolOutput, error) {
	return function(ctx, invocation, output)
}

type fakeProvider struct {
	mu       sync.Mutex
	name     string
	streams  [][]fabricrunner.ModelEvent
	requests []fabricrunner.ModelRequest
}

func (provider *fakeProvider) Name() string { return provider.name }
func (provider *fakeProvider) Models(context.Context) ([]fabricrunner.ModelDescriptor, error) {
	return nil, nil
}
func (provider *fakeProvider) Stream(_ context.Context, request fabricrunner.ModelRequest) (fabricrunner.ModelStream, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.requests = append(provider.requests, fabricrunner.CloneModelRequest(request))
	request.Messages[0].Content[0].Text = "provider mutation"
	index := len(provider.requests) - 1
	if index >= len(provider.streams) {
		return nil, errors.New("unexpected stream")
	}
	return &sliceStream{events: provider.streams[index]}, nil
}

type sliceStream struct {
	events     []fabricrunner.ModelEvent
	index      int
	closeCount atomic.Int32
}

func (stream *sliceStream) Recv(context.Context) (fabricrunner.ModelEvent, error) {
	if stream.index >= len(stream.events) {
		return fabricrunner.ModelEvent{}, io.EOF
	}
	event := stream.events[stream.index]
	stream.index++
	return event, nil
}
func (stream *sliceStream) Close() error { stream.closeCount.Add(1); return nil }

type blockingProvider struct{ stream *blockingStream }

func (*blockingProvider) Name() string { return "blocking" }
func (*blockingProvider) Models(context.Context) ([]fabricrunner.ModelDescriptor, error) {
	return nil, nil
}
func (provider *blockingProvider) Stream(context.Context, fabricrunner.ModelRequest) (fabricrunner.ModelStream, error) {
	return provider.stream, nil
}

type blockingStream struct {
	entered    chan struct{}
	once       sync.Once
	closeCount atomic.Int32
}

func (stream *blockingStream) Recv(ctx context.Context) (fabricrunner.ModelEvent, error) {
	stream.once.Do(func() { close(stream.entered) })
	<-ctx.Done()
	return fabricrunner.ModelEvent{}, ctx.Err()
}
func (stream *blockingStream) Close() error { stream.closeCount.Add(1); return nil }

type fakeTool struct {
	execute      func(context.Context, fabricrunner.ToolInvocation) (fabricrunner.ToolOutput, error)
	close        func(context.Context) error
	executeCount atomic.Int32
	closeCount   atomic.Int32
}

func (tool *fakeTool) Execute(ctx context.Context, invocation fabricrunner.ToolInvocation) (fabricrunner.ToolOutput, error) {
	tool.executeCount.Add(1)
	if tool.execute == nil {
		return successfulTool(ctx, invocation)
	}
	return tool.execute(ctx, invocation)
}
func (tool *fakeTool) Close(ctx context.Context) error {
	tool.closeCount.Add(1)
	if tool.close != nil {
		return tool.close(ctx)
	}
	return nil
}

type overlapTool struct {
	active   atomic.Int32
	maximum  atomic.Int32
	closeCnt atomic.Int32
}

func (tool *overlapTool) Execute(context.Context, fabricrunner.ToolInvocation) (fabricrunner.ToolOutput, error) {
	active := tool.active.Add(1)
	for {
		maximum := tool.maximum.Load()
		if active <= maximum || tool.maximum.CompareAndSwap(maximum, active) {
			break
		}
	}
	time.Sleep(20 * time.Millisecond)
	tool.active.Add(-1)
	return fabricrunner.ToolOutput{JSON: json.RawMessage(`{"value":"ok"}`), Classification: fabricrunner.ClassInternal}, nil
}
func (tool *overlapTool) Close(context.Context) error { tool.closeCnt.Add(1); return nil }

type recordingSink struct {
	mu      sync.Mutex
	events  []fabricrunner.LoopEvent
	failAt  int
	failure error
	mutate  bool
}

func (sink *recordingSink) RecordLoopEvent(_ context.Context, event fabricrunner.LoopEvent) error {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if sink.failAt > 0 && int(event.Sequence) == sink.failAt {
		return sink.failure
	}
	sink.events = append(sink.events, event.Clone())
	if sink.mutate && event.Message != nil && len(event.Message.Content) > 0 {
		event.Message.Content[0].Text = "sink mutation"
	}
	return nil
}

func testID(t *testing.T) fabricrunner.ID {
	t.Helper()
	id, err := fabricrunner.NewID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
