// Package memory provides a concurrency-safe, append-only in-memory event
// store for tests and ephemeral runs.
package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/agincgit/fabricrunner"
)

type Store struct {
	mu      sync.RWMutex
	streams map[string][]fabricrunner.Event
	now     func() time.Time
}

func New() *Store {
	return &Store{
		streams: make(map[string][]fabricrunner.Event),
		now:     time.Now,
	}
}

func (s *Store) Append(
	ctx context.Context,
	aggregate fabricrunner.AggregateRef,
	expectedSequence uint64,
	drafts ...fabricrunner.EventDraft,
) ([]fabricrunner.Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := aggregate.Validate(); err != nil {
		return nil, err
	}
	for i, draft := range drafts {
		if err := draft.Validate(); err != nil {
			return nil, fmt.Errorf("draft %d: %w", i, err)
		}
	}
	if len(drafts) == 0 {
		return []fabricrunner.Event{}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := streamKey(aggregate)
	actual := uint64(len(s.streams[key]))
	if actual != expectedSequence {
		return nil, &fabricrunner.VersionConflictError{
			Aggregate: aggregate,
			Expected:  expectedSequence,
			Actual:    actual,
		}
	}

	created := make([]fabricrunner.Event, 0, len(drafts))
	for i, draft := range drafts {
		event, err := fabricrunner.NewEvent(aggregate, actual+uint64(i)+1, draft, s.now())
		if err != nil {
			return nil, fmt.Errorf("materialize draft %d: %w", i, err)
		}
		created = append(created, event)
	}

	s.streams[key] = append(s.streams[key], cloneEvents(created)...)
	return cloneEvents(created), nil
}

func (s *Store) Load(
	ctx context.Context,
	aggregate fabricrunner.AggregateRef,
	afterSequence uint64,
) ([]fabricrunner.Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := aggregate.Validate(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	events := s.streams[streamKey(aggregate)]
	if afterSequence >= uint64(len(events)) {
		return []fabricrunner.Event{}, nil
	}
	return cloneEvents(events[afterSequence:]), nil
}

func streamKey(ref fabricrunner.AggregateRef) string {
	return ref.Type + "\x00" + ref.ID.String()
}

func cloneEvents(events []fabricrunner.Event) []fabricrunner.Event {
	cloned := make([]fabricrunner.Event, len(events))
	copy(cloned, events)
	for i := range cloned {
		cloned[i].Payload = append([]byte(nil), events[i].Payload...)
	}
	return cloned
}

var _ fabricrunner.EventStore = (*Store)(nil)
