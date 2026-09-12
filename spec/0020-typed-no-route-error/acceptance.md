# Typed no-route error acceptance

**Specification:** [spec.md](spec.md)

**Status:** Draft

Each criterion below is the name of a test to be written before the
implementation exists.

The implementation is accepted when:

- `TestNoRouteErrorExposesExclusionCodes` — `errors.As` yields each candidate's
  assessment and exclusion code;
- `TestPolicyExclusionDistinguishedFromUnhealthy` — an all-excluded-by-policy
  result is distinguishable from an all-unhealthy result without inspecting
  candidates separately;
- `TestErrorsIsStillMatches` — `errors.Is(err, ErrNoEligibleCandidates)` reports
  true;
- `TestCarriedDecisionMatchesRecorded` — the decision on the error is identical
  to the one in the event stream;
- `TestExclusionCodeSetIsClosed` — every code returned is a member of the
  documented set; and
- build, vet, static analysis, race tests, and leak scanning pass.

## Evidence

Pending implementation.
