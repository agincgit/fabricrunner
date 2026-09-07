# Execution engine task ledger

**Specification:** [spec.md](spec.md)

**Plan:** [plan.md](plan.md)

**Acceptance:** [acceptance.md](acceptance.md)

- [ ] Write failing acceptance tests named for each criterion before declaring
  any engine type.
- [ ] Define the `Engine` type and its dependency set (FR-ENG-001).
- [ ] Implement the transition loop with persist-before-expose (FR-ENG-002).
- [ ] Bind execution policy and record prevented attempts (FR-ENG-003).
- [ ] Bind egress policy and deny undecidable crossings (FR-ENG-004).
- [ ] Persist the routing decision against the model turn (FR-ENG-005).
- [ ] Implement replay with no provider or tool contact (FR-ENG-006).
- [ ] Implement cancellation to permitted states (FR-ENG-007).
- [ ] Implement budget termination with the exhausted dimension (FR-ENG-008).
- [ ] Adopt the 0009 observer contract (FR-ENG-009).
- [ ] Record every discovered signature change in the plan's table.
- [ ] Capture acceptance evidence.
