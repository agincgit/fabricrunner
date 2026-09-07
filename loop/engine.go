// Package loop implements Fabric Runner's provider-neutral turn and tool loop.
package loop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/agincgit/fabricrunner"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const cleanupTimeout = 5 * time.Second

type Engine struct{}

type compiledTool struct {
	binding fabricrunner.ToolBinding
	input   *jsonschema.Schema
	output  *jsonschema.Schema
}

type toolExecution struct {
	output  fabricrunner.ToolOutput
	failure *fabricrunner.ToolFailure
}

type turnData struct {
	text     strings.Builder
	calls    []fabricrunner.ToolCall
	stop     fabricrunner.StopReason
	modelErr *fabricrunner.ModelError
}

func (Engine) Run(
	ctx context.Context,
	request fabricrunner.LoopRequest,
) (result fabricrunner.LoopResult, returnErr error) {
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := request.Validate(); err != nil {
		return result, err
	}
	request = request.Clone()
	result.Messages = fabricrunner.CloneMessages(request.Initial.Messages)

	loopCtx, cancel := context.WithTimeoutCause(ctx, request.Budget.MaxWallTime, fabricrunner.ErrLoopBudgetExceeded)
	defer cancel()
	emitter := loopEmitter{sink: request.Sink}
	currentTurn := 0
	currentModel := fabricrunner.ModelRef{}
	defer func() {
		if recovered := recover(); recovered != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("%w: %v\n%s", fabricrunner.ErrLoopPanic, recovered, debug.Stack()))
		}
		if cleanupErr := closeTools(request.Tools); cleanupErr != nil {
			returnErr = errors.Join(returnErr, cleanupErr)
		}
		if !emitter.failed {
			terminalCtx, terminalCancel := context.WithTimeout(context.Background(), cleanupTimeout)
			stop := result.Stop
			if errors.Is(returnErr, context.Canceled) {
				stop = fabricrunner.StopCancelled
			} else if returnErr != nil || stop == "" {
				stop = fabricrunner.StopUnknown
			}
			terminalErr := emitter.emit(terminalCtx, fabricrunner.LoopEvent{
				Type: fabricrunner.LoopEventStopped, Turn: currentTurn, Model: currentModel,
				Stop: stop, FailureCode: loopFailureCode(returnErr, result),
			})
			terminalCancel()
			returnErr = errors.Join(returnErr, terminalErr)
		}
	}()

	tools, err := compileTools(request.Tools)
	if err != nil {
		return result, err
	}
	toolCallIDs := make(map[string]struct{})
	for turn := 1; ; turn++ {
		currentTurn = turn
		if err := contextResult(loopCtx, ctx, &result); err != nil {
			return result, err
		}
		if result.ModelCalls >= request.Budget.MaxModelCalls {
			return budgetFailure(result, "model_calls")
		}
		selection, err := request.Selector.SelectTurn(loopCtx, fabricrunner.TurnContext{
			WorkloadID: request.WorkloadID,
			StepID:     request.StepID,
			AttemptID:  request.AttemptID,
			Turn:       turn,
			Messages:   fabricrunner.CloneMessages(result.Messages),
			Usage:      result.Usage,
		})
		if err != nil {
			if contextErr := contextResult(loopCtx, ctx, &result); contextErr != nil {
				return result, contextErr
			}
			return result, err
		}
		if selection.Provider == nil {
			return result, errors.New("turn selector returned a nil provider")
		}
		if err := selection.Model.Validate(); err != nil {
			return result, fmt.Errorf("selected model: %w", err)
		}
		if strings.TrimSpace(selection.Provider.Name()) == "" {
			return result, errors.New("selected provider name is empty")
		}
		currentModel = selection.Model
		if err := emitter.emit(loopCtx, fabricrunner.LoopEvent{
			Type: fabricrunner.LoopEventTurnStarted, Turn: turn, Model: selection.Model,
		}); err != nil {
			return result, err
		}

		modelRequest := fabricrunner.CloneModelRequest(request.Initial)
		modelRequest.Model = selection.Model
		modelRequest.Messages = fabricrunner.CloneMessages(result.Messages)
		modelRequest.Tools = toolDefinitions(request.Tools)
		result.ModelCalls++
		stream, err := selection.Provider.Stream(loopCtx, modelRequest)
		if err != nil {
			if contextErr := contextResult(loopCtx, ctx, &result); contextErr != nil {
				return result, contextErr
			}
			return result, err
		}
		data, err := consumeTurn(loopCtx, ctx, stream, turn, selection.Model, &emitter, &result, request.Budget)
		if err != nil {
			return result, err
		}
		assistant, err := assistantMessage(data, request.ModelOutputClassification, toolCallIDs)
		if err != nil {
			return result, err
		}
		if assistant != nil {
			result.Messages = append(result.Messages, *assistant)
			if err := emitter.emit(loopCtx, fabricrunner.LoopEvent{
				Type: fabricrunner.LoopEventMessageAppended, Turn: turn, Model: selection.Model, Message: assistant,
			}); err != nil {
				return result, err
			}
		}
		if data.modelErr != nil {
			return result, fmt.Errorf("model %s: %s", data.modelErr.Code, data.modelErr.Message)
		}
		result.Stop = data.stop
		if data.stop != fabricrunner.StopToolUse {
			return result, nil
		}
		if len(data.calls) == 0 {
			return result, fmt.Errorf("%w: tool-use stop contained no completed tool call", fabricrunner.ErrLoopStream)
		}
		if result.ToolCalls+len(data.calls) > request.Budget.MaxToolCalls {
			return budgetFailure(result, "tool_calls")
		}
		result.ToolCalls += len(data.calls)
		for index := range data.calls {
			call := data.calls[index]
			if err := emitter.emit(loopCtx, fabricrunner.LoopEvent{
				Type: fabricrunner.LoopEventToolStarted, Turn: turn, Model: selection.Model, ToolCall: &call,
			}); err != nil {
				return result, err
			}
		}
		executions, batchErr := executeBatch(loopCtx, ctx, &result, request, turn, data.calls, tools)
		toolMessage := fabricrunner.Message{Role: fabricrunner.RoleTool}
		for index, execution := range executions {
			call := data.calls[index]
			if execution.failure != nil {
				result.Failures = append(result.Failures, *execution.failure)
			}
			parts := outputParts(call, execution.output)
			toolMessage.Content = append(toolMessage.Content, parts...)
			event := fabricrunner.LoopEvent{
				Type: fabricrunner.LoopEventToolCompleted, Turn: turn, Model: selection.Model,
				ToolCall: &call, ToolOutput: &execution.output,
			}
			if execution.failure != nil {
				event.FailureCode = execution.failure.Code
			}
			if err := emitter.emit(loopCtx, event); err != nil {
				return result, err
			}
		}
		if batchErr != nil {
			return result, batchErr
		}
		if err := toolMessage.Validate(); err != nil {
			return result, fmt.Errorf("assembled tool message: %w", err)
		}
		result.Messages = append(result.Messages, toolMessage)
		if err := emitter.emit(loopCtx, fabricrunner.LoopEvent{
			Type: fabricrunner.LoopEventMessageAppended, Turn: turn, Model: selection.Model, Message: &toolMessage,
		}); err != nil {
			return result, err
		}
	}
}

func consumeTurn(
	loopCtx, parentCtx context.Context,
	stream fabricrunner.ModelStream,
	turn int,
	model fabricrunner.ModelRef,
	emitter *loopEmitter,
	result *fabricrunner.LoopResult,
	budget fabricrunner.Budget,
) (data turnData, returnErr error) {
	if stream == nil {
		return data, errors.New("provider returned a nil model stream")
	}
	defer func() {
		returnErr = errors.Join(returnErr, stream.Close())
	}()
	validator := fabricrunner.ModelStreamValidator{}
	for {
		if err := contextResult(loopCtx, parentCtx, result); err != nil {
			return data, err
		}
		event, err := stream.Recv(loopCtx)
		if errors.Is(err, io.EOF) {
			if err := validator.Complete(); err != nil {
				return data, fmt.Errorf("%w: %w", fabricrunner.ErrLoopStream, err)
			}
			return data, nil
		}
		if err != nil {
			if contextErr := contextResult(loopCtx, parentCtx, result); contextErr != nil {
				return data, contextErr
			}
			return data, err
		}
		if err := validator.Accept(event); err != nil {
			return data, fmt.Errorf("%w: %w", fabricrunner.ErrLoopStream, err)
		}
		if err := emitter.emit(loopCtx, fabricrunner.LoopEvent{
			Type: fabricrunner.LoopEventModel, Turn: turn, Model: model, ModelEvent: &event,
		}); err != nil {
			return data, err
		}
		switch event.Type {
		case fabricrunner.ModelEventTextDelta:
			data.text.WriteString(event.Text)
		case fabricrunner.ModelEventToolCall:
			data.calls = append(data.calls, cloneCall(*event.ToolCall))
		case fabricrunner.ModelEventUsage:
			if err := addUsage(&result.Usage, event.Usage); err != nil {
				return data, err
			}
			if kind := exceededUsage(result.Usage, budget); kind != "" {
				result.BudgetExceeded = kind
				return data, fmt.Errorf("%w: %s", fabricrunner.ErrLoopBudgetExceeded, kind)
			}
		case fabricrunner.ModelEventStop:
			data.stop = event.Stop
		case fabricrunner.ModelEventError:
			modelError := *event.Error
			data.modelErr = &modelError
		}
	}
}

func assistantMessage(
	data turnData,
	classification fabricrunner.Classification,
	seen map[string]struct{},
) (*fabricrunner.Message, error) {
	message := fabricrunner.Message{Role: fabricrunner.RoleAssistant}
	if data.text.Len() > 0 {
		message.Content = append(message.Content, fabricrunner.ContentPart{
			Type: fabricrunner.ContentText, Classification: classification, Text: data.text.String(),
		})
	}
	for _, call := range data.calls {
		if _, exists := seen[call.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate tool-call ID %q", fabricrunner.ErrLoopStream, call.ID)
		}
		seen[call.ID] = struct{}{}
		message.Content = append(message.Content, fabricrunner.ContentPart{
			Type: fabricrunner.ContentToolCall, Classification: classification,
			ToolCallID: call.ID, ToolName: call.Name, JSON: append(json.RawMessage(nil), call.Input...),
		})
	}
	if len(message.Content) == 0 {
		return nil, nil
	}
	if err := message.Validate(); err != nil {
		return nil, err
	}
	return &message, nil
}

func executeBatch(
	ctx context.Context,
	parentCtx context.Context,
	loopResult *fabricrunner.LoopResult,
	request fabricrunner.LoopRequest,
	turn int,
	calls []fabricrunner.ToolCall,
	tools map[string]compiledTool,
) ([]toolExecution, error) {
	parallel := request.ParallelTools
	for _, call := range calls {
		tool, exists := tools[call.Name]
		parallel = parallel && exists && tool.binding.ParallelSafe
	}
	if !parallel {
		results := make([]toolExecution, 0, len(calls))
		for _, call := range calls {
			if err := contextResult(ctx, parentCtx, loopResult); err != nil {
				return results, err
			}
			execution := executeTool(ctx, request, turn, call, tools)
			if execution.failure == nil {
				execution = finalizeToolResult(ctx, request, turn, call, tools, execution.output)
			}
			results = append(results, execution)
		}
		return results, nil
	}
	results := make([]toolExecution, len(calls))
	var group sync.WaitGroup
	group.Add(len(calls))
	for index, call := range calls {
		go func() {
			defer group.Done()
			results[index] = executeTool(ctx, request, turn, call, tools)
		}()
	}
	group.Wait()
	if err := contextResult(ctx, parentCtx, loopResult); err != nil {
		return results, err
	}
	finalized := make([]toolExecution, 0, len(results))
	for index, execution := range results {
		if err := contextResult(ctx, parentCtx, loopResult); err != nil {
			return finalized, err
		}
		if execution.failure == nil {
			execution = finalizeToolResult(ctx, request, turn, calls[index], tools, execution.output)
		}
		finalized = append(finalized, execution)
	}
	return finalized, nil
}

func executeTool(
	ctx context.Context,
	request fabricrunner.LoopRequest,
	turn int,
	call fabricrunner.ToolCall,
	tools map[string]compiledTool,
) (execution toolExecution) {
	invocation := fabricrunner.ToolInvocation{
		WorkloadID: request.WorkloadID, StepID: request.StepID, AttemptID: request.AttemptID,
		Turn: turn, Call: cloneCall(call),
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			execution = failedExecution(request, invocation,
				"tool_panicked", "tool execution panicked",
				fmt.Errorf("%w: %v\n%s", fabricrunner.ErrLoopPanic, recovered, debug.Stack()))
		}
	}()
	tool, exists := tools[call.Name]
	if !exists {
		return failedExecution(request, invocation, "tool_not_found", "requested tool is not declared",
			fmt.Errorf("%w: undeclared tool %q", fabricrunner.ErrLoopToolInput, call.Name))
	}
	if err := validateJSON(tool.input, call.Input); err != nil {
		return failedExecution(request, invocation, "tool_input_invalid", "tool input failed schema validation",
			fmt.Errorf("%w: %w", fabricrunner.ErrLoopToolInput, err))
	}
	output, err := tool.binding.Handler.Execute(ctx, invocation)
	if err != nil {
		code, message := fabricrunner.NormalizeToolError(err)
		return failedExecution(request, invocation, code, message, err)
	}
	execution.output = output.Clone()
	return execution
}

func finalizeToolResult(
	ctx context.Context,
	request fabricrunner.LoopRequest,
	turn int,
	call fabricrunner.ToolCall,
	tools map[string]compiledTool,
	output fabricrunner.ToolOutput,
) (execution toolExecution) {
	invocation := fabricrunner.ToolInvocation{
		WorkloadID: request.WorkloadID, StepID: request.StepID, AttemptID: request.AttemptID,
		Turn: turn, Call: cloneCall(call),
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			execution = failedExecution(request, invocation,
				"tool_result_middleware_panicked", "tool result processing panicked",
				fmt.Errorf("%w: %v\n%s", fabricrunner.ErrLoopPanic, recovered, debug.Stack()))
		}
	}()
	var err error
	for _, middleware := range request.Middleware {
		output, err = middleware.ProcessToolResult(ctx, invocation, output.Clone())
		if err != nil {
			return failedExecution(request, invocation, "tool_result_middleware_failed", "tool result processing failed", err)
		}
	}
	if err := output.Validate(); err != nil {
		return failedExecution(request, invocation, "tool_output_invalid", "tool output is invalid",
			fmt.Errorf("%w: %w", fabricrunner.ErrLoopToolOutput, err))
	}
	tool := tools[call.Name]
	if tool.output != nil {
		if len(output.JSON) == 0 {
			return failedExecution(request, invocation, "tool_output_invalid", "tool output JSON is required",
				fmt.Errorf("%w: output schema requires JSON", fabricrunner.ErrLoopToolOutput))
		}
		if err := validateJSON(tool.output, output.JSON); err != nil {
			return failedExecution(request, invocation, "tool_output_invalid", "tool output failed schema validation",
				fmt.Errorf("%w: %w", fabricrunner.ErrLoopToolOutput, err))
		}
	}
	execution.output = output.Clone()
	return execution
}

func failedExecution(
	request fabricrunner.LoopRequest,
	invocation fabricrunner.ToolInvocation,
	code, message string,
	cause error,
) toolExecution {
	payload, _ := json.Marshal(map[string]any{"error": map[string]string{"code": code, "message": message}})
	return toolExecution{
		output: fabricrunner.ToolOutput{JSON: payload, Classification: request.ToolErrorClassification, IsError: true},
		failure: &fabricrunner.ToolFailure{
			CallID: invocation.Call.ID, ToolName: invocation.Call.Name,
			Code: code, Message: message, Cause: cause,
		},
	}
}

func outputParts(call fabricrunner.ToolCall, output fabricrunner.ToolOutput) []fabricrunner.ContentPart {
	parts := make([]fabricrunner.ContentPart, 0, len(output.Content)+1)
	if len(output.JSON) > 0 {
		parts = append(parts, fabricrunner.ContentPart{
			Type: fabricrunner.ContentToolResult, Classification: output.Classification,
			ToolCallID: call.ID, ToolName: call.Name, JSON: append(json.RawMessage(nil), output.JSON...), IsError: output.IsError,
		})
	}
	for _, content := range fabricrunner.CloneContentParts(output.Content) {
		content.ToolCallID = call.ID
		content.ToolName = call.Name
		parts = append(parts, content)
	}
	return parts
}

func compileTools(bindings []fabricrunner.ToolBinding) (map[string]compiledTool, error) {
	result := make(map[string]compiledTool, len(bindings))
	for index, binding := range bindings {
		input, err := compileSchema(fmt.Sprintf("urn:fabricrunner:tool:%d:input", index), binding.Definition.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("tool %q input schema: %w", binding.Definition.Name, err)
		}
		var output *jsonschema.Schema
		if len(binding.Definition.OutputSchema) > 0 {
			output, err = compileSchema(fmt.Sprintf("urn:fabricrunner:tool:%d:output", index), binding.Definition.OutputSchema)
			if err != nil {
				return nil, fmt.Errorf("tool %q output schema: %w", binding.Definition.Name, err)
			}
		}
		result[binding.Definition.Name] = compiledTool{binding: binding, input: input, output: output}
	}
	return result, nil
}

func compileSchema(location string, raw json.RawMessage) (*jsonschema.Schema, error) {
	var document any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.UseLoader(deniedLoader{})
	if err := compiler.AddResource(location, document); err != nil {
		return nil, err
	}
	return compiler.Compile(location)
}

type deniedLoader struct{}

func (deniedLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("external schema reference %q is disabled", url)
}

func validateJSON(schema *jsonschema.Schema, raw json.RawMessage) error {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	return schema.Validate(value)
}

func addUsage(total *fabricrunner.Usage, usage *fabricrunner.Usage) error {
	if usage == nil {
		return errors.New("usage event has no usage")
	}
	if usage.InputTokens < 0 || usage.OutputTokens < 0 || usage.CacheReadTokens < 0 ||
		usage.CacheWriteTokens < 0 || usage.Cost < 0 {
		return errors.New("usage values cannot be negative")
	}
	total.InputTokens += usage.InputTokens
	total.OutputTokens += usage.OutputTokens
	total.CacheReadTokens += usage.CacheReadTokens
	total.CacheWriteTokens += usage.CacheWriteTokens
	total.Cost += usage.Cost
	if total.InputTokens < 0 || total.OutputTokens < 0 || total.CacheReadTokens < 0 ||
		total.CacheWriteTokens < 0 || total.Cost < 0 {
		return errors.New("usage values overflowed")
	}
	return nil
}

func exceededUsage(usage fabricrunner.Usage, budget fabricrunner.Budget) string {
	if usage.InputTokens > budget.MaxInputTokens {
		return "input_tokens"
	}
	if usage.OutputTokens > budget.MaxOutputTokens {
		return "output_tokens"
	}
	if usage.Cost > budget.MaxCost {
		return "cost"
	}
	return ""
}

func budgetFailure(result fabricrunner.LoopResult, kind string) (fabricrunner.LoopResult, error) {
	result.BudgetExceeded = kind
	return result, fmt.Errorf("%w: %s", fabricrunner.ErrLoopBudgetExceeded, kind)
}

func contextResult(loopCtx, parentCtx context.Context, result *fabricrunner.LoopResult) error {
	if err := loopCtx.Err(); err == nil {
		return nil
	}
	if errors.Is(context.Cause(loopCtx), fabricrunner.ErrLoopBudgetExceeded) && parentCtx.Err() == nil {
		result.BudgetExceeded = "wall_time"
		return fmt.Errorf("%w: wall_time", fabricrunner.ErrLoopBudgetExceeded)
	}
	return loopCtx.Err()
}

func closeTools(bindings []fabricrunner.ToolBinding) error {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	var result error
	for index := len(bindings) - 1; index >= 0; index-- {
		if err := closeTool(ctx, bindings[index]); err != nil {
			result = errors.Join(result, err)
		}
	}
	return result
}

func closeTool(ctx context.Context, binding fabricrunner.ToolBinding) (returnErr error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			returnErr = fmt.Errorf("%w: close tool %q panicked: %v", fabricrunner.ErrLoopCleanup, binding.Definition.Name, recovered)
		}
	}()
	if err := binding.Handler.Close(ctx); err != nil {
		return fmt.Errorf("%w: close tool %q: %w", fabricrunner.ErrLoopCleanup, binding.Definition.Name, err)
	}
	return nil
}

func toolDefinitions(bindings []fabricrunner.ToolBinding) []fabricrunner.ToolDefinition {
	definitions := make([]fabricrunner.ToolDefinition, len(bindings))
	for index, binding := range bindings {
		definitions[index] = binding.Definition
		definitions[index].InputSchema = append(json.RawMessage(nil), binding.Definition.InputSchema...)
		definitions[index].OutputSchema = append(json.RawMessage(nil), binding.Definition.OutputSchema...)
	}
	return definitions
}

func cloneCall(call fabricrunner.ToolCall) fabricrunner.ToolCall {
	call.Input = append(json.RawMessage(nil), call.Input...)
	return call
}

type loopEmitter struct {
	sink     fabricrunner.LoopSink
	sequence uint64
	failed   bool
}

func (emitter *loopEmitter) emit(ctx context.Context, event fabricrunner.LoopEvent) error {
	emitter.sequence++
	event.Sequence = emitter.sequence
	if emitter.sink == nil {
		return nil
	}
	if err := emitter.sink.RecordLoopEvent(ctx, event.Clone()); err != nil {
		emitter.failed = true
		return fmt.Errorf("loop sink: %w", err)
	}
	return nil
}

func loopFailureCode(err error, result fabricrunner.LoopResult) string {
	if err == nil {
		return ""
	}
	if result.BudgetExceeded != "" {
		return "budget_" + result.BudgetExceeded
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline_exceeded"
	}
	if errors.Is(err, fabricrunner.ErrLoopPanic) {
		return "panic"
	}
	return "failed"
}

var _ fabricrunner.TurnLoop = Engine{}
