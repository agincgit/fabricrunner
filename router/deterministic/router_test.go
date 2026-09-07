package deterministic_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agincgit/fabricrunner"
	"github.com/agincgit/fabricrunner/router/deterministic"
)

func TestEligibilityUsesStableOrderedExclusions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		code   string
		mutate func(*fabricrunner.RoutingRequest, *fabricrunner.RoutingCandidate)
	}{
		{name: "unauthenticated first", code: "identity.unauthenticated", mutate: func(_ *fabricrunner.RoutingRequest, candidate *fabricrunner.RoutingCandidate) {
			candidate.Authenticated = false
			candidate.Compatible = false
			candidate.Healthy = false
		}},
		{name: "incompatible", code: "compatibility.unsatisfied", mutate: func(_ *fabricrunner.RoutingRequest, candidate *fabricrunner.RoutingCandidate) {
			candidate.Compatible = false
		}},
		{name: "unhealthy", code: "health.unhealthy", mutate: func(_ *fabricrunner.RoutingRequest, candidate *fabricrunner.RoutingCandidate) {
			candidate.Healthy = false
		}},
		{name: "draining", code: "health.draining", mutate: func(_ *fabricrunner.RoutingRequest, candidate *fabricrunner.RoutingCandidate) {
			candidate.Draining = true
		}},
		{name: "zone denied", code: "policy.zone.denied", mutate: func(request *fabricrunner.RoutingRequest, candidate *fabricrunner.RoutingCandidate) {
			candidate.Zone = fabricrunner.ZoneManagedCloud
			request.Requirements.AllowedZones = []fabricrunner.Zone{fabricrunner.ZonePersonal}
		}},
		{name: "execution invalid", code: "policy.execution.invalid", mutate: func(_ *fabricrunner.RoutingRequest, candidate *fabricrunner.RoutingCandidate) {
			candidate.ExecutionVerdict = fabricrunner.PolicyVerdict{}
		}},
		{name: "execution manifest mismatch", code: "policy.execution.manifest_mismatch", mutate: func(_ *fabricrunner.RoutingRequest, candidate *fabricrunner.RoutingCandidate) {
			candidate.ExecutionVerdict.ManifestSHA256 = strings.Repeat("b", 64)
		}},
		{name: "execution scope mismatch", code: "policy.execution.scope_mismatch", mutate: func(_ *fabricrunner.RoutingRequest, candidate *fabricrunner.RoutingCandidate) {
			candidate.ExecutionVerdict.Scope.Target.Zone = fabricrunner.ZoneSelfCloud
		}},
		{name: "execution not allowed", code: "policy.execution.not_allowed", mutate: func(_ *fabricrunner.RoutingRequest, candidate *fabricrunner.RoutingCandidate) {
			candidate.ExecutionVerdict.Action = fabricrunner.PolicyRequireApproval
		}},
		{name: "egress missing", code: "egress.verdict.missing", mutate: func(_ *fabricrunner.RoutingRequest, candidate *fabricrunner.RoutingCandidate) {
			candidate.Zone = fabricrunner.ZoneManagedCloud
			candidate.ExecutionVerdict.Scope.Target.Zone = fabricrunner.ZoneManagedCloud
		}},
		{name: "egress manifest mismatch", code: "policy.egress.manifest_mismatch", mutate: func(request *fabricrunner.RoutingRequest, candidate *fabricrunner.RoutingCandidate) {
			candidate.Zone = fabricrunner.ZoneManagedCloud
			candidate.ExecutionVerdict.Scope.Target.Zone = fabricrunner.ZoneManagedCloud
			verdict := allowVerdict(request.ManifestSHA256, dataEgressScope(*request, *candidate))
			verdict.ManifestSHA256 = strings.Repeat("b", 64)
			candidate.EgressVerdict = &verdict
		}},
		{name: "egress scope mismatch", code: "policy.egress.scope_mismatch", mutate: func(request *fabricrunner.RoutingRequest, candidate *fabricrunner.RoutingCandidate) {
			candidate.Zone = fabricrunner.ZoneManagedCloud
			candidate.ExecutionVerdict.Scope.Target.Zone = fabricrunner.ZoneManagedCloud
			verdict := allowVerdict(request.ManifestSHA256, dataEgressScope(*request, *candidate))
			verdict.Scope.Destination = fabricrunner.ZoneSelfCloud
			candidate.EgressVerdict = &verdict
		}},
		{name: "egress not allowed", code: "policy.egress.not_allowed", mutate: func(request *fabricrunner.RoutingRequest, candidate *fabricrunner.RoutingCandidate) {
			candidate.Zone = fabricrunner.ZoneManagedCloud
			candidate.ExecutionVerdict.Scope.Target.Zone = fabricrunner.ZoneManagedCloud
			verdict := allowVerdict(request.ManifestSHA256, dataEgressScope(*request, *candidate))
			verdict.Action = fabricrunner.PolicyRedact
			verdict.Transformation = "redact and reevaluate"
			candidate.EgressVerdict = &verdict
		}},
		{name: "modality missing", code: "capability.modality.missing", mutate: func(_ *fabricrunner.RoutingRequest, candidate *fabricrunner.RoutingCandidate) {
			candidate.Model.Capabilities.Modalities = []string{"image"}
		}},
		{name: "tool missing", code: "capability.tool.missing", mutate: func(_ *fabricrunner.RoutingRequest, candidate *fabricrunner.RoutingCandidate) {
			candidate.AvailableTools = []string{"read"}
		}},
		{name: "tool use missing", code: "capability.tool_use.missing", mutate: func(request *fabricrunner.RoutingRequest, candidate *fabricrunner.RoutingCandidate) {
			request.Requirements.NativeToolUse = false
			candidate.Model.Capabilities.ToolUse = fabricrunner.ToolUseNone
		}},
		{name: "native tool use missing", code: "capability.native_tool_use.missing", mutate: func(_ *fabricrunner.RoutingRequest, candidate *fabricrunner.RoutingCandidate) {
			candidate.Model.Capabilities.ToolUse = fabricrunner.ToolUseParsed
		}},
		{name: "structured output missing", code: "capability.structured_output.missing", mutate: func(_ *fabricrunner.RoutingRequest, candidate *fabricrunner.RoutingCandidate) {
			candidate.Model.Capabilities.StructuredOutput = false
		}},
		{name: "context insufficient", code: "limit.context.insufficient", mutate: func(request *fabricrunner.RoutingRequest, _ *fabricrunner.RoutingCandidate) {
			request.Requirements.MinContextTokens = 8192
		}},
		{name: "output insufficient", code: "limit.output.insufficient", mutate: func(request *fabricrunner.RoutingRequest, _ *fabricrunner.RoutingCandidate) {
			request.Budget.MaxOutputTokens = 4096
		}},
		{name: "cost unreservable", code: "budget.cost.unreservable", mutate: func(request *fabricrunner.RoutingRequest, _ *fabricrunner.RoutingCandidate) {
			request.Budget.MaxCost = 99
		}},
		{name: "capacity unavailable", code: "capacity.unavailable", mutate: func(_ *fabricrunner.RoutingRequest, candidate *fabricrunner.RoutingCandidate) {
			candidate.AvailableSlots = 0
		}},
		{name: "deadline missed", code: "capacity.deadline", mutate: func(request *fabricrunner.RoutingRequest, candidate *fabricrunner.RoutingCandidate) {
			candidate.ExpectedLatency = request.Deadline.Sub(request.ObservedAt) + time.Nanosecond
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := routeRequest(t)
			test.mutate(&request, &request.Candidates[0])
			decision, err := fabricrunner.EvaluateRouter(context.Background(), deterministic.Router{}, request)
			if !errors.Is(err, fabricrunner.ErrNoEligibleCandidates) {
				t.Fatalf("route error = %v, want ErrNoEligibleCandidates", err)
			}
			assessment := decision.Assessments[0]
			if assessment.Exclusion == nil || assessment.Exclusion.Code != test.code || assessment.Score != nil {
				t.Fatalf("assessment = %#v, want exclusion %q and no score", assessment, test.code)
			}
		})
	}
}

func TestSameZoneDoesNotRequireEgressVerdict(t *testing.T) {
	t.Parallel()

	request := routeRequest(t)
	if request.Candidates[0].EgressVerdict != nil {
		t.Fatal("test candidate unexpectedly has egress verdict")
	}
	decision, err := fabricrunner.EvaluateRouter(context.Background(), deterministic.Router{}, request)
	if err != nil || decision.SelectedCandidateID != request.Candidates[0].ID {
		t.Fatalf("same-zone route = %#v, %v", decision, err)
	}
}

func TestCrossZoneRequiresAndAcceptsBoundEgressVerdict(t *testing.T) {
	t.Parallel()

	request := routeRequest(t)
	candidate := &request.Candidates[0]
	candidate.Zone = fabricrunner.ZoneManagedCloud
	candidate.ExecutionVerdict = allowVerdict(request.ManifestSHA256, executionScope(request, *candidate))
	egressVerdict := allowVerdict(request.ManifestSHA256, dataEgressScope(request, *candidate))
	candidate.EgressVerdict = &egressVerdict
	decision, err := fabricrunner.EvaluateRouter(context.Background(), deterministic.Router{}, request)
	if err != nil || decision.SelectedCandidateID != candidate.ID {
		t.Fatalf("cross-zone route = %#v, %v", decision, err)
	}
}

func TestExcludedCandidateCannotBeScoredOrSelected(t *testing.T) {
	t.Parallel()

	request := routeRequest(t)
	excluded := request.Candidates[0].Clone()
	excluded.Authenticated = false
	excluded.QualityPermille = fabricrunner.ScorePermilleMax
	eligible := request.Candidates[0].Clone()
	eligible.ID = testID(t)
	eligible.QualityPermille = 1
	request.Candidates = []fabricrunner.RoutingCandidate{excluded, eligible}

	decision, err := fabricrunner.EvaluateRouter(context.Background(), deterministic.Router{}, request)
	if err != nil {
		t.Fatal(err)
	}
	if decision.SelectedCandidateID != eligible.ID || decision.Assessments[0].Score != nil ||
		decision.Assessments[1].Score == nil {
		t.Fatalf("excluded candidate influenced route: %#v", decision)
	}
}

func TestDeterministicScoreComponents(t *testing.T) {
	t.Parallel()

	request := routeRequest(t)
	request.Candidates[0].AvailableAt = request.ObservedAt.Add(time.Second)
	decision, err := fabricrunner.EvaluateRouter(context.Background(), deterministic.Router{}, request)
	if err != nil {
		t.Fatal(err)
	}
	want := fabricrunner.RoutingScore{
		Quality:       800,
		Locality:      1000,
		Availability:  200,
		CacheAffinity: 400,
		Cost:          900,
		Latency:       800,
		InverseLoad:   800,
		Total:         755,
	}
	if got := *decision.Assessments[0].Score; got != want {
		t.Fatalf("score = %#v, want %#v", got, want)
	}
}

func TestScoringHandlesExactDeadlineAndLargeIntegerInputs(t *testing.T) {
	t.Parallel()

	request := routeRequest(t)
	request.Budget.MaxCost = fabricrunner.CostMicros(^uint64(0) >> 1)
	candidate := &request.Candidates[0]
	candidate.ExpectedCost = request.Budget.MaxCost
	candidate.AvailableSlots = int(^uint(0) >> 1)
	candidate.ExpectedLatency = request.Deadline.Sub(request.ObservedAt)
	decision, err := fabricrunner.EvaluateRouter(context.Background(), deterministic.Router{}, request)
	if err != nil {
		t.Fatal(err)
	}
	score := decision.Assessments[0].Score
	if score == nil || score.Cost != 0 || score.Availability != fabricrunner.ScorePermilleMax || score.Latency != 0 {
		t.Fatalf("large-input score = %#v", score)
	}
}

func TestTieUsesLexicallySmallestCandidateAndIsRepeatable(t *testing.T) {
	t.Parallel()

	request := routeRequest(t)
	first := request.Candidates[0].Clone()
	second := first.Clone()
	second.ID = testID(t)
	request.Candidates = []fabricrunner.RoutingCandidate{first, second}
	want := first.ID
	if string(second.ID) < string(first.ID) {
		want = second.ID
	}

	decisionOne, err := fabricrunner.EvaluateRouter(context.Background(), deterministic.Router{}, request)
	if err != nil {
		t.Fatal(err)
	}
	request.Candidates[0], request.Candidates[1] = request.Candidates[1], request.Candidates[0]
	decisionTwo, err := fabricrunner.EvaluateRouter(context.Background(), deterministic.Router{}, request)
	if err != nil {
		t.Fatal(err)
	}
	if decisionOne.SelectedCandidateID != want || decisionTwo.SelectedCandidateID != want {
		t.Fatalf("tie selections = %s, %s; want %s", decisionOne.SelectedCandidateID, decisionTwo.SelectedCandidateID, want)
	}

	repeatedRequest := routeRequest(t)
	firstRun, err := fabricrunner.EvaluateRouter(context.Background(), deterministic.Router{}, repeatedRequest)
	if err != nil {
		t.Fatal(err)
	}
	secondRun, err := fabricrunner.EvaluateRouter(context.Background(), deterministic.Router{}, repeatedRequest)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := json.Marshal(firstRun)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(secondRun)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("repeated decisions differ:\n%s\n%s", firstJSON, secondJSON)
	}
}

func TestRouterHonorsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	decision, err := deterministic.Router{}.Route(ctx, routeRequest(t))
	if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(decision, fabricrunner.RoutingDecision{}) {
		t.Fatalf("canceled route = %#v, %v", decision, err)
	}
}

func routeRequest(t *testing.T) fabricrunner.RoutingRequest {
	t.Helper()
	observedAt := time.Date(2026, time.September, 7, 12, 0, 0, 0, time.UTC)
	manifestDigest := strings.Repeat("a", 64)
	request := fabricrunner.RoutingRequest{
		WorkloadID:     testID(t),
		StepID:         testID(t),
		AttemptID:      testID(t),
		ManifestSHA256: manifestDigest,
		SourceZone:     fabricrunner.ZonePersonal,
		EgressPurpose:  "route model input",
		Requirements: fabricrunner.Requirements{
			Modalities:       []string{"text"},
			RequiredTools:    []string{"search"},
			MinContextTokens: 1024,
			MinQualityClass:  "standard",
			AllowedZones: []fabricrunner.Zone{
				fabricrunner.ZonePersonal,
				fabricrunner.ZoneSelfCloud,
				fabricrunner.ZoneManagedCloud,
			},
			NativeToolUse:    true,
			StructuredOutput: true,
		},
		Budget: fabricrunner.Budget{
			MaxOutputTokens: 1000,
			MaxCost:         1000,
		},
		ObservedAt: observedAt,
		Deadline:   observedAt.Add(10 * time.Second),
		Weights:    fabricrunner.DefaultRoutingWeights(),
	}
	request.Candidates = []fabricrunner.RoutingCandidate{candidate(t, request)}
	return request
}

func candidate(t *testing.T, request fabricrunner.RoutingRequest) fabricrunner.RoutingCandidate {
	t.Helper()
	candidate := fabricrunner.RoutingCandidate{
		ID:   testID(t),
		Zone: fabricrunner.ZonePersonal,
		Model: fabricrunner.ModelDescriptor{
			Ref: fabricrunner.ModelRef{Provider: "local", Model: "reasoner"},
			Capabilities: fabricrunner.ModelCapabilities{
				ContextTokens:    4096,
				MaxOutputTokens:  2048,
				Modalities:       []string{"text"},
				ToolUse:          fabricrunner.ToolUseNative,
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
	candidate.ExecutionVerdict = allowVerdict(
		request.ManifestSHA256,
		executionScope(request, candidate),
	)
	return candidate
}

func allowVerdict(
	manifestDigest string,
	scope fabricrunner.PolicyScope,
) fabricrunner.PolicyVerdict {
	return fabricrunner.PolicyVerdict{
		Policy:         "test",
		PolicyVersion:  "1",
		RuleID:         "test.allow",
		Action:         fabricrunner.PolicyAllow,
		Reason:         "allowed for routing test",
		ManifestSHA256: manifestDigest,
		Scope:          scope,
	}
}

func executionScope(
	request fabricrunner.RoutingRequest,
	candidate fabricrunner.RoutingCandidate,
) fabricrunner.PolicyScope {
	return fabricrunner.PolicyScopeForExecution(fabricrunner.ExecutionPolicyRequest{
		WorkloadID: request.WorkloadID,
		StepID:     request.StepID,
		AttemptID:  request.AttemptID,
		StepKind:   fabricrunner.StepModelTurn,
		Autonomous: request.Autonomous,
		Target: fabricrunner.ExecutionTarget{
			Kind:  fabricrunner.ExecutionTargetModel,
			Zone:  candidate.Zone,
			Model: &candidate.Model.Ref,
		},
	})
}

func dataEgressScope(
	request fabricrunner.RoutingRequest,
	candidate fabricrunner.RoutingCandidate,
) fabricrunner.PolicyScope {
	return fabricrunner.PolicyScopeForDataEgress(fabricrunner.DataEgressPolicyRequest{
		WorkloadID:     request.WorkloadID,
		StepID:         request.StepID,
		Source:         request.SourceZone,
		Destination:    candidate.Zone,
		Purpose:        request.EgressPurpose,
		DerivedFromSHA: request.DerivedFromSHA,
	})
}

func testID(t *testing.T) fabricrunner.ID {
	t.Helper()
	id, err := fabricrunner.NewID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
