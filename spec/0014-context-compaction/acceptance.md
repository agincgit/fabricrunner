# Context accounting and auditable compaction acceptance

**Specification:** [spec.md](spec.md)

**Status:** Implemented

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

See the dated local acceptance evidence below.

## Local acceptance evidence — 2026-09-08 UTC

| Requirement | Observable evidence | Result |
|---|---|---|
| FR-CTX-001 | `TestContextAccountingMatchesEventStream`, including compaction usage/call totals | Pass |
| FR-CTX-002 | Explicit-enabled compaction and unset-overflow tests | Pass |
| FR-CTX-003 | Exact inputs, summary, model, token counts and prior event range checked | Pass |
| FR-CTX-004 | `TestReplayAppliesRecordedSummary` makes no new provider call | Pass |
| FR-CTX-005 | Secret-bearing message excluded before summary request and index recorded | Pass |
| FR-CTX-006 | `TestCompactionBudgetExhaustionTerminates` | Pass |
| Regression | Provider capability hint still does not enable compaction | Pass |
| Checks | Windows tests/vet; Linux full race suite; staticcheck; patched-toolchain vulnerability scan; redacted leak scan | Pass |

The supplied context counter accounts for provider-specific input/framing. The
actual usage event stream remains authoritative for reported usage. A summary
input that cannot fit the declared window is refused rather than truncated.
