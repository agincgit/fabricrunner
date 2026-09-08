# Approval gates, budgets, and lifecycle task ledger

**Specification:** [spec.md](spec.md)

**Plan:** [plan.md](plan.md)

**Acceptance:** [acceptance.md](acceptance.md)

- [x] Write failing acceptance tests before declaring any type.
- [x] Add the `Approver` contract and no-pause default (FR-GATE-001).
- [x] Bound approval request content by classification (FR-GATE-002).
- [x] Terminate on denial or timeout with the decision recorded (FR-GATE-003).
- [x] Make replay skip approval re-requests (FR-GATE-004).
- [x] Enforce every budget dimension before spend (FR-BUD-001, FR-BUD-002).
- [x] Propagate cancellation within a bounded interval (FR-LIFE-001).
- [x] Run uniform cleanup on every termination path (FR-LIFE-002).
- [x] Record cleanup failure without masking the cause (FR-LIFE-003).
- [x] Capture acceptance evidence.
