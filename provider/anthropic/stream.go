package anthropic

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/agincgit/fabricrunner"
)

type messageStream struct {
	ctx          context.Context
	cancel       context.CancelFunc
	body         io.ReadCloser
	decoder      *sseDecoder
	closeOnce    sync.Once
	closeErr     error
	sequence     uint64
	queue        []fabricrunner.ModelEvent
	started      bool
	messageDelta bool
	done         bool
	stopReason   string
	blocks       map[int]*blockState
	seenCallIDs  map[string]struct{}
	nextBlock    int
	usage        usageTotals
}

type blockState struct {
	kind, id, name string
	input          strings.Builder
}
type usageTotals struct{ input, output, cacheRead, cacheWrite int64 }

type streamEnvelope struct {
	Type    string `json:"type"`
	Index   int    `json:"index"`
	Message *struct {
		Usage wireUsage `json:"usage"`
	} `json:"message"`
	ContentBlock *struct {
		Type  string          `json:"type"`
		Text  string          `json:"text"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"content_block"`
	Delta *struct {
		Type        string  `json:"type"`
		Text        string  `json:"text"`
		PartialJSON string  `json:"partial_json"`
		StopReason  *string `json:"stop_reason"`
	} `json:"delta"`
	Usage *wireUsage      `json:"usage"`
	Error json.RawMessage `json:"error"`
}

type wireUsage struct {
	InputTokens      *int64 `json:"input_tokens"`
	OutputTokens     *int64 `json:"output_tokens"`
	CacheReadTokens  *int64 `json:"cache_read_input_tokens"`
	CacheWriteTokens *int64 `json:"cache_creation_input_tokens"`
}

func newMessageStream(ctx context.Context, cancel context.CancelFunc, body io.ReadCloser, maxEventBytes int) *messageStream {
	return &messageStream{ctx: ctx, cancel: cancel, body: body, decoder: newSSEDecoder(body, maxEventBytes), blocks: make(map[int]*blockState), seenCallIDs: make(map[string]struct{})}
}

func (stream *messageStream) Recv(ctx context.Context) (fabricrunner.ModelEvent, error) {
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
		eventName, data, err := stream.nextSSE(ctx)
		if err != nil {
			if stream.ctx.Err() != nil {
				return fabricrunner.ModelEvent{}, stream.ctx.Err()
			}
			if ctx.Err() != nil {
				return fabricrunner.ModelEvent{}, ctx.Err()
			}
			if errors.Is(err, io.EOF) {
				return fabricrunner.ModelEvent{}, fmt.Errorf("%w: stream ended before message_stop", ErrSSEProtocol)
			}
			return fabricrunner.ModelEvent{}, fmt.Errorf("%w: %v", ErrSSEProtocol, err)
		}
		if data == "" {
			continue
		}
		if err := stream.consume(eventName, []byte(data)); err != nil {
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
	event, data string
	err         error
}

func (stream *messageStream) nextSSE(ctx context.Context) (string, string, error) {
	result := make(chan sseResult, 1)
	go func() {
		event, data, err := stream.decoder.Next()
		result <- sseResult{event: event, data: data, err: err}
	}()
	select {
	case next := <-result:
		if err := ctx.Err(); err != nil {
			_ = stream.Close()
			return "", "", err
		}
		return next.event, next.data, next.err
	case <-ctx.Done():
		_ = stream.Close()
		return "", "", ctx.Err()
	case <-stream.ctx.Done():
		_ = stream.Close()
		return "", "", stream.ctx.Err()
	}
}

func (stream *messageStream) Close() error {
	stream.closeOnce.Do(func() { stream.cancel(); stream.closeErr = stream.body.Close() })
	return stream.closeErr
}

func (stream *messageStream) consume(eventName string, data []byte) error {
	var envelope streamEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil || envelope.Type == "" {
		return fmt.Errorf("%w: malformed event JSON", ErrSSEProtocol)
	}
	if eventName == "" || eventName != envelope.Type {
		return fmt.Errorf("%w: SSE event name does not match payload type", ErrSSEProtocol)
	}
	if envelope.Type == "ping" {
		return nil
	}
	if envelope.Type == "error" {
		if stream.done {
			return fmt.Errorf("%w: event followed terminal event", ErrSSEProtocol)
		}
		if !stream.started {
			stream.started = true
			stream.enqueue(fabricrunner.ModelEvent{Type: fabricrunner.ModelEventStart})
		}
		stream.enqueue(fabricrunner.ModelEvent{Type: fabricrunner.ModelEventError, Error: &fabricrunner.ModelError{Code: "provider_error", Message: "provider reported an error"}})
		stream.done = true
		return nil
	}
	if !stream.started && envelope.Type != "message_start" {
		return fmt.Errorf("%w: first event is not message_start", ErrSSEProtocol)
	}
	if stream.messageDelta && envelope.Type != "message_delta" && envelope.Type != "message_stop" {
		return fmt.Errorf("%w: content event followed message_delta", ErrSSEProtocol)
	}
	switch envelope.Type {
	case "message_start":
		if stream.started || envelope.Message == nil {
			return fmt.Errorf("%w: repeated or incomplete message_start", ErrSSEProtocol)
		}
		stream.started = true
		stream.enqueue(fabricrunner.ModelEvent{Type: fabricrunner.ModelEventStart})
		return stream.updateUsage(envelope.Message.Usage)
	case "content_block_start":
		if envelope.ContentBlock == nil || envelope.Index != stream.nextBlock || len(stream.blocks) != 0 {
			return fmt.Errorf("%w: invalid content block start", ErrSSEProtocol)
		}
		block := &blockState{kind: envelope.ContentBlock.Type, id: envelope.ContentBlock.ID, name: envelope.ContentBlock.Name}
		switch block.kind {
		case "text":
			if envelope.ContentBlock.Text != "" {
				stream.enqueue(fabricrunner.ModelEvent{Type: fabricrunner.ModelEventTextDelta, Text: envelope.ContentBlock.Text})
			}
		case "tool_use":
			if block.id == "" || block.name == "" {
				return fmt.Errorf("%w: tool block lacks ID or name", ErrSSEProtocol)
			}
			if len(envelope.ContentBlock.Input) > 0 && string(envelope.ContentBlock.Input) != "{}" {
				block.input.Write(envelope.ContentBlock.Input)
			}
			delta := fabricrunner.ToolCallDelta{Index: envelope.Index, ID: block.id, Name: block.name}
			stream.enqueue(fabricrunner.ModelEvent{Type: fabricrunner.ModelEventToolCallDelta, ToolCallDelta: &delta})
		default:
			return fmt.Errorf("%w: unsupported content block %q", ErrSSEProtocol, block.kind)
		}
		stream.blocks[envelope.Index] = block
		stream.nextBlock++
		return nil
	case "content_block_delta":
		block := stream.blocks[envelope.Index]
		if block == nil || envelope.Delta == nil {
			return fmt.Errorf("%w: delta has no open content block", ErrSSEProtocol)
		}
		switch envelope.Delta.Type {
		case "text_delta":
			if block.kind != "text" || envelope.Delta.Text == "" {
				return fmt.Errorf("%w: invalid text delta", ErrSSEProtocol)
			}
			stream.enqueue(fabricrunner.ModelEvent{Type: fabricrunner.ModelEventTextDelta, Text: envelope.Delta.Text})
		case "input_json_delta":
			if block.kind != "tool_use" || envelope.Delta.PartialJSON == "" {
				return fmt.Errorf("%w: invalid tool-input delta", ErrSSEProtocol)
			}
			block.input.WriteString(envelope.Delta.PartialJSON)
			delta := fabricrunner.ToolCallDelta{Index: envelope.Index, InputFragment: envelope.Delta.PartialJSON}
			stream.enqueue(fabricrunner.ModelEvent{Type: fabricrunner.ModelEventToolCallDelta, ToolCallDelta: &delta})
		default:
			return fmt.Errorf("%w: unsupported delta %q", ErrSSEProtocol, envelope.Delta.Type)
		}
		return nil
	case "content_block_stop":
		block := stream.blocks[envelope.Index]
		if block == nil {
			return fmt.Errorf("%w: stop has no open content block", ErrSSEProtocol)
		}
		delete(stream.blocks, envelope.Index)
		if block.kind == "tool_use" {
			input := block.input.String()
			if input == "" {
				input = "{}"
			}
			call := fabricrunner.ToolCall{ID: block.id, Name: block.name, Input: json.RawMessage(input)}
			probe := fabricrunner.ModelEvent{Type: fabricrunner.ModelEventToolCall, Sequence: 1, ToolCall: &call}
			if err := probe.Validate(); err != nil {
				return fmt.Errorf("%w: invalid completed tool input", ErrSSEProtocol)
			}
			if _, exists := stream.seenCallIDs[call.ID]; exists {
				return fmt.Errorf("%w: duplicate completed tool-call ID", ErrSSEProtocol)
			}
			stream.seenCallIDs[call.ID] = struct{}{}
			stream.enqueue(fabricrunner.ModelEvent{Type: fabricrunner.ModelEventToolCall, ToolCall: &call})
		}
		return nil
	case "message_delta":
		if len(stream.blocks) != 0 || envelope.Delta == nil || envelope.Delta.StopReason == nil {
			return fmt.Errorf("%w: incomplete message_delta", ErrSSEProtocol)
		}
		if stream.stopReason != "" && stream.stopReason != *envelope.Delta.StopReason {
			return fmt.Errorf("%w: conflicting stop reasons", ErrSSEProtocol)
		}
		stream.stopReason = *envelope.Delta.StopReason
		stream.messageDelta = true
		if envelope.Usage != nil {
			return stream.updateUsage(*envelope.Usage)
		}
		return nil
	case "message_stop":
		if len(stream.blocks) != 0 || stream.stopReason == "" {
			return fmt.Errorf("%w: message_stop before completed message", ErrSSEProtocol)
		}
		stream.enqueue(fabricrunner.ModelEvent{Type: fabricrunner.ModelEventStop, Stop: normalizeStopReason(stream.stopReason)})
		stream.done = true
		return nil
	default:
		return fmt.Errorf("%w: unsupported event type %q", ErrSSEProtocol, envelope.Type)
	}
}

func (stream *messageStream) updateUsage(next wireUsage) error {
	total := stream.usage
	if next.InputTokens != nil {
		total.input = *next.InputTokens
	}
	if next.OutputTokens != nil {
		total.output = *next.OutputTokens
	}
	if next.CacheReadTokens != nil {
		total.cacheRead = *next.CacheReadTokens
	}
	if next.CacheWriteTokens != nil {
		total.cacheWrite = *next.CacheWriteTokens
	}
	values := []int64{total.input, total.output, total.cacheRead, total.cacheWrite}
	for _, value := range values {
		if value < 0 {
			return fmt.Errorf("%w: negative usage", ErrSSEProtocol)
		}
	}
	if total.input < stream.usage.input || total.output < stream.usage.output || total.cacheRead < stream.usage.cacheRead || total.cacheWrite < stream.usage.cacheWrite {
		return fmt.Errorf("%w: cumulative usage decreased", ErrSSEProtocol)
	}
	delta := fabricrunner.Usage{InputTokens: total.input - stream.usage.input, OutputTokens: total.output - stream.usage.output, CacheReadTokens: total.cacheRead - stream.usage.cacheRead, CacheWriteTokens: total.cacheWrite - stream.usage.cacheWrite}
	stream.usage = total
	if delta.InputTokens != 0 || delta.OutputTokens != 0 || delta.CacheReadTokens != 0 || delta.CacheWriteTokens != 0 {
		stream.enqueue(fabricrunner.ModelEvent{Type: fabricrunner.ModelEventUsage, Usage: &delta})
	}
	return nil
}

func (stream *messageStream) enqueue(event fabricrunner.ModelEvent) {
	stream.sequence++
	event.Sequence = stream.sequence
	stream.queue = append(stream.queue, event)
}
func normalizeStopReason(reason string) fabricrunner.StopReason {
	switch reason {
	case "end_turn", "stop_sequence", "pause_turn":
		return fabricrunner.StopEndTurn
	case "tool_use":
		return fabricrunner.StopToolUse
	case "max_tokens", "model_context_window_exceeded":
		return fabricrunner.StopMaxTokens
	case "refusal":
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

func newSSEDecoder(reader io.Reader, max int) *sseDecoder {
	scanner := bufio.NewScanner(reader)
	initial := 64 << 10
	if max < initial {
		initial = max
	}
	scanner.Buffer(make([]byte, initial), max+1)
	return &sseDecoder{scanner: scanner, maxEventBytes: max}
}
func (decoder *sseDecoder) Next() (string, string, error) {
	if decoder.exhausted {
		return "", "", io.EOF
	}
	var event string
	var data strings.Builder
	for decoder.scanner.Scan() {
		line := decoder.scanner.Text()
		if line == "" {
			if data.Len() == 0 {
				event = ""
				continue
			}
			return event, data.String(), nil
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			if event != "" {
				return "", "", errors.New("repeated SSE event field")
			}
			event = value
		case "data":
			additional := len(value)
			if data.Len() > 0 {
				additional++
			}
			if data.Len()+additional > decoder.maxEventBytes {
				return "", "", errors.New("SSE event exceeds configured size limit")
			}
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(value)
		}
	}
	decoder.exhausted = true
	if err := decoder.scanner.Err(); err != nil {
		return "", "", err
	}
	if data.Len() > 0 {
		return event, data.String(), nil
	}
	return "", "", io.EOF
}

var _ fabricrunner.ModelStream = (*messageStream)(nil)
