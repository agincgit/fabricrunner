# Typed no-route error implementation plan

**Specification:** [spec.md](spec.md)

**Status:** Draft

## Design

The error carries the `RoutingDecision` by value and implements `Unwrap` to
`ErrNoEligibleCandidates`. That single choice satisfies both halves of the
requirement: `errors.Is` keeps working for every caller that only wants to know
there was no route, and `errors.As` gives the assessments to callers that need
to know why.

Carrying the same decision value the engine records is what keeps FR-ROUTE-004
true by construction rather than by discipline — there is one decision, handed
to two consumers, not two derivations that could drift.

## Delivery sequence

1. Close and document the exclusion-code set.
2. Add the error type wrapping the sentinel.
3. Return it from `EvaluateRouter`, carrying the recorded decision.
4. Prove existing `errors.Is` call sites are unaffected.
