# Context accounting and auditable compaction task ledger

**Specification:** [spec.md](spec.md)

**Plan:** [plan.md](plan.md)

**Acceptance:** [acceptance.md](acceptance.md)

- [ ] Write failing acceptance tests before changing any type.
- [ ] Derive context accounting from the event stream (FR-CTX-001).
- [ ] Give `AutomaticCompaction` behavior; fail on overflow when unset
  (FR-CTX-002).
- [ ] Append the auditable compaction event (FR-CTX-003).
- [ ] Make replay apply the recorded summary (FR-CTX-004).
- [ ] Exclude above-classification content and record the exclusion
  (FR-CTX-005).
- [ ] Terminate when compaction exceeds budget (FR-CTX-006).
- [ ] Document the distinction between the informational provider capability
  and the new request control; update Phase 1a regression coverage for the
  explicitly enabled behavior without making the capability itself a control.
- [ ] Capture acceptance evidence.
