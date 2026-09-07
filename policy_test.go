package fabricrunner

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestContentManifestValidation(t *testing.T) {
	t.Parallel()

	valid := contentManifest(ClassInternal)
	tests := []struct {
		name     string
		manifest ContentManifest
	}{
		{name: "empty", manifest: ContentManifest{}},
		{name: "short digest", manifest: ContentManifest{Items: []ContentItem{{SHA256: "abc", MediaType: "text/plain"}}}},
		{name: "invalid digest", manifest: ContentManifest{Items: []ContentItem{{SHA256: strings.Repeat("z", 64), MediaType: "text/plain"}}}},
		{name: "uppercase digest", manifest: ContentManifest{Items: []ContentItem{{SHA256: strings.Repeat("A", 64), MediaType: "text/plain"}}}},
		{name: "missing media type", manifest: ContentManifest{Items: []ContentItem{{SHA256: strings.Repeat("a", 64)}}}},
		{name: "negative size", manifest: ContentManifest{Items: []ContentItem{{SHA256: strings.Repeat("a", 64), MediaType: "text/plain", SizeBytes: -1}}}},
		{name: "unknown classification", manifest: ContentManifest{Items: []ContentItem{{SHA256: strings.Repeat("a", 64), MediaType: "text/plain", Classification: "restricted"}}}},
		{name: "duplicate digest", manifest: ContentManifest{Items: []ContentItem{valid.Items[0], valid.Items[0]}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.manifest.Validate(); err == nil {
				t.Fatal("Validate() accepted invalid manifest")
			}
		})
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate(valid) error = %v", err)
	}
}

func TestContentManifestEffectiveClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		items []ContentItem
		want  Classification
	}{
		{
			name: "unlabeled defaults confidential",
			items: []ContentItem{{
				SHA256: strings.Repeat("a", 64), MediaType: "text/plain",
			}},
			want: ClassConfidential,
		},
		{
			name: "most restrictive wins",
			items: []ContentItem{
				{SHA256: strings.Repeat("a", 64), MediaType: "text/plain", Classification: ClassPublic},
				{SHA256: strings.Repeat("b", 64), MediaType: "text/plain", Classification: ClassSecret},
				{SHA256: strings.Repeat("c", 64), MediaType: "text/plain", Classification: ClassInternal},
			},
			want: ClassSecret,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := (ContentManifest{Items: test.items}).EffectiveClassification()
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("EffectiveClassification() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestContentManifestDigestBindsCanonicalManifest(t *testing.T) {
	t.Parallel()

	manifest := ContentManifest{Items: []ContentItem{
		{SHA256: strings.Repeat("a", 64), MediaType: "text/plain", SizeBytes: 1, Classification: ClassPublic},
		{SHA256: strings.Repeat("b", 64), MediaType: "application/json", SizeBytes: 2, Classification: ClassInternal},
	}}
	first, err := manifest.Digest()
	if err != nil {
		t.Fatal(err)
	}
	second, err := manifest.Clone().Digest()
	if err != nil {
		t.Fatal(err)
	}
	if first != second || len(first) != 64 {
		t.Fatalf("digest is not stable: %q != %q", first, second)
	}
	reordered := ContentManifest{Items: []ContentItem{manifest.Items[1], manifest.Items[0]}}
	reorderedDigest, err := reordered.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if reorderedDigest == first {
		t.Fatal("digest did not bind manifest item order")
	}
}

func TestPolicyRequestValidation(t *testing.T) {
	t.Parallel()

	execution := validExecutionRequest(t)
	if err := execution.Validate(); err != nil {
		t.Fatalf("ExecutionPolicyRequest.Validate() error = %v", err)
	}

	invalidExecution := execution
	invalidExecution.AttemptID = ""
	if err := invalidExecution.Validate(); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("invalid execution request error = %v, want ErrInvalidID", err)
	}
	invalidExecution = execution
	invalidExecution.Target.Model = nil
	if err := invalidExecution.Validate(); err == nil {
		t.Fatal("execution request accepted model target without model")
	}
	invalidExecution = execution
	invalidExecution.StepKind = StepTool
	if err := invalidExecution.Validate(); err == nil {
		t.Fatal("execution request accepted mismatched step and target kinds")
	}

	egress := DataEgressPolicyRequest{
		WorkloadID:  execution.WorkloadID,
		StepID:      execution.StepID,
		Source:      ZonePersonal,
		Destination: ZoneManagedCloud,
		Manifest:    execution.Manifest,
		Purpose:     "bounded reasoning",
	}
	if err := egress.Validate(); err != nil {
		t.Fatalf("DataEgressPolicyRequest.Validate() error = %v", err)
	}
	egress.Source = "unknown"
	if err := egress.Validate(); err == nil {
		t.Fatal("egress request accepted unknown source zone")
	}
}

func TestPolicyVerdictValidation(t *testing.T) {
	t.Parallel()

	request := validExecutionRequest(t)
	manifestDigest, err := request.Manifest.Digest()
	if err != nil {
		t.Fatal(err)
	}
	valid := PolicyVerdict{
		Policy:         "test",
		PolicyVersion:  "1",
		RuleID:         "rule.allow",
		Action:         PolicyAllow,
		Reason:         "allowed for test",
		ManifestSHA256: manifestDigest,
		Scope:          PolicyScopeForExecution(request),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate(valid) error = %v", err)
	}
	if !valid.Allows() || valid.RequiresApproval() {
		t.Fatal("allow verdict helpers returned incorrect values")
	}

	tests := []PolicyVerdict{
		{},
		{Policy: "test", PolicyVersion: "1", RuleID: "rule", Action: "unknown", Reason: "bad"},
		{Policy: "test", PolicyVersion: "1", RuleID: "rule", Action: PolicyTransform, Reason: "transform"},
		{Policy: "test", PolicyVersion: "1", RuleID: "rule", Action: PolicyAllow, Reason: "allow", Transformation: "redact"},
	}
	for _, verdict := range tests {
		if err := verdict.Validate(); err == nil {
			t.Fatalf("Validate() accepted invalid verdict %#v", verdict)
		}
	}

	approval := valid
	approval.Action = PolicyRequireApproval
	if !approval.RequiresApproval() || approval.Allows() {
		t.Fatal("approval verdict helpers returned incorrect values")
	}
	transform := valid
	transform.Action = PolicyTransform
	transform.Transformation = "replace with derived manifest"
	if err := transform.Validate(); err != nil {
		t.Fatalf("valid transform verdict error = %v", err)
	}
}

func TestEvaluateExecutionPolicyFailsClosedAndIsolatesInput(t *testing.T) {
	t.Parallel()

	request := validExecutionRequest(t)
	original := request.Clone()
	policy := executionPolicyFunc(func(_ context.Context, received ExecutionPolicyRequest) (PolicyVerdict, error) {
		manifestDigest, err := received.Manifest.Digest()
		if err != nil {
			return PolicyVerdict{}, err
		}
		scope := PolicyScopeForExecution(received)
		received.Manifest.Items[0].MediaType = "application/mutated"
		received.Target.Model.Model = "mutated"
		return PolicyVerdict{
			Policy:         "test",
			PolicyVersion:  "1",
			RuleID:         "test.allow",
			Action:         PolicyAllow,
			Reason:         "valid test verdict",
			ManifestSHA256: manifestDigest,
			Scope:          scope,
		}, nil
	})
	verdict, err := EvaluateExecutionPolicy(context.Background(), policy, request)
	if err != nil || !verdict.Allows() {
		t.Fatalf("EvaluateExecutionPolicy() = %#v, %v", verdict, err)
	}
	if !reflect.DeepEqual(request, original) {
		t.Fatalf("policy mutated caller request:\ngot = %#v\nwant = %#v", request, original)
	}

	failed, err := EvaluateExecutionPolicy(
		context.Background(),
		executionPolicyFunc(func(context.Context, ExecutionPolicyRequest) (PolicyVerdict, error) {
			return PolicyVerdict{Action: PolicyAllow}, errors.New("policy backend failed")
		}),
		request,
	)
	if !errors.Is(err, ErrPolicyEvaluation) || failed.Action != PolicyDeny || failed.Allows() {
		t.Fatalf("policy error did not fail closed: verdict = %#v, error = %v", failed, err)
	}

	invalid, err := EvaluateExecutionPolicy(
		context.Background(),
		executionPolicyFunc(func(context.Context, ExecutionPolicyRequest) (PolicyVerdict, error) {
			return PolicyVerdict{Action: PolicyAllow}, nil
		}),
		request,
	)
	if !errors.Is(err, ErrPolicyEvaluation) || invalid.Action != PolicyDeny {
		t.Fatalf("invalid verdict did not fail closed: verdict = %#v, error = %v", invalid, err)
	}
}

func TestEvaluateExecutionPolicyEnforcesSecretModelInvariant(t *testing.T) {
	t.Parallel()

	request := validExecutionRequest(t)
	request.Manifest = contentManifest(ClassSecret)
	called := false
	verdict, err := EvaluateExecutionPolicy(
		context.Background(),
		executionPolicyFunc(func(context.Context, ExecutionPolicyRequest) (PolicyVerdict, error) {
			called = true
			return PolicyVerdict{}, nil
		}),
		request,
	)
	if err != nil || verdict.Action != PolicyDeny || called {
		t.Fatalf("secret model invariant = %#v, %v, policy called = %t", verdict, err, called)
	}
}

func TestEvaluatePolicyRejectsManifestMismatch(t *testing.T) {
	t.Parallel()

	request := validExecutionRequest(t)
	verdict, err := EvaluateExecutionPolicy(
		context.Background(),
		executionPolicyFunc(func(context.Context, ExecutionPolicyRequest) (PolicyVerdict, error) {
			return PolicyVerdict{
				Policy:         "test",
				PolicyVersion:  "1",
				RuleID:         "test.allow",
				Action:         PolicyAllow,
				Reason:         "wrong manifest",
				ManifestSHA256: strings.Repeat("b", 64),
				Scope:          PolicyScopeForExecution(request),
			}, nil
		}),
		request,
	)
	if !errors.Is(err, ErrPolicyEvaluation) || verdict.Action != PolicyDeny {
		t.Fatalf("manifest mismatch did not fail closed: %#v, %v", verdict, err)
	}
}

func TestEvaluatePolicyRejectsScopeMismatch(t *testing.T) {
	t.Parallel()

	request := validExecutionRequest(t)
	verdict, err := EvaluateExecutionPolicy(
		context.Background(),
		executionPolicyFunc(func(_ context.Context, received ExecutionPolicyRequest) (PolicyVerdict, error) {
			manifestDigest, digestErr := received.Manifest.Digest()
			if digestErr != nil {
				return PolicyVerdict{}, digestErr
			}
			scope := PolicyScopeForExecution(received)
			scope.Target.Zone = ZonePersonal
			return PolicyVerdict{
				Policy:         "test",
				PolicyVersion:  "1",
				RuleID:         "test.allow",
				Action:         PolicyAllow,
				Reason:         "wrong execution scope",
				ManifestSHA256: manifestDigest,
				Scope:          scope,
			}, nil
		}),
		request,
	)
	if !errors.Is(err, ErrPolicyEvaluation) || verdict.Action != PolicyDeny {
		t.Fatalf("scope mismatch did not fail closed: %#v, %v", verdict, err)
	}
}

func TestEvaluatePoliciesHonorCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	executionVerdict, err := EvaluateExecutionPolicy(ctx, executionPolicyFunc(nil), validExecutionRequest(t))
	if !errors.Is(err, context.Canceled) || executionVerdict != (PolicyVerdict{}) {
		t.Fatalf("canceled execution = %#v, %v", executionVerdict, err)
	}
	egressVerdict, err := EvaluateDataEgressPolicy(ctx, dataEgressPolicyFunc(nil), validEgressRequest(t))
	if !errors.Is(err, context.Canceled) || egressVerdict != (PolicyVerdict{}) {
		t.Fatalf("canceled egress = %#v, %v", egressVerdict, err)
	}
}

func TestEvaluateDataEgressPolicyFailsClosed(t *testing.T) {
	t.Parallel()

	verdict, err := EvaluateDataEgressPolicy(context.Background(), nil, validEgressRequest(t))
	if !errors.Is(err, ErrPolicyEvaluation) || verdict.Action != PolicyDeny {
		t.Fatalf("nil egress policy = %#v, %v", verdict, err)
	}
}

func TestEvaluateDataEgressPolicyIsolatesInput(t *testing.T) {
	t.Parallel()

	request := validEgressRequest(t)
	original := request.Clone()
	verdict, err := EvaluateDataEgressPolicy(
		context.Background(),
		dataEgressPolicyFunc(func(_ context.Context, received DataEgressPolicyRequest) (PolicyVerdict, error) {
			manifestDigest, digestErr := received.Manifest.Digest()
			if digestErr != nil {
				return PolicyVerdict{}, digestErr
			}
			received.Manifest.Items[0].MediaType = "application/mutated"
			return PolicyVerdict{
				Policy:         "test",
				PolicyVersion:  "1",
				RuleID:         "test.allow",
				Action:         PolicyAllow,
				Reason:         "valid test verdict",
				ManifestSHA256: manifestDigest,
				Scope:          PolicyScopeForDataEgress(received),
			}, nil
		}),
		request,
	)
	if err != nil || !verdict.Allows() {
		t.Fatalf("EvaluateDataEgressPolicy() = %#v, %v", verdict, err)
	}
	if !reflect.DeepEqual(request, original) {
		t.Fatalf("policy mutated caller request:\ngot = %#v\nwant = %#v", request, original)
	}
}

type executionPolicyFunc func(context.Context, ExecutionPolicyRequest) (PolicyVerdict, error)

func (function executionPolicyFunc) EvaluateExecution(
	ctx context.Context,
	request ExecutionPolicyRequest,
) (PolicyVerdict, error) {
	return function(ctx, request)
}

type dataEgressPolicyFunc func(context.Context, DataEgressPolicyRequest) (PolicyVerdict, error)

func (function dataEgressPolicyFunc) EvaluateDataEgress(
	ctx context.Context,
	request DataEgressPolicyRequest,
) (PolicyVerdict, error) {
	return function(ctx, request)
}

func validExecutionRequest(t *testing.T) ExecutionPolicyRequest {
	t.Helper()
	return ExecutionPolicyRequest{
		WorkloadID: testPolicyID(t),
		StepID:     testPolicyID(t),
		AttemptID:  testPolicyID(t),
		StepKind:   StepModelTurn,
		Target: ExecutionTarget{
			Kind:  ExecutionTargetModel,
			Zone:  ZoneManagedCloud,
			Model: &ModelRef{Provider: "anthropic", Model: "frontier"},
		},
		Manifest: contentManifest(ClassPublic),
	}
}

func validEgressRequest(t *testing.T) DataEgressPolicyRequest {
	t.Helper()
	return DataEgressPolicyRequest{
		WorkloadID:  testPolicyID(t),
		StepID:      testPolicyID(t),
		Source:      ZonePersonal,
		Destination: ZoneManagedCloud,
		Manifest:    contentManifest(ClassPublic),
		Purpose:     "test egress",
	}
}

func contentManifest(classification Classification) ContentManifest {
	return ContentManifest{Items: []ContentItem{{
		SHA256:         strings.Repeat("a", 64),
		MediaType:      "text/plain",
		SizeBytes:      5,
		Classification: classification,
	}}}
}

func testPolicyID(t *testing.T) ID {
	t.Helper()
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
