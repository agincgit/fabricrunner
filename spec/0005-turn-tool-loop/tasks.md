# Provider-neutral turn and tool loop task ledger

**Specification:** [spec.md](spec.md)

**Plan:** [plan.md](plan.md)

**Acceptance:** [acceptance.md](acceptance.md)

- [x] Define canonical loop, selection, tool, middleware, event, result, and
  error contracts (FR-LOOP-001 through FR-LOOP-014).
- [x] Implement Draft 2020-12-compatible schema validation (FR-LOOP-005).
- [x] Implement per-turn selection and ordered stream handling (FR-LOOP-001,
  FR-LOOP-003, FR-LOOP-004, FR-LOOP-010).
- [x] Implement sequential and parallel-safe ordered tool execution
  (FR-LOOP-005 through FR-LOOP-008).
- [x] Implement budgets, cancellation, events, panic recovery, and cleanup
  (FR-LOOP-009, FR-LOOP-011 through FR-LOOP-013).
- [x] Add contract, stream, schema, ordering, failure, budget, cancellation,
  panic, cleanup, and isolation tests.
- [x] Capture local and published acceptance evidence.
