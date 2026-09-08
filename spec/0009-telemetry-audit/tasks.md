# Telemetry and audit task ledger

**Specification:** [spec.md](spec.md)

**Plan:** [plan.md](plan.md)

**Acceptance:** [acceptance.md](acceptance.md)

- [x] Write failing acceptance tests named for each criterion before declaring
  any telemetry type.
- [x] Define the `Record` envelope, kinds, and closed attribute key set
  (FR-TEL-002, FR-TEL-006).
- [x] Define the `Observer` contract and nil-observer behavior (FR-TEL-001,
  FR-TEL-004).
- [x] Implement observation isolation: panic recovery, error discard, deadline
  bound (FR-TEL-003, FR-TEL-007).
- [x] Implement classification filtering at record construction (FR-TEL-005).
- [x] Add the `observertest` conformance suite (FR-TEL-001).
- [x] Prove `LoopSink` is unchanged and independently usable (FR-TEL-008).
- [x] Prove an audit view is derivable with no observer attached (FR-TEL-009).
- [x] Capture acceptance evidence.
