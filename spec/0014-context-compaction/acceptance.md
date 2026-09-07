# Context accounting and auditable compaction acceptance

**Specification:** [spec.md](spec.md)

**Status:** Draft

Each criterion below is the name of a test to be written before the
implementation exists.

The implementation is accepted when:

- `TestContextAccountingMatchesEventStream` — accounting derived from the
  stream matches the provider-reported consumption;
- `TestAutomaticCompactionUnsetFailsOnOverflow` — a workload without the flag
  fails on overflow rather than compacting;
- `TestAutomaticCompactionSetCompacts` — a workload with the flag compacts at
  the threshold;
- `TestCompactionEventRecordsReplacedRange` — the compaction event carries
  inputs, summary, model, token counts, and the replaced event range;
- `TestReplayAppliesRecordedSummary` — replay of a compacted workload contacts
  no provider, asserted with a provider that fails the test if called;
- `TestCompactionExcludesAboveClassification` — content above the workload's
  egress classification is absent from the summary input and its exclusion is
  recorded;
- `TestCompactionBudgetExhaustionTerminates` — a workload that cannot afford
  compaction terminates with the reason recorded rather than looping; and
- build, vet, static analysis, race tests, and leak scanning pass.

## Evidence

Pending implementation.
