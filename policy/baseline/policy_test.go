package baseline_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/agincgit/fabricrunner"
	"github.com/agincgit/fabricrunner/policy/baseline"
)

func TestExecutionPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		target         fabricrunner.ExecutionTarget
		classification fabricrunner.Classification
		want           fabricrunner.PolicyAction
	}{
		{name: "public model", target: modelTarget(), classification: fabricrunner.ClassPublic, want: fabricrunner.PolicyAllow},
		{name: "confidential model", target: modelTarget(), classification: fabricrunner.ClassConfidential, want: fabricrunner.PolicyAllow},
		{name: "secret model", target: modelTarget(), classification: fabricrunner.ClassSecret, want: fabricrunner.PolicyDeny},
		{name: "public tool", target: toolTarget(), classification: fabricrunner.ClassPublic, want: fabricrunner.PolicyRequireApproval},
		{name: "secret tool", target: toolTarget(), classification: fabricrunner.ClassSecret, want: fabricrunner.PolicyDeny},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := executionRequest(t, test.target, test.classification)
			verdict, err := fabricrunner.EvaluateExecutionPolicy(context.Background(), baseline.Policy{}, request)
			if err != nil {
				t.Fatal(err)
			}
			if verdict.Action != test.want {
				t.Fatalf("action = %q, want %q", verdict.Action, test.want)
			}
			if err := verdict.Validate(); err != nil {
				t.Fatalf("verdict validation error = %v", err)
			}
		})
	}
}

func TestDataEgressPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		source         fabricrunner.Zone
		destination    fabricrunner.Zone
		classification fabricrunner.Classification
		want           fabricrunner.PolicyAction
	}{
		{name: "same zone secret", source: fabricrunner.ZonePersonal, destination: fabricrunner.ZonePersonal, classification: fabricrunner.ClassSecret, want: fabricrunner.PolicyAllow},
		{name: "cross zone public", source: fabricrunner.ZonePersonal, destination: fabricrunner.ZoneManagedCloud, classification: fabricrunner.ClassPublic, want: fabricrunner.PolicyAllow},
		{name: "cross zone internal", source: fabricrunner.ZonePersonal, destination: fabricrunner.ZoneSelfCloud, classification: fabricrunner.ClassInternal, want: fabricrunner.PolicyRequireApproval},
		{name: "cross zone confidential", source: fabricrunner.ZoneSelfCloud, destination: fabricrunner.ZoneManagedCloud, classification: fabricrunner.ClassConfidential, want: fabricrunner.PolicyRequireApproval},
		{name: "cross zone secret", source: fabricrunner.ZonePersonal, destination: fabricrunner.ZoneSelfCloud, classification: fabricrunner.ClassSecret, want: fabricrunner.PolicyDeny},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := egressRequest(t, test.source, test.destination, test.classification)
			first, err := fabricrunner.EvaluateDataEgressPolicy(context.Background(), baseline.Policy{}, request)
			if err != nil {
				t.Fatal(err)
			}
			second, err := fabricrunner.EvaluateDataEgressPolicy(context.Background(), baseline.Policy{}, request)
			if err != nil {
				t.Fatal(err)
			}
			if first.Action != test.want {
				t.Fatalf("action = %q, want %q", first.Action, test.want)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatalf("baseline verdict is not deterministic: %#v != %#v", first, second)
			}
		})
	}
}

func TestUnlabeledCrossZoneContentRequiresApproval(t *testing.T) {
	t.Parallel()

	request := egressRequest(t, fabricrunner.ZonePersonal, fabricrunner.ZoneManagedCloud, "")
	verdict, err := fabricrunner.EvaluateDataEgressPolicy(context.Background(), baseline.Policy{}, request)
	if err != nil {
		t.Fatal(err)
	}
	if verdict.Action != fabricrunner.PolicyRequireApproval {
		t.Fatalf("action = %q, want require_approval", verdict.Action)
	}
}

func TestBaselineHonorsCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	verdict, err := baseline.Policy{}.EvaluateExecution(
		ctx,
		executionRequest(t, modelTarget(), fabricrunner.ClassPublic),
	)
	if !errors.Is(err, context.Canceled) || verdict != (fabricrunner.PolicyVerdict{}) {
		t.Fatalf("canceled evaluation = %#v, %v", verdict, err)
	}
}

func executionRequest(
	t *testing.T,
	target fabricrunner.ExecutionTarget,
	classification fabricrunner.Classification,
) fabricrunner.ExecutionPolicyRequest {
	t.Helper()
	stepKind := fabricrunner.StepModelTurn
	if target.Kind == fabricrunner.ExecutionTargetTool {
		stepKind = fabricrunner.StepTool
	}
	return fabricrunner.ExecutionPolicyRequest{
		WorkloadID: testID(t),
		StepID:     testID(t),
		AttemptID:  testID(t),
		StepKind:   stepKind,
		Target:     target,
		Manifest:   manifest(classification),
	}
}

func egressRequest(
	t *testing.T,
	source fabricrunner.Zone,
	destination fabricrunner.Zone,
	classification fabricrunner.Classification,
) fabricrunner.DataEgressPolicyRequest {
	t.Helper()
	return fabricrunner.DataEgressPolicyRequest{
		WorkloadID:  testID(t),
		StepID:      testID(t),
		Source:      source,
		Destination: destination,
		Manifest:    manifest(classification),
		Purpose:     "bounded test transfer",
	}
}

func modelTarget() fabricrunner.ExecutionTarget {
	return fabricrunner.ExecutionTarget{
		Kind:  fabricrunner.ExecutionTargetModel,
		Zone:  fabricrunner.ZoneManagedCloud,
		Model: &fabricrunner.ModelRef{Provider: "managed", Model: "frontier"},
	}
}

func toolTarget() fabricrunner.ExecutionTarget {
	return fabricrunner.ExecutionTarget{
		Kind:     fabricrunner.ExecutionTargetTool,
		Zone:     fabricrunner.ZonePersonal,
		ToolName: "read",
	}
}

func manifest(classification fabricrunner.Classification) fabricrunner.ContentManifest {
	return fabricrunner.ContentManifest{Items: []fabricrunner.ContentItem{{
		SHA256:         strings.Repeat("a", 64),
		MediaType:      "text/plain",
		SizeBytes:      5,
		Classification: classification,
	}}}
}

func testID(t *testing.T) fabricrunner.ID {
	t.Helper()
	id, err := fabricrunner.NewID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}
