package fabricrunner

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewEventCopiesAndHashesPayload(t *testing.T) {
	t.Parallel()

	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	payload := json.RawMessage(`{"goal":"route locally"}`)
	event, err := NewEvent(
		AggregateRef{Type: "workload", ID: id},
		1,
		EventDraft{Type: "workload.created", SchemaVersion: 1, Payload: payload},
		time.Now(),
	)
	if err != nil {
		t.Fatalf("NewEvent() error = %v", err)
	}

	payload[0] = '['
	if !json.Valid(event.Payload) {
		t.Fatal("event payload changed with caller-owned buffer")
	}
	if err := event.Validate(); err != nil {
		t.Fatalf("Event.Validate() error = %v", err)
	}
}

func TestEventRejectsInvalidPayload(t *testing.T) {
	t.Parallel()

	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewEvent(
		AggregateRef{Type: "workload", ID: id},
		1,
		EventDraft{Type: "workload.created", SchemaVersion: 1, Payload: json.RawMessage(`{`)},
		time.Now(),
	)
	if err == nil {
		t.Fatal("NewEvent() accepted malformed JSON")
	}
}
