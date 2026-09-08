# Context accounting and auditable compaction task ledger

**Specification:** [spec.md](spec.md)

**Plan:** [plan.md](plan.md)

**Acceptance:** [acceptance.md](acceptance.md)

- [x] Write failing acceptance tests before changing any type.
- [x] Derive context accounting from the event stream (FR-CTX-001).
- [x] Give `AutomaticCompaction` behavior; fail on overflow when unset
  (FR-CTX-002).
- [x] Append the auditable compaction event (FR-CTX-003).
- [x] Make replay apply the recorded summary (FR-CTX-004).
- [x] Exclude above-classification content and record the exclusion
  (FR-CTX-005).
- [x] Terminate when compaction exceeds budget (FR-CTX-006).
- [x] Document the distinction between the informational provider capability
  and the new request control; update Phase 1a regression coverage for the
  explicitly enabled behavior without making the capability itself a control.
- [x] Capture acceptance evidence.
