# Execution engine acceptance

**Specification:** [spec.md](spec.md)

**Status:** Implemented

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
- `TestStoreFailurePreventsProviderCall`, `TestStoreFailurePreventsToolCall`,
  and `TestDuplicateRunNeverRepeatsEffects` — failed persistence and duplicate
  submissions cannot repeat or initiate external effects (FR-ENG-010); and
- build, vet, static analysis, race tests, and leak scanning pass.

## Evidence

Local verification on 2026-09-07:

- All named engine criteria pass in `engine_test.go` and
  `engine_boundary_test.go`. Token input/output, cost, model calls, tool calls,
  and wall time each have a real loop exhaustion scenario. No child steps are
  created; delegation and advance spend reservations remain later work.
- `TestEngineWithOpenAICompatibleAdapter` uses the HTTP adapter and SQLite,
  verifies committed turn state while the HTTP request is in flight, then
  replays with the test server shut down. No paid provider is contacted.
- SQLite close/reopen reproduces the complete terminal projection, including
  routing, policy, messages, usage, and the result.
- Confidential tool output prevents the next cloud turn; denied remote model
  content never reaches the durable content log.
- Malformed execution records are rejected without projection mutation.
- Windows: `go test ./...`, `go build ./...`, and `go vet ./...` pass on Go
  1.25.13. Ubuntu/WSL: `GOTOOLCHAIN=go1.25.13 go test ./... -race -count=1` passes.
- Staticcheck v0.8.1 passes; Gitleaks v8.30.1 finds no leaks. Govulncheck reports
  no reachable vulnerabilities.

Local implementation acceptance does not complete the Phase 1 gate. Built-in
tools, sandboxing, approvals, compaction, and restart continuation remain
outstanding. Replay is read-only. The outbound worker remains behind Phase 1.
