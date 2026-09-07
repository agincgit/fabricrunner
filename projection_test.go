package fabricrunner_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/agincgit/fabricrunner"
	"github.com/agincgit/fabricrunner/store/memory"
)

func TestLoadWorkloadProjectionReconstructsDeterministically(t *testing.T) {
	t.Parallel()

	aggregate := workloadAggregate(t)
	sessionID := testID(t)
	stepID := testID(t)
	store := memory.New()
	drafts := []fabricrunner.EventDraft{
		coreDraft(t, fabricrunner.EventTypeWorkloadCreated, fabricrunner.WorkloadCreatedPayload{
			SessionID: sessionID,
			Goal:      "route a bounded workload",
			Budget:    fabricrunner.Budget{MaxModelCalls: 4},
		}),
		coreDraft(t, fabricrunner.EventTypeWorkloadTransitioned, fabricrunner.WorkloadTransitionedPayload{
			From: fabricrunner.WorkloadCreated,
			To:   fabricrunner.WorkloadRunning,
		}),
		coreDraft(t, fabricrunner.EventTypeStepCreated, fabricrunner.StepCreatedPayload{
			ID:      stepID,
			Kind:    fabricrunner.StepModelTurn,
			Attempt: 1,
			Requirements: fabricrunner.Requirements{
				AllowedZones:  []fabricrunner.Zone{fabricrunner.ZonePersonal},
				RequiredTools: []string{"read"},
			},
			Budget: fabricrunner.Budget{MaxModelCalls: 1},
		}),
		coreDraft(t, fabricrunner.EventTypeStepTransitioned, fabricrunner.StepTransitionedPayload{
			StepID: stepID,
			From:   fabricrunner.StepPending,
			To:     fabricrunner.StepReady,
		}),
		coreDraft(t, fabricrunner.EventTypeStepTransitioned, fabricrunner.StepTransitionedPayload{
			StepID: stepID,
			From:   fabricrunner.StepReady,
			To:     fabricrunner.StepLeased,
		}),
	}
	if _, err := store.Append(context.Background(), aggregate, 0, drafts...); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	first, err := fabricrunner.LoadWorkloadProjection(context.Background(), store, aggregate)
	if err != nil {
		t.Fatalf("LoadWorkloadProjection() error = %v", err)
	}
	second, err := fabricrunner.LoadWorkloadProjection(context.Background(), store, aggregate)
	if err != nil {
		t.Fatalf("second LoadWorkloadProjection() error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated replay differs:\nfirst = %#v\nsecond = %#v", first, second)
	}
	if first.Version != uint64(len(drafts)) {
		t.Fatalf("version = %d, want %d", first.Version, len(drafts))
	}
	if first.Workload.ID != aggregate.ID || first.Workload.SessionID != sessionID ||
		first.Workload.State != fabricrunner.WorkloadRunning {
		t.Fatalf("workload = %#v", first.Workload)
	}
	step, exists := first.Steps[stepID]
	if !exists {
		t.Fatalf("step %s was not projected", stepID)
	}
	if step.State != fabricrunner.StepLeased || step.WorkloadID != aggregate.ID || step.Attempt != 1 {
		t.Fatalf("step = %#v", step)
	}
}

func TestProjectionRejectsInvalidEventsAtomically(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		wantCause error
		event     func(*testing.T, fabricrunner.AggregateRef, *fabricrunner.WorkloadProjection) fabricrunner.Event
	}{
		{
			name: "sequence gap",
			event: func(t *testing.T, aggregate fabricrunner.AggregateRef, _ *fabricrunner.WorkloadProjection) fabricrunner.Event {
				return coreEvent(t, aggregate, 3, fabricrunner.EventTypeWorkloadTransitioned, fabricrunner.WorkloadTransitionedPayload{
					From: fabricrunner.WorkloadCreated,
					To:   fabricrunner.WorkloadRunning,
				})
			},
		},
		{
			name: "aggregate mismatch",
			event: func(t *testing.T, _ fabricrunner.AggregateRef, _ *fabricrunner.WorkloadProjection) fabricrunner.Event {
				return coreEvent(t, workloadAggregate(t), 2, fabricrunner.EventTypeWorkloadTransitioned, fabricrunner.WorkloadTransitionedPayload{
					From: fabricrunner.WorkloadCreated,
					To:   fabricrunner.WorkloadRunning,
				})
			},
		},
		{
			name: "unsupported schema",
			event: func(t *testing.T, aggregate fabricrunner.AggregateRef, _ *fabricrunner.WorkloadProjection) fabricrunner.Event {
				payload := mustJSON(t, fabricrunner.WorkloadTransitionedPayload{
					From: fabricrunner.WorkloadCreated,
					To:   fabricrunner.WorkloadRunning,
				})
				return materializedEvent(t, aggregate, 2, fabricrunner.EventDraft{
					Type:          fabricrunner.EventTypeWorkloadTransitioned,
					SchemaVersion: 2,
					Payload:       payload,
				})
			},
		},
		{
			name: "unknown event",
			event: func(t *testing.T, aggregate fabricrunner.AggregateRef, _ *fabricrunner.WorkloadProjection) fabricrunner.Event {
				return coreEvent(t, aggregate, 2, "workload.unrecognized", struct{}{})
			},
		},
		{
			name: "unknown payload field",
			event: func(t *testing.T, aggregate fabricrunner.AggregateRef, _ *fabricrunner.WorkloadProjection) fabricrunner.Event {
				return materializedEvent(t, aggregate, 2, fabricrunner.EventDraft{
					Type:          fabricrunner.EventTypeWorkloadTransitioned,
					SchemaVersion: fabricrunner.CoreEventSchemaV1,
					Payload:       json.RawMessage(`{"from":"created","to":"running","ignored":true}`),
				})
			},
		},
		{
			name:      "invalid transition",
			wantCause: fabricrunner.ErrInvalidTransition,
			event: func(t *testing.T, aggregate fabricrunner.AggregateRef, _ *fabricrunner.WorkloadProjection) fabricrunner.Event {
				return coreEvent(t, aggregate, 2, fabricrunner.EventTypeWorkloadTransitioned, fabricrunner.WorkloadTransitionedPayload{
					From: fabricrunner.WorkloadCreated,
					To:   fabricrunner.WorkloadSucceeded,
				})
			},
		},
		{
			name: "duplicate creation",
			event: func(t *testing.T, aggregate fabricrunner.AggregateRef, projection *fabricrunner.WorkloadProjection) fabricrunner.Event {
				return coreEvent(t, aggregate, 2, fabricrunner.EventTypeWorkloadCreated, fabricrunner.WorkloadCreatedPayload{
					SessionID: projection.Workload.SessionID,
					Goal:      "duplicate",
				})
			},
		},
		{
			name: "missing parent",
			event: func(t *testing.T, aggregate fabricrunner.AggregateRef, _ *fabricrunner.WorkloadProjection) fabricrunner.Event {
				return coreEvent(t, aggregate, 2, fabricrunner.EventTypeStepCreated, fabricrunner.StepCreatedPayload{
					ID:           testID(t),
					ParentStepID: testID(t),
					Kind:         fabricrunner.StepDelegate,
					Attempt:      1,
				})
			},
		},
		{
			name: "payload hash mismatch",
			event: func(t *testing.T, aggregate fabricrunner.AggregateRef, _ *fabricrunner.WorkloadProjection) fabricrunner.Event {
				event := coreEvent(t, aggregate, 2, fabricrunner.EventTypeWorkloadTransitioned, fabricrunner.WorkloadTransitionedPayload{
					From: fabricrunner.WorkloadCreated,
					To:   fabricrunner.WorkloadRunning,
				})
				event.Payload[0] = '['
				return event
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			aggregate, projection := createdProjection(t)
			before := cloneProjection(projection)
			err := projection.Apply(test.event(t, aggregate, projection))
			if !errors.Is(err, fabricrunner.ErrInvalidProjectionEvent) {
				t.Fatalf("Apply() error = %v, want ErrInvalidProjectionEvent", err)
			}
			if test.wantCause != nil && !errors.Is(err, test.wantCause) {
				t.Fatalf("Apply() error = %v, want cause %v", err, test.wantCause)
			}
			if !reflect.DeepEqual(projection, before) {
				t.Fatalf("projection mutated after rejected event:\nbefore = %#v\nafter = %#v", before, projection)
			}
		})
	}
}

func TestLoadWorkloadProjectionReturnsNotFound(t *testing.T) {
	t.Parallel()

	_, err := fabricrunner.LoadWorkloadProjection(
		context.Background(),
		memory.New(),
		workloadAggregate(t),
	)
	if !errors.Is(err, fabricrunner.ErrAggregateNotFound) {
		t.Fatalf("LoadWorkloadProjection() error = %v, want ErrAggregateNotFound", err)
	}
}

func TestLoadWorkloadProjectionHonorsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := fabricrunner.LoadWorkloadProjection(ctx, memory.New(), workloadAggregate(t))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("LoadWorkloadProjection() error = %v, want context.Canceled", err)
	}
}

func TestNewWorkloadProjectionRejectsWrongAggregateType(t *testing.T) {
	t.Parallel()

	_, err := fabricrunner.NewWorkloadProjection(fabricrunner.AggregateRef{
		Type: "step",
		ID:   testID(t),
	})
	if err == nil {
		t.Fatal("NewWorkloadProjection() accepted a step aggregate")
	}
}

func createdProjection(t *testing.T) (fabricrunner.AggregateRef, *fabricrunner.WorkloadProjection) {
	t.Helper()
	aggregate := workloadAggregate(t)
	projection, err := fabricrunner.NewWorkloadProjection(aggregate)
	if err != nil {
		t.Fatal(err)
	}
	event := coreEvent(t, aggregate, 1, fabricrunner.EventTypeWorkloadCreated, fabricrunner.WorkloadCreatedPayload{
		SessionID: testID(t),
		Goal:      "test workload",
	})
	if err := projection.Apply(event); err != nil {
		t.Fatalf("Apply(created) error = %v", err)
	}
	return aggregate, projection
}

func cloneProjection(source *fabricrunner.WorkloadProjection) *fabricrunner.WorkloadProjection {
	clone := *source
	clone.Steps = make(map[fabricrunner.ID]fabricrunner.Step, len(source.Steps))
	for id, step := range source.Steps {
		step.Requirements.Modalities = append([]string(nil), step.Requirements.Modalities...)
		step.Requirements.RequiredTools = append([]string(nil), step.Requirements.RequiredTools...)
		step.Requirements.AllowedZones = append([]fabricrunner.Zone(nil), step.Requirements.AllowedZones...)
		clone.Steps[id] = step
	}
	return &clone
}

func coreEvent(
	t *testing.T,
	aggregate fabricrunner.AggregateRef,
	sequence uint64,
	eventType string,
	payload any,
) fabricrunner.Event {
	t.Helper()
	return materializedEvent(t, aggregate, sequence, coreDraft(t, eventType, payload))
}

func coreDraft(t *testing.T, eventType string, payload any) fabricrunner.EventDraft {
	t.Helper()
	draft, err := fabricrunner.NewCoreEventDraft(eventType, payload)
	if err != nil {
		t.Fatalf("NewCoreEventDraft() error = %v", err)
	}
	return draft
}

func materializedEvent(
	t *testing.T,
	aggregate fabricrunner.AggregateRef,
	sequence uint64,
	draft fabricrunner.EventDraft,
) fabricrunner.Event {
	t.Helper()
	event, err := fabricrunner.NewEvent(
		aggregate,
		sequence,
		draft,
		time.Date(2026, time.September, 7, 12, 0, int(sequence), 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("NewEvent() error = %v", err)
	}
	return event
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func workloadAggregate(t *testing.T) fabricrunner.AggregateRef {
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
