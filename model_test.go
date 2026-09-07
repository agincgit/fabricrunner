package fabricrunner

import (
	"context"
	"errors"
	"io"
	"testing"
)

func TestModelRequestValidation(t *testing.T) {
	t.Parallel()

	valid := ModelRequest{
		Model: ModelRef{Provider: "local", Model: "qwen"},
		Messages: []Message{{Role: RoleUser, Content: []ContentPart{{
			Type:           ContentText,
			Classification: ClassInternal,
			Text:           "hello",
		}}}},
		Tools: []ToolDefinition{{
			Name:        "read",
			InputSchema: []byte(`{"type":"object"}`),
		}},
		ToolChoice: ToolChoice{Mode: ToolChoiceAuto},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}

	invalid := valid
	invalid.ToolChoice = ToolChoice{Mode: ToolChoiceNamed}
	if err := invalid.Validate(); err == nil {
		t.Fatal("named tool choice without name accepted")
	}
}

func TestDrainStreamRequiresOrderedTerminalEvent(t *testing.T) {
	t.Parallel()

	stream := &sliceStream{events: []ModelEvent{
		{Type: ModelEventStart, Sequence: 1},
		{Type: ModelEventTextDelta, Sequence: 2, Text: "done"},
		{Type: ModelEventStop, Sequence: 3, Stop: StopEndTurn},
	}}
	events, err := DrainStream(context.Background(), stream)
	if err != nil {
		t.Fatalf("DrainStream() error = %v", err)
	}
	if len(events) != 3 || !stream.closed {
		t.Fatalf("events = %d, closed = %v; want 3, true", len(events), stream.closed)
	}
}

func TestDrainStreamRejectsMissingTerminalEvent(t *testing.T) {
	t.Parallel()

	_, err := DrainStream(context.Background(), &sliceStream{events: []ModelEvent{
		{Type: ModelEventStart, Sequence: 1},
	}})
	if err == nil {
		t.Fatal("DrainStream() accepted stream without terminal event")
	}
}

func TestDrainStreamRejectsOutOfOrderEvent(t *testing.T) {
	t.Parallel()

	_, err := DrainStream(context.Background(), &sliceStream{events: []ModelEvent{
		{Type: ModelEventStart, Sequence: 2},
		{Type: ModelEventStop, Sequence: 3, Stop: StopEndTurn},
	}})
	if err == nil {
		t.Fatal("DrainStream() accepted out-of-order event")
	}
}

type sliceStream struct {
	events []ModelEvent
	index  int
	closed bool
	err    error
}

func (s *sliceStream) Recv(ctx context.Context) (ModelEvent, error) {
	if err := ctx.Err(); err != nil {
		return ModelEvent{}, err
	}
	if s.err != nil {
		err := s.err
		s.err = nil
		return ModelEvent{}, err
	}
	if s.index >= len(s.events) {
		return ModelEvent{}, io.EOF
	}
	event := s.events[s.index]
	s.index++
	return event, nil
}

func (s *sliceStream) Close() error {
	s.closed = true
	return nil
}

var _ ModelStream = (*sliceStream)(nil)

func TestDrainStreamReturnsProviderError(t *testing.T) {
	t.Parallel()

	want := errors.New("provider unavailable")
	_, err := DrainStream(context.Background(), &sliceStream{err: want})
	if !errors.Is(err, want) {
		t.Fatalf("DrainStream() error = %v, want %v", err, want)
	}
}

func TestModelRequestRejectsUnclassifiedContent(t *testing.T) {
	t.Parallel()

	request := ModelRequest{
		Model: ModelRef{Provider: "local", Model: "qwen"},
		Messages: []Message{{Role: RoleUser, Content: []ContentPart{{
			Type: ContentText,
			Text: "sensitive by default",
		}}}},
	}
	if err := request.Validate(); err == nil {
		t.Fatal("request accepted unclassified content")
	}
}

func TestDrainStreamRejectsInvalidTerminalEvent(t *testing.T) {
	t.Parallel()

	_, err := DrainStream(context.Background(), &sliceStream{events: []ModelEvent{
		{Type: ModelEventStart, Sequence: 1},
		{Type: ModelEventStop, Sequence: 2},
	}})
	if err == nil {
		t.Fatal("DrainStream() accepted stop without reason")
	}
}

func TestDrainStreamRejectsEventAfterTerminal(t *testing.T) {
	t.Parallel()

	_, err := DrainStream(context.Background(), &sliceStream{events: []ModelEvent{
		{Type: ModelEventStart, Sequence: 1},
		{Type: ModelEventStop, Sequence: 2, Stop: StopEndTurn},
		{Type: ModelEventTextDelta, Sequence: 3, Text: "too late"},
	}})
	if err == nil {
		t.Fatal("DrainStream() accepted an event after terminal")
	}
}
