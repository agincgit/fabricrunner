# Eval harness acceptance

**Specification:** [spec.md](spec.md)

**Status:** Draft

Each criterion below is the name of a test to be written before the
implementation exists.

The implementation is accepted when:

- `TestRunnerProducesVersionedReport` — a supplied case set produces a report
  carrying its format version;
- `TestReportIsStableAcrossRuns` — two runs over identical inputs produce
  byte-identical reports;
- `TestRescoreFromRecordedStream` — a case is re-scored from a recorded event
  stream with a provider and tool executor that fail the test if called;
- `TestEvaluationIsNotPrivileged` — a case denied by execution policy, budget,
  or sandbox is denied identically to any other workload;
- `TestFailingCaseDoesNotAbortRun` — one failing case leaves the remaining
  cases executed, and the report distinguishes case failure from harness error;
- `TestNoCaseContentInModule` — no file in this repository contains case
  content, asserted by scanning the module tree; and
- build, vet, static analysis, race tests, and leak scanning pass.

## Evidence

Pending implementation.
