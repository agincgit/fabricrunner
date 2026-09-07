package memory_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/agincgit/fabricrunner"
	"github.com/agincgit/fabricrunner/store/memory"
)

func TestAppendLoadAndReplay(t *testing.T) {
	t.Parallel()

	store := memory.New()
	aggregate := newAggregate(t)
	drafts := []fabricrunner.EventDraft{
		{Type: "workload.created", SchemaVersion: 1, Payload: json.RawMessage(`{"state":"created"}`)},
		{Type: "workload.started", SchemaVersion: 1, Payload: json.RawMessage(`{"state":"running"}`)},
	}

	created, err := store.Append(context.Background(), aggregate, 0, drafts...)
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if len(created) != 2 || created[0].Sequence != 1 || created[1].Sequence != 2 {
		t.Fatalf("created sequences = %#v", created)
	}

	var got []string
	if err := fabricrunner.Replay(context.Background(), store, aggregate, func(event fabricrunner.Event) error {
		got = append(got, event.Type)
		return nil
	}); err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	if len(got) != 2 || got[0] != drafts[0].Type || got[1] != drafts[1].Type {
		t.Fatalf("replayed event types = %v", got)
	}
}

func TestAppendUsesOptimisticConcurrency(t *testing.T) {
	t.Parallel()

	store := memory.New()
	aggregate := newAggregate(t)
	draft := fabricrunner.EventDraft{Type: "step.created", SchemaVersion: 1, Payload: json.RawMessage(`{}`)}
	if _, err := store.Append(context.Background(), aggregate, 0, draft); err != nil {
		t.Fatal(err)
	}
	_, err := store.Append(context.Background(), aggregate, 0, draft)
	if !errors.Is(err, fabricrunner.ErrVersionConflict) {
		t.Fatalf("Append() error = %v, want ErrVersionConflict", err)
	}
}

func TestLoadReturnsCopies(t *testing.T) {
	t.Parallel()

	store := memory.New()
	aggregate := newAggregate(t)
	draft := fabricrunner.EventDraft{Type: "step.created", SchemaVersion: 1, Payload: json.RawMessage(`{"safe":true}`)}
	if _, err := store.Append(context.Background(), aggregate, 0, draft); err != nil {
		t.Fatal(err)
	}

	first, err := store.Load(context.Background(), aggregate, 0)
	if err != nil {
		t.Fatal(err)
	}
	first[0].Payload[0] = '['
	second, err := store.Load(context.Background(), aggregate, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(second[0].Payload) {
		t.Fatal("caller mutated persisted payload")
	}
}

func TestAppendReturnsCopies(t *testing.T) {
	t.Parallel()

	store := memory.New()
	aggregate := newAggregate(t)
	created, err := store.Append(context.Background(), aggregate, 0, fabricrunner.EventDraft{
		Type:          "step.created",
		SchemaVersion: 1,
		Payload:       json.RawMessage(`{"safe":true}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	created[0].Payload[0] = '['

	loaded, err := store.Load(context.Background(), aggregate, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(loaded[0].Payload) {
		t.Fatal("caller mutated persisted payload through Append result")
	}
}

func TestAppendBatchIsAtomicWhenDraftIsInvalid(t *testing.T) {
	t.Parallel()

	store := memory.New()
	aggregate := newAggregate(t)
	_, err := store.Append(
		context.Background(),
		aggregate,
		0,
		fabricrunner.EventDraft{Type: "workload.created", SchemaVersion: 1, Payload: json.RawMessage(`{}`)},
		fabricrunner.EventDraft{Type: "workload.started", SchemaVersion: 1, Payload: json.RawMessage(`{`)},
	)
	if err == nil {
		t.Fatal("Append() accepted a batch containing an invalid draft")
	}
	loaded, loadErr := store.Load(context.Background(), aggregate, 0)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if len(loaded) != 0 {
		t.Fatalf("invalid batch appended %d events", len(loaded))
	}
}

func TestConcurrentWritersHaveSingleWinner(t *testing.T) {
	t.Parallel()

	store := memory.New()
	aggregate := newAggregate(t)
	draft := fabricrunner.EventDraft{Type: "step.created", SchemaVersion: 1, Payload: json.RawMessage(`{}`)}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := store.Append(context.Background(), aggregate, 0, draft)
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	var successes, conflicts int
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, fabricrunner.ErrVersionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected Append() error = %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes = %d, conflicts = %d; want 1 and 1", successes, conflicts)
	}
}

func newAggregate(t *testing.T) fabricrunner.AggregateRef {
	t.Helper()
	id, err := fabricrunner.NewID()
	if err != nil {
		t.Fatal(err)
	}
	return fabricrunner.AggregateRef{Type: "workload", ID: id}
}
