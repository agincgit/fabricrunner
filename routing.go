package fabricrunner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const ScorePermilleMax = 1000

// RoutingWeights controls the deterministic contribution of each normalized
// score component. Valid weights are nonnegative and total 1000.
type RoutingWeights struct {
	Quality       int `json:"quality"`
	Locality      int `json:"locality"`
	Availability  int `json:"availability"`
	CacheAffinity int `json:"cache_affinity"`
	Cost          int `json:"cost"`
	Latency       int `json:"latency"`
	InverseLoad   int `json:"inverse_load"`
}

func DefaultRoutingWeights() RoutingWeights {
	return RoutingWeights{
		Quality:       250,
		Locality:      200,
		Availability:  100,
		CacheAffinity: 100,
		Cost:          150,
		Latency:       150,
		InverseLoad:   50,
	}
}

func (weights RoutingWeights) Validate() error {
	values := []struct {
		name  string
		value int
	}{
		{name: "quality", value: weights.Quality},
		{name: "locality", value: weights.Locality},
		{name: "availability", value: weights.Availability},
		{name: "cache affinity", value: weights.CacheAffinity},
		{name: "cost", value: weights.Cost},
		{name: "latency", value: weights.Latency},
		{name: "inverse load", value: weights.InverseLoad},
	}
	total := 0
	for _, entry := range values {
		if entry.value < 0 || entry.value > ScorePermilleMax {
			return fmt.Errorf("routing %s weight must be between 0 and %d", entry.name, ScorePermilleMax)
		}
		total += entry.value
	}
	if total != ScorePermilleMax {
		return fmt.Errorf("routing weights total %d, want %d", total, ScorePermilleMax)
	}
	return nil
}

// RoutingCandidate is one immutable placement observation supplied to a
// router. Extension fields from providers deliberately have no representation.
type RoutingCandidate struct {
	ID                    ID              `json:"id"`
	Zone                  Zone            `json:"zone"`
	Model                 ModelDescriptor `json:"model"`
	Authenticated         bool            `json:"authenticated"`
	Compatible            bool            `json:"compatible"`
	Healthy               bool            `json:"healthy"`
	Draining              bool            `json:"draining"`
	ExecutionVerdict      PolicyVerdict   `json:"execution_verdict"`
	EgressVerdict         *PolicyVerdict  `json:"egress_verdict,omitempty"`
	AvailableTools        []string        `json:"available_tools,omitempty"`
	AvailableSlots        int             `json:"available_slots"`
	AvailableAt           time.Time       `json:"available_at"`
	ExpectedCost          CostMicros      `json:"expected_cost_micros"`
	ExpectedLatency       time.Duration   `json:"expected_latency"`
	QualityPermille       int             `json:"quality_permille"`
	CacheAffinityPermille int             `json:"cache_affinity_permille"`
	LoadPermille          int             `json:"load_permille"`
}

func (candidate RoutingCandidate) Validate() error {
	if err := candidate.ID.Validate(); err != nil {
		return fmt.Errorf("candidate ID: %w", err)
	}
	if err := candidate.Zone.Validate(); err != nil {
		return fmt.Errorf("candidate zone: %w", err)
	}
	if err := candidate.Model.Ref.Validate(); err != nil {
		return fmt.Errorf("candidate model: %w", err)
	}
	if err := validateModelCapabilities(candidate.Model.Capabilities); err != nil {
		return fmt.Errorf("candidate model capabilities: %w", err)
	}
	if err := validateStringSet("candidate available tool", candidate.AvailableTools); err != nil {
		return err
	}
	if candidate.AvailableSlots < 0 {
		return errors.New("candidate available slots cannot be negative")
	}
	if candidate.AvailableAt.IsZero() {
		return errors.New("candidate available-at time is required")
	}
	if candidate.ExpectedCost < 0 {
		return errors.New("candidate expected cost cannot be negative")
	}
	if candidate.ExpectedLatency < 0 {
		return errors.New("candidate expected latency cannot be negative")
	}
	for _, observation := range []struct {
		name  string
		value int
	}{
		{name: "quality", value: candidate.QualityPermille},
		{name: "cache affinity", value: candidate.CacheAffinityPermille},
		{name: "load", value: candidate.LoadPermille},
	} {
		if observation.value < 0 || observation.value > ScorePermilleMax {
			return fmt.Errorf("candidate %s must be between 0 and %d", observation.name, ScorePermilleMax)
		}
	}
	return nil
}

func (candidate RoutingCandidate) Clone() RoutingCandidate {
	candidate.Model.Capabilities.Modalities = append(
		[]string(nil), candidate.Model.Capabilities.Modalities...,
	)
	if candidate.Model.Labels != nil {
		labels := make(map[string]string, len(candidate.Model.Labels))
		for key, value := range candidate.Model.Labels {
			labels[key] = value
		}
		candidate.Model.Labels = labels
	}
	candidate.AvailableTools = append([]string(nil), candidate.AvailableTools...)
	candidate.ExecutionVerdict = candidate.ExecutionVerdict.Clone()
	if candidate.EgressVerdict != nil {
		verdict := candidate.EgressVerdict.Clone()
		candidate.EgressVerdict = &verdict
	}
	return candidate
}

// RoutingRequest is a fixed observation used for one replayable placement.
type RoutingRequest struct {
	WorkloadID     ID                 `json:"workload_id"`
	StepID         ID                 `json:"step_id"`
	AttemptID      ID                 `json:"attempt_id"`
	ManifestSHA256 string             `json:"manifest_sha256"`
	SourceZone     Zone               `json:"source_zone"`
	Autonomous     bool               `json:"autonomous"`
	EgressPurpose  string             `json:"egress_purpose,omitempty"`
	DerivedFromSHA string             `json:"derived_from_sha256,omitempty"`
	Requirements   Requirements       `json:"requirements"`
	Budget         Budget             `json:"budget"`
	ObservedAt     time.Time          `json:"observed_at"`
	Deadline       time.Time          `json:"deadline"`
	Weights        RoutingWeights     `json:"weights"`
	Candidates     []RoutingCandidate `json:"candidates"`
}

func (request RoutingRequest) Validate() error {
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
	if err := validateSHA256("routing manifest SHA-256", request.ManifestSHA256); err != nil {
		return err
	}
	if err := request.SourceZone.Validate(); err != nil {
		return fmt.Errorf("routing source zone: %w", err)
	}
	if request.DerivedFromSHA != "" {
		if err := validateSHA256("routing derived-from SHA-256", request.DerivedFromSHA); err != nil {
			return err
		}
	}
	if err := request.Requirements.Validate(); err != nil {
		return err
	}
	if err := validateStringSet("required modality", request.Requirements.Modalities); err != nil {
		return err
	}
	if err := validateStringSet("required tool", request.Requirements.RequiredTools); err != nil {
		return err
	}
	if err := validateZoneSet(request.Requirements.AllowedZones); err != nil {
		return err
	}
	if err := request.Budget.Validate(); err != nil {
		return err
	}
	if request.ObservedAt.IsZero() {
		return errors.New("routing observation time is required")
	}
	if request.Deadline.IsZero() || !request.Deadline.After(request.ObservedAt) {
		return errors.New("routing deadline must be after observation time")
	}
	window := request.Deadline.Sub(request.ObservedAt)
	if !request.ObservedAt.Add(window).Equal(request.Deadline) {
		return errors.New("routing observation-to-deadline window exceeds time.Duration range")
	}
	if err := request.Weights.Validate(); err != nil {
		return err
	}
	if len(request.Candidates) == 0 {
		return errors.New("routing candidates are required")
	}
	seen := make(map[ID]struct{}, len(request.Candidates))
	crossZone := false
	for index, candidate := range request.Candidates {
		if err := candidate.Validate(); err != nil {
			return fmt.Errorf("routing candidate %d: %w", index, err)
		}
		if _, exists := seen[candidate.ID]; exists {
			return fmt.Errorf("routing candidate %d duplicates ID %s", index, candidate.ID)
		}
		seen[candidate.ID] = struct{}{}
		crossZone = crossZone || candidate.Zone != request.SourceZone
	}
	if crossZone && strings.TrimSpace(request.EgressPurpose) == "" {
		return errors.New("routing egress purpose is required for cross-zone candidates")
	}
	return nil
}

func (request RoutingRequest) Clone() RoutingRequest {
	request.Requirements.Modalities = append([]string(nil), request.Requirements.Modalities...)
	request.Requirements.RequiredTools = append([]string(nil), request.Requirements.RequiredTools...)
	request.Requirements.AllowedZones = append([]Zone(nil), request.Requirements.AllowedZones...)
	request.Candidates = append([]RoutingCandidate(nil), request.Candidates...)
	for index := range request.Candidates {
		request.Candidates[index] = request.Candidates[index].Clone()
	}
	return request
}

type RoutingExclusion struct {
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

// CandidateEligibility is the mandatory pre-scoring result for one candidate.
type CandidateEligibility struct {
	CandidateID ID                `json:"candidate_id"`
	Zone        Zone              `json:"zone"`
	Model       ModelRef          `json:"model"`
	Exclusion   *RoutingExclusion `json:"exclusion,omitempty"`
}

func (eligibility CandidateEligibility) Eligible() bool {
	return eligibility.Exclusion == nil
}

// EvaluateRoutingEligibility applies the canonical ordered gates. Scorers may
// consume this result but cannot alter it in a valid routing decision.
func EvaluateRoutingEligibility(
	ctx context.Context,
	request RoutingRequest,
) ([]CandidateEligibility, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	results := make([]CandidateEligibility, 0, len(request.Candidates))
	for _, candidate := range request.Candidates {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		results = append(results, CandidateEligibility{
			CandidateID: candidate.ID,
			Zone:        candidate.Zone,
			Model:       candidate.Model.Ref,
			Exclusion:   candidateExclusion(request, candidate),
		})
	}
	return results, nil
}

func candidateExclusion(
	request RoutingRequest,
	candidate RoutingCandidate,
) *RoutingExclusion {
	if !candidate.Authenticated {
		return routingExcluded("identity.unauthenticated", "candidate is not authenticated")
	}
	if !candidate.Compatible {
		return routingExcluded("compatibility.unsatisfied", "candidate is not protocol or workload compatible")
	}
	if !candidate.Healthy {
		return routingExcluded("health.unhealthy", "candidate is not healthy")
	}
	if candidate.Draining {
		return routingExcluded("health.draining", "candidate is draining")
	}
	if len(request.Requirements.AllowedZones) > 0 &&
		!routingContainsZone(request.Requirements.AllowedZones, candidate.Zone) {
		return routingExcluded("policy.zone.denied", "candidate zone is not allowed by workload requirements")
	}
	if exclusion := routingPolicyExclusion(
		"execution",
		candidate.ExecutionVerdict,
		request.ManifestSHA256,
		routingExecutionScope(request, candidate),
	); exclusion != nil {
		return exclusion
	}
	if candidate.Zone != request.SourceZone {
		if candidate.EgressVerdict == nil {
			return routingExcluded("egress.verdict.missing", "cross-zone candidate requires an egress verdict")
		}
		if exclusion := routingPolicyExclusion(
			"egress",
			*candidate.EgressVerdict,
			request.ManifestSHA256,
			routingEgressScope(request, candidate),
		); exclusion != nil {
			return exclusion
		}
	}

	capabilities := candidate.Model.Capabilities
	for _, modality := range request.Requirements.Modalities {
		if !routingContainsString(capabilities.Modalities, modality) {
			return routingExcluded("capability.modality.missing", fmt.Sprintf("candidate does not support required modality %q", modality))
		}
	}
	for _, tool := range request.Requirements.RequiredTools {
		if !routingContainsString(candidate.AvailableTools, tool) {
			return routingExcluded("capability.tool.missing", fmt.Sprintf("candidate does not provide required tool %q", tool))
		}
	}
	if len(request.Requirements.RequiredTools) > 0 && capabilities.ToolUse == ToolUseNone {
		return routingExcluded("capability.tool_use.missing", "candidate model cannot request tools")
	}
	if request.Requirements.NativeToolUse && capabilities.ToolUse != ToolUseNative {
		return routingExcluded("capability.native_tool_use.missing", "candidate does not support native tool use")
	}
	if request.Requirements.StructuredOutput && !capabilities.StructuredOutput {
		return routingExcluded("capability.structured_output.missing", "candidate does not support structured output")
	}
	if capabilities.ContextTokens < request.Requirements.MinContextTokens {
		return routingExcluded("limit.context.insufficient", "candidate context limit is insufficient")
	}
	if int64(capabilities.MaxOutputTokens) < request.Budget.MaxOutputTokens {
		return routingExcluded("limit.output.insufficient", "candidate output limit is insufficient")
	}
	if candidate.ExpectedCost > request.Budget.MaxCost {
		return routingExcluded("budget.cost.unreservable", "candidate expected cost exceeds the step budget")
	}
	if candidate.AvailableSlots == 0 {
		return routingExcluded("capacity.unavailable", "candidate has no available capacity")
	}
	start := routingMaxTime(request.ObservedAt, candidate.AvailableAt)
	if start.After(request.Deadline) || candidate.ExpectedLatency > request.Deadline.Sub(start) {
		return routingExcluded("capacity.deadline", "candidate cannot complete before the deadline")
	}
	return nil
}

func routingPolicyExclusion(
	kind string,
	verdict PolicyVerdict,
	manifestDigest string,
	expectedScope PolicyScope,
) *RoutingExclusion {
	if err := verdict.Validate(); err != nil {
		return routingExcluded("policy."+kind+".invalid", "candidate has an invalid "+kind+" policy verdict")
	}
	if verdict.ManifestSHA256 != manifestDigest {
		return routingExcluded("policy."+kind+".manifest_mismatch", "candidate policy verdict does not bind the routing manifest")
	}
	if !verdict.Scope.Equal(expectedScope) {
		return routingExcluded("policy."+kind+".scope_mismatch", "candidate policy verdict does not bind the routing scope")
	}
	if !verdict.Allows() {
		return routingExcluded("policy."+kind+".not_allowed", "candidate policy verdict does not allow the operation")
	}
	return nil
}

func routingExecutionScope(request RoutingRequest, candidate RoutingCandidate) PolicyScope {
	return PolicyScopeForExecution(ExecutionPolicyRequest{
		WorkloadID: request.WorkloadID,
		StepID:     request.StepID,
		AttemptID:  request.AttemptID,
		StepKind:   StepModelTurn,
		Autonomous: request.Autonomous,
		Target: ExecutionTarget{
			Kind:  ExecutionTargetModel,
			Zone:  candidate.Zone,
			Model: &candidate.Model.Ref,
		},
	})
}

func routingEgressScope(request RoutingRequest, candidate RoutingCandidate) PolicyScope {
	return PolicyScopeForDataEgress(DataEgressPolicyRequest{
		WorkloadID:     request.WorkloadID,
		StepID:         request.StepID,
		Source:         request.SourceZone,
		Destination:    candidate.Zone,
		Purpose:        request.EgressPurpose,
		DerivedFromSHA: request.DerivedFromSHA,
	})
}

func routingExcluded(code, reason string) *RoutingExclusion {
	return &RoutingExclusion{Code: code, Reason: reason}
}

func routingContainsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func routingContainsZone(values []Zone, wanted Zone) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func routingMaxTime(left, right time.Time) time.Time {
	if right.After(left) {
		return right
	}
	return left
}

func (exclusion RoutingExclusion) Validate() error {
	if strings.TrimSpace(exclusion.Code) == "" {
		return errors.New("routing exclusion code is required")
	}
	if strings.TrimSpace(exclusion.Reason) == "" {
		return errors.New("routing exclusion reason is required")
	}
	return nil
}

type RoutingScore struct {
	Quality       int `json:"quality"`
	Locality      int `json:"locality"`
	Availability  int `json:"availability"`
	CacheAffinity int `json:"cache_affinity"`
	Cost          int `json:"cost"`
	Latency       int `json:"latency"`
	InverseLoad   int `json:"inverse_load"`
	Total         int `json:"total"`
}

func (score RoutingScore) Validate() error {
	for _, component := range []struct {
		name  string
		value int
	}{
		{name: "quality", value: score.Quality},
		{name: "locality", value: score.Locality},
		{name: "availability", value: score.Availability},
		{name: "cache affinity", value: score.CacheAffinity},
		{name: "cost", value: score.Cost},
		{name: "latency", value: score.Latency},
		{name: "inverse load", value: score.InverseLoad},
		{name: "total", value: score.Total},
	} {
		if component.value < 0 || component.value > ScorePermilleMax {
			return fmt.Errorf("routing score %s must be between 0 and %d", component.name, ScorePermilleMax)
		}
	}
	return nil
}

type CandidateAssessment struct {
	CandidateID ID                `json:"candidate_id"`
	Zone        Zone              `json:"zone"`
	Model       ModelRef          `json:"model"`
	Eligible    bool              `json:"eligible"`
	Exclusion   *RoutingExclusion `json:"exclusion,omitempty"`
	Score       *RoutingScore     `json:"score,omitempty"`
}

func (assessment CandidateAssessment) Validate() error {
	if err := assessment.CandidateID.Validate(); err != nil {
		return fmt.Errorf("assessed candidate ID: %w", err)
	}
	if err := assessment.Zone.Validate(); err != nil {
		return fmt.Errorf("assessed candidate zone: %w", err)
	}
	if err := assessment.Model.Validate(); err != nil {
		return fmt.Errorf("assessed candidate model: %w", err)
	}
	if assessment.Eligible {
		if assessment.Exclusion != nil || assessment.Score == nil {
			return errors.New("eligible candidate requires a score and no exclusion")
		}
		return assessment.Score.Validate()
	}
	if assessment.Exclusion == nil || assessment.Score != nil {
		return errors.New("excluded candidate requires one exclusion and no score")
	}
	return assessment.Exclusion.Validate()
}

// RoutingDecision is an auditable result for every supplied candidate.
type RoutingDecision struct {
	Router              string                `json:"router"`
	RouterVersion       string                `json:"router_version"`
	WorkloadID          ID                    `json:"workload_id"`
	StepID              ID                    `json:"step_id"`
	AttemptID           ID                    `json:"attempt_id"`
	ManifestSHA256      string                `json:"manifest_sha256"`
	SourceZone          Zone                  `json:"source_zone"`
	Autonomous          bool                  `json:"autonomous"`
	EgressPurpose       string                `json:"egress_purpose,omitempty"`
	DerivedFromSHA      string                `json:"derived_from_sha256,omitempty"`
	ObservedAt          time.Time             `json:"observed_at"`
	Deadline            time.Time             `json:"deadline"`
	Weights             RoutingWeights        `json:"weights"`
	Assessments         []CandidateAssessment `json:"assessments"`
	SelectedCandidateID ID                    `json:"selected_candidate_id,omitempty"`
}

func (decision RoutingDecision) Validate(request RoutingRequest) error {
	if err := request.Validate(); err != nil {
		return fmt.Errorf("routing decision request: %w", err)
	}
	if strings.TrimSpace(decision.Router) == "" || strings.TrimSpace(decision.RouterVersion) == "" {
		return errors.New("routing decision requires router name and version")
	}
	if decision.WorkloadID != request.WorkloadID || decision.StepID != request.StepID ||
		decision.AttemptID != request.AttemptID {
		return errors.New("routing decision request identity mismatch")
	}
	if decision.ManifestSHA256 != request.ManifestSHA256 {
		return errors.New("routing decision manifest mismatch")
	}
	if decision.SourceZone != request.SourceZone || decision.Autonomous != request.Autonomous ||
		decision.EgressPurpose != request.EgressPurpose || decision.DerivedFromSHA != request.DerivedFromSHA ||
		!decision.ObservedAt.Equal(request.ObservedAt) || !decision.Deadline.Equal(request.Deadline) {
		return errors.New("routing decision request snapshot mismatch")
	}
	if decision.Weights != request.Weights {
		return errors.New("routing decision weights mismatch")
	}
	if len(decision.Assessments) != len(request.Candidates) {
		return errors.New("routing decision must assess every candidate")
	}
	selectedFound := false
	eligibleCount := 0
	for index, assessment := range decision.Assessments {
		if assessment.CandidateID != request.Candidates[index].ID {
			return fmt.Errorf("routing assessment %d does not preserve candidate order", index)
		}
		if assessment.Zone != request.Candidates[index].Zone ||
			assessment.Model != request.Candidates[index].Model.Ref {
			return fmt.Errorf("routing assessment %d candidate snapshot mismatch", index)
		}
		if err := assessment.Validate(); err != nil {
			return fmt.Errorf("routing assessment %d: %w", index, err)
		}
		expectedExclusion := candidateExclusion(request, request.Candidates[index])
		if expectedExclusion == nil && !assessment.Eligible {
			return fmt.Errorf("routing assessment %d excluded an eligible candidate", index)
		}
		if expectedExclusion != nil {
			if assessment.Eligible || assessment.Exclusion == nil ||
				*assessment.Exclusion != *expectedExclusion {
				return fmt.Errorf("routing assessment %d changed mandatory eligibility", index)
			}
		}
		if assessment.Eligible {
			eligibleCount++
			if assessment.CandidateID == decision.SelectedCandidateID {
				selectedFound = true
			}
		}
	}
	if decision.SelectedCandidateID.IsZero() {
		if eligibleCount != 0 {
			return errors.New("routing decision omitted selection with eligible candidates")
		}
		return nil
	}
	if err := decision.SelectedCandidateID.Validate(); err != nil {
		return fmt.Errorf("selected candidate ID: %w", err)
	}
	if !selectedFound {
		return errors.New("routing decision selected an ineligible or unknown candidate")
	}
	return nil
}

type Router interface {
	Route(context.Context, RoutingRequest) (RoutingDecision, error)
}

var (
	ErrNoEligibleCandidates = errors.New("no eligible routing candidates")
	ErrRoutingEvaluation    = errors.New("routing evaluation failed")
)

// EvaluateRouter validates and isolates input and rejects malformed router
// output. A complete no-route decision is preserved with its sentinel error.
func EvaluateRouter(
	ctx context.Context,
	router Router,
	request RoutingRequest,
) (RoutingDecision, error) {
	if err := ctx.Err(); err != nil {
		return RoutingDecision{}, err
	}
	if err := request.Validate(); err != nil {
		return RoutingDecision{}, fmt.Errorf("%w: %w", ErrRoutingEvaluation, err)
	}
	if router == nil {
		return RoutingDecision{}, fmt.Errorf("%w: router is nil", ErrRoutingEvaluation)
	}
	decision, err := router.Route(ctx, request.Clone())
	if contextError := ctx.Err(); contextError != nil {
		return RoutingDecision{}, contextError
	}
	if err != nil && !errors.Is(err, ErrNoEligibleCandidates) {
		return RoutingDecision{}, fmt.Errorf("%w: %w", ErrRoutingEvaluation, err)
	}
	if validationError := decision.Validate(request); validationError != nil {
		return RoutingDecision{}, fmt.Errorf("%w: %w", ErrRoutingEvaluation, validationError)
	}
	if errors.Is(err, ErrNoEligibleCandidates) {
		if !decision.SelectedCandidateID.IsZero() {
			return RoutingDecision{}, fmt.Errorf("%w: no-route result selected a candidate", ErrRoutingEvaluation)
		}
		return decision, ErrNoEligibleCandidates
	}
	if decision.SelectedCandidateID.IsZero() {
		return RoutingDecision{}, fmt.Errorf("%w: successful route omitted a selection", ErrRoutingEvaluation)
	}
	return decision, nil
}

func validateModelCapabilities(capabilities ModelCapabilities) error {
	if capabilities.ContextTokens < 0 || capabilities.MaxOutputTokens < 0 {
		return errors.New("model token capabilities cannot be negative")
	}
	if err := validateStringSet("model modality", capabilities.Modalities); err != nil {
		return err
	}
	switch capabilities.ToolUse {
	case ToolUseNone, ToolUseNative, ToolUseConstrained, ToolUseParsed:
		return nil
	default:
		return fmt.Errorf("unknown model tool-use mode %q", capabilities.ToolUse)
	}
}

func validateStringSet(name string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s %d is empty", name, index)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("%s %d duplicates %q", name, index, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateZoneSet(zones []Zone) error {
	seen := make(map[Zone]struct{}, len(zones))
	for index, zone := range zones {
		if err := zone.Validate(); err != nil {
			return fmt.Errorf("allowed zone %d: %w", index, err)
		}
		if _, exists := seen[zone]; exists {
			return fmt.Errorf("allowed zone %d duplicates %q", index, zone)
		}
		seen[zone] = struct{}{}
	}
	return nil
}
