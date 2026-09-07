package sqlite_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/agincgit/fabricrunner"
	storesqlite "github.com/agincgit/fabricrunner/store/sqlite"
)

func TestOpenConfiguresSQLiteAndSecureFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "fabricrunner.db")
	store, err := storesqlite.OpenWithOptions(context.Background(), path, storesqlite.Options{
		BusyTimeout:        2 * time.Second,
		MaxOpenConnections: 8,
	})
	if err != nil {
		t.Fatalf("OpenWithOptions() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	diagnostics, err := store.Diagnostics(context.Background())
	if err != nil {
		t.Fatalf("Diagnostics() error = %v", err)
	}
	if diagnostics.JournalMode != "wal" || !diagnostics.ForeignKeys ||
		diagnostics.BusyTimeout != 2*time.Second || diagnostics.SchemaVersion != 1 {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if permissions := info.Mode().Perm(); permissions&0o077 != 0 {
			t.Fatalf("database permissions = %04o, want owner-only", permissions)
		}
	}

	var waitGroup sync.WaitGroup
	errorsChannel := make(chan error, 16)
	for range 16 {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			settings, err := store.Diagnostics(context.Background())
			if err == nil && (!settings.ForeignKeys || settings.JournalMode != "wal") {
				err = errors.New("connection did not inherit required SQLite settings")
			}
			errorsChannel <- err
		}()
	}
	waitGroup.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestAppendLoadRestartAndProjection(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "fabricrunner.db")
	aggregate := testWorkloadAggregate(t)
	sessionID := testID(t)
	stepID := testID(t)
	causationID := testID(t)
	correlationID := testID(t)
	attemptID := testID(t)

	store, err := storesqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	drafts := []fabricrunner.EventDraft{
		testDraft(t, fabricrunner.EventTypeWorkloadCreated, fabricrunner.WorkloadCreatedPayload{
			SessionID: sessionID,
			Goal:      "survive restart",
			Budget:    fabricrunner.Budget{MaxModelCalls: 3},
		}),
		testDraft(t, fabricrunner.EventTypeWorkloadTransitioned, fabricrunner.WorkloadTransitionedPayload{
			From: fabricrunner.WorkloadCreated,
			To:   fabricrunner.WorkloadRunning,
		}),
		testDraft(t, fabricrunner.EventTypeStepCreated, fabricrunner.StepCreatedPayload{
			ID:      stepID,
			Kind:    fabricrunner.StepModelTurn,
			Attempt: 1,
			Requirements: fabricrunner.Requirements{
				AllowedZones: []fabricrunner.Zone{fabricrunner.ZonePersonal},
			},
		}),
	}
	drafts[0].CausationID = causationID
	drafts[0].CorrelationID = correlationID
	drafts[0].AttemptID = attemptID

	created, err := store.Append(context.Background(), aggregate, 0, drafts...)
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	wantEvents := cloneTestEvents(created)
	created[0].Payload[0] = '['

	projectionBefore, err := store.LoadWorkloadProjection(context.Background(), aggregate)
	if err != nil {
		t.Fatalf("LoadWorkloadProjection() error = %v", err)
	}
	projectionBefore.Steps[stepID] = fabricrunner.Step{State: fabricrunner.StepFailed}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = storesqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("reopen error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	loaded, err := store.Load(context.Background(), aggregate, 0)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(loaded, wantEvents) {
		t.Fatalf("events after restart differ:\ngot  = %#v\nwant = %#v", loaded, wantEvents)
	}
	if loaded[0].CausationID != causationID || loaded[0].CorrelationID != correlationID ||
		loaded[0].AttemptID != attemptID {
		t.Fatalf("causal identifiers were not preserved: %#v", loaded[0])
	}

	afterSecond, err := store.Load(context.Background(), aggregate, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterSecond) != 1 || afterSecond[0].Sequence != 3 {
		t.Fatalf("Load(after=2) = %#v", afterSecond)
	}

	persisted, err := store.LoadWorkloadProjection(context.Background(), aggregate)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := fabricrunner.LoadWorkloadProjection(context.Background(), store, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(persisted, replayed) {
		t.Fatalf("persisted projection differs from replay:\npersisted = %#v\nreplayed = %#v", persisted, replayed)
	}
	if persisted.Steps[stepID].State != fabricrunner.StepPending {
		t.Fatalf("caller mutation changed persisted projection: %#v", persisted.Steps[stepID])
	}
}

func TestConcurrentHandlesHaveOneVersionWinner(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "fabricrunner.db")
	first, err := storesqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := storesqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })

	aggregate := testWorkloadAggregate(t)
	draft := testDraft(t, fabricrunner.EventTypeWorkloadCreated, fabricrunner.WorkloadCreatedPayload{
		SessionID: testID(t),
		Goal:      "one writer wins",
	})

	start := make(chan struct{})
	results := make(chan error, 2)
	var waitGroup sync.WaitGroup
	for _, store := range []*storesqlite.Store{first, second} {
		waitGroup.Add(1)
		go func(store *storesqlite.Store) {
			defer waitGroup.Done()
			<-start
			_, err := store.Append(context.Background(), aggregate, 0, draft)
			results <- err
		}(store)
	}
	close(start)
	waitGroup.Wait()
	close(results)

	var successes, conflicts int
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, fabricrunner.ErrVersionConflict):
			conflicts++
			var conflict *fabricrunner.VersionConflictError
			if !errors.As(err, &conflict) || conflict.Actual != 1 {
				t.Fatalf("version conflict = %#v", err)
			}
		default:
			t.Fatalf("unexpected append error = %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes = %d, conflicts = %d; want 1 and 1", successes, conflicts)
	}
}

func TestProjectionFailureRollsBackEntireAppend(t *testing.T) {
	t.Parallel()

	store, err := storesqlite.Open(context.Background(), filepath.Join(t.TempDir(), "fabricrunner.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	aggregate := testWorkloadAggregate(t)
	created := testDraft(t, fabricrunner.EventTypeWorkloadCreated, fabricrunner.WorkloadCreatedPayload{
		SessionID: testID(t),
		Goal:      "rollback invalid state",
	})
	if _, err := store.Append(context.Background(), aggregate, 0, created); err != nil {
		t.Fatal(err)
	}
	before, err := store.LoadWorkloadProjection(context.Background(), aggregate)
	if err != nil {
		t.Fatal(err)
	}

	invalid := testDraft(t, fabricrunner.EventTypeWorkloadTransitioned, fabricrunner.WorkloadTransitionedPayload{
		From: fabricrunner.WorkloadCreated,
		To:   fabricrunner.WorkloadSucceeded,
	})
	if _, err := store.Append(context.Background(), aggregate, 1, invalid); !errors.Is(err, fabricrunner.ErrInvalidTransition) {
		t.Fatalf("Append(invalid transition) error = %v, want ErrInvalidTransition", err)
	}

	events, err := store.Load(context.Background(), aggregate, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("rollback left %d events, want 1", len(events))
	}
	after, err := store.LoadWorkloadProjection(context.Background(), aggregate)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("projection changed after rollback:\nbefore = %#v\nafter = %#v", before, after)
	}

	valid := testDraft(t, fabricrunner.EventTypeWorkloadTransitioned, fabricrunner.WorkloadTransitionedPayload{
		From: fabricrunner.WorkloadCreated,
		To:   fabricrunner.WorkloadRunning,
	})
	if _, err := store.Append(context.Background(), aggregate, 1, valid); err != nil {
		t.Fatalf("version claim was not rolled back: %v", err)
	}
}

func TestCancellationNeverCreatesOrPartiallyCommits(t *testing.T) {
	t.Parallel()

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	path := filepath.Join(t.TempDir(), "cancelled.db")
	if _, err := storesqlite.Open(cancelled, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("Open(cancelled) error = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled Open created a database: %v", err)
	}

	store, err := storesqlite.Open(context.Background(), filepath.Join(t.TempDir(), "fabricrunner.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	aggregate := testWorkloadAggregate(t)
	draft := testDraft(t, fabricrunner.EventTypeWorkloadCreated, fabricrunner.WorkloadCreatedPayload{
		SessionID: testID(t),
		Goal:      "cancel cleanly",
	})
	if _, err := store.Append(cancelled, aggregate, 0, draft); !errors.Is(err, context.Canceled) {
		t.Fatalf("Append(cancelled) error = %v, want context.Canceled", err)
	}
	events, err := store.Load(context.Background(), aggregate, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("cancelled append stored %d events", len(events))
	}
	if _, err := store.Load(cancelled, aggregate, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("Load(cancelled) error = %v, want context.Canceled", err)
	}
}

func TestAppendDeadlineWhileDatabaseIsLockedDoesNotCommit(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "fabricrunner.db")
	store, err := storesqlite.OpenWithOptions(context.Background(), path, storesqlite.Options{
		BusyTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	locker, err := sql.Open("sqlite", path+"?_txlock=immediate&_busy_timeout=5000")
	if err != nil {
		t.Fatal(err)
	}
	defer locker.Close()
	lockTransaction, err := locker.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer lockTransaction.Rollback()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	aggregate := testWorkloadAggregate(t)
	draft := testDraft(t, fabricrunner.EventTypeWorkloadCreated, fabricrunner.WorkloadCreatedPayload{
		SessionID: testID(t),
		Goal:      "deadline under lock",
	})
	if _, err := store.Append(ctx, aggregate, 0, draft); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Append(locked) error = %v, want context.DeadlineExceeded", err)
	}

	if err := lockTransaction.Rollback(); err != nil {
		t.Fatal(err)
	}
	events, err := store.Load(context.Background(), aggregate, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("deadline append stored %d events", len(events))
	}
}

func TestFutureSchemaIsRejectedWithoutModification(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "future.db")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`PRAGMA user_version = 99`); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := storesqlite.Open(context.Background(), path); !errors.Is(err, storesqlite.ErrFutureSchema) {
		t.Fatalf("Open(future schema) error = %v, want ErrFutureSchema", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("future database bytes changed during rejected open")
	}
	database, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var version int
	if err := database.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 99 {
		t.Fatalf("future schema changed to %d", version)
	}
}

func TestCorruptEventAndProjectionFailClosed(t *testing.T) {
	t.Parallel()

	t.Run("event payload", func(t *testing.T) {
		path, store, aggregate := populatedStore(t)
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		rawDatabase, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := rawDatabase.Exec(`UPDATE events SET payload = '{"changed":true}' WHERE sequence = 1`); err != nil {
			t.Fatal(err)
		}
		if err := rawDatabase.Close(); err != nil {
			t.Fatal(err)
		}
		store, err = storesqlite.Open(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		if _, err := store.Load(context.Background(), aggregate, 0); !errors.Is(err, storesqlite.ErrCorruptStore) {
			t.Fatalf("Load(corrupt event) error = %v, want ErrCorruptStore", err)
		}
	})

	t.Run("projection document", func(t *testing.T) {
		path, store, aggregate := populatedStore(t)
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		rawDatabase, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := rawDatabase.Exec(`UPDATE workload_projections SET document = '{}'`); err != nil {
			t.Fatal(err)
		}
		if err := rawDatabase.Close(); err != nil {
			t.Fatal(err)
		}
		store, err = storesqlite.Open(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		if _, err := store.LoadWorkloadProjection(context.Background(), aggregate); !errors.Is(err, storesqlite.ErrCorruptStore) {
			t.Fatalf("LoadWorkloadProjection(corrupt) error = %v, want ErrCorruptStore", err)
		}
	})
}

func populatedStore(t *testing.T) (string, *storesqlite.Store, fabricrunner.AggregateRef) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fabricrunner.db")
	store, err := storesqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	aggregate := testWorkloadAggregate(t)
	draft := testDraft(t, fabricrunner.EventTypeWorkloadCreated, fabricrunner.WorkloadCreatedPayload{
		SessionID: testID(t),
		Goal:      "detect corruption",
	})
	if _, err := store.Append(context.Background(), aggregate, 0, draft); err != nil {
		store.Close()
		t.Fatal(err)
	}
	return path, store, aggregate
}

func testDraft(t *testing.T, eventType string, payload any) fabricrunner.EventDraft {
	t.Helper()
	draft, err := fabricrunner.NewCoreEventDraft(eventType, payload)
	if err != nil {
		t.Fatal(err)
	}
	return draft
}

func testWorkloadAggregate(t *testing.T) fabricrunner.AggregateRef {
	t.Helper()
	return fabricrunner.AggregateRef{Type: fabricrunner.WorkloadAggregateType, ID: testID(t)}
}

func testID(t *testing.T) fabricrunner.ID {
	t.Helper()
	id, err := fabricrunner.NewID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func cloneTestEvents(events []fabricrunner.Event) []fabricrunner.Event {
	cloned := make([]fabricrunner.Event, len(events))
	copy(cloned, events)
	for index := range cloned {
		cloned[index].Payload = append(json.RawMessage(nil), events[index].Payload...)
	}
	return cloned
}
