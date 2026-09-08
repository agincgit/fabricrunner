# Approval gates, budgets, and lifecycle acceptance

**Specification:** [spec.md](spec.md)

**Status:** Implemented

Each criterion below is the name of a test to be written before the
implementation exists.

The implementation is accepted when:

- `TestUnconfiguredApproverDoesNotPause` — a workload with no approver runs to
  completion without blocking;
- `TestApprovalRequestExcludesConfidentialContent` — no approval request
  carries content above `ClassConfidential`;
- `TestApprovalDenialTerminatesInPermittedState` — denial and timeout each
  terminate in a state the transition table permits, with the decision recorded;
- `TestReplayDoesNotReRequestApproval` — replaying an approved workload makes no
  approval request, asserted with an approver that fails the test if called;
- `TestEachBudgetDimensionTerminates` — token, cost, call, time, and step
  exhaustion each terminate with that dimension recorded;
- `TestBudgetCheckedBeforeSpend` — a call that would exceed the budget is never
  made, asserted with a provider that fails the test if called;
- `TestCancellationPropagatesWithinBound` — cancellation reaches in-flight
  provider and tool calls within the bounded interval;
- `TestCleanupRunsOnEveryTerminationPath` — success, failure, cancellation,
  budget exhaustion, and approval denial each release every acquired resource;
- `TestCleanupFailureDoesNotMaskCause` — a failing cleanup is recorded and the
  original termination reason survives; and
- build, vet, static analysis, race tests, and leak scanning pass.

## Evidence

See the dated local acceptance evidence below.

## Local acceptance evidence — 2026-09-08 UTC

| Requirement | Observable evidence | Result |
|---|---|---|
| FR-GATE-001, FR-GATE-002 | No-pause default; approval receives manifest/scope without prompt content | Pass |
| FR-GATE-003 | Denial and timeout terminate with attributed decision records | Pass |
| FR-GATE-004 | `TestReplayDoesNotReRequestApproval` | Pass |
| FR-BUD-001, FR-BUD-002 | Pre-call input/output/cost/step denial; model-call, tool-call and wall-time tests; missing bound denies | Pass |
| FR-LIFE-001 | `TestCancellationPropagatesWithinBound`; confined command cancellation | Pass |
| FR-LIFE-002 | `TestCleanupRunsOnEveryTerminationPath` | Pass |
| FR-LIFE-003 | `TestCleanupFailureDoesNotMaskCause` | Pass |
| Checks | Windows tests/vet; Linux full race suite; staticcheck; patched-toolchain vulnerability scan; redacted leak scan | Pass |

Approvals bind the selected model and actual zone. The optional initial gate is
resolved after routing but before reservation/provider I/O. Live resume does not
re-request an already completed initial gate. Bounds are supplied by a trusted
application and must include provider defaults; they are conservative charges,
not a pricing model. In-process collaborators must honor their context.
