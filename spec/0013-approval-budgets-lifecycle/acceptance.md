# Approval gates, budgets, and lifecycle acceptance

**Specification:** [spec.md](spec.md)

**Status:** Draft

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

Pending implementation.
