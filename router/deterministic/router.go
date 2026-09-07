// Package deterministic provides Fabric Runner's initial pure routing engine.
package deterministic

import (
	"context"
	"math/bits"
	"time"

	"github.com/agincgit/fabricrunner"
)

const (
	routerName    = "fabricrunner.deterministic"
	routerVersion = "1"
)

type Router struct{}

func (Router) Route(
	ctx context.Context,
	request fabricrunner.RoutingRequest,
) (fabricrunner.RoutingDecision, error) {
	if err := ctx.Err(); err != nil {
		return fabricrunner.RoutingDecision{}, err
	}
	eligibilities, err := fabricrunner.EvaluateRoutingEligibility(ctx, request)
	if err != nil {
		return fabricrunner.RoutingDecision{}, err
	}

	decision := fabricrunner.RoutingDecision{
		Router:         routerName,
		RouterVersion:  routerVersion,
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
		Assessments:    make([]fabricrunner.CandidateAssessment, 0, len(request.Candidates)),
	}
	bestScore := -1
	for index, candidate := range request.Candidates {
		if err := ctx.Err(); err != nil {
			return fabricrunner.RoutingDecision{}, err
		}
		assessment := fabricrunner.CandidateAssessment{
			CandidateID: candidate.ID,
			Zone:        candidate.Zone,
			Model:       candidate.Model.Ref,
		}
		if !eligibilities[index].Eligible() {
			assessment.Exclusion = eligibilities[index].Exclusion
			decision.Assessments = append(decision.Assessments, assessment)
			continue
		}
		assessment.Eligible = true
		score := scoreCandidate(request, candidate)
		assessment.Score = &score
		decision.Assessments = append(decision.Assessments, assessment)
		if score.Total > bestScore ||
			(score.Total == bestScore && string(candidate.ID) < string(decision.SelectedCandidateID)) {
			bestScore = score.Total
			decision.SelectedCandidateID = candidate.ID
		}
	}
	if decision.SelectedCandidateID.IsZero() {
		return decision, fabricrunner.ErrNoEligibleCandidates
	}
	return decision, nil
}

func scoreCandidate(
	request fabricrunner.RoutingRequest,
	candidate fabricrunner.RoutingCandidate,
) fabricrunner.RoutingScore {
	availability := fabricrunner.ScorePermilleMax
	if candidate.AvailableSlots < 10 {
		availability = candidate.AvailableSlots * 100
	}
	cost := fabricrunner.ScorePermilleMax
	if request.Budget.MaxCost > 0 {
		cost -= scalePermille(int64(candidate.ExpectedCost), int64(request.Budget.MaxCost))
	}
	start := maxTime(request.ObservedAt, candidate.AvailableAt)
	totalLatency := start.Sub(request.ObservedAt) + candidate.ExpectedLatency
	latency := fabricrunner.ScorePermilleMax - scalePermille(
		int64(totalLatency),
		int64(request.Deadline.Sub(request.ObservedAt)),
	)
	score := fabricrunner.RoutingScore{
		Quality:       candidate.QualityPermille,
		Locality:      boolPermille(candidate.Zone == request.SourceZone),
		Availability:  availability,
		CacheAffinity: candidate.CacheAffinityPermille,
		Cost:          cost,
		Latency:       latency,
		InverseLoad:   fabricrunner.ScorePermilleMax - candidate.LoadPermille,
	}
	weights := request.Weights
	score.Total = (score.Quality*weights.Quality +
		score.Locality*weights.Locality +
		score.Availability*weights.Availability +
		score.CacheAffinity*weights.CacheAffinity +
		score.Cost*weights.Cost +
		score.Latency*weights.Latency +
		score.InverseLoad*weights.InverseLoad) / fabricrunner.ScorePermilleMax
	return score
}

func scalePermille(numerator, denominator int64) int {
	hi, lo := bits.Mul64(uint64(numerator), fabricrunner.ScorePermilleMax)
	quotient, _ := bits.Div64(hi, lo, uint64(denominator))
	return int(quotient)
}

func boolPermille(value bool) int {
	if value {
		return fabricrunner.ScorePermilleMax
	}
	return 0
}

func maxTime(left, right time.Time) time.Time {
	if right.After(left) {
		return right
	}
	return left
}

var _ fabricrunner.Router = Router{}
