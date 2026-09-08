package fabricrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	WorkloadAggregateType = "workload"
	CoreEventSchemaV1     = uint32(1)

	EventTypeWorkloadCreated      = "workload.created"
	EventTypeWorkloadTransitioned = "workload.transitioned"
	EventTypeStepCreated          = "step.created"
	EventTypeStepTransitioned     = "step.transitioned"
)

var (
	ErrAggregateNotFound      = errors.New("aggregate not found")
	ErrInvalidProjectionEvent = errors.New("invalid projection event")
)

type WorkloadCreatedPayload struct {
	SessionID ID     `json:"session_id"`
	Goal      string `json:"goal"`
	Budget    Budget `json:"budget"`
}

type WorkloadTransitionedPayload struct {
	From WorkloadState `json:"from"`
	To   WorkloadState `json:"to"`
}

type StepCreatedPayload struct {
	ID             ID           `json:"id"`
	ParentStepID   ID           `json:"parent_step_id,omitempty"`
	Kind           StepKind     `json:"kind"`
	Requirements   Requirements `json:"requirements"`
	Budget         Budget       `json:"budget"`
	Attempt        int          `json:"attempt"`
	IdempotencyKey string       `json:"idempotency_key,omitempty"`
}

type StepTransitionedPayload struct {
	StepID ID        `json:"step_id"`
	From   StepState `json:"from"`
	To     StepState `json:"to"`
}

// WorkloadProjection is the current state reconstructed from one workload
// aggregate stream. Construct it with NewWorkloadProjection before applying
// events.
type WorkloadProjection struct {
	Aggregate AggregateRef
	Version   uint64
	Workload  Workload
	Steps     map[ID]Step
	Execution []ExecutionRecord
}

func NewWorkloadProjection(aggregate AggregateRef) (*WorkloadProjection, error) {
	if err := aggregate.Validate(); err != nil {
		return nil, err
	}
	if aggregate.Type != WorkloadAggregateType {
		return nil, fmt.Errorf("workload projection requires aggregate type %q", WorkloadAggregateType)
	}
	return &WorkloadProjection{
		Aggregate: aggregate,
		Steps:     make(map[ID]Step),
	}, nil
}

// NewCoreEventDraft serializes a version-one core event payload.
func NewCoreEventDraft(eventType string, payload any) (EventDraft, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return EventDraft{}, fmt.Errorf("marshal %s payload: %w", eventType, err)
	}
	draft := EventDraft{
		Type:          eventType,
		SchemaVersion: CoreEventSchemaV1,
		Payload:       encoded,
	}
	if err := draft.Validate(); err != nil {
		return EventDraft{}, err
	}
	return draft, nil
}

// LoadWorkloadProjection replays the complete workload stream without
// contacting providers or executing tools.
func LoadWorkloadProjection(
	ctx context.Context,
	store EventStore,
	aggregate AggregateRef,
) (*WorkloadProjection, error) {
	if store == nil {
		return nil, errors.New("event store is nil")
	}
	projection, err := NewWorkloadProjection(aggregate)
	if err != nil {
		return nil, err
	}
	if err := Replay(ctx, store, aggregate, projection.Apply); err != nil {
		return nil, err
	}
	if projection.Version == 0 {
		return nil, fmt.Errorf("%w: %s %s", ErrAggregateNotFound, aggregate.Type, aggregate.ID)
	}
	return projection, nil
}

// Apply validates and applies one event. Any error leaves the projection
// unchanged.
func (p *WorkloadProjection) Apply(event Event) error {
	if p == nil {
		return fmt.Errorf("%w: projection is nil", ErrInvalidProjectionEvent)
	}
	if err := p.Aggregate.Validate(); err != nil {
		return fmt.Errorf("%w: projection aggregate: %w", ErrInvalidProjectionEvent, err)
	}
	if p.Aggregate.Type != WorkloadAggregateType {
		return fmt.Errorf("%w: projection aggregate type %q", ErrInvalidProjectionEvent, p.Aggregate.Type)
	}
	if err := event.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidProjectionEvent, err)
	}
	if event.Aggregate != p.Aggregate {
		return fmt.Errorf("%w: event aggregate does not match projection", ErrInvalidProjectionEvent)
	}
	if event.Sequence != p.Version+1 {
		return fmt.Errorf(
			"%w: event sequence %d, want %d",
			ErrInvalidProjectionEvent,
			event.Sequence,
			p.Version+1,
		)
	}
	if event.SchemaVersion != CoreEventSchemaV1 {
		return fmt.Errorf(
			"%w: schema version %d is unsupported",
			ErrInvalidProjectionEvent,
			event.SchemaVersion,
		)
	}

	var err error
	switch event.Type {
	case EventTypeWorkloadCreated:
		err = p.applyWorkloadCreated(event)
	case EventTypeWorkloadTransitioned:
		err = p.applyWorkloadTransitioned(event)
	case EventTypeStepCreated:
		err = p.applyStepCreated(event)
	case EventTypeStepTransitioned:
		err = p.applyStepTransitioned(event)
	case EventTypeExecutionRecorded:
		err = p.applyExecutionRecorded(event)
	default:
		err = fmt.Errorf("unknown event type %q", event.Type)
	}
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidProjectionEvent, err)
	}
	p.Version = event.Sequence
	return nil
}

func (p *WorkloadProjection) applyWorkloadCreated(event Event) error {
	if p.Version != 0 || !p.Workload.ID.IsZero() {
		return errors.New("workload already created")
	}
	var payload WorkloadCreatedPayload
	if err := decodeProjectionPayload(event.Payload, &payload); err != nil {
		return err
	}
	if err := payload.SessionID.Validate(); err != nil {
		return fmt.Errorf("session ID: %w", err)
	}
	if strings.TrimSpace(payload.Goal) == "" {
		return errors.New("workload goal is required")
	}
	if err := payload.Budget.Validate(); err != nil {
		return fmt.Errorf("workload budget: %w", err)
	}

	p.Workload = Workload{
		ID:        p.Aggregate.ID,
		SessionID: payload.SessionID,
		Goal:      payload.Goal,
		State:     WorkloadCreated,
		Budget:    payload.Budget,
		CreatedAt: event.OccurredAt,
		UpdatedAt: event.OccurredAt,
	}
	if p.Steps == nil {
		p.Steps = make(map[ID]Step)
	}
	return nil
}

func (p *WorkloadProjection) applyWorkloadTransitioned(event Event) error {
	if p.Workload.ID.IsZero() {
		return errors.New("workload has not been created")
	}
	var payload WorkloadTransitionedPayload
	if err := decodeProjectionPayload(event.Payload, &payload); err != nil {
		return err
	}
	if err := payload.From.Validate(); err != nil {
		return err
	}
	if err := payload.To.Validate(); err != nil {
		return err
	}
	if payload.From != p.Workload.State {
		return fmt.Errorf("workload state is %q, event expects %q", p.Workload.State, payload.From)
	}
	candidate := p.Workload
	if err := candidate.Transition(payload.To, event.OccurredAt); err != nil {
		return err
	}
	p.Workload = candidate
	return nil
}

func (p *WorkloadProjection) applyStepCreated(event Event) error {
	if p.Workload.ID.IsZero() {
		return errors.New("workload has not been created")
	}
	var payload StepCreatedPayload
	if err := decodeProjectionPayload(event.Payload, &payload); err != nil {
		return err
	}
	if err := payload.ID.Validate(); err != nil {
		return fmt.Errorf("step ID: %w", err)
	}
	if _, exists := p.Steps[payload.ID]; exists {
		return fmt.Errorf("step %s already exists", payload.ID)
	}
	if err := payload.Kind.Validate(); err != nil {
		return err
	}
	if err := payload.Requirements.Validate(); err != nil {
		return fmt.Errorf("step requirements: %w", err)
	}
	if err := payload.Budget.Validate(); err != nil {
		return fmt.Errorf("step budget: %w", err)
	}
	if payload.Attempt < 1 {
		return errors.New("step attempt must be positive")
	}
	if !payload.ParentStepID.IsZero() {
		if err := payload.ParentStepID.Validate(); err != nil {
			return fmt.Errorf("parent step ID: %w", err)
		}
		if _, exists := p.Steps[payload.ParentStepID]; !exists {
			return fmt.Errorf("parent step %s does not exist", payload.ParentStepID)
		}
	}

	step := Step{
		ID:             payload.ID,
		WorkloadID:     p.Workload.ID,
		ParentStepID:   payload.ParentStepID,
		Kind:           payload.Kind,
		State:          StepPending,
		Requirements:   cloneRequirements(payload.Requirements),
		Budget:         payload.Budget,
		Attempt:        payload.Attempt,
		IdempotencyKey: payload.IdempotencyKey,
		CreatedAt:      event.OccurredAt,
		UpdatedAt:      event.OccurredAt,
	}
	if p.Steps == nil {
		p.Steps = make(map[ID]Step)
	}
	p.Steps[step.ID] = step
	return nil
}

func (p *WorkloadProjection) applyStepTransitioned(event Event) error {
	if p.Workload.ID.IsZero() {
		return errors.New("workload has not been created")
	}
	var payload StepTransitionedPayload
	if err := decodeProjectionPayload(event.Payload, &payload); err != nil {
		return err
	}
	if err := payload.StepID.Validate(); err != nil {
		return fmt.Errorf("step ID: %w", err)
	}
	if err := payload.From.Validate(); err != nil {
		return err
	}
	if err := payload.To.Validate(); err != nil {
		return err
	}
	step, exists := p.Steps[payload.StepID]
	if !exists {
		return fmt.Errorf("step %s does not exist", payload.StepID)
	}
	if payload.From != step.State {
		return fmt.Errorf("step state is %q, event expects %q", step.State, payload.From)
	}
	if err := step.Transition(payload.To, event.OccurredAt); err != nil {
		return err
	}
	p.Steps[step.ID] = step
	return nil
}

func decodeProjectionPayload(payload json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode event payload: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("event payload contains multiple JSON values")
		}
		return fmt.Errorf("decode trailing event payload: %w", err)
	}
	return nil
}

func cloneRequirements(requirements Requirements) Requirements {
	requirements.Modalities = append([]string(nil), requirements.Modalities...)
	requirements.RequiredTools = append([]string(nil), requirements.RequiredTools...)
	requirements.AllowedZones = append([]Zone(nil), requirements.AllowedZones...)
	return requirements
}
