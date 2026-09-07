package fabricrunner

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestTerminalWorkloadStatesCannotTransition(t *testing.T) {
	t.Parallel()

	for _, state := range []WorkloadState{WorkloadSucceeded, WorkloadFailed, WorkloadCancelled} {
		if CanTransitionWorkload(state, WorkloadRunning) {
			t.Errorf("terminal workload state %q transitioned to running", state)
		}
	}
}

func TestWorkloadTransition(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, time.September, 7, 1, 2, 3, 0, time.FixedZone("test", -5*60*60))
	w := Workload{State: WorkloadCreated}
	if err := w.Transition(WorkloadRunning, at); err != nil {
		t.Fatalf("Transition() error = %v", err)
	}
	if w.State != WorkloadRunning {
		t.Fatalf("state = %q, want %q", w.State, WorkloadRunning)
	}
	if !w.UpdatedAt.Equal(at) || w.UpdatedAt.Location() != time.UTC {
		t.Fatalf("updated time = %v, want UTC %v", w.UpdatedAt, at.UTC())
	}

	beforeInvalid := w
	if err := w.Transition(WorkloadCreated, at.Add(time.Minute)); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("invalid Transition() error = %v, want ErrInvalidTransition", err)
	}
	if w != beforeInvalid {
		t.Fatalf("invalid transition mutated workload: before = %#v, after = %#v", beforeInvalid, w)
	}
}

func TestUncertainStepRequiresResolution(t *testing.T) {
	t.Parallel()

	if !CanTransitionStep(StepRunning, StepUncertain) {
		t.Fatal("running step cannot become uncertain")
	}
	if CanTransitionStep(StepUncertain, StepRunning) {
		t.Fatal("uncertain step resumed without resolution")
	}
	if !CanTransitionStep(StepUncertain, StepReady) {
		t.Fatal("uncertain step cannot be explicitly approved for retry")
	}
}

func TestTerminalStepStatesCannotTransition(t *testing.T) {
	t.Parallel()

	for _, state := range []StepState{StepSucceeded, StepFailed, StepCancelled} {
		step := Step{State: state, UpdatedAt: time.Unix(1, 0)}
		before := step
		if err := step.Transition(StepReady, time.Unix(2, 0)); !errors.Is(err, ErrInvalidTransition) {
			t.Errorf("terminal step state %q error = %v, want ErrInvalidTransition", state, err)
		}
		if !reflect.DeepEqual(step, before) {
			t.Errorf("terminal step state %q mutated", state)
		}
	}
}

func TestBudgetRejectsNegativeValues(t *testing.T) {
	t.Parallel()

	if err := (Budget{MaxCost: -1}).Validate(); err == nil {
		t.Fatal("negative budget accepted")
	}
	if err := (Budget{}).Validate(); err != nil {
		t.Fatalf("zero budget rejected: %v", err)
	}
}
