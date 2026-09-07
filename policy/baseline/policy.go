// Package baseline provides Fabric Runner's deterministic minimum policy.
package baseline

import (
	"context"
	"fmt"

	"github.com/agincgit/fabricrunner"
)

const (
	policyName    = "fabricrunner.baseline"
	policyVersion = "1"
)

type Policy struct{}

func (Policy) EvaluateExecution(
	ctx context.Context,
	request fabricrunner.ExecutionPolicyRequest,
) (fabricrunner.PolicyVerdict, error) {
	if err := ctx.Err(); err != nil {
		return fabricrunner.PolicyVerdict{}, err
	}
	if err := request.Validate(); err != nil {
		return fabricrunner.PolicyVerdict{}, err
	}
	classification, err := request.Manifest.EffectiveClassification()
	if err != nil {
		return fabricrunner.PolicyVerdict{}, err
	}
	manifestDigest, err := request.Manifest.Digest()
	if err != nil {
		return fabricrunner.PolicyVerdict{}, err
	}
	scope := fabricrunner.PolicyScopeForExecution(request)
	if classification == fabricrunner.ClassSecret {
		return verdict(manifestDigest, scope,
			fabricrunner.PolicyDeny,
			"execution.secret.denied",
			"baseline policy does not grant execution access to secret content",
		), nil
	}
	switch request.Target.Kind {
	case fabricrunner.ExecutionTargetModel:
		return verdict(manifestDigest, scope,
			fabricrunner.PolicyAllow,
			"execution.model.eligible",
			"model execution is eligible after content classification checks",
		), nil
	case fabricrunner.ExecutionTargetTool:
		return verdict(manifestDigest, scope,
			fabricrunner.PolicyRequireApproval,
			"execution.tool.approval",
			"baseline policy requires approval for tool execution",
		), nil
	default:
		return fabricrunner.PolicyVerdict{}, fmt.Errorf("unsupported target kind %q", request.Target.Kind)
	}
}

func (Policy) EvaluateDataEgress(
	ctx context.Context,
	request fabricrunner.DataEgressPolicyRequest,
) (fabricrunner.PolicyVerdict, error) {
	if err := ctx.Err(); err != nil {
		return fabricrunner.PolicyVerdict{}, err
	}
	if err := request.Validate(); err != nil {
		return fabricrunner.PolicyVerdict{}, err
	}
	manifestDigest, err := request.Manifest.Digest()
	if err != nil {
		return fabricrunner.PolicyVerdict{}, err
	}
	scope := fabricrunner.PolicyScopeForDataEgress(request)
	if request.Source == request.Destination {
		return verdict(manifestDigest, scope,
			fabricrunner.PolicyAllow,
			"egress.same-zone",
			"content remains within the same trust zone",
		), nil
	}
	classification, err := request.Manifest.EffectiveClassification()
	if err != nil {
		return fabricrunner.PolicyVerdict{}, err
	}
	switch classification {
	case fabricrunner.ClassPublic:
		return verdict(manifestDigest, scope,
			fabricrunner.PolicyAllow,
			"egress.public",
			"public content may cross trust zones",
		), nil
	case fabricrunner.ClassInternal, fabricrunner.ClassConfidential:
		return verdict(manifestDigest, scope,
			fabricrunner.PolicyRequireApproval,
			"egress.approval",
			"non-public content requires approval before crossing trust zones",
		), nil
	case fabricrunner.ClassSecret:
		return verdict(manifestDigest, scope,
			fabricrunner.PolicyDeny,
			"egress.secret.denied",
			"secret content cannot cross trust zones under baseline policy",
		), nil
	default:
		return fabricrunner.PolicyVerdict{}, fmt.Errorf("unsupported classification %q", classification)
	}
}

func verdict(
	manifestDigest string,
	scope fabricrunner.PolicyScope,
	action fabricrunner.PolicyAction,
	ruleID, reason string,
) fabricrunner.PolicyVerdict {
	return fabricrunner.PolicyVerdict{
		Policy:         policyName,
		PolicyVersion:  policyVersion,
		RuleID:         ruleID,
		Action:         action,
		Reason:         reason,
		ManifestSHA256: manifestDigest,
		Scope:          scope,
	}
}

var (
	_ fabricrunner.ExecutionPolicy  = Policy{}
	_ fabricrunner.DataEgressPolicy = Policy{}
)
