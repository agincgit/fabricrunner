package fabricrunner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

func (mode ToolUseMode) Validate() error {
	switch mode {
	case ToolUseNone, ToolUseNative, ToolUseConstrained, ToolUseParsed:
		return nil
	default:
		return fmt.Errorf("unknown tool-use mode %q", mode)
	}
}

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
		if len(p.JSON) > 0 && !json.Valid(p.JSON) {
			return errors.New("tool result JSON must be valid")
		}
		return nil
	default:
		return fmt.Errorf("unknown content type %q", p.Type)
	}
}

type ToolDefinition struct {
	SideEffecting bool
	Name          string
	Description   string
	InputSchema   json.RawMessage
	OutputSchema  json.RawMessage
	Strict        bool
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
	if strings.TrimSpace(r.Provider) == "" || r.Provider != strings.TrimSpace(r.Provider) {
		return errors.New("provider is required without surrounding whitespace")
	}
	if strings.TrimSpace(r.Model) == "" || r.Model != strings.TrimSpace(r.Model) {
		return errors.New("model is required without surrounding whitespace")
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
	AutomaticCompaction bool
	Model               ModelRef
	Messages            []Message
	Tools               []ToolDefinition
	ToolChoice          ToolChoice
	Output              *OutputConstraint
	MaxOutputTokens     int
	Metadata            map[string]string
	Extension           map[string]json.RawMessage
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
	switch r.ToolChoice.Mode {
	case ToolChoiceAuto, ToolChoiceNone, ToolChoiceRequired, ToolChoiceNamed:
	default:
		return fmt.Errorf("unknown tool choice mode %q", r.ToolChoice.Mode)
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
	// AutomaticCompaction describes a provider capability. It does not enable
	// runner context accounting or compaction; those are planned under spec 0014.
	AutomaticCompaction bool
	ReasoningSummaries  bool
}

func (capabilities ModelCapabilities) Validate() error {
	if capabilities.ContextTokens < 0 {
		return errors.New("context tokens cannot be negative")
	}
	if capabilities.MaxOutputTokens < 0 {
		return errors.New("maximum output tokens cannot be negative")
	}
	if capabilities.ContextTokens > 0 && capabilities.MaxOutputTokens > capabilities.ContextTokens {
		return errors.New("maximum output tokens cannot exceed context tokens")
	}
	if err := capabilities.ToolUse.Validate(); err != nil {
		return err
	}
	if capabilities.ToolUse == ToolUseNone && (capabilities.ParallelToolCalls ||
		capabilities.StrictToolSchemas || capabilities.DynamicTools ||
		capabilities.MultimodalToolResults) {
		return errors.New("tool capabilities require a tool-use mode")
	}
	seenModalities := make(map[string]struct{}, len(capabilities.Modalities))
	for index, modality := range capabilities.Modalities {
		if strings.TrimSpace(modality) == "" || modality != strings.TrimSpace(modality) {
			return fmt.Errorf("modality %d is empty or has surrounding whitespace", index)
		}
		if _, exists := seenModalities[modality]; exists {
			return fmt.Errorf("modality %d duplicates %q", index, modality)
		}
		seenModalities[modality] = struct{}{}
	}
	return nil
}

type ModelDescriptor struct {
	Ref          ModelRef
	Capabilities ModelCapabilities
	Labels       map[string]string
}

func (descriptor ModelDescriptor) Validate() error {
	if err := descriptor.Ref.Validate(); err != nil {
		return err
	}
	if err := descriptor.Capabilities.Validate(); err != nil {
		return fmt.Errorf("model capabilities: %w", err)
	}
	for key := range descriptor.Labels {
		if strings.TrimSpace(key) == "" || key != strings.TrimSpace(key) {
			return errors.New("model label key is empty or has surrounding whitespace")
		}
	}
	return nil
}

func (descriptor ModelDescriptor) Clone() ModelDescriptor {
	descriptor.Capabilities.Modalities = append([]string(nil), descriptor.Capabilities.Modalities...)
	descriptor.Labels = cloneStringMap(descriptor.Labels)
	return descriptor
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

type ToolCallDelta struct {
	Index         int
	ID            string
	Name          string
	InputFragment string
}

func (delta ToolCallDelta) Validate() error {
	if delta.Index < 0 {
		return errors.New("tool-call delta index cannot be negative")
	}
	if delta.ID == "" && delta.Name == "" && delta.InputFragment == "" {
		return errors.New("tool-call delta requires an ID, name, or input fragment")
	}
	if delta.ID != "" && (strings.TrimSpace(delta.ID) == "" || delta.ID != strings.TrimSpace(delta.ID)) {
		return errors.New("tool-call delta ID has invalid whitespace")
	}
	if delta.Name != "" && (strings.TrimSpace(delta.Name) == "" || delta.Name != strings.TrimSpace(delta.Name)) {
		return errors.New("tool-call delta name has invalid whitespace")
	}
	return nil
}

type Usage struct {
	InputTokens      int64
	OutputTokens     int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	Cost             CostMicros
}

func (usage Usage) Validate() error {
	if usage.InputTokens < 0 || usage.OutputTokens < 0 || usage.CacheReadTokens < 0 ||
		usage.CacheWriteTokens < 0 || usage.Cost < 0 {
		return errors.New("usage values cannot be negative")
	}
	return nil
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

func (reason StopReason) Validate() error {
	switch reason {
	case StopEndTurn, StopToolUse, StopMaxTokens, StopRefusal, StopCancelled, StopUnknown:
		return nil
	default:
		return fmt.Errorf("unknown stop reason %q", reason)
	}
}

type ModelError struct {
	Code    string
	Message string
	// Retryable is an informational provider hint. It does not authorize or
	// trigger automatic retries or rerouting in the runner.
	Retryable     bool
	SafeToReroute bool
}

type ModelEvent struct {
	Type          ModelEventType
	Sequence      uint64
	Text          string
	ToolCallDelta *ToolCallDelta
	ToolCall      *ToolCall
	Usage         *Usage
	Stop          StopReason
	Error         *ModelError
	Extension     map[string]json.RawMessage
}

func (e ModelEvent) Validate() error {
	if e.Sequence == 0 {
		return errors.New("model event sequence must be positive")
	}
	switch e.Type {
	case ModelEventStart:
		return e.validatePayload(false, false, false, false, false)
	case ModelEventTextDelta:
		if e.Text == "" {
			return errors.New("text delta is empty")
		}
		return e.validatePayload(true, false, false, false, false)
	case ModelEventToolCallDelta:
		if e.ToolCallDelta == nil {
			return errors.New("tool-call delta event requires a delta")
		}
		if err := e.ToolCallDelta.Validate(); err != nil {
			return err
		}
		return e.validatePayload(false, true, false, false, false)
	case ModelEventUsage:
		if e.Usage == nil {
			return errors.New("usage event requires usage")
		}
		if err := e.Usage.Validate(); err != nil {
			return err
		}
		return e.validatePayload(false, false, false, true, false)
	case ModelEventToolCall:
		if e.ToolCall == nil || strings.TrimSpace(e.ToolCall.ID) == "" ||
			e.ToolCall.ID != strings.TrimSpace(e.ToolCall.ID) || strings.TrimSpace(e.ToolCall.Name) == "" ||
			e.ToolCall.Name != strings.TrimSpace(e.ToolCall.Name) ||
			len(e.ToolCall.Input) == 0 || !json.Valid(e.ToolCall.Input) {
			return errors.New("completed tool-call event is invalid")
		}
		return e.validatePayload(false, false, true, false, false)
	case ModelEventStop:
		if err := e.Stop.Validate(); err != nil {
			return err
		}
		return e.validatePayload(false, false, false, false, true)
	case ModelEventError:
		if e.Error == nil || strings.TrimSpace(e.Error.Code) == "" ||
			e.Error.Code != strings.TrimSpace(e.Error.Code) || strings.TrimSpace(e.Error.Message) == "" {
			return errors.New("error event requires a code and message")
		}
		return e.validatePayload(false, false, false, false, false)
	default:
		return fmt.Errorf("unknown model event type %q", e.Type)
	}
}

func (e ModelEvent) validatePayload(text, delta, call, usage, stop bool) error {
	if !text && e.Text != "" {
		return fmt.Errorf("model event %q contains text payload", e.Type)
	}
	if !delta && e.ToolCallDelta != nil {
		return fmt.Errorf("model event %q contains tool-call delta payload", e.Type)
	}
	if !call && e.ToolCall != nil {
		return fmt.Errorf("model event %q contains completed tool-call payload", e.Type)
	}
	if !usage && e.Usage != nil {
		return fmt.Errorf("model event %q contains usage payload", e.Type)
	}
	if !stop && e.Stop != "" {
		return fmt.Errorf("model event %q contains stop payload", e.Type)
	}
	if e.Type != ModelEventError && e.Error != nil {
		return fmt.Errorf("model event %q contains error payload", e.Type)
	}
	return nil
}

func (e ModelEvent) Clone() ModelEvent {
	if e.ToolCallDelta != nil {
		delta := *e.ToolCallDelta
		e.ToolCallDelta = &delta
	}
	if e.ToolCall != nil {
		call := cloneToolCall(*e.ToolCall)
		e.ToolCall = &call
	}
	if e.Usage != nil {
		usage := *e.Usage
		e.Usage = &usage
	}
	if e.Error != nil {
		modelError := *e.Error
		e.Error = &modelError
	}
	if e.Extension != nil {
		extension := make(map[string]json.RawMessage, len(e.Extension))
		for key, value := range e.Extension {
			extension[key] = append(json.RawMessage(nil), value...)
		}
		e.Extension = extension
	}
	return e
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
func DrainStream(ctx context.Context, stream ModelStream) (events []ModelEvent, returnErr error) {
	if stream == nil {
		return nil, errors.New("model stream is nil")
	}
	defer func() {
		returnErr = errors.Join(returnErr, stream.Close())
	}()

	validator := ModelStreamValidator{}
	for {
		event, err := stream.Recv(ctx)
		if errors.Is(err, io.EOF) {
			if err := validator.Complete(); err != nil {
				return events, err
			}
			return events, nil
		}
		if err != nil {
			if ctx.Err() != nil {
				return events, ctx.Err()
			}
			return events, err
		}
		if err := validator.Accept(event); err != nil {
			return events, err
		}
		events = append(events, event.Clone())
	}
}
