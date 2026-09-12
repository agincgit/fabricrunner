# Caller intent acceptance

**Specification:** [spec.md](spec.md)

**Status:** Draft

Each criterion below is the name of a test to be written before the
implementation exists.

The implementation is accepted when:

- `TestIntentAppearsOnReplayedWorkload` — an intent supplied on the request is
  recoverable from the replayed workload;
- `TestIntentReachesEveryEgressEvaluation` — a `DataEgressPolicy` receives the
  intent on every cross-zone evaluation the engine performs;
- `TestPurposeUnchangedByIntent` — `Purpose` carries the same values at every
  existing call site with and without an intent;
- `TestOmittedIntentIsNoOp` — a request with no intent produces an event stream
  identical to one produced before the field existed;
- `TestCoreDoesNotInterpretIntent` — routing decisions and baseline policy
  verdicts are identical across differing intents;
- `TestIntentIsTreatedAsData` — intent text resembling an instruction changes no
  engine behavior; and
- build, vet, static analysis, race tests, and leak scanning pass.

## Evidence

Pending implementation.
