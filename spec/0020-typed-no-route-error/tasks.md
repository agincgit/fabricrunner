# Typed no-route error task ledger

**Specification:** [spec.md](spec.md)

**Plan:** [plan.md](plan.md)

**Acceptance:** [acceptance.md](acceptance.md)

- [ ] Write failing acceptance tests before adding the type.
- [ ] Close and document the exclusion-code set (FR-ROUTE-005).
- [ ] Add the error type wrapping `ErrNoEligibleCandidates` (FR-ROUTE-001).
- [ ] Carry the recorded `RoutingDecision` (FR-ROUTE-002, FR-ROUTE-004).
- [ ] Prove existing `errors.Is` checks still pass (FR-ROUTE-003).
- [ ] Capture acceptance evidence.
