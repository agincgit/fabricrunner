package fabricrunner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type AggregateRef struct {
	Type string
	ID   ID
}

func (r AggregateRef) Validate() error {
	if r.Type == "" {
		return errors.New("aggregate type is required")
	}
	if err := r.ID.Validate(); err != nil {
		return fmt.Errorf("aggregate ID: %w", err)
	}
	return nil
}

// EventDraft is an event before the store assigns identity and sequence.
type EventDraft struct {
	Type          string
	SchemaVersion uint32
	CausationID   ID
	CorrelationID ID
	AttemptID     ID
	Payload       json.RawMessage
}

func (d EventDraft) Validate() error {
	if d.Type == "" {
		return errors.New("event type is required")
	}
	if d.SchemaVersion == 0 {
		return errors.New("event schema version must be positive")
	}
	if len(d.Payload) == 0 || !json.Valid(d.Payload) {
		return errors.New("event payload must be valid JSON")
	}
	for name, id := range map[string]ID{
		"causation ID":   d.CausationID,
		"correlation ID": d.CorrelationID,
		"attempt ID":     d.AttemptID,
	} {
		if !id.IsZero() {
			if err := id.Validate(); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
	}
	return nil
}

// Event is an immutable record in an aggregate stream.
type Event struct {
	ID            ID
	Aggregate     AggregateRef
	Sequence      uint64
	Type          string
	SchemaVersion uint32
	CausationID   ID
	CorrelationID ID
	AttemptID     ID
	OccurredAt    time.Time
	Payload       json.RawMessage
	PayloadSHA256 string
}

func NewEvent(ref AggregateRef, sequence uint64, draft EventDraft, occurredAt time.Time) (Event, error) {
	if err := ref.Validate(); err != nil {
		return Event{}, err
	}
	if sequence == 0 {
		return Event{}, errors.New("event sequence must be positive")
	}
	if err := draft.Validate(); err != nil {
		return Event{}, err
	}
	if occurredAt.IsZero() {
		return Event{}, errors.New("event occurrence time is required")
	}

	id, err := NewID()
	if err != nil {
		return Event{}, err
	}
	payload := append(json.RawMessage(nil), draft.Payload...)
	digest := sha256.Sum256(payload)

	return Event{
		ID:            id,
		Aggregate:     ref,
		Sequence:      sequence,
		Type:          draft.Type,
		SchemaVersion: draft.SchemaVersion,
		CausationID:   draft.CausationID,
		CorrelationID: draft.CorrelationID,
		AttemptID:     draft.AttemptID,
		OccurredAt:    occurredAt.UTC(),
		Payload:       payload,
		PayloadSHA256: hex.EncodeToString(digest[:]),
	}, nil
}

func (e Event) Validate() error {
	if err := e.ID.Validate(); err != nil {
		return fmt.Errorf("event ID: %w", err)
	}
	if err := e.Aggregate.Validate(); err != nil {
		return err
	}
	if e.Sequence == 0 {
		return errors.New("event sequence must be positive")
	}
	if err := (EventDraft{
		Type:          e.Type,
		SchemaVersion: e.SchemaVersion,
		CausationID:   e.CausationID,
		CorrelationID: e.CorrelationID,
		AttemptID:     e.AttemptID,
		Payload:       e.Payload,
	}).Validate(); err != nil {
		return err
	}
	if e.OccurredAt.IsZero() {
		return errors.New("event occurrence time is required")
	}
	digest := sha256.Sum256(e.Payload)
	if e.PayloadSHA256 != hex.EncodeToString(digest[:]) {
		return errors.New("event payload hash mismatch")
	}
	return nil
}
