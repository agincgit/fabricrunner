# Approval gates, budgets, and lifecycle task ledger

**Specification:** [spec.md](spec.md)

**Plan:** [plan.md](plan.md)

**Acceptance:** [acceptance.md](acceptance.md)

- [ ] Write failing acceptance tests before declaring any type.
- [ ] Add the `Approver` contract and no-pause default (FR-GATE-001).
- [ ] Bound approval request content by classification (FR-GATE-002).
- [ ] Terminate on denial or timeout with the decision recorded (FR-GATE-003).
- [ ] Make replay skip approval re-requests (FR-GATE-004).
- [ ] Enforce every budget dimension before spend (FR-BUD-001, FR-BUD-002).
- [ ] Propagate cancellation within a bounded interval (FR-LIFE-001).
- [ ] Run uniform cleanup on every termination path (FR-LIFE-002).
- [ ] Record cleanup failure without masking the cause (FR-LIFE-003).
- [ ] Capture acceptance evidence.
