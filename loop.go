package fabricrunner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type TurnContext struct {
	WorkloadID ID
	StepID     ID
	AttemptID  ID
	Turn       int
	Messages   []Message
	Usage      Usage
}

type TurnSelection struct {
	Provider Provider
	Model    ModelRef
}

type TurnSelector interface {
	SelectTurn(context.Context, TurnContext) (TurnSelection, error)
}

type ToolInvocation struct {
	WorkloadID ID
	StepID     ID
	AttemptID  ID
	Turn       int
	Call       ToolCall
}

type ToolOutput struct {
	JSON           json.RawMessage
	Content        []ContentPart
	Classification Classification
	IsError        bool
}

func (output ToolOutput) Validate() error {
	if err := output.Classification.Validate(); err != nil {
		return err
	}
	if len(output.JSON) == 0 && len(output.Content) == 0 {
		return errors.New("tool output requires JSON or content")
	}
	if len(output.JSON) > 0 && !json.Valid(output.JSON) {
		return errors.New("tool output JSON is invalid")
	}
	for index, part := range output.Content {
		if err := part.Validate(); err != nil {
			return fmt.Errorf("tool output content %d: %w", index, err)
		}
		switch part.Type {
		case ContentText, ContentImage, ContentDocument, ContentArtifact:
		default:
			return fmt.Errorf("tool output content %d has unsupported type %q", index, part.Type)
		}
	}
	return nil
}

func (output ToolOutput) Clone() ToolOutput {
	output.JSON = append(json.RawMessage(nil), output.JSON...)
	output.Content = CloneContentParts(output.Content)
	return output
}

type ToolHandler interface {
	Execute(context.Context, ToolInvocation) (ToolOutput, error)
	Close(context.Context) error
}

type ToolBinding struct {
	Definition   ToolDefinition
	Handler      ToolHandler
	ParallelSafe bool
}

type ToolResultMiddleware interface {
	ProcessToolResult(context.Context, ToolInvocation, ToolOutput) (ToolOutput, error)
}

type LoopEventType string

const (
	LoopEventTurnStarted     LoopEventType = "turn_started"
	LoopEventModel           LoopEventType = "model_event"
	LoopEventMessageAppended LoopEventType = "message_appended"
	LoopEventToolStarted     LoopEventType = "tool_started"
	LoopEventToolCompleted   LoopEventType = "tool_completed"
	LoopEventStopped         LoopEventType = "loop_stopped"
)

type LoopEvent struct {
	Sequence    uint64
	Type        LoopEventType
	Turn        int
	Model       ModelRef
	ModelEvent  *ModelEvent
	Message     *Message
	ToolCall    *ToolCall
	ToolOutput  *ToolOutput
	FailureCode string
	Stop        StopReason
}

func (event LoopEvent) Clone() LoopEvent {
	if event.ModelEvent != nil {
		modelEvent := event.ModelEvent.Clone()
		event.ModelEvent = &modelEvent
	}
	if event.Message != nil {
		message := Message{Role: event.Message.Role, Content: CloneContentParts(event.Message.Content)}
		event.Message = &message
	}
	if event.ToolCall != nil {
		call := cloneToolCall(*event.ToolCall)
		event.ToolCall = &call
	}
	if event.ToolOutput != nil {
		output := event.ToolOutput.Clone()
		event.ToolOutput = &output
	}
	return event
}

type LoopSink interface {
	RecordLoopEvent(context.Context, LoopEvent) error
}

type ToolFailure struct {
	CallID   string `json:"call_id"`
	ToolName string `json:"tool_name"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Cause    error  `json:"-"`
}

type PublicToolError interface {
	error
	ToolErrorCode() string
	ToolErrorMessage() string
}

type ToolError struct {
	Code    string
	Message string
	Cause   error
}

func (e *ToolError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return e.Message
}

func (e *ToolError) Unwrap() error            { return e.Cause }
func (e *ToolError) ToolErrorCode() string    { return e.Code }
func (e *ToolError) ToolErrorMessage() string { return e.Message }

type LoopRequest struct {
	WorkloadID                ID
	StepID                    ID
	AttemptID                 ID
	Initial                   ModelRequest
	Tools                     []ToolBinding
	Middleware                []ToolResultMiddleware
	Selector                  TurnSelector
	Sink                      LoopSink
	Budget                    Budget
	ParallelTools             bool
	ModelOutputClassification Classification
	ToolErrorClassification   Classification
}

func (request LoopRequest) Validate() error {
	for _, field := range []struct {
		name string
		id   ID
	}{
		{name: "workload ID", id: request.WorkloadID},
		{name: "step ID", id: request.StepID},
		{name: "attempt ID", id: request.AttemptID},
	} {
		if err := field.id.Validate(); err != nil {
			return fmt.Errorf("loop %s: %w", field.name, err)
		}
	}
	if request.Selector == nil {
		return errors.New("loop turn selector is required")
	}
	if err := request.Budget.Validate(); err != nil {
		return err
	}
	if request.Budget.MaxModelCalls <= 0 {
		return errors.New("loop model-call budget must be positive")
	}
	if request.Budget.MaxWallTime <= 0 {
		return errors.New("loop wall-time budget must be positive")
	}
	if err := request.ModelOutputClassification.Validate(); err != nil {
		return fmt.Errorf("model output classification: %w", err)
	}
	if err := request.ToolErrorClassification.Validate(); err != nil {
		return fmt.Errorf("tool error classification: %w", err)
	}
	definitions := make([]ToolDefinition, 0, len(request.Tools))
	seen := make(map[string]struct{}, len(request.Tools))
	for index, binding := range request.Tools {
		if binding.Handler == nil {
			return fmt.Errorf("loop tool %d handler is nil", index)
		}
		if err := binding.Definition.Validate(); err != nil {
			return fmt.Errorf("loop tool %d: %w", index, err)
		}
		if _, exists := seen[binding.Definition.Name]; exists {
			return fmt.Errorf("loop tool %d duplicates name %q", index, binding.Definition.Name)
		}
		seen[binding.Definition.Name] = struct{}{}
		definitions = append(definitions, cloneToolDefinition(binding.Definition))
	}
	for index, middleware := range request.Middleware {
		if middleware == nil {
			return fmt.Errorf("loop result middleware %d is nil", index)
		}
	}
	initial := CloneModelRequest(request.Initial)
	initial.Tools = definitions
	if err := initial.Validate(); err != nil {
		return fmt.Errorf("initial model request: %w", err)
	}
	return nil
}

func (request LoopRequest) Clone() LoopRequest {
	request.Initial = CloneModelRequest(request.Initial)
	request.Tools = append([]ToolBinding(nil), request.Tools...)
	for index := range request.Tools {
		request.Tools[index].Definition = cloneToolDefinition(request.Tools[index].Definition)
	}
	request.Middleware = append([]ToolResultMiddleware(nil), request.Middleware...)
	return request
}

type LoopResult struct {
	Messages       []Message
	Usage          Usage
	Stop           StopReason
	ModelCalls     int
	ToolCalls      int
	Failures       []ToolFailure
	BudgetExceeded string
}

func (result LoopResult) Clone() LoopResult {
	result.Messages = CloneMessages(result.Messages)
	result.Failures = append([]ToolFailure(nil), result.Failures...)
	return result
}

type TurnLoop interface {
	Run(context.Context, LoopRequest) (LoopResult, error)
}

var (
	ErrLoopBudgetExceeded = errors.New("loop budget exceeded")
	ErrLoopStream         = errors.New("invalid loop model stream")
	ErrLoopToolInput      = errors.New("invalid tool input")
	ErrLoopToolOutput     = errors.New("invalid tool output")
	ErrLoopPanic          = errors.New("loop collaborator panicked")
	ErrLoopCleanup        = errors.New("loop cleanup failed")
)

func NormalizeToolError(err error) (code, message string) {
	if err == nil {
		return "", ""
	}
	var publicError PublicToolError
	if errors.As(err, &publicError) {
		code = strings.TrimSpace(publicError.ToolErrorCode())
		message = strings.TrimSpace(publicError.ToolErrorMessage())
		if code != "" && message != "" {
			return code, message
		}
	}
	return "tool_execution_failed", "tool execution failed"
}

func CloneMessages(messages []Message) []Message {
	cloned := make([]Message, len(messages))
	for index, message := range messages {
		cloned[index] = Message{Role: message.Role, Content: CloneContentParts(message.Content)}
	}
	return cloned
}

func CloneContentParts(parts []ContentPart) []ContentPart {
	cloned := make([]ContentPart, len(parts))
	for index, part := range parts {
		cloned[index] = part
		cloned[index].JSON = append(json.RawMessage(nil), part.JSON...)
		if part.Extension != nil {
			cloned[index].Extension = make(map[string]json.RawMessage, len(part.Extension))
			for key, value := range part.Extension {
				cloned[index].Extension[key] = append(json.RawMessage(nil), value...)
			}
		}
	}
	return cloned
}

func CloneModelRequest(request ModelRequest) ModelRequest {
	request.Messages = CloneMessages(request.Messages)
	request.Tools = append([]ToolDefinition(nil), request.Tools...)
	for index := range request.Tools {
		request.Tools[index] = cloneToolDefinition(request.Tools[index])
	}
	if request.Output != nil {
		output := *request.Output
		output.Schema = append(json.RawMessage(nil), output.Schema...)
		request.Output = &output
	}
	request.Metadata = cloneStringMap(request.Metadata)
	if request.Extension != nil {
		request.Extension = make(map[string]json.RawMessage, len(request.Extension))
		for key, value := range request.Extension {
			request.Extension[key] = append(json.RawMessage(nil), value...)
		}
	}
	return request
}

func cloneToolDefinition(definition ToolDefinition) ToolDefinition {
	definition.InputSchema = append(json.RawMessage(nil), definition.InputSchema...)
	definition.OutputSchema = append(json.RawMessage(nil), definition.OutputSchema...)
	return definition
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func cloneToolCall(call ToolCall) ToolCall {
	call.Input = append(json.RawMessage(nil), call.Input...)
	return call
}
