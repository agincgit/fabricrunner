package openaicompat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/agincgit/fabricrunner"
)

type chatRequest struct {
	Model               string              `json:"model"`
	Messages            []chatMessage       `json:"messages"`
	Tools               []chatTool          `json:"tools,omitempty"`
	ToolChoice          any                 `json:"tool_choice,omitempty"`
	ResponseFormat      *chatResponseFormat `json:"response_format,omitempty"`
	MaxTokens           *int                `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int                `json:"max_completion_tokens,omitempty"`
	Stream              bool                `json:"stream"`
	StreamOptions       chatStreamOptions   `json:"stream_options"`
}

type chatStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    *string        `json:"content,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type chatToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function chatToolFunction `json:"function"`
}

type chatToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Arguments   string          `json:"arguments,omitempty"`
	Strict      bool            `json:"strict,omitempty"`
}

type chatTool struct {
	Type     string           `json:"type"`
	Function chatToolFunction `json:"function"`
}

type namedToolChoice struct {
	Type     string `json:"type"`
	Function struct {
		Name string `json:"name"`
	} `json:"function"`
}

type chatResponseFormat struct {
	Type       string         `json:"type"`
	JSONSchema chatJSONSchema `json:"json_schema"`
}

type chatJSONSchema struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
	Strict bool            `json:"strict"`
}

func (provider *Provider) translateRequest(request fabricrunner.ModelRequest) (chatRequest, error) {
	request = fabricrunner.CloneModelRequest(request)
	if err := request.Validate(); err != nil {
		return chatRequest{}, fmt.Errorf("%w: %v", ErrTranslation, err)
	}
	if request.Model.Provider != provider.name {
		return chatRequest{}, fmt.Errorf("%w: model provider %q does not match %q",
			ErrTranslation, request.Model.Provider, provider.name)
	}
	messages, err := translateMessages(request.Messages)
	if err != nil {
		return chatRequest{}, err
	}
	result := chatRequest{
		Model: request.Model.Model, Messages: messages, Stream: true,
		StreamOptions: chatStreamOptions{IncludeUsage: true},
	}
	for _, definition := range request.Tools {
		result.Tools = append(result.Tools, chatTool{
			Type: "function",
			Function: chatToolFunction{
				Name: definition.Name, Description: definition.Description,
				Parameters: append(json.RawMessage(nil), definition.InputSchema...), Strict: definition.Strict,
			},
		})
	}
	switch request.ToolChoice.Mode {
	case fabricrunner.ToolChoiceAuto, fabricrunner.ToolChoiceNone, fabricrunner.ToolChoiceRequired:
		result.ToolChoice = string(request.ToolChoice.Mode)
	case fabricrunner.ToolChoiceNamed:
		choice := namedToolChoice{Type: "function"}
		choice.Function.Name = request.ToolChoice.Name
		result.ToolChoice = choice
	default:
		return chatRequest{}, fmt.Errorf("%w: unknown tool choice", ErrTranslation)
	}
	if request.Output != nil {
		if strings.TrimSpace(request.Output.Name) == "" || request.Output.Name != strings.TrimSpace(request.Output.Name) {
			return chatRequest{}, fmt.Errorf("%w: output schema name is empty or has surrounding whitespace", ErrTranslation)
		}
		result.ResponseFormat = &chatResponseFormat{
			Type: "json_schema",
			JSONSchema: chatJSONSchema{
				Name: request.Output.Name, Schema: append(json.RawMessage(nil), request.Output.Schema...),
				Strict: request.Output.Strict,
			},
		}
	}
	if request.MaxOutputTokens > 0 {
		limit := request.MaxOutputTokens
		if provider.tokenLimitField == TokenLimitMaxCompletionTokens {
			result.MaxCompletionTokens = &limit
		} else {
			result.MaxTokens = &limit
		}
	}
	return result, nil
}

func translateMessages(messages []fabricrunner.Message) ([]chatMessage, error) {
	result := make([]chatMessage, 0, len(messages))
	for messageIndex, message := range messages {
		switch message.Role {
		case fabricrunner.RoleSystem, fabricrunner.RoleUser:
			content, err := textOnlyContent(message.Content)
			if err != nil {
				return nil, fmt.Errorf("%w: message %d: %v", ErrTranslation, messageIndex, err)
			}
			result = append(result, chatMessage{Role: string(message.Role), Content: &content})
		case fabricrunner.RoleAssistant:
			translated, err := translateAssistant(message.Content)
			if err != nil {
				return nil, fmt.Errorf("%w: message %d: %v", ErrTranslation, messageIndex, err)
			}
			result = append(result, translated)
		case fabricrunner.RoleTool:
			translated, err := translateToolResults(message.Content)
			if err != nil {
				return nil, fmt.Errorf("%w: message %d: %v", ErrTranslation, messageIndex, err)
			}
			result = append(result, translated...)
		default:
			return nil, fmt.Errorf("%w: message %d has unsupported role %q", ErrTranslation, messageIndex, message.Role)
		}
	}
	return result, nil
}

func textOnlyContent(parts []fabricrunner.ContentPart) (string, error) {
	var content strings.Builder
	for _, part := range parts {
		if part.Type != fabricrunner.ContentText {
			return "", fmt.Errorf("content type %q is unsupported", part.Type)
		}
		content.WriteString(part.Text)
	}
	return content.String(), nil
}

func translateAssistant(parts []fabricrunner.ContentPart) (chatMessage, error) {
	result := chatMessage{Role: "assistant"}
	var text strings.Builder
	hasText := false
	for _, part := range parts {
		switch part.Type {
		case fabricrunner.ContentText:
			hasText = true
			text.WriteString(part.Text)
		case fabricrunner.ContentToolCall:
			result.ToolCalls = append(result.ToolCalls, chatToolCall{
				ID: part.ToolCallID, Type: "function",
				Function: chatToolFunction{Name: part.ToolName, Arguments: string(part.JSON)},
			})
		default:
			return chatMessage{}, fmt.Errorf("assistant content type %q is unsupported", part.Type)
		}
	}
	if hasText {
		content := text.String()
		result.Content = &content
	}
	if result.Content == nil && len(result.ToolCalls) == 0 {
		return chatMessage{}, errors.New("assistant message has no translatable content")
	}
	return result, nil
}

func translateToolResults(parts []fabricrunner.ContentPart) ([]chatMessage, error) {
	type group struct {
		id      string
		content strings.Builder
		parts   int
	}
	groups := make([]*group, 0, len(parts))
	byID := make(map[string]*group, len(parts))
	for _, part := range parts {
		if part.ToolCallID == "" {
			return nil, errors.New("tool result content has no call ID")
		}
		current := byID[part.ToolCallID]
		if current == nil {
			current = &group{id: part.ToolCallID}
			byID[part.ToolCallID] = current
			groups = append(groups, current)
		}
		if current.parts > 0 {
			current.content.WriteByte('\n')
		}
		switch part.Type {
		case fabricrunner.ContentToolResult:
			if len(part.JSON) == 0 {
				return nil, errors.New("tool result has no JSON content")
			}
			current.content.Write(part.JSON)
		case fabricrunner.ContentText:
			current.content.WriteString(part.Text)
		default:
			return nil, fmt.Errorf("tool result content type %q is unsupported", part.Type)
		}
		current.parts++
	}
	result := make([]chatMessage, len(groups))
	for index, current := range groups {
		content := current.content.String()
		result[index] = chatMessage{Role: "tool", ToolCallID: current.id, Content: &content}
	}
	return result, nil
}
