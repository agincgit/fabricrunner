package fabricrunner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrApprovalDenied = fmt.Errorf("%w: approval denied", ErrExecutionDenied)

type ApprovalRequest struct {
	Scope    PolicyScope     `json:"scope"`
	Manifest ContentManifest `json:"manifest"`
}
type ApprovalDecision struct {
	Approved bool   `json:"approved"`
	Actor    string `json:"actor"`
	Reason   string `json:"reason,omitempty"`
}
type Approver interface {
	Approve(context.Context, ApprovalRequest) (ApprovalDecision, error)
}
type ApprovalRecord struct {
	Request  ApprovalRequest  `json:"request"`
	Decision ApprovalDecision `json:"decision"`
}

// SpendBound is an application-supplied upper bound, not an average estimate.
// The caller is responsible for tokenizer, framing and pricing accuracy.
type SpendBound struct {
	InputTokens  int64      `json:"input_tokens"`
	OutputTokens int64      `json:"output_tokens"`
	Cost         CostMicros `json:"cost_micros"`
}
type SpendEstimator interface {
	Estimate(context.Context, ModelRequest, ModelDescriptor) (SpendBound, error)
}

func (b SpendBound) Validate() error {
	if b.InputTokens < 0 || b.OutputTokens < 0 || b.Cost < 0 {
		return errors.New("negative spend bound")
	}
	return nil
}

type BudgetRecord struct {
	Reserved  SpendBound `json:"reserved"`
	Dimension string     `json:"dimension,omitempty"`
}
type CleanupRecord struct {
	Failed   bool   `json:"failed"`
	Resource string `json:"resource"`
}

func (s *engineRun) approve(ctx context.Context, turn int, manifest ContentManifest, scope PolicyScope) error {
	if s.engine.Approver == nil {
		return ErrApprovalDenied
	}
	class, err := manifest.EffectiveClassification()
	if err != nil {
		return err
	}
	if class == ClassSecret {
		return ErrApprovalDenied
	}
	timeout := s.engine.ApprovalTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	approvalCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	request := ApprovalRequest{Scope: scope.Clone(), Manifest: manifest.Clone()}
	decision, approvalErr := s.engine.Approver.Approve(approvalCtx, ApprovalRequest{Scope: scope.Clone(), Manifest: manifest.Clone()})
	if approvalErr != nil || approvalCtx.Err() != nil || strings.TrimSpace(decision.Actor) == "" {
		decision = ApprovalDecision{Actor: "fabricrunner", Reason: "approval_failed"}
		if approvalCtx.Err() != nil {
			decision.Reason = "approval_timeout"
		}
	}
	finalCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer stop()
	if err := s.record(finalCtx, ExecutionRecord{Turn: turn, Stage: ExecutionApproval, Classification: class, Approval: &ApprovalRecord{Request: request, Decision: decision}}); err != nil {
		return err
	}
	if !decision.Approved {
		return errors.Join(ErrApprovalDenied, approvalErr, approvalCtx.Err())
	}
	return ctx.Err()
}
func (s *engineRun) resolveApproval(ctx context.Context, turn int, manifest ContentManifest, v PolicyVerdict) (PolicyVerdict, error) {
	if !v.RequiresApproval() {
		return v, nil
	}
	if err := s.approve(ctx, turn, manifest, v.Scope); err != nil {
		return v, err
	}
	v.Policy = "fabricrunner.approval"
	v.PolicyVersion = "1"
	v.RuleID = "approval.granted"
	v.Action = PolicyAllow
	v.Reason = "recorded approval granted"
	return v, s.recordPolicy(ctx, turn, manifest, v)
}
func (s *engineRun) budgetFailure(ctx context.Context, turn int, dimension string) error {
	s.budgetDimension = dimension
	err := s.record(ctx, ExecutionRecord{Turn: turn, Stage: ExecutionBudget, Budget: &BudgetRecord{Dimension: dimension}})
	return errors.Join(fmt.Errorf("%w: %s", ErrLoopBudgetExceeded, dimension), err)
}
func (s *engineRun) reserve(ctx context.Context, turn int, request ModelRequest, descriptor ModelDescriptor) (SpendBound, error) {
	if s.engine.SpendEstimator == nil {
		return SpendBound{}, errors.New("engine requires conservative spend estimator before model calls")
	}
	bound, err := s.engine.SpendEstimator.Estimate(ctx, CloneModelRequest(request), descriptor.Clone())
	if err != nil {
		return bound, err
	}
	if err := bound.Validate(); err != nil {
		return bound, err
	}
	budget := s.request.Budget
	dimension := ""
	switch {
	case bound.InputTokens > budget.MaxInputTokens-s.reserved.InputTokens:
		dimension = "input_tokens"
	case bound.OutputTokens > budget.MaxOutputTokens-s.reserved.OutputTokens:
		dimension = "output_tokens"
	case bound.Cost > budget.MaxCost-s.reserved.Cost:
		dimension = "cost"
	}
	if dimension != "" {
		return bound, s.budgetFailure(ctx, turn, dimension)
	}
	if int64(request.MaxOutputTokens) > bound.OutputTokens {
		return bound, errors.New("requested output cap exceeds spend bound")
	}
	if err := s.record(ctx, ExecutionRecord{Turn: turn, Stage: ExecutionBudget, Budget: &BudgetRecord{Reserved: bound}}); err != nil {
		return bound, err
	}
	s.reserved.InputTokens += bound.InputTokens
	s.reserved.OutputTokens += bound.OutputTokens
	s.reserved.Cost += bound.Cost
	return bound, nil
}
func (s *engineRun) admit(ctx context.Context) error {
	if s.request.Budget.MaxSteps < 1 {
		return s.budgetFailure(ctx, 0, "steps")
	}
	if s.engine.Approver == nil {
		return nil
	}
	manifest, err := modelManifest(s.request.Initial, s.request.Classification)
	if err != nil {
		return err
	}
	request := ExecutionPolicyRequest{WorkloadID: s.request.WorkloadID, StepID: s.request.StepID, AttemptID: s.request.AttemptID, StepKind: StepModelTurn, Target: ExecutionTarget{Kind: ExecutionTargetModel, Zone: s.request.SourceZone, Model: &s.request.Initial.Model}, Manifest: manifest, Autonomous: s.request.Autonomous}
	return s.approve(ctx, 0, manifest, PolicyScopeForExecution(request))
}
func (s *engineRun) cleanup(ctx context.Context) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	var result error
	for _, binding := range s.request.Tools {
		err := binding.Handler.Close(WithSandboxEventSink(cleanupCtx, s))
		recordErr := s.record(cleanupCtx, ExecutionRecord{Stage: ExecutionCleanup, Cleanup: &CleanupRecord{Failed: err != nil, Resource: binding.Definition.Name}})
		result = errors.Join(result, err, recordErr)
	}
	return result
}
