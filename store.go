package fabricrunner

import (
	"context"
	"errors"
	"fmt"
)

var ErrVersionConflict = errors.New("event stream version conflict")

type VersionConflictError struct {
	Aggregate AggregateRef
	Expected  uint64
	Actual    uint64
}

func (e *VersionConflictError) Error() string {
	return fmt.Sprintf("%s %s: %v: expected %d, actual %d", e.Aggregate.Type, e.Aggregate.ID, ErrVersionConflict, e.Expected, e.Actual)
}

func (e *VersionConflictError) Unwrap() error { return ErrVersionConflict }

// EventStore appends and loads immutable aggregate event streams.
type EventStore interface {
	Append(ctx context.Context, aggregate AggregateRef, expectedSequence uint64, drafts ...EventDraft) ([]Event, error)
	Load(ctx context.Context, aggregate AggregateRef, afterSequence uint64) ([]Event, error)
}

// Replay loads an aggregate from the beginning and applies each event in order.
func Replay(ctx context.Context, store EventStore, aggregate AggregateRef, apply func(Event) error) error {
	events, err := store.Load(ctx, aggregate, 0)
	if err != nil {
		return err
	}
	for _, event := range events {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := apply(event); err != nil {
			return fmt.Errorf("apply event %s at sequence %d: %w", event.ID, event.Sequence, err)
		}
	}
	return nil
}
