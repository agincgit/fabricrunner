package fabricrunner

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// ContentItem describes one immutable item considered by policy.
type ContentItem struct {
	SHA256         string         `json:"sha256"`
	MediaType      string         `json:"media_type"`
	SizeBytes      int64          `json:"size_bytes"`
	Classification Classification `json:"classification,omitempty"`
}

func (item ContentItem) Validate() error {
	if err := validateSHA256("content SHA-256", item.SHA256); err != nil {
		return err
	}
	if strings.TrimSpace(item.MediaType) == "" {
		return errors.New("content media type is required")
	}
	if item.SizeBytes < 0 {
		return errors.New("content size cannot be negative")
	}
	if item.Classification != "" {
		if err := item.Classification.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// EffectiveClassification treats an unlabeled item as confidential.
func (item ContentItem) EffectiveClassification() Classification {
	if item.Classification == "" {
		return ClassConfidential
	}
	return item.Classification
}

// ContentManifest is the complete immutable content set considered by one
// policy evaluation.
type ContentManifest struct {
	Items []ContentItem `json:"items"`
}

func (manifest ContentManifest) Validate() error {
	if len(manifest.Items) == 0 {
		return errors.New("content manifest cannot be empty")
	}
	seen := make(map[string]struct{}, len(manifest.Items))
	for index, item := range manifest.Items {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("content item %d: %w", index, err)
		}
		if _, exists := seen[item.SHA256]; exists {
			return fmt.Errorf("content item %d duplicates SHA-256 %s", index, item.SHA256)
		}
		seen[item.SHA256] = struct{}{}
	}
	return nil
}

func (manifest ContentManifest) Clone() ContentManifest {
	return ContentManifest{Items: append([]ContentItem(nil), manifest.Items...)}
}

// Digest returns a stable SHA-256 binding for the validated manifest. Item
// order is preserved because it is part of the request presented to policy.
func (manifest ContentManifest) Digest() (string, error) {
	if err := manifest.Validate(); err != nil {
		return "", err
	}
	return manifest.digestUnchecked(), nil
}

func (manifest ContentManifest) digestUnchecked() string {
	hash := sha256.New()
	_, _ = hash.Write([]byte("fabricrunner.content-manifest.v1\x00"))
	for _, item := range manifest.Items {
		writeDigestString(hash, item.SHA256)
		writeDigestString(hash, item.MediaType)
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(item.SizeBytes))
		_, _ = hash.Write(size[:])
		writeDigestString(hash, string(item.Classification))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

type digestWriter interface {
	Write([]byte) (int, error)
}

func writeDigestString(writer digestWriter, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = writer.Write(length[:])
	_, _ = writer.Write([]byte(value))
}

func (manifest ContentManifest) EffectiveClassification() (Classification, error) {
	if err := manifest.Validate(); err != nil {
		return "", err
	}
	result := ClassPublic
	for _, item := range manifest.Items {
		classification := item.EffectiveClassification()
		if classificationRank(classification) > classificationRank(result) {
			result = classification
		}
	}
	return result, nil
}

func classificationRank(classification Classification) int {
	switch classification {
	case ClassPublic:
		return 0
	case ClassInternal:
		return 1
	case ClassConfidential:
		return 2
	case ClassSecret:
		return 3
	default:
		return -1
	}
}

type ExecutionTargetKind string

const (
	ExecutionTargetModel ExecutionTargetKind = "model"
	ExecutionTargetTool  ExecutionTargetKind = "tool"
)

// ExecutionTarget identifies the bounded model or tool placement under
// consideration. Sandbox availability is deliberately absent.
type ExecutionTarget struct {
	Kind     ExecutionTargetKind `json:"kind"`
	Zone     Zone                `json:"zone"`
	Model    *ModelRef           `json:"model,omitempty"`
	ToolName string              `json:"tool_name,omitempty"`
}

func (target ExecutionTarget) Validate() error {
	if err := target.Zone.Validate(); err != nil {
		return err
	}
	switch target.Kind {
	case ExecutionTargetModel:
		if target.Model == nil {
			return errors.New("model target requires a model")
		}
		if err := target.Model.Validate(); err != nil {
			return err
		}
		if target.ToolName != "" {
			return errors.New("model target cannot name a tool")
		}
	case ExecutionTargetTool:
		if strings.TrimSpace(target.ToolName) == "" {
			return errors.New("tool target requires a tool name")
		}
		if target.Model != nil {
			return errors.New("tool target cannot name a model")
		}
	default:
		return fmt.Errorf("unknown execution target kind %q", target.Kind)
	}
	return nil
}

func (target ExecutionTarget) clone() ExecutionTarget {
	if target.Model != nil {
		model := *target.Model
		target.Model = &model
	}
	return target
}

// ExecutionPolicyRequest describes one candidate before it reaches router
// scoring.
type ExecutionPolicyRequest struct {
	WorkloadID ID              `json:"workload_id"`
	StepID     ID              `json:"step_id"`
	AttemptID  ID              `json:"attempt_id"`
	StepKind   StepKind        `json:"step_kind"`
	Target     ExecutionTarget `json:"target"`
	Manifest   ContentManifest `json:"manifest"`
	Autonomous bool            `json:"autonomous"`
}

func (request ExecutionPolicyRequest) Validate() error {
	for _, field := range []struct {
		name string
		id   ID
	}{
		{name: "workload ID", id: request.WorkloadID},
		{name: "step ID", id: request.StepID},
		{name: "attempt ID", id: request.AttemptID},
	} {
		if err := field.id.Validate(); err != nil {
			return fmt.Errorf("%s: %w", field.name, err)
		}
	}
	if err := request.StepKind.Validate(); err != nil {
		return err
	}
	if err := request.Target.Validate(); err != nil {
		return fmt.Errorf("execution target: %w", err)
	}
	if request.StepKind == StepModelTurn && request.Target.Kind != ExecutionTargetModel {
		return errors.New("model-turn step requires a model target")
	}
	if request.StepKind == StepTool && request.Target.Kind != ExecutionTargetTool {
		return errors.New("tool step requires a tool target")
	}
	if request.StepKind != StepModelTurn && request.StepKind != StepTool {
		return fmt.Errorf("execution policy does not evaluate %q steps", request.StepKind)
	}
	if err := request.Manifest.Validate(); err != nil {
		return fmt.Errorf("execution manifest: %w", err)
	}
	return nil
}

func (request ExecutionPolicyRequest) Clone() ExecutionPolicyRequest {
	request.Target = request.Target.clone()
	request.Manifest = request.Manifest.Clone()
	return request
}

// DataEgressPolicyRequest describes content movement between trust zones.
type DataEgressPolicyRequest struct {
	WorkloadID     ID              `json:"workload_id"`
	StepID         ID              `json:"step_id"`
	Source         Zone            `json:"source"`
	Destination    Zone            `json:"destination"`
	Manifest       ContentManifest `json:"manifest"`
	Purpose        string          `json:"purpose"`
	DerivedFromSHA string          `json:"derived_from_sha256,omitempty"`
}

func (request DataEgressPolicyRequest) Validate() error {
	if err := request.WorkloadID.Validate(); err != nil {
		return fmt.Errorf("workload ID: %w", err)
	}
	if err := request.StepID.Validate(); err != nil {
		return fmt.Errorf("step ID: %w", err)
	}
	if err := request.Source.Validate(); err != nil {
		return fmt.Errorf("source zone: %w", err)
	}
	if err := request.Destination.Validate(); err != nil {
		return fmt.Errorf("destination zone: %w", err)
	}
	if err := request.Manifest.Validate(); err != nil {
		return fmt.Errorf("egress manifest: %w", err)
	}
	if strings.TrimSpace(request.Purpose) == "" {
		return errors.New("egress purpose is required")
	}
	if request.DerivedFromSHA != "" {
		if err := validateSHA256("derived-from SHA-256", request.DerivedFromSHA); err != nil {
			return err
		}
	}
	return nil
}

func (request DataEgressPolicyRequest) Clone() DataEgressPolicyRequest {
	request.Manifest = request.Manifest.Clone()
	return request
}

type PolicyAction string

const (
	PolicyAllow           PolicyAction = "allow"
	PolicyDeny            PolicyAction = "deny"
	PolicyRequireApproval PolicyAction = "require_approval"
	PolicyTransform       PolicyAction = "transform"
	PolicyRedact          PolicyAction = "redact"
)

func (action PolicyAction) Validate() error {
	switch action {
	case PolicyAllow, PolicyDeny, PolicyRequireApproval, PolicyTransform, PolicyRedact:
		return nil
	default:
		return fmt.Errorf("unknown policy action %q", action)
	}
}

type PolicyScopeKind string

const (
	PolicyScopeExecution  PolicyScopeKind = "execution"
	PolicyScopeDataEgress PolicyScopeKind = "data_egress"
)

// PolicyScope binds a verdict to one canonical execution target or data path.
type PolicyScope struct {
	Kind           PolicyScopeKind  `json:"kind"`
	WorkloadID     ID               `json:"workload_id"`
	StepID         ID               `json:"step_id"`
	AttemptID      ID               `json:"attempt_id,omitempty"`
	StepKind       StepKind         `json:"step_kind,omitempty"`
	Autonomous     bool             `json:"autonomous,omitempty"`
	Target         *ExecutionTarget `json:"target,omitempty"`
	Source         Zone             `json:"source,omitempty"`
	Destination    Zone             `json:"destination,omitempty"`
	Purpose        string           `json:"purpose,omitempty"`
	DerivedFromSHA string           `json:"derived_from_sha256,omitempty"`
}

func PolicyScopeForExecution(request ExecutionPolicyRequest) PolicyScope {
	target := request.Target.clone()
	return PolicyScope{
		Kind:       PolicyScopeExecution,
		WorkloadID: request.WorkloadID,
		StepID:     request.StepID,
		AttemptID:  request.AttemptID,
		StepKind:   request.StepKind,
		Autonomous: request.Autonomous,
		Target:     &target,
	}
}

func PolicyScopeForDataEgress(request DataEgressPolicyRequest) PolicyScope {
	return PolicyScope{
		Kind:           PolicyScopeDataEgress,
		WorkloadID:     request.WorkloadID,
		StepID:         request.StepID,
		Source:         request.Source,
		Destination:    request.Destination,
		Purpose:        request.Purpose,
		DerivedFromSHA: request.DerivedFromSHA,
	}
}

func (scope PolicyScope) Validate() error {
	if err := scope.WorkloadID.Validate(); err != nil {
		return fmt.Errorf("policy scope workload ID: %w", err)
	}
	if err := scope.StepID.Validate(); err != nil {
		return fmt.Errorf("policy scope step ID: %w", err)
	}
	switch scope.Kind {
	case PolicyScopeExecution:
		if err := scope.AttemptID.Validate(); err != nil {
			return fmt.Errorf("policy scope attempt ID: %w", err)
		}
		if scope.Target == nil {
			return errors.New("execution policy scope requires a target")
		}
		if err := scope.Target.Validate(); err != nil {
			return fmt.Errorf("execution policy scope target: %w", err)
		}
		if err := scope.StepKind.Validate(); err != nil {
			return fmt.Errorf("execution policy scope step kind: %w", err)
		}
		if (scope.StepKind == StepModelTurn && scope.Target.Kind != ExecutionTargetModel) ||
			(scope.StepKind == StepTool && scope.Target.Kind != ExecutionTargetTool) ||
			(scope.StepKind != StepModelTurn && scope.StepKind != StepTool) {
			return errors.New("execution policy scope step and target kinds do not match")
		}
		if scope.Source != "" || scope.Destination != "" || scope.Purpose != "" || scope.DerivedFromSHA != "" {
			return errors.New("execution policy scope cannot contain an egress path")
		}
	case PolicyScopeDataEgress:
		if !scope.AttemptID.IsZero() || scope.StepKind != "" || scope.Autonomous || scope.Target != nil {
			return errors.New("data-egress policy scope cannot contain execution fields")
		}
		if err := scope.Source.Validate(); err != nil {
			return fmt.Errorf("policy scope source: %w", err)
		}
		if err := scope.Destination.Validate(); err != nil {
			return fmt.Errorf("policy scope destination: %w", err)
		}
		if strings.TrimSpace(scope.Purpose) == "" {
			return errors.New("data-egress policy scope purpose is required")
		}
		if scope.DerivedFromSHA != "" {
			if err := validateSHA256("policy scope derived-from SHA-256", scope.DerivedFromSHA); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unknown policy scope kind %q", scope.Kind)
	}
	return nil
}

func (scope PolicyScope) Clone() PolicyScope {
	if scope.Target != nil {
		target := scope.Target.clone()
		scope.Target = &target
	}
	return scope
}

func (scope PolicyScope) Equal(other PolicyScope) bool {
	if scope.Kind != other.Kind || scope.WorkloadID != other.WorkloadID ||
		scope.StepID != other.StepID || scope.AttemptID != other.AttemptID ||
		scope.StepKind != other.StepKind || scope.Autonomous != other.Autonomous ||
		scope.Source != other.Source || scope.Destination != other.Destination ||
		scope.Purpose != other.Purpose || scope.DerivedFromSHA != other.DerivedFromSHA {
		return false
	}
	if scope.Target == nil || other.Target == nil {
		return scope.Target == nil && other.Target == nil
	}
	return executionTargetsEqual(*scope.Target, *other.Target)
}

func executionTargetsEqual(left, right ExecutionTarget) bool {
	if left.Kind != right.Kind || left.Zone != right.Zone || left.ToolName != right.ToolName {
		return false
	}
	if left.Model == nil || right.Model == nil {
		return left.Model == nil && right.Model == nil
	}
	return *left.Model == *right.Model
}

// PolicyVerdict is a deterministic result ready to be recorded as an event.
type PolicyVerdict struct {
	Policy         string       `json:"policy"`
	PolicyVersion  string       `json:"policy_version"`
	RuleID         string       `json:"rule_id"`
	Action         PolicyAction `json:"action"`
	Reason         string       `json:"reason"`
	ManifestSHA256 string       `json:"manifest_sha256,omitempty"`
	Scope          PolicyScope  `json:"scope"`
	Transformation string       `json:"transformation,omitempty"`
}

func (verdict PolicyVerdict) Validate() error {
	if strings.TrimSpace(verdict.Policy) == "" {
		return errors.New("policy name is required")
	}
	if strings.TrimSpace(verdict.PolicyVersion) == "" {
		return errors.New("policy version is required")
	}
	if strings.TrimSpace(verdict.RuleID) == "" {
		return errors.New("policy rule ID is required")
	}
	if err := verdict.Action.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(verdict.Reason) == "" {
		return errors.New("policy reason is required")
	}
	if err := validateSHA256("verdict manifest SHA-256", verdict.ManifestSHA256); err != nil {
		return err
	}
	if err := verdict.Scope.Validate(); err != nil {
		return err
	}
	requiresTransformation := verdict.Action == PolicyTransform || verdict.Action == PolicyRedact
	if requiresTransformation && strings.TrimSpace(verdict.Transformation) == "" {
		return errors.New("transform and redact verdicts require transformation instructions")
	}
	if !requiresTransformation && verdict.Transformation != "" {
		return errors.New("transformation instructions require a transform or redact action")
	}
	return nil
}

func (verdict PolicyVerdict) Clone() PolicyVerdict {
	verdict.Scope = verdict.Scope.Clone()
	return verdict
}

func (verdict PolicyVerdict) Allows() bool {
	return verdict.Action == PolicyAllow
}

func (verdict PolicyVerdict) RequiresApproval() bool {
	return verdict.Action == PolicyRequireApproval
}

type ExecutionPolicy interface {
	EvaluateExecution(context.Context, ExecutionPolicyRequest) (PolicyVerdict, error)
}

type DataEgressPolicy interface {
	EvaluateDataEgress(context.Context, DataEgressPolicyRequest) (PolicyVerdict, error)
}

var ErrPolicyEvaluation = errors.New("policy evaluation failed")

// EvaluateExecutionPolicy validates and isolates the request and converts
// policy failures or invalid verdicts into a deny verdict.
func EvaluateExecutionPolicy(
	ctx context.Context,
	policy ExecutionPolicy,
	request ExecutionPolicyRequest,
) (PolicyVerdict, error) {
	if err := ctx.Err(); err != nil {
		return PolicyVerdict{}, err
	}
	manifestDigest := request.Manifest.digestUnchecked()
	scope := PolicyScopeForExecution(request)
	if err := request.Validate(); err != nil {
		return failClosedVerdict("execution.request.invalid", manifestDigest, scope), fmt.Errorf("%w: %w", ErrPolicyEvaluation, err)
	}
	classification, err := request.Manifest.EffectiveClassification()
	if err != nil {
		return failClosedVerdict("execution.manifest.invalid", manifestDigest, scope), fmt.Errorf("%w: %w", ErrPolicyEvaluation, err)
	}
	if request.Target.Kind == ExecutionTargetModel && classification == ClassSecret {
		return failClosedVerdict("execution.secret-model.denied", manifestDigest, scope), nil
	}
	if policy == nil {
		return failClosedVerdict("execution.policy.missing", manifestDigest, scope), fmt.Errorf("%w: execution policy is nil", ErrPolicyEvaluation)
	}
	verdict, err := policy.EvaluateExecution(ctx, request.Clone())
	if contextError := ctx.Err(); contextError != nil {
		return PolicyVerdict{}, contextError
	}
	if err != nil {
		return failClosedVerdict("execution.policy.error", manifestDigest, scope), fmt.Errorf("%w: %w", ErrPolicyEvaluation, err)
	}
	if err := verdict.Validate(); err != nil {
		return failClosedVerdict("execution.verdict.invalid", manifestDigest, scope), fmt.Errorf("%w: %w", ErrPolicyEvaluation, err)
	}
	if verdict.ManifestSHA256 != "" && verdict.ManifestSHA256 != manifestDigest {
		return failClosedVerdict("execution.verdict.manifest-mismatch", manifestDigest, scope), fmt.Errorf("%w: verdict does not match evaluated manifest", ErrPolicyEvaluation)
	}
	if !verdict.Scope.Equal(scope) {
		return failClosedVerdict("execution.verdict.scope-mismatch", manifestDigest, scope), fmt.Errorf("%w: verdict does not match execution scope", ErrPolicyEvaluation)
	}
	return verdict, nil
}

// EvaluateDataEgressPolicy validates and isolates the request and converts
// policy failures or invalid verdicts into a deny verdict.
func EvaluateDataEgressPolicy(
	ctx context.Context,
	policy DataEgressPolicy,
	request DataEgressPolicyRequest,
) (PolicyVerdict, error) {
	if err := ctx.Err(); err != nil {
		return PolicyVerdict{}, err
	}
	manifestDigest := request.Manifest.digestUnchecked()
	scope := PolicyScopeForDataEgress(request)
	if err := request.Validate(); err != nil {
		return failClosedVerdict("egress.request.invalid", manifestDigest, scope), fmt.Errorf("%w: %w", ErrPolicyEvaluation, err)
	}
	if policy == nil {
		return failClosedVerdict("egress.policy.missing", manifestDigest, scope), fmt.Errorf("%w: data egress policy is nil", ErrPolicyEvaluation)
	}
	verdict, err := policy.EvaluateDataEgress(ctx, request.Clone())
	if contextError := ctx.Err(); contextError != nil {
		return PolicyVerdict{}, contextError
	}
	if err != nil {
		return failClosedVerdict("egress.policy.error", manifestDigest, scope), fmt.Errorf("%w: %w", ErrPolicyEvaluation, err)
	}
	if err := verdict.Validate(); err != nil {
		return failClosedVerdict("egress.verdict.invalid", manifestDigest, scope), fmt.Errorf("%w: %w", ErrPolicyEvaluation, err)
	}
	if verdict.ManifestSHA256 != "" && verdict.ManifestSHA256 != manifestDigest {
		return failClosedVerdict("egress.verdict.manifest-mismatch", manifestDigest, scope), fmt.Errorf("%w: verdict does not match evaluated manifest", ErrPolicyEvaluation)
	}
	if !verdict.Scope.Equal(scope) {
		return failClosedVerdict("egress.verdict.scope-mismatch", manifestDigest, scope), fmt.Errorf("%w: verdict does not match egress scope", ErrPolicyEvaluation)
	}
	return verdict, nil
}

func failClosedVerdict(ruleID, manifestDigest string, scope PolicyScope) PolicyVerdict {
	return PolicyVerdict{
		Policy:         "fabricrunner.fail-closed",
		PolicyVersion:  "1",
		RuleID:         ruleID,
		Action:         PolicyDeny,
		Reason:         "policy evaluation did not produce a valid authorization",
		ManifestSHA256: manifestDigest,
		Scope:          scope.Clone(),
	}
}

func validateSHA256(name, value string) error {
	if len(value) != 64 {
		return fmt.Errorf("%s must contain 64 hexadecimal characters", name)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if value != strings.ToLower(value) {
		return fmt.Errorf("%s must use lowercase hexadecimal", name)
	}
	return nil
}
