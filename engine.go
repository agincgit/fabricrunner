package fabricrunner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var ErrExecutionDenied = errors.New("execution not authorized")

// Engine composes the public single-node contracts. Configure before calling
// Run; dependencies must support concurrent calls if an Engine is shared.
type Engine struct {
	Store            EventStore
	ExecutionPolicy  ExecutionPolicy
	DataEgressPolicy DataEgressPolicy
	Router           Router
	Loop             TurnLoop
	Providers        map[string]Provider
	Observer         Observer
	ObserverTimeout  time.Duration
}

// EngineRequest describes one bounded workload. IDs are caller-owned so a
// duplicate submission conflicts before any effect. Replay never resumes effects.
// Classification labels request metadata and tool definitions; unlabeled data
// defaults to confidential. Tools execute in SourceZone.
type EngineRequest struct {
	WorkloadID                ID
	SessionID                 ID
	StepID                    ID
	AttemptID                 ID
	Goal                      string
	SourceZone                Zone
	Classification            Classification
	ModelOutputClassification Classification
	Initial                   ModelRequest
	Tools                     []ToolBinding
	Requirements              Requirements
	Candidates                []RoutingCandidate
	Budget                    Budget
	Autonomous                bool
	ParallelTools             bool
}

func (e *Engine) Replay(ctx context.Context, id ID) (*WorkloadProjection, error) {
	if e == nil {
		return nil, errors.New("engine is nil")
	}
	return LoadWorkloadProjection(ctx, e.Store, AggregateRef{Type: WorkloadAggregateType, ID: id})
}
func (e *Engine) Run(ctx context.Context, request EngineRequest) (projection *WorkloadProjection, returnErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if e == nil || e.Store == nil || e.Loop == nil || e.Router == nil {
		return nil, errors.New("engine requires store, loop and router")
	}
	if err := request.SessionID.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.Goal) == "" {
		return nil, errors.New("workload goal is required")
	}
	if err := request.SourceZone.Validate(); err != nil {
		return nil, err
	}
	if err := request.Requirements.Validate(); err != nil {
		return nil, err
	}
	request.Requirements = cloneRequirements(request.Requirements)
	if request.Classification == "" {
		request.Classification = ClassConfidential
	}
	if err := request.Classification.Validate(); err != nil {
		return nil, err
	}
	if request.ModelOutputClassification == "" {
		request.ModelOutputClassification = ClassConfidential
	}
	request.Initial = CloneModelRequest(request.Initial)
	for i := range request.Initial.Messages {
		for j := range request.Initial.Messages[i].Content {
			if request.Initial.Messages[i].Content[j].Classification == "" {
				request.Initial.Messages[i].Content[j].Classification = ClassConfidential
			}
		}
	}
	request.Candidates = append([]RoutingCandidate(nil), request.Candidates...)
	for i := range request.Candidates {
		request.Candidates[i] = request.Candidates[i].Clone()
		if err := request.Candidates[i].Validate(); err != nil {
			return nil, err
		}
	}
	if len(request.Candidates) == 0 {
		return nil, errors.New("engine requires routing candidates")
	}
	runCtx, cancel := context.WithTimeoutCause(ctx, request.Budget.MaxWallTime, ErrLoopBudgetExceeded)
	defer cancel()
	runCtx, stop := context.WithCancelCause(runCtx)
	defer stop(nil)
	s := &engineRun{engine: e, request: request, aggregate: AggregateRef{Type: WorkloadAggregateType, ID: request.WorkloadID}, observation: NewObservation(e.Observer, e.ObserverTimeout), stop: stop}
	loopRequest := LoopRequest{WorkloadID: request.WorkloadID, StepID: request.StepID, AttemptID: request.AttemptID, Initial: request.Initial, Tools: request.Tools, Selector: s, Sink: s, Budget: request.Budget, ParallelTools: request.ParallelTools, ModelOutputClassification: request.ModelOutputClassification, ToolErrorClassification: request.Classification}
	if err := loopRequest.Validate(); err != nil {
		return nil, err
	}
	loopRequest = loopRequest.Clone()
	s.request.Tools = loopRequest.Tools
	for i := range loopRequest.Tools {
		loopRequest.Tools[i].Handler = &engineToolHandler{run: s, handler: loopRequest.Tools[i].Handler}
	}
	initial := []engineDraft{
		{EventTypeWorkloadCreated, WorkloadCreatedPayload{SessionID: request.SessionID, Goal: request.Goal, Budget: request.Budget}},
		{EventTypeWorkloadTransitioned, WorkloadTransitionedPayload{From: WorkloadCreated, To: WorkloadRunning}},
		{EventTypeStepCreated, StepCreatedPayload{ID: request.StepID, Kind: StepModelTurn, Requirements: request.Requirements, Budget: request.Budget, Attempt: 1}},
		{EventTypeStepTransitioned, StepTransitionedPayload{StepID: request.StepID, From: StepPending, To: StepReady}},
		{EventTypeStepTransitioned, StepTransitionedPayload{StepID: request.StepID, From: StepReady, To: StepLeased}},
		{EventTypeStepTransitioned, StepTransitionedPayload{StepID: request.StepID, From: StepLeased, To: StepRunning}},
	}
	if err := s.append(runCtx, initial...); err != nil {
		return nil, err
	}
	s.observe(ctx, RecordWorkload, "started")
	s.observe(ctx, RecordStep, "started")
	s.observe(ctx, RecordAttempt, "started")
	result, runErr := e.Loop.Run(runCtx, loopRequest)
	if result.Stop == StopCancelled {
		runErr = errors.Join(runErr, context.Canceled)
	}
	if cause := context.Cause(runCtx); cause != nil {
		runErr = errors.Join(runErr, cause)
		if errors.Is(cause, ErrLoopBudgetExceeded) {
			result.BudgetExceeded = "wall_time"
		}
	}
	state, stepState := WorkloadSucceeded, StepSucceeded
	failure := ""
	if runErr != nil {
		state, stepState = WorkloadFailed, StepFailed
		failure = "execution_failed"
	}
	if ctx.Err() != nil || result.Stop == StopCancelled {
		state, stepState = WorkloadCancelled, StepCancelled
		failure = "cancelled"
	}
	if result.BudgetExceeded != "" {
		failure = "budget_exceeded"
	}
	finalCtx, finalCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finalCancel()
	resultClass := request.Classification
	for _, message := range result.Messages {
		for _, part := range message.Content {
			resultClass = higherClassification(resultClass, part.Classification)
		}
	}
	finish := ExecutionRecord{StepID: request.StepID, Stage: ExecutionFinished, Classification: resultClass, Result: &result, FailureCode: failure}
	err := s.append(finalCtx, engineDraft{EventTypeExecutionRecorded, finish}, engineDraft{EventTypeStepTransitioned, StepTransitionedPayload{StepID: request.StepID, From: StepRunning, To: stepState}}, engineDraft{EventTypeWorkloadTransitioned, WorkloadTransitionedPayload{From: WorkloadRunning, To: state}})
	if err == nil {
		s.observe(ctx, RecordAttempt, string(state))
		s.observe(ctx, RecordStep, string(state))
		s.observe(ctx, RecordWorkload, string(state))
	}
	projection, loadErr := e.Replay(finalCtx, request.WorkloadID)
	return projection, errors.Join(runErr, err, loadErr)
}

type engineDraft struct {
	kind    string
	payload any
}
type engineRun struct {
	engine              *Engine
	request             EngineRequest
	aggregate           AggregateRef
	mu                  sync.Mutex
	sequence            uint64
	cause               ID
	storeErr            error
	observation         *Observation
	observationSequence atomic.Uint64
	stop                context.CancelCauseFunc
}

func (s *engineRun) append(ctx context.Context, entries ...engineDraft) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.storeErr != nil {
		return s.storeErr
	}
	drafts := make([]EventDraft, len(entries))
	for i, entry := range entries {
		d, err := NewCoreEventDraft(entry.kind, entry.payload)
		if err != nil {
			s.storeErr = err
			return err
		}
		d.CausationID = s.cause
		d.CorrelationID = s.request.WorkloadID
		d.AttemptID = s.request.AttemptID
		drafts[i] = d
	}
	events, err := s.engine.Store.Append(ctx, s.aggregate, s.sequence, drafts...)
	if err != nil {
		// Cancellation before append can be finalized using a fresh context.
		// If the store did commit, its version check rejects finalization against
		// our stale sequence. Never retry an external effect or the failed append.
		if ctx.Err() != nil {
			return err
		}
		s.storeErr = err
		return err
	}
	if len(events) != len(drafts) {
		s.storeErr = errors.New("store returned incomplete append")
		return s.storeErr
	}
	if len(events) > 0 {
		s.sequence = events[len(events)-1].Sequence
		s.cause = events[len(events)-1].ID
	}
	return nil
}
func (s *engineRun) observe(ctx context.Context, kind RecordKind, outcome string) {
	if s.observation == nil {
		return
	}
	// These attributes are engine constants, never model content or policy reasons.
	r, _ := NewRecord(RecordInput{Kind: kind, WorkloadID: s.request.WorkloadID, StepID: s.request.StepID, AttemptID: s.request.AttemptID, Sequence: s.observationSequence.Add(1), Classification: s.request.Classification, Attributes: map[AttributeKey]string{AttrOutcome: outcome}})
	s.observation.Emit(ctx, r)
}
func (s *engineRun) record(ctx context.Context, r ExecutionRecord) error {
	r.StepID = s.request.StepID
	if r.Classification == "" {
		r.Classification = s.request.Classification
	}
	return s.append(ctx, engineDraft{EventTypeExecutionRecorded, r})
}
func (s *engineRun) recordPolicy(ctx context.Context, turn int, manifest ContentManifest, v PolicyVerdict) error {
	class, err := manifest.EffectiveClassification()
	if err != nil {
		return err
	}
	if err := s.record(ctx, ExecutionRecord{Turn: turn, Stage: ExecutionPolicyEvaluated, Classification: class, Manifest: &manifest, Verdict: &v}); err != nil {
		return err
	}
	s.observe(ctx, RecordPolicy, string(v.Action))
	return nil
}
func (s *engineRun) SelectTurn(ctx context.Context, turn TurnContext) (TurnSelection, error) {
	modelRequest := CloneModelRequest(s.request.Initial)
	modelRequest.Messages = CloneMessages(turn.Messages)
	modelRequest.Tools = nil
	for _, binding := range s.request.Tools {
		modelRequest.Tools = append(modelRequest.Tools, binding.Definition)
	}
	manifest, err := modelManifest(modelRequest, s.request.Classification)
	if err != nil {
		return TurnSelection{}, err
	}
	digest, _ := manifest.Digest()
	now := time.Now().UTC()
	deadline, _ := ctx.Deadline()
	budget := s.request.Budget
	budget.MaxInputTokens -= turn.Usage.InputTokens
	budget.MaxOutputTokens -= turn.Usage.OutputTokens
	budget.MaxCost -= turn.Usage.Cost
	routing := RoutingRequest{WorkloadID: s.request.WorkloadID, StepID: s.request.StepID, AttemptID: s.request.AttemptID, ManifestSHA256: digest, SourceZone: s.request.SourceZone, Autonomous: s.request.Autonomous, EgressPurpose: "model_request", Requirements: s.request.Requirements, Budget: budget, ObservedAt: now, Deadline: deadline, Weights: DefaultRoutingWeights()}
	for _, original := range s.request.Candidates {
		candidate := original.Clone()
		v, policyErr := EvaluateExecutionPolicy(ctx, s.engine.ExecutionPolicy, ExecutionPolicyRequest{WorkloadID: s.request.WorkloadID, StepID: s.request.StepID, AttemptID: s.request.AttemptID, StepKind: StepModelTurn, Autonomous: s.request.Autonomous, Target: ExecutionTarget{Kind: ExecutionTargetModel, Zone: candidate.Zone, Model: &candidate.Model.Ref}, Manifest: manifest})
		if ctx.Err() != nil {
			return TurnSelection{}, ctx.Err()
		}
		if err := s.recordPolicy(ctx, turn.Turn, manifest, v); err != nil {
			return TurnSelection{}, err
		}
		candidate.ExecutionVerdict = v
		_ = policyErr // A failed evaluator supplies a persisted fail-closed verdict.
		candidate.EgressVerdict = nil
		if candidate.Zone != s.request.SourceZone {
			v, _ := EvaluateDataEgressPolicy(ctx, s.engine.DataEgressPolicy, DataEgressPolicyRequest{WorkloadID: s.request.WorkloadID, StepID: s.request.StepID, Source: s.request.SourceZone, Destination: candidate.Zone, Purpose: routing.EgressPurpose, Manifest: manifest})
			if ctx.Err() != nil {
				return TurnSelection{}, ctx.Err()
			}
			if err := s.recordPolicy(ctx, turn.Turn, manifest, v); err != nil {
				return TurnSelection{}, err
			}
			candidate.EgressVerdict = &v
		}
		routing.Candidates = append(routing.Candidates, candidate)
	}
	decision, routeErr := EvaluateRouter(ctx, s.engine.Router, routing)
	if routeErr != nil && !errors.Is(routeErr, ErrNoEligibleCandidates) {
		return TurnSelection{}, routeErr
	}
	if err := s.record(ctx, ExecutionRecord{Turn: turn.Turn, Stage: ExecutionRoutingDecided, Routing: &RoutingRecord{Request: routing, Decision: decision}}); err != nil {
		return TurnSelection{}, err
	}
	s.observe(ctx, RecordRouting, "decided")
	if routeErr != nil {
		return TurnSelection{}, errors.Join(ErrExecutionDenied, routeErr)
	}
	for _, candidate := range routing.Candidates {
		if candidate.ID == decision.SelectedCandidateID {
			provider := s.engine.Providers[candidate.Model.Ref.Provider]
			if provider == nil {
				return TurnSelection{}, fmt.Errorf("provider %q is unavailable", candidate.Model.Ref.Provider)
			}
			return TurnSelection{Provider: &engineProvider{run: s, provider: provider, zone: candidate.Zone, turn: turn.Turn}, Model: candidate.Model.Ref}, nil
		}
	}
	return TurnSelection{}, errors.New("selected candidate unavailable")
}
func (s *engineRun) RecordLoopEvent(ctx context.Context, event LoopEvent) error {
	class := s.request.Classification
	if event.ModelEvent != nil || event.ToolCall != nil {
		class = higherClassification(class, s.request.ModelOutputClassification)
	}
	if event.ToolOutput != nil {
		class = higherClassification(class, event.ToolOutput.Classification)
	}
	if event.Message != nil {
		for _, part := range event.Message.Content {
			class = higherClassification(class, part.Classification)
		}
	}
	return s.record(ctx, ExecutionRecord{Turn: event.Turn, Stage: ExecutionLoopEvent, Classification: class, Loop: &event})
}

func higherClassification(a, b Classification) Classification {
	if b == "" {
		b = ClassConfidential
	}
	if classificationRank(b) > classificationRank(a) {
		return b
	}
	return a
}

type engineProvider struct {
	run      *engineRun
	provider Provider
	zone     Zone
	turn     int
}

func (p *engineProvider) Name() string { return p.provider.Name() }
func (p *engineProvider) Models(ctx context.Context) ([]ModelDescriptor, error) {
	return p.provider.Models(ctx)
}
func (p *engineProvider) Stream(ctx context.Context, r ModelRequest) (ModelStream, error) {
	p.run.observe(ctx, RecordProvider, "started")
	stream, err := p.provider.Stream(ctx, r)
	if err != nil {
		p.run.observe(ctx, RecordProvider, "failed")
		return stream, err
	}
	if stream == nil {
		return nil, errors.New("provider returned nil stream")
	}
	return &engineModelStream{ModelStream: stream, run: p.run, ctx: ctx, zone: p.zone, turn: p.turn}, nil
}

type engineModelStream struct {
	ModelStream
	run  *engineRun
	ctx  context.Context
	zone Zone
	turn int
}

// Provider bytes are quarantined inside the adapter until the return-path
// verdict permits exposing them to the loop, durable content log or tool.
func (s *engineModelStream) Recv(ctx context.Context) (ModelEvent, error) {
	event, err := s.ModelStream.Recv(ctx)
	if err != nil {
		return event, err
	}
	if s.zone == s.run.request.SourceZone {
		return event, nil
	}
	switch event.Type {
	case ModelEventTextDelta, ModelEventToolCallDelta, ModelEventToolCall, ModelEventError:
	default:
		return event, nil
	}
	manifest, err := valueManifest(event, s.run.request.ModelOutputClassification)
	if err != nil {
		return ModelEvent{}, err
	}
	v, _ := EvaluateDataEgressPolicy(ctx, s.run.engine.DataEgressPolicy, DataEgressPolicyRequest{WorkloadID: s.run.request.WorkloadID, StepID: s.run.request.StepID, Source: s.zone, Destination: s.run.request.SourceZone, Purpose: "model_result", Manifest: manifest})
	if ctx.Err() != nil {
		return ModelEvent{}, ctx.Err()
	}
	if err := s.run.recordPolicy(ctx, s.turn, manifest, v); err != nil {
		return ModelEvent{}, err
	}
	if !v.Allows() {
		return ModelEvent{}, ErrExecutionDenied
	}
	return event, nil
}

func (s *engineModelStream) Close() error {
	err := s.ModelStream.Close()
	s.run.observe(s.ctx, RecordProvider, "finished")
	return err
}

type engineToolHandler struct {
	run     *engineRun
	handler ToolHandler
}

func (h *engineToolHandler) Execute(ctx context.Context, call ToolInvocation) (ToolOutput, error) {
	manifest, err := valueManifest(call.Call, h.run.request.ModelOutputClassification)
	if err != nil {
		return ToolOutput{}, err
	}
	v, _ := EvaluateExecutionPolicy(ctx, h.run.engine.ExecutionPolicy, ExecutionPolicyRequest{WorkloadID: call.WorkloadID, StepID: call.StepID, AttemptID: call.AttemptID, StepKind: StepTool, Autonomous: h.run.request.Autonomous, Target: ExecutionTarget{Kind: ExecutionTargetTool, Zone: h.run.request.SourceZone, ToolName: call.Call.Name}, Manifest: manifest})
	if ctx.Err() != nil {
		return ToolOutput{}, ctx.Err()
	}
	if err := h.run.recordPolicy(ctx, call.Turn, manifest, v); err != nil {
		h.run.stop(err)
		return ToolOutput{}, err
	}
	if !v.Allows() {
		h.run.stop(ErrExecutionDenied)
		return ToolOutput{}, ErrExecutionDenied
	}
	h.run.observe(ctx, RecordTool, "started")
	defer h.run.observe(ctx, RecordTool, "finished")
	return h.handler.Execute(ctx, call)
}
func (h *engineToolHandler) Close(ctx context.Context) error { return h.handler.Close(ctx) }
func valueManifest(value any, class Classification) (ContentManifest, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return ContentManifest{}, err
	}
	digest := sha256.Sum256(encoded)
	return ContentManifest{Items: []ContentItem{{SHA256: hex.EncodeToString(digest[:]), MediaType: "application/json", SizeBytes: int64(len(encoded)), Classification: class}}}, nil
}

// The exact ordered request is hashed as one item, classified at the highest
// level of metadata and content. Providers may not dereference artifact IDs;
// materializing artifacts needs a separate policy-bound transfer.
func modelManifest(request ModelRequest, class Classification) (ContentManifest, error) {
	for _, message := range request.Messages {
		for _, part := range message.Content {
			if classificationRank(part.Classification) > classificationRank(class) {
				class = part.Classification
			}
			if part.Type == ContentArtifact || part.Type == ContentImage || part.Type == ContentDocument {
				return ContentManifest{}, errors.New("engine artifact materialization is not implemented")
			}
		}
	}
	// Placement is evaluated separately. Every candidate receives identical content.
	request.Model = ModelRef{}
	return valueManifest(request, class)
}
