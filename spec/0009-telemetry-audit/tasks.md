# Telemetry and audit task ledger

**Specification:** [spec.md](spec.md)

**Plan:** [plan.md](plan.md)

**Acceptance:** [acceptance.md](acceptance.md)

- [ ] Write failing acceptance tests named for each criterion before declaring
  any telemetry type.
- [ ] Define the `Record` envelope, kinds, and closed attribute key set
  (FR-TEL-002, FR-TEL-006).
- [ ] Define the `Observer` contract and nil-observer behavior (FR-TEL-001,
  FR-TEL-004).
- [ ] Implement observation isolation: panic recovery, error discard, deadline
  bound (FR-TEL-003, FR-TEL-007).
- [ ] Implement classification filtering at record construction (FR-TEL-005).
- [ ] Add the `observertest` conformance suite (FR-TEL-001).
- [ ] Prove `LoopSink` is unchanged and independently usable (FR-TEL-008).
- [ ] Prove an audit view is derivable with no observer attached (FR-TEL-009).
- [ ] Capture acceptance evidence.
