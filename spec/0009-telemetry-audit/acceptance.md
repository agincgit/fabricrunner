# Telemetry and audit acceptance

**Specification:** [spec.md](spec.md)

**Status:** Implemented

Each criterion below is the name of a test to be written before the
implementation exists.

The implementation is accepted when:

- `TestNilObserverIsValid` — a workload runs to completion with no observer and
  emits an identical event stream to one that ran with an observer attached;
- `TestPanickingObserverDoesNotFailWorkload` — an observer that panics on every
  record leaves the workload outcome and event stream unchanged;
- `TestErroringObserverIsIgnored` — an observer returning an error on every
  record leaves the workload outcome and event stream unchanged;
- `TestSlowObserverDoesNotBlockExecution` — an observer that exceeds its
  deadline is abandoned and execution proceeds within a bounded delay;
- `TestObserverCoversFullPath` — a single workload produces records for
  workload and step lifecycle, attempt boundaries, routing decision, policy
  verdict, provider call, and tool call;
- `TestConfidentialContentNeverEnteresRecord` — content classified at or above
  `ClassConfidential` appears in no record; its identifier, size, and hash may;
- `TestAttributeKeysAreClosedSet` — a record built with a key outside the
  versioned set is rejected at construction;
- `TestObservationCancelsWithContext` — cancelling the execution context stops
  in-flight observation without blocking the caller;
- `TestLoopSinkUnchanged` — the `LoopSink` signature and `LoopEvent` shape are
  byte-identical to the prior release, and a loop runs with a sink and no
  observer;
- `TestAuditDerivableWithoutObserver` — a completed workload's audit view is
  reconstructed from the event store alone; and
- build, vet, static analysis, race tests, and leak scanning pass.

## Evidence

Local verification on 2026-09-07:

- `telemetry_test.go` and `engine_test.go` cover every named criterion above;
  callback saturation is additionally checked to bound stuck goroutines.
- `observertest.Run` exercises every v1 record kind and exporter cancellation.
- `git diff -- loop.go` is empty; the existing loop/sink suite passes unchanged.
- Windows: `go test ./...`, `go build ./...`, and `go vet ./...` pass on Go
  1.25.13.
- Ubuntu/WSL: `GOTOOLCHAIN=go1.25.13 go test ./... -race -count=1` passes.
- Staticcheck v0.8.1 passes; Gitleaks v8.30.1 finds no leaks in the working tree.
- Govulncheck reports no reachable vulnerabilities (one module-level advisory
  has no affected imported package or called symbol).

These are local acceptance results, not published CI evidence.
