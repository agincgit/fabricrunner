package fabricrunner

import (
	"errors"
	"fmt"
	"time"
)

type Zone string

const (
	ZonePersonal     Zone = "personal"
	ZoneSelfCloud    Zone = "self_cloud"
	ZoneManagedCloud Zone = "managed_cloud"
)

func (z Zone) Validate() error {
	switch z {
	case ZonePersonal, ZoneSelfCloud, ZoneManagedCloud:
		return nil
	default:
		return fmt.Errorf("unknown zone %q", z)
	}
}

type Classification string

const (
	ClassPublic       Classification = "public"
	ClassInternal     Classification = "internal"
	ClassConfidential Classification = "confidential"
	ClassSecret       Classification = "secret"
)

func (c Classification) Validate() error {
	switch c {
	case ClassPublic, ClassInternal, ClassConfidential, ClassSecret:
		return nil
	default:
		return fmt.Errorf("unknown classification %q", c)
	}
}

// CostMicros is millionths of one US dollar. Integer storage avoids floating
// point errors while keeping sub-cent model pricing representable.
type CostMicros int64

type Budget struct {
	MaxSteps        int           `json:"max_steps"`
	MaxInputTokens  int64         `json:"max_input_tokens"`
	MaxOutputTokens int64         `json:"max_output_tokens"`
	MaxModelCalls   int           `json:"max_model_calls"`
	MaxToolCalls    int           `json:"max_tool_calls"`
	MaxChildSteps   int           `json:"max_child_steps"`
	MaxCost         CostMicros    `json:"max_cost_micros"`
	MaxWallTime     time.Duration `json:"max_wall_time"`
}

func (b Budget) Validate() error {
	if b.MaxSteps < 0 || b.MaxInputTokens < 0 || b.MaxOutputTokens < 0 || b.MaxModelCalls < 0 ||
		b.MaxToolCalls < 0 || b.MaxChildSteps < 0 || b.MaxCost < 0 || b.MaxWallTime < 0 {
		return errors.New("budget values cannot be negative")
	}
	return nil
}

type WorkloadState string

const (
	WorkloadCreated   WorkloadState = "created"
	WorkloadRunning   WorkloadState = "running"
	WorkloadWaiting   WorkloadState = "waiting"
	WorkloadSucceeded WorkloadState = "succeeded"
	WorkloadFailed    WorkloadState = "failed"
	WorkloadCancelled WorkloadState = "cancelled"
)

func (s WorkloadState) Validate() error {
	switch s {
	case WorkloadCreated, WorkloadRunning, WorkloadWaiting, WorkloadSucceeded, WorkloadFailed, WorkloadCancelled:
		return nil
	default:
		return fmt.Errorf("unknown workload state %q", s)
	}
}

type Workload struct {
	ID        ID
	SessionID ID
	Goal      string
	State     WorkloadState
	Budget    Budget
	CreatedAt time.Time
	UpdatedAt time.Time
}

type StepKind string

const (
	StepModelTurn StepKind = "model_turn"
	StepTool      StepKind = "tool"
	StepDelegate  StepKind = "delegate"
	StepTransform StepKind = "transform"
	StepVerify    StepKind = "verify"
)

func (k StepKind) Validate() error {
	switch k {
	case StepModelTurn, StepTool, StepDelegate, StepTransform, StepVerify:
		return nil
	default:
		return fmt.Errorf("unknown step kind %q", k)
	}
}

type StepState string

const (
	StepPending         StepState = "pending"
	StepReady           StepState = "ready"
	StepLeased          StepState = "leased"
	StepRunning         StepState = "running"
	StepWaitingApproval StepState = "waiting_approval"
	StepUncertain       StepState = "uncertain"
	StepSucceeded       StepState = "succeeded"
	StepFailed          StepState = "failed"
	StepCancelled       StepState = "cancelled"
)

func (s StepState) Validate() error {
	switch s {
	case StepPending, StepReady, StepLeased, StepRunning, StepWaitingApproval,
		StepUncertain, StepSucceeded, StepFailed, StepCancelled:
		return nil
	default:
		return fmt.Errorf("unknown step state %q", s)
	}
}

type Requirements struct {
	Modalities       []string `json:"modalities,omitempty"`
	RequiredTools    []string `json:"required_tools,omitempty"`
	MinContextTokens int      `json:"min_context_tokens,omitempty"`
	MinQualityClass  string   `json:"min_quality_class,omitempty"`
	AllowedZones     []Zone   `json:"allowed_zones,omitempty"`
	NativeToolUse    bool     `json:"native_tool_use,omitempty"`
	StructuredOutput bool     `json:"structured_output,omitempty"`
}

func (r Requirements) Validate() error {
	if r.MinContextTokens < 0 {
		return errors.New("minimum context tokens cannot be negative")
	}
	for i, zone := range r.AllowedZones {
		if err := zone.Validate(); err != nil {
			return fmt.Errorf("allowed zone %d: %w", i, err)
		}
	}
	return nil
}

type Step struct {
	ID             ID
	WorkloadID     ID
	ParentStepID   ID
	Kind           StepKind
	State          StepState
	Requirements   Requirements
	Budget         Budget
	Attempt        int
	IdempotencyKey string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

var ErrInvalidTransition = errors.New("invalid state transition")

func CanTransitionWorkload(from, to WorkloadState) bool {
	switch from {
	case WorkloadCreated:
		return to == WorkloadRunning || to == WorkloadCancelled
	case WorkloadRunning:
		return to == WorkloadWaiting || to == WorkloadSucceeded || to == WorkloadFailed || to == WorkloadCancelled
	case WorkloadWaiting:
		return to == WorkloadRunning || to == WorkloadFailed || to == WorkloadCancelled
	default:
		return false
	}
}

func CanTransitionStep(from, to StepState) bool {
	switch from {
	case StepPending:
		return to == StepReady || to == StepCancelled
	case StepReady:
		return to == StepLeased || to == StepFailed || to == StepCancelled
	case StepLeased:
		return to == StepRunning || to == StepReady || to == StepUncertain || to == StepCancelled
	case StepRunning:
		return to == StepSucceeded || to == StepFailed || to == StepWaitingApproval || to == StepUncertain || to == StepCancelled
	case StepWaitingApproval:
		return to == StepRunning || to == StepFailed || to == StepCancelled
	case StepUncertain:
		return to == StepSucceeded || to == StepFailed || to == StepReady || to == StepCancelled
	default:
		return false
	}
}

func (w *Workload) Transition(to WorkloadState, at time.Time) error {
	if !CanTransitionWorkload(w.State, to) {
		return fmt.Errorf("%w: workload %s to %s", ErrInvalidTransition, w.State, to)
	}
	w.State = to
	w.UpdatedAt = at.UTC()
	return nil
}

func (s *Step) Transition(to StepState, at time.Time) error {
	if !CanTransitionStep(s.State, to) {
		return fmt.Errorf("%w: step %s to %s", ErrInvalidTransition, s.State, to)
	}
	s.State = to
	s.UpdatedAt = at.UTC()
	return nil
}
