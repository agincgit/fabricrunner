package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/agincgit/fabricrunner"
)

type messageRequest struct {
	Model        string          `json:"model"`
	MaxTokens    int             `json:"max_tokens"`
	System       []contentBlock  `json:"system,omitempty"`
	Messages     []wireMessage   `json:"messages"`
	Tools        []wireTool      `json:"tools,omitempty"`
	ToolChoice   *wireToolChoice `json:"tool_choice,omitempty"`
	OutputConfig *outputConfig   `json:"output_config,omitempty"`
	Stream       bool            `json:"stream"`
}

type wireMessage struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}
type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}
type wireTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
	Strict      bool            `json:"strict,omitempty"`
}
type wireToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}
type outputConfig struct {
	Format outputFormat `json:"format"`
}
type outputFormat struct {
	Type   string          `json:"type"`
	Schema json.RawMessage `json:"schema"`
}

func (provider *Provider) translateRequest(request fabricrunner.ModelRequest) (messageRequest, error) {
	request = fabricrunner.CloneModelRequest(request)
	if err := request.Validate(); err != nil {
		return messageRequest{}, fmt.Errorf("%w: %v", ErrTranslation, err)
	}
	if request.Model.Provider != provider.name {
		return messageRequest{}, fmt.Errorf("%w: model provider mismatch", ErrTranslation)
	}
	result := messageRequest{Model: request.Model.Model, MaxTokens: provider.defaultOutputTokens, Stream: true}
	if request.MaxOutputTokens > 0 {
		result.MaxTokens = request.MaxOutputTokens
	}
	sawConversation := false
	for index, message := range request.Messages {
		if message.Role == fabricrunner.RoleSystem {
			if sawConversation {
				return messageRequest{}, fmt.Errorf("%w: message %d: system content must precede conversation", ErrTranslation, index)
			}
			blocks, err := translateTextBlocks(message.Content)
			if err != nil {
				return messageRequest{}, fmt.Errorf("%w: message %d: %v", ErrTranslation, index, err)
			}
			result.System = append(result.System, blocks...)
			continue
		}
		sawConversation = true
		translated, err := translateMessage(message)
		if err != nil {
			return messageRequest{}, fmt.Errorf("%w: message %d: %v", ErrTranslation, index, err)
		}
		result.Messages = append(result.Messages, translated)
	}
	if len(result.Messages) == 0 {
		return messageRequest{}, fmt.Errorf("%w: conversation has no user or assistant message", ErrTranslation)
	}
	for _, tool := range request.Tools {
		result.Tools = append(result.Tools, wireTool{Name: tool.Name, Description: tool.Description, InputSchema: append(json.RawMessage(nil), tool.InputSchema...), Strict: tool.Strict})
	}
	if len(result.Tools) > 0 {
		choice := &wireToolChoice{}
		switch request.ToolChoice.Mode {
		case fabricrunner.ToolChoiceAuto:
			choice.Type = "auto"
		case fabricrunner.ToolChoiceNone:
			choice.Type = "none"
		case fabricrunner.ToolChoiceRequired:
			choice.Type = "any"
		case fabricrunner.ToolChoiceNamed:
			choice.Type, choice.Name = "tool", request.ToolChoice.Name
		default:
			return messageRequest{}, fmt.Errorf("%w: unknown tool choice", ErrTranslation)
		}
		result.ToolChoice = choice
	} else if request.ToolChoice.Mode == fabricrunner.ToolChoiceRequired {
		return messageRequest{}, fmt.Errorf("%w: required tool choice has no tools", ErrTranslation)
	}
	if request.Output != nil {
		result.OutputConfig = &outputConfig{Format: outputFormat{Type: "json_schema", Schema: append(json.RawMessage(nil), request.Output.Schema...)}}
	}
	return result, nil
}

func translateTextBlocks(parts []fabricrunner.ContentPart) ([]contentBlock, error) {
	blocks := make([]contentBlock, len(parts))
	for index, part := range parts {
		if part.Type != fabricrunner.ContentText {
			return nil, fmt.Errorf("content type %q is unsupported", part.Type)
		}
		blocks[index] = contentBlock{Type: "text", Text: part.Text}
	}
	return blocks, nil
}

func translateMessage(message fabricrunner.Message) (wireMessage, error) {
	result := wireMessage{}
	switch message.Role {
	case fabricrunner.RoleUser:
		result.Role = "user"
		blocks, err := translateTextBlocks(message.Content)
		if err != nil {
			return wireMessage{}, err
		}
		result.Content = blocks
	case fabricrunner.RoleAssistant:
		result.Role = "assistant"
		for _, part := range message.Content {
			switch part.Type {
			case fabricrunner.ContentText:
				result.Content = append(result.Content, contentBlock{Type: "text", Text: part.Text})
			case fabricrunner.ContentToolCall:
				result.Content = append(result.Content, contentBlock{Type: "tool_use", ID: part.ToolCallID, Name: part.ToolName, Input: append(json.RawMessage(nil), part.JSON...)})
			default:
				return wireMessage{}, fmt.Errorf("assistant content type %q is unsupported", part.Type)
			}
		}
	case fabricrunner.RoleTool:
		result.Role = "user"
		for _, part := range message.Content {
			if part.Type != fabricrunner.ContentToolResult {
				return wireMessage{}, fmt.Errorf("tool content type %q is unsupported", part.Type)
			}
			content := string(part.JSON)
			if content == "" {
				content = part.Text
			}
			result.Content = append(result.Content, contentBlock{Type: "tool_result", ToolUseID: part.ToolCallID, Content: content, IsError: part.IsError})
		}
	default:
		return wireMessage{}, fmt.Errorf("message role %q is unsupported", message.Role)
	}
	return result, nil
}
