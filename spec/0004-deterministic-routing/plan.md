# Deterministic routing implementation plan

**Specification:** [spec.md](spec.md)

**Status:** In Progress

## Design

Place canonical request, candidate, assessment, score, and decision contracts
in the root module. Put the pure initial implementation in
`router/deterministic`.

Keep eligibility and scoring structurally separate. Eligibility returns the
first exclusion from an ordered gate list. Only the resulting eligible slice is
passed to scoring. The public boundary recomputes eligibility over router output
so a replaceable scorer cannot change it. Clone candidates, model labels,
capability slices, required tools, allowed zones, and policy data at the public
router boundary.

Use integer arithmetic with overflow-safe validation. All time calculations use
the timestamps carried by the request.

## Delivery sequence

1. Define validated routing snapshots, weights, scores, exclusions, and
   decisions.
2. Define the provider-neutral `Router` interface and its isolating evaluation
   boundary.
3. Implement ordered eligibility filters.
4. Implement deterministic integer scoring and stable tie-breaking.
5. Add validation, eligibility-order, policy-binding, scoring, tie, no-route,
   isolation, and cancellation tests.
6. Run local release checks and published CI, then capture acceptance evidence.
