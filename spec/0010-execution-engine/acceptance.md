# Execution engine acceptance

**Specification:** [spec.md](spec.md)

**Status:** Draft

Each criterion below is the name of a test to be written before the
implementation exists.

The implementation is accepted when:

- `TestEndToEndProjectionReconstructsTerminalState` — a workload driven end to
  end emits an event stream from which `LoadWorkloadProjection` reconstructs
  identical terminal state;
- `TestPolicyDenyPreventsModelCall` — a deny verdict from `ExecutionPolicy`
  prevents the model call, and the prevented attempt is recorded with its
  verdict and reason;
- `TestEgressDenyPreventsCrossing` — a deny verdict from `DataEgressPolicy`
  prevents the crossing and is recorded, and an undecidable classification is
  denied;
- `TestRoutingDecisionBoundToTurn` — the routing decision that produced a
  placement is recoverable from the event stream and bound to the model turn it
  produced, including candidate exclusions;
- `TestReplayContactsNoProviderOrTool` — replay of a completed workload
  contacts no provider and executes no tool, using stubs that fail the test if
  called;
- `TestReplayReconstructsIdenticalState` — replay produces terminal state
  identical to the original run;
- `TestCancellationLeavesPermittedState` — cancellation mid-turn leaves the
  workload in a state the transition table permits, asserted against the table
  rather than a fixed expectation;
- `TestNoPartialTransitionObservable` — no caller observes state the event
  store has not committed;
- `TestBudgetExhaustionTerminates` — each budget dimension terminates the
  workload in a permitted terminal state with that dimension recorded;
- `TestEngineRunsWithoutObserver` — the engine produces an identical event
  stream with and without an observer attached; and
- build, vet, static analysis, race tests, and leak scanning pass.

## Evidence

Pending implementation.
