# Execution engine task ledger

**Specification:** [spec.md](spec.md)

**Plan:** [plan.md](plan.md)

**Acceptance:** [acceptance.md](acceptance.md)

- [x] Write failing acceptance tests named for each criterion before declaring
  any engine type.
- [x] Define the `Engine` type and its dependency set (FR-ENG-001).
- [x] Implement the transition loop with persist-before-expose (FR-ENG-002).
- [x] Bind execution policy and record prevented attempts (FR-ENG-003).
- [x] Bind egress policy and deny undecidable crossings (FR-ENG-004).
- [x] Persist the routing decision against the model turn (FR-ENG-005).
- [x] Implement replay with no provider or tool contact (FR-ENG-006).
- [x] Implement cancellation to permitted states (FR-ENG-007).
- [x] Implement budget termination with the exhausted dimension (FR-ENG-008).
- [x] Adopt the 0009 observer contract (FR-ENG-009).
- [x] Prevent effects after failed persistence or duplicate submission
  (FR-ENG-010).
- [x] Record every discovered signature change in the plan's table.
- [x] Capture acceptance evidence.
