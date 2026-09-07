package fabricrunner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

type ContentType string

const (
	ContentText       ContentType = "text"
	ContentImage      ContentType = "image"
	ContentDocument   ContentType = "document"
	ContentArtifact   ContentType = "artifact"
	ContentToolCall   ContentType = "tool_call"
	ContentToolResult ContentType = "tool_result"
)

// ContentPart is a provider-neutral message block. Provider-specific fields
// may be retained in Extension, but core policy cannot depend on them.
type ContentPart struct {
	Type           ContentType
	Classification Classification
	Text           string
	MediaType      string
	ArtifactID     ID
	ToolCallID     string
	ToolName       string
	JSON           json.RawMessage
	IsError        bool
	Extension      map[string]json.RawMessage
}

type Message struct {
	Role    MessageRole
	Content []ContentPart
}

func (m Message) Validate() error {
	switch m.Role {
	case RoleSystem, RoleUser, RoleAssistant, RoleTool:
	default:
		return fmt.Errorf("unknown message role %q", m.Role)
	}
	if len(m.Content) == 0 {
		return errors.New("message content is required")
	}
	for i, part := range m.Content {
		if err := part.Validate(); err != nil {
			return fmt.Errorf("content part %d: %w", i, err)
		}
	}
	return nil
}

func (p ContentPart) Validate() error {
	if err := p.Classification.Validate(); err != nil {
		return err
	}
	switch p.Type {
	case ContentText:
		return nil
	case ContentImage, ContentDocument, ContentArtifact:
		if err := p.ArtifactID.Validate(); err != nil {
			return fmt.Errorf("artifact ID: %w", err)
		}
		return nil
	case ContentToolCall:
		if p.ToolCallID == "" || p.ToolName == "" {
			return errors.New("tool call ID and name are required")
		}
		if len(p.JSON) == 0 || !json.Valid(p.JSON) {
			return errors.New("tool call input must be valid JSON")
		}
		return nil
	case ContentToolResult:
		if p.ToolCallID == "" {
			return errors.New("tool result call ID is required")
		}
		return nil
	default:
		return fmt.Errorf("unknown content type %q", p.Type)
	}
}

type ToolDefinition struct {
	Name         string
	Description  string
	InputSchema  json.RawMessage
	OutputSchema json.RawMessage
	Strict       bool
}

func (t ToolDefinition) Validate() error {
	if t.Name == "" {
		return errors.New("tool name is required")
	}
	if len(t.InputSchema) == 0 || !json.Valid(t.InputSchema) {
		return fmt.Errorf("tool %q input schema must be valid JSON", t.Name)
	}
	if len(t.OutputSchema) > 0 && !json.Valid(t.OutputSchema) {
		return fmt.Errorf("tool %q output schema must be valid JSON", t.Name)
	}
	return nil
}

type ModelRef struct {
	Provider string
	Model    string
}

func (r ModelRef) Validate() error {
	if r.Provider == "" {
		return errors.New("provider is required")
	}
	if r.Model == "" {
		return errors.New("model is required")
	}
	return nil
}

type ToolChoiceMode string

const (
	ToolChoiceAuto     ToolChoiceMode = "auto"
	ToolChoiceNone     ToolChoiceMode = "none"
	ToolChoiceRequired ToolChoiceMode = "required"
	ToolChoiceNamed    ToolChoiceMode = "named"
)

type ToolChoice struct {
	Mode ToolChoiceMode
	Name string
}

type OutputConstraint struct {
	Name   string
	Schema json.RawMessage
	Strict bool
}

type ModelRequest struct {
	Model           ModelRef
	Messages        []Message
	Tools           []ToolDefinition
	ToolChoice      ToolChoice
	Output          *OutputConstraint
	MaxOutputTokens int
	Metadata        map[string]string
	Extension       map[string]json.RawMessage
}

func (r ModelRequest) Validate() error {
	if err := r.Model.Validate(); err != nil {
		return err
	}
	if len(r.Messages) == 0 {
		return errors.New("at least one message is required")
	}
	if r.MaxOutputTokens < 0 {
		return errors.New("maximum output tokens cannot be negative")
	}
	for i, message := range r.Messages {
		if err := message.Validate(); err != nil {
			return fmt.Errorf("message %d: %w", i, err)
		}
	}
	toolNames := make(map[string]struct{}, len(r.Tools))
	for i, tool := range r.Tools {
		if err := tool.Validate(); err != nil {
			return fmt.Errorf("tool %d: %w", i, err)
		}
		if _, exists := toolNames[tool.Name]; exists {
			return fmt.Errorf("duplicate tool name %q", tool.Name)
		}
		toolNames[tool.Name] = struct{}{}
	}
	if r.Output != nil && (len(r.Output.Schema) == 0 || !json.Valid(r.Output.Schema)) {
		return errors.New("output constraint schema must be valid JSON")
	}
	if r.ToolChoice.Mode == ToolChoiceNamed && r.ToolChoice.Name == "" {
		return errors.New("named tool choice requires a name")
	}
	if r.ToolChoice.Mode == ToolChoiceNamed {
		if _, exists := toolNames[r.ToolChoice.Name]; !exists {
			return fmt.Errorf("named tool choice %q is not defined", r.ToolChoice.Name)
		}
	}
	return nil
}

type ToolUseMode string

const (
	ToolUseNone        ToolUseMode = "none"
	ToolUseNative      ToolUseMode = "native"
	ToolUseConstrained ToolUseMode = "constrained"
	ToolUseParsed      ToolUseMode = "parsed"
)

// ModelCapabilities drives eligibility. Values may come from configuration or
// discovery and should be tracked separately from conformance observations.
type ModelCapabilities struct {
	ContextTokens         int
	MaxOutputTokens       int
	Modalities            []string
	ToolUse               ToolUseMode
	ParallelToolCalls     bool
	StrictToolSchemas     bool
	DynamicTools          bool
	MultimodalToolResults bool
	Streaming             bool
	StructuredOutput      bool
	PromptCaching         bool
	AutomaticCompaction   bool
	ReasoningSummaries    bool
}

type ModelDescriptor struct {
	Ref          ModelRef
	Capabilities ModelCapabilities
	Labels       map[string]string
}

type ModelEventType string

const (
	ModelEventStart         ModelEventType = "start"
	ModelEventTextDelta     ModelEventType = "text_delta"
	ModelEventToolCallDelta ModelEventType = "tool_call_delta"
	ModelEventToolCall      ModelEventType = "tool_call"
	ModelEventUsage         ModelEventType = "usage"
	ModelEventStop          ModelEventType = "stop"
	ModelEventError         ModelEventType = "error"
)

type ToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

type Usage struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	Cost             CostMicros
}

type StopReason string

const (
	StopEndTurn   StopReason = "end_turn"
	StopToolUse   StopReason = "tool_use"
	StopMaxTokens StopReason = "max_tokens"
	StopRefusal   StopReason = "refusal"
	StopCancelled StopReason = "cancelled"
	StopUnknown   StopReason = "unknown"
)

type ModelError struct {
	Code          string
	Message       string
	Retryable     bool
	SafeToReroute bool
}

type ModelEvent struct {
	Type      ModelEventType
	Sequence  uint64
	Text      string
	ToolCall  *ToolCall
	Usage     *Usage
	Stop      StopReason
	Error     *ModelError
	Extension map[string]json.RawMessage
}

func (e ModelEvent) Validate() error {
	if e.Sequence == 0 {
		return errors.New("model event sequence must be positive")
	}
	switch e.Type {
	case ModelEventStart, ModelEventTextDelta, ModelEventToolCallDelta, ModelEventUsage:
		return nil
	case ModelEventToolCall:
		if e.ToolCall == nil || e.ToolCall.ID == "" || e.ToolCall.Name == "" ||
			len(e.ToolCall.Input) == 0 || !json.Valid(e.ToolCall.Input) {
			return errors.New("completed tool-call event is invalid")
		}
		return nil
	case ModelEventStop:
		if e.Stop == "" {
			return errors.New("stop event requires a reason")
		}
		return nil
	case ModelEventError:
		if e.Error == nil || e.Error.Code == "" {
			return errors.New("error event requires an error code")
		}
		return nil
	default:
		return fmt.Errorf("unknown model event type %q", e.Type)
	}
}

// ModelStream emits ordered events and ends with io.EOF after one terminal
// stop or error event. Close must cancel the underlying provider operation.
type ModelStream interface {
	Recv(ctx context.Context) (ModelEvent, error)
	Close() error
}

// Provider translates one bounded model turn. It does not run tools or own an
// agent loop.
type Provider interface {
	Name() string
	Models(ctx context.Context) ([]ModelDescriptor, error)
	Stream(ctx context.Context, request ModelRequest) (ModelStream, error)
}

// DrainStream consumes a stream while preserving terminal-event validation.
// It is primarily useful to adapter conformance tests and non-streaming clients.
func DrainStream(ctx context.Context, stream ModelStream) ([]ModelEvent, error) {
	if stream == nil {
		return nil, errors.New("model stream is nil")
	}
	defer stream.Close()

	var events []ModelEvent
	var lastSequence uint64
	terminal := false
	for {
		event, err := stream.Recv(ctx)
		if errors.Is(err, io.EOF) {
			if !terminal {
				return nil, errors.New("model stream ended without a terminal event")
			}
			return events, nil
		}
		if err != nil {
			return nil, err
		}
		if terminal {
			return nil, errors.New("model stream emitted an event after its terminal event")
		}
		if err := event.Validate(); err != nil {
			return nil, fmt.Errorf("invalid model event: %w", err)
		}
		if event.Sequence != lastSequence+1 {
			return nil, fmt.Errorf("model stream sequence %d followed %d", event.Sequence, lastSequence)
		}
		lastSequence = event.Sequence
		events = append(events, event)
		terminal = event.Type == ModelEventStop || event.Type == ModelEventError
	}
}
