# Caller intent task ledger

**Specification:** [spec.md](spec.md)

**Plan:** [plan.md](plan.md)

**Acceptance:** [acceptance.md](acceptance.md)

- [ ] Write failing acceptance tests before adding the field.
- [ ] Add `EngineRequest.Intent` (FR-INT-001).
- [ ] Record intent on the workload alongside `Goal` (FR-INT-002).
- [ ] Carry intent into `DataEgressPolicyRequest` beside `Purpose`
  (FR-INT-003).
- [ ] Prove routing and baseline policy ignore it (FR-INT-004).
- [ ] Prove omission is a no-op for existing callers (FR-INT-005).
- [ ] Classify intent as recorded content, never as instruction (FR-INT-006).
- [ ] Record the change in 0010's signature-change table.
- [ ] Capture acceptance evidence.
