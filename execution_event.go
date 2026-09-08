package fabricrunner

import (
	"errors"
	"fmt"
)

const EventTypeExecutionRecorded = "execution.recorded"

type ExecutionStage string

const (
	ExecutionPolicyEvaluated ExecutionStage = "policy"
	ExecutionRoutingDecided  ExecutionStage = "routing"
	ExecutionLoopEvent       ExecutionStage = "loop"
	ExecutionFinished        ExecutionStage = "finished"
	ExecutionSandbox         ExecutionStage = "sandbox"
)

type RoutingRecord struct {
	Request  RoutingRequest  `json:"request"`
	Decision RoutingDecision `json:"decision"`
}

// ExecutionRecord is part of the durable workload stream, never telemetry.
// A turn number binds policy, placement and loop activity to one model turn.
type ExecutionRecord struct {
	StepID         ID               `json:"step_id"`
	Turn           int              `json:"turn"`
	Stage          ExecutionStage   `json:"stage"`
	Classification Classification   `json:"classification"`
	Manifest       *ContentManifest `json:"manifest,omitempty"`
	Verdict        *PolicyVerdict   `json:"verdict,omitempty"`
	Routing        *RoutingRecord   `json:"routing,omitempty"`
	Loop           *LoopEvent       `json:"loop,omitempty"`
	Result         *LoopResult      `json:"result,omitempty"`
	FailureCode    string           `json:"failure_code,omitempty"`
	Sandbox        *SandboxEvent    `json:"sandbox,omitempty"`
}

func (p *WorkloadProjection) applyExecutionRecorded(event Event) error {
	var r ExecutionRecord
	if err := decodeProjectionPayload(event.Payload, &r); err != nil {
		return err
	}
	if p.Workload.State != WorkloadRunning {
		return errors.New("execution record requires a running workload")
	}
	step, ok := p.Steps[r.StepID]
	if !ok || step.State != StepRunning {
		return errors.New("execution record requires a running step")
	}
	if err := event.AttemptID.Validate(); err != nil {
		return err
	}
	if event.CorrelationID != p.Workload.ID {
		return errors.New("execution correlation does not match workload")
	}
	var previousLoop *LoopEvent
	var lastRouting *RoutingRecord
	for _, previous := range p.Execution {
		if previous.StepID != r.StepID {
			continue
		}
		if previous.Stage == ExecutionFinished {
			return errors.New("execution already finished")
		}
		if previous.Loop != nil {
			previousLoop = previous.Loop
		}
		if previous.Routing != nil && previous.Turn == r.Turn {
			lastRouting = previous.Routing
		}
	}
	if r.Turn < 0 {
		return errors.New("negative turn")
	}
	if err := r.Classification.Validate(); err != nil {
		return err
	}
	count := 0
	for _, set := range []bool{r.Verdict != nil, r.Routing != nil, r.Loop != nil, r.Result != nil, r.Sandbox != nil} {
		if set {
			count++
		}
	}
	if count != 1 {
		return errors.New("execution record must have exactly one payload")
	}
	switch r.Stage {
	case ExecutionSandbox:
		if r.Sandbox == nil || r.Sandbox.Backend == "" {
			return errors.New("sandbox record requires backend")
		}
		switch r.Sandbox.Phase {
		case "established", "denied", "teardown", "execution_completed", "execution_failed":
		default:
			return errors.New("invalid sandbox lifecycle phase")
		}
	case ExecutionPolicyEvaluated:
		if r.Verdict == nil || r.Manifest == nil {
			return errors.New("policy record requires verdict and manifest")
		}
		if err := r.Verdict.Validate(); err != nil {
			return err
		}
		digest, err := r.Manifest.Digest()
		if err != nil {
			return err
		}
		if digest != r.Verdict.ManifestSHA256 || r.Verdict.Scope.WorkloadID != p.Workload.ID || r.Verdict.Scope.StepID != r.StepID {
			return errors.New("policy record scope or manifest mismatch")
		}
		if r.Verdict.Scope.Kind == PolicyScopeExecution && r.Verdict.Scope.AttemptID != event.AttemptID {
			return errors.New("policy attempt mismatch")
		}
	case ExecutionRoutingDecided:
		if r.Routing == nil || r.Turn < 1 {
			return errors.New("routing record requires a turn and decision")
		}
		if err := r.Routing.Decision.Validate(r.Routing.Request); err != nil {
			return err
		}
		if r.Routing.Request.WorkloadID != p.Workload.ID || r.Routing.Request.StepID != r.StepID || r.Routing.Request.AttemptID != event.AttemptID {
			return errors.New("routing identity mismatch")
		}
	case ExecutionLoopEvent:
		if r.Loop == nil || r.Loop.Sequence < 1 || r.Loop.Turn != r.Turn {
			return errors.New("invalid recorded loop event")
		}
		wantSequence := uint64(1)
		if previousLoop != nil {
			wantSequence = previousLoop.Sequence + 1
			if previousLoop.Type == LoopEventStopped {
				return errors.New("loop already stopped")
			}
		}
		if r.Loop.Sequence != wantSequence {
			return errors.New("recorded loop sequence gap")
		}
		if r.Loop.Type != LoopEventStopped {
			if lastRouting == nil || lastRouting.Decision.SelectedCandidateID.IsZero() {
				return errors.New("loop activity lacks routing decision")
			}
			matched := false
			for _, c := range lastRouting.Request.Candidates {
				if c.ID == lastRouting.Decision.SelectedCandidateID && c.Model.Ref == r.Loop.Model {
					matched = true
				}
			}
			if !matched {
				return errors.New("loop model differs from selected placement")
			}
		}
		switch r.Loop.Type {
		case LoopEventTurnStarted, LoopEventModel, LoopEventMessageAppended, LoopEventToolStarted, LoopEventToolCompleted, LoopEventStopped:
		default:
			return errors.New("unknown recorded loop event")
		}
		if r.Loop.ModelEvent != nil {
			if err := r.Loop.ModelEvent.Validate(); err != nil {
				return err
			}
		}
		if r.Loop.Type == LoopEventModel && r.Loop.ModelEvent == nil {
			return errors.New("model event payload missing")
		}
		if r.Loop.Type == LoopEventMessageAppended && r.Loop.Message == nil {
			return errors.New("message event payload missing")
		}
		if (r.Loop.Type == LoopEventToolStarted || r.Loop.Type == LoopEventToolCompleted) && r.Loop.ToolCall == nil {
			return errors.New("tool event payload missing")
		}
		if r.Loop.Message != nil {
			if err := r.Loop.Message.Validate(); err != nil {
				return err
			}
		}
	case ExecutionFinished:
		if r.Result == nil {
			return errors.New("finished record requires result")
		}
		if err := r.Result.Usage.Validate(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown execution stage %q", r.Stage)
	}
	if r.Stage != ExecutionPolicyEvaluated && r.Manifest != nil {
		return errors.New("unexpected manifest")
	}
	p.Execution = append(p.Execution, r)
	return nil
}
