package openaicompat

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/agincgit/fabricrunner"
)

type chatStream struct {
	ctx       context.Context
	cancel    context.CancelFunc
	body      io.ReadCloser
	decoder   *sseDecoder
	closeOnce sync.Once
	closeErr  error

	sequence uint64
	queue    []fabricrunner.ModelEvent
	done     bool
	finish   string
	calls    map[int]*callAccumulator
	usage    *fabricrunner.Usage
}

type callAccumulator struct {
	id        strings.Builder
	name      strings.Builder
	arguments strings.Builder
}

type completionChunk struct {
	Choices []completionChoice `json:"choices"`
	Usage   *completionUsage   `json:"usage"`
	Error   *providerError     `json:"error"`
}

type completionChoice struct {
	Index        int             `json:"index"`
	Delta        completionDelta `json:"delta"`
	FinishReason *string         `json:"finish_reason"`
}

type completionDelta struct {
	Content   *string         `json:"content"`
	Refusal   *string         `json:"refusal"`
	ToolCalls []toolCallChunk `json:"tool_calls"`
}

type toolCallChunk struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type completionUsage struct {
	PromptTokens        int64 `json:"prompt_tokens"`
	CompletionTokens    int64 `json:"completion_tokens"`
	PromptTokensDetails struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

type providerError struct{}

func newChatStream(
	ctx context.Context,
	cancel context.CancelFunc,
	body io.ReadCloser,
	maxEventBytes int,
) *chatStream {
	stream := &chatStream{
		ctx: ctx, cancel: cancel, body: body,
		decoder: newSSEDecoder(body, maxEventBytes), calls: make(map[int]*callAccumulator),
	}
	stream.enqueue(fabricrunner.ModelEvent{Type: fabricrunner.ModelEventStart})
	return stream
}

func (stream *chatStream) Recv(ctx context.Context) (fabricrunner.ModelEvent, error) {
	if err := ctx.Err(); err != nil {
		return fabricrunner.ModelEvent{}, err
	}
	if err := stream.ctx.Err(); err != nil {
		return fabricrunner.ModelEvent{}, err
	}
	for len(stream.queue) == 0 {
		if stream.done {
			return fabricrunner.ModelEvent{}, io.EOF
		}
		data, err := stream.nextSSE(ctx)
		if err != nil {
			if stream.ctx.Err() != nil {
				return fabricrunner.ModelEvent{}, stream.ctx.Err()
			}
			if ctx.Err() != nil {
				return fabricrunner.ModelEvent{}, ctx.Err()
			}
			if errors.Is(err, io.EOF) {
				return fabricrunner.ModelEvent{}, fmt.Errorf("%w: stream ended before [DONE]", ErrSSEProtocol)
			}
			return fabricrunner.ModelEvent{}, fmt.Errorf("%w: %v", ErrSSEProtocol, err)
		}
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			if err := stream.finalize(); err != nil {
				return fabricrunner.ModelEvent{}, err
			}
			stream.done = true
			continue
		}
		if err := stream.consumeChunk([]byte(data)); err != nil {
			return fabricrunner.ModelEvent{}, err
		}
	}
	event := stream.queue[0]
	stream.queue = stream.queue[1:]
	if err := event.Validate(); err != nil {
		return fabricrunner.ModelEvent{}, fmt.Errorf("%w: generated event: %v", ErrSSEProtocol, err)
	}
	return event.Clone(), nil
}

type sseResult struct {
	data string
	err  error
}

func (stream *chatStream) nextSSE(ctx context.Context) (string, error) {
	result := make(chan sseResult, 1)
	go func() {
		data, err := stream.decoder.Next()
		result <- sseResult{data: data, err: err}
	}()
	select {
	case next := <-result:
		if err := ctx.Err(); err != nil {
			_ = stream.Close()
			return "", err
		}
		return next.data, next.err
	case <-ctx.Done():
		_ = stream.Close()
		return "", ctx.Err()
	case <-stream.ctx.Done():
		_ = stream.Close()
		return "", stream.ctx.Err()
	}
}

func (stream *chatStream) Close() error {
	stream.closeOnce.Do(func() {
		stream.cancel()
		stream.closeErr = stream.body.Close()
	})
	return stream.closeErr
}

func (stream *chatStream) consumeChunk(data []byte) error {
	var chunk completionChunk
	if err := json.Unmarshal(data, &chunk); err != nil {
		return fmt.Errorf("%w: malformed chunk JSON", ErrSSEProtocol)
	}
	if chunk.Error != nil {
		stream.enqueue(fabricrunner.ModelEvent{
			Type:  fabricrunner.ModelEventError,
			Error: &fabricrunner.ModelError{Code: "provider_error", Message: "provider reported an error"},
		})
		stream.done = true
		return nil
	}
	if len(chunk.Choices) > 1 {
		return fmt.Errorf("%w: multiple completion choices are unsupported", ErrSSEProtocol)
	}
	if chunk.Usage != nil {
		if stream.usage != nil {
			return fmt.Errorf("%w: repeated usage payload", ErrSSEProtocol)
		}
		usage := fabricrunner.Usage{
			InputTokens: chunk.Usage.PromptTokens, OutputTokens: chunk.Usage.CompletionTokens,
			CacheReadTokens: chunk.Usage.PromptTokensDetails.CachedTokens,
		}
		if err := usage.Validate(); err != nil {
			return fmt.Errorf("%w: %v", ErrSSEProtocol, err)
		}
		stream.usage = &usage
	}
	if len(chunk.Choices) == 0 {
		return nil
	}
	choice := chunk.Choices[0]
	if choice.Index != 0 {
		return fmt.Errorf("%w: completion choice index %d is unsupported", ErrSSEProtocol, choice.Index)
	}
	if choice.Delta.Content != nil && *choice.Delta.Content != "" {
		stream.enqueue(fabricrunner.ModelEvent{
			Type: fabricrunner.ModelEventTextDelta, Text: *choice.Delta.Content,
		})
	}
	if choice.Delta.Refusal != nil && *choice.Delta.Refusal != "" {
		stream.enqueue(fabricrunner.ModelEvent{
			Type: fabricrunner.ModelEventTextDelta, Text: *choice.Delta.Refusal,
		})
	}
	for _, toolCall := range choice.Delta.ToolCalls {
		if toolCall.Index < 0 || (toolCall.Type != "" && toolCall.Type != "function") {
			return fmt.Errorf("%w: invalid function-call fragment", ErrSSEProtocol)
		}
		delta := fabricrunner.ToolCallDelta{
			Index: toolCall.Index, ID: toolCall.ID, Name: toolCall.Function.Name,
			InputFragment: toolCall.Function.Arguments,
		}
		if err := delta.Validate(); err != nil {
			return fmt.Errorf("%w: %v", ErrSSEProtocol, err)
		}
		accumulator := stream.calls[toolCall.Index]
		if accumulator == nil {
			accumulator = &callAccumulator{}
			stream.calls[toolCall.Index] = accumulator
		}
		accumulator.id.WriteString(toolCall.ID)
		accumulator.name.WriteString(toolCall.Function.Name)
		accumulator.arguments.WriteString(toolCall.Function.Arguments)
		stream.enqueue(fabricrunner.ModelEvent{
			Type: fabricrunner.ModelEventToolCallDelta, ToolCallDelta: &delta,
		})
	}
	if choice.FinishReason != nil {
		if stream.finish != "" && stream.finish != *choice.FinishReason {
			return fmt.Errorf("%w: conflicting finish reasons", ErrSSEProtocol)
		}
		stream.finish = *choice.FinishReason
	}
	return nil
}

func (stream *chatStream) finalize() error {
	if stream.finish == "" {
		return fmt.Errorf("%w: [DONE] arrived without a finish reason", ErrSSEProtocol)
	}
	indices := make([]int, 0, len(stream.calls))
	for index := range stream.calls {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	stop := normalizeFinishReason(stream.finish)
	if stop == fabricrunner.StopToolUse && len(indices) == 0 {
		return fmt.Errorf("%w: tool finish contained no calls", ErrSSEProtocol)
	}
	if stop != fabricrunner.StopToolUse && len(indices) > 0 {
		return fmt.Errorf("%w: completed calls have finish reason %q", ErrSSEProtocol, stream.finish)
	}
	seenCallIDs := make(map[string]struct{}, len(indices))
	for _, index := range indices {
		call := stream.calls[index]
		completed := fabricrunner.ToolCall{
			ID: call.id.String(), Name: call.name.String(), Input: json.RawMessage(call.arguments.String()),
		}
		if err := (fabricrunner.ModelEvent{Type: fabricrunner.ModelEventToolCall, Sequence: 1, ToolCall: &completed}).Validate(); err != nil {
			return fmt.Errorf("%w: completed function call %d: %v", ErrSSEProtocol, index, err)
		}
		if _, exists := seenCallIDs[completed.ID]; exists {
			return fmt.Errorf("%w: duplicate completed call ID %q", ErrSSEProtocol, completed.ID)
		}
		seenCallIDs[completed.ID] = struct{}{}
		event := fabricrunner.ModelEvent{Type: fabricrunner.ModelEventToolCall, ToolCall: &completed}
		stream.enqueue(event)
	}
	if stream.usage != nil {
		usage := *stream.usage
		stream.enqueue(fabricrunner.ModelEvent{Type: fabricrunner.ModelEventUsage, Usage: &usage})
	}
	stream.enqueue(fabricrunner.ModelEvent{Type: fabricrunner.ModelEventStop, Stop: stop})
	return nil
}

func (stream *chatStream) enqueue(event fabricrunner.ModelEvent) {
	stream.sequence++
	event.Sequence = stream.sequence
	stream.queue = append(stream.queue, event)
}

func normalizeFinishReason(reason string) fabricrunner.StopReason {
	switch reason {
	case "stop":
		return fabricrunner.StopEndTurn
	case "tool_calls", "function_call":
		return fabricrunner.StopToolUse
	case "length":
		return fabricrunner.StopMaxTokens
	case "content_filter":
		return fabricrunner.StopRefusal
	default:
		return fabricrunner.StopUnknown
	}
}

type sseDecoder struct {
	scanner       *bufio.Scanner
	maxEventBytes int
	exhausted     bool
}

func newSSEDecoder(reader io.Reader, maxEventBytes int) *sseDecoder {
	scanner := bufio.NewScanner(reader)
	initial := 64 * 1024
	if maxEventBytes < initial {
		initial = maxEventBytes
	}
	scanner.Buffer(make([]byte, initial), maxEventBytes+1)
	return &sseDecoder{scanner: scanner, maxEventBytes: maxEventBytes}
}

func (decoder *sseDecoder) Next() (string, error) {
	if decoder.exhausted {
		return "", io.EOF
	}
	var data strings.Builder
	for decoder.scanner.Scan() {
		line := decoder.scanner.Text()
		if line == "" {
			if data.Len() == 0 {
				continue
			}
			return data.String(), nil
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, found := strings.Cut(line, ":")
		if !found || field != "data" {
			continue
		}
		value = strings.TrimPrefix(value, " ")
		additional := len(value)
		if data.Len() > 0 {
			additional++
		}
		if data.Len()+additional > decoder.maxEventBytes {
			return "", errors.New("SSE event exceeds configured size limit")
		}
		if data.Len() > 0 {
			data.WriteByte('\n')
		}
		data.WriteString(value)
	}
	decoder.exhausted = true
	if err := decoder.scanner.Err(); err != nil {
		return "", err
	}
	if data.Len() > 0 {
		return data.String(), nil
	}
	return "", io.EOF
}

var _ fabricrunner.ModelStream = (*chatStream)(nil)
