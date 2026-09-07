package fabricrunner

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRoutingWeightsValidation(t *testing.T) {
	t.Parallel()

	if err := DefaultRoutingWeights().Validate(); err != nil {
		t.Fatalf("DefaultRoutingWeights().Validate() error = %v", err)
	}
	invalid := DefaultRoutingWeights()
	invalid.Quality++
	if err := invalid.Validate(); err == nil {
		t.Fatal("weights with invalid total were accepted")
	}
	invalid = DefaultRoutingWeights()
	invalid.Quality = -1
	invalid.Locality++
	if err := invalid.Validate(); err == nil {
		t.Fatal("negative weight was accepted")
	}
}

func TestRoutingRequestValidation(t *testing.T) {
	t.Parallel()

	valid := validRoutingRequest(t)
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate(valid) error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*RoutingRequest)
	}{
		{name: "missing ID", mutate: func(request *RoutingRequest) { request.AttemptID = "" }},
		{name: "bad manifest", mutate: func(request *RoutingRequest) { request.ManifestSHA256 = "bad" }},
		{name: "bad source", mutate: func(request *RoutingRequest) { request.SourceZone = "unknown" }},
		{name: "bad deadline", mutate: func(request *RoutingRequest) { request.Deadline = request.ObservedAt }},
		{name: "no candidates", mutate: func(request *RoutingRequest) { request.Candidates = nil }},
		{name: "duplicate candidate", mutate: func(request *RoutingRequest) { request.Candidates = append(request.Candidates, request.Candidates[0]) }},
		{name: "bad weights", mutate: func(request *RoutingRequest) { request.Weights.Quality++ }},
		{name: "duplicate modality", mutate: func(request *RoutingRequest) { request.Requirements.Modalities = []string{"text", "text"} }},
		{name: "duplicate zone", mutate: func(request *RoutingRequest) { request.Requirements.AllowedZones = []Zone{ZonePersonal, ZonePersonal} }},
		{name: "negative slots", mutate: func(request *RoutingRequest) { request.Candidates[0].AvailableSlots = -1 }},
		{name: "bad tool mode", mutate: func(request *RoutingRequest) { request.Candidates[0].Model.Capabilities.ToolUse = "unknown" }},
		{name: "duplicate available tool", mutate: func(request *RoutingRequest) { request.Candidates[0].AvailableTools = []string{"search", "search"} }},
		{name: "out of range quality", mutate: func(request *RoutingRequest) { request.Candidates[0].QualityPermille = 1001 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := valid.Clone()
			test.mutate(&request)
			if err := request.Validate(); err == nil {
				t.Fatal("Validate() accepted invalid routing request")
			}
		})
	}
}

func TestEvaluateRouterIsolatesInputAndValidatesOutput(t *testing.T) {
	t.Parallel()

	request := validRoutingRequest(t)
	original := request.Clone()
	router := routerFunc(func(_ context.Context, received RoutingRequest) (RoutingDecision, error) {
		received.Requirements.Modalities[0] = "mutated"
		received.Requirements.RequiredTools[0] = "mutated"
		received.Requirements.AllowedZones[0] = ZoneManagedCloud
		received.Candidates[0].AvailableTools[0] = "mutated"
		received.Candidates[0].Model.Capabilities.Modalities[0] = "mutated"
		received.Candidates[0].Model.Labels["quality_class"] = "mutated"
		return validRoutingDecision(received), nil
	})
	decision, err := EvaluateRouter(context.Background(), router, request)
	if err != nil || decision.SelectedCandidateID != request.Candidates[0].ID {
		t.Fatalf("EvaluateRouter() = %#v, %v", decision, err)
	}
	if !reflect.DeepEqual(request, original) {
		t.Fatalf("router mutated caller request:\ngot = %#v\nwant = %#v", request, original)
	}

	decision, err = EvaluateRouter(
		context.Background(),
		routerFunc(func(context.Context, RoutingRequest) (RoutingDecision, error) {
			return RoutingDecision{}, nil
		}),
		request,
	)
	if !errors.Is(err, ErrRoutingEvaluation) || !reflect.DeepEqual(decision, RoutingDecision{}) {
		t.Fatalf("invalid router result = %#v, %v", decision, err)
	}
}

func TestEvaluateRouterPreservesNoEligibleDecision(t *testing.T) {
	t.Parallel()

	request := validRoutingRequest(t)
	request.Candidates[0].Authenticated = false
	router := routerFunc(func(_ context.Context, received RoutingRequest) (RoutingDecision, error) {
		decision := validRoutingDecision(received)
		decision.SelectedCandidateID = ""
		decision.Assessments[0] = CandidateAssessment{
			CandidateID: received.Candidates[0].ID,
			Zone:        received.Candidates[0].Zone,
			Model:       received.Candidates[0].Model.Ref,
			Exclusion: &RoutingExclusion{
				Code: "identity.unauthenticated", Reason: "candidate is not authenticated",
			},
		}
		return decision, ErrNoEligibleCandidates
	})
	decision, err := EvaluateRouter(context.Background(), router, request)
	if !errors.Is(err, ErrNoEligibleCandidates) || len(decision.Assessments) != 1 ||
		decision.Assessments[0].Exclusion == nil {
		t.Fatalf("no-route decision = %#v, %v", decision, err)
	}
}

func TestEvaluateRouterRejectsRestoredPolicyExclusion(t *testing.T) {
	t.Parallel()

	request := validRoutingRequest(t)
	request.Candidates[0].ExecutionVerdict.Action = PolicyDeny
	decision, err := EvaluateRouter(
		context.Background(),
		routerFunc(func(_ context.Context, received RoutingRequest) (RoutingDecision, error) {
			return validRoutingDecision(received), nil
		}),
		request,
	)
	if !errors.Is(err, ErrRoutingEvaluation) || !reflect.DeepEqual(decision, RoutingDecision{}) {
		t.Fatalf("restored exclusion = %#v, %v", decision, err)
	}
}

func TestEvaluateRouterHonorsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	decision, err := EvaluateRouter(ctx, routerFunc(nil), validRoutingRequest(t))
	if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(decision, RoutingDecision{}) {
		t.Fatalf("canceled routing = %#v, %v", decision, err)
	}
}

type routerFunc func(context.Context, RoutingRequest) (RoutingDecision, error)

func (function routerFunc) Route(
	ctx context.Context,
	request RoutingRequest,
) (RoutingDecision, error) {
	return function(ctx, request)
}

func validRoutingRequest(t *testing.T) RoutingRequest {
	t.Helper()
	observedAt := time.Date(2026, time.September, 7, 12, 0, 0, 0, time.UTC)
	manifestDigest := strings.Repeat("a", 64)
	request := RoutingRequest{
		WorkloadID:     testRoutingID(t),
		StepID:         testRoutingID(t),
		AttemptID:      testRoutingID(t),
		ManifestSHA256: manifestDigest,
		SourceZone:     ZonePersonal,
		EgressPurpose:  "route model input",
		Requirements: Requirements{
			Modalities:       []string{"text"},
			RequiredTools:    []string{"search"},
			MinContextTokens: 1024,
			MinQualityClass:  "standard",
			AllowedZones:     []Zone{ZonePersonal, ZoneSelfCloud, ZoneManagedCloud},
			NativeToolUse:    true,
			StructuredOutput: true,
		},
		Budget: Budget{
			MaxOutputTokens: 1000,
			MaxCost:         1000,
		},
		ObservedAt: observedAt,
		Deadline:   observedAt.Add(10 * time.Second),
		Weights:    DefaultRoutingWeights(),
	}
	request.Candidates = []RoutingCandidate{validRoutingCandidate(t, request)}
	return request
}

func validRoutingCandidate(t *testing.T, request RoutingRequest) RoutingCandidate {
	t.Helper()
	candidate := RoutingCandidate{
		ID:   testRoutingID(t),
		Zone: ZonePersonal,
		Model: ModelDescriptor{
			Ref: ModelRef{Provider: "local", Model: "reasoner"},
			Capabilities: ModelCapabilities{
				ContextTokens:    4096,
				MaxOutputTokens:  2048,
				Modalities:       []string{"text"},
				ToolUse:          ToolUseNative,
				StructuredOutput: true,
			},
			Labels: map[string]string{"quality_class": "standard"},
		},
		Authenticated:         true,
		Compatible:            true,
		Healthy:               true,
		AvailableTools:        []string{"search"},
		AvailableSlots:        2,
		AvailableAt:           request.ObservedAt,
		ExpectedCost:          100,
		ExpectedLatency:       time.Second,
		QualityPermille:       800,
		CacheAffinityPermille: 400,
		LoadPermille:          200,
	}
	candidate.ExecutionVerdict = routingAllowVerdict(
		request.ManifestSHA256,
		routingExecutionScope(request, candidate),
	)
	return candidate
}

func routingAllowVerdict(manifestDigest string, scope PolicyScope) PolicyVerdict {
	return PolicyVerdict{
		Policy:         "test",
		PolicyVersion:  "1",
		RuleID:         "test.allow",
		Action:         PolicyAllow,
		Reason:         "allowed for routing test",
		ManifestSHA256: manifestDigest,
		Scope:          scope,
	}
}

func validRoutingDecision(request RoutingRequest) RoutingDecision {
	score := RoutingScore{Total: 1}
	return RoutingDecision{
		Router:         "test",
		RouterVersion:  "1",
		WorkloadID:     request.WorkloadID,
		StepID:         request.StepID,
		AttemptID:      request.AttemptID,
		ManifestSHA256: request.ManifestSHA256,
		SourceZone:     request.SourceZone,
		Autonomous:     request.Autonomous,
		EgressPurpose:  request.EgressPurpose,
		DerivedFromSHA: request.DerivedFromSHA,
		ObservedAt:     request.ObservedAt,
		Deadline:       request.Deadline,
		Weights:        request.Weights,
		Assessments: []CandidateAssessment{{
			CandidateID: request.Candidates[0].ID,
			Zone:        request.Candidates[0].Zone,
			Model:       request.Candidates[0].Model.Ref,
			Eligible:    true,
			Score:       &score,
		}},
		SelectedCandidateID: request.Candidates[0].ID,
	}
}

func testRoutingID(t *testing.T) ID {
	t.Helper()
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
