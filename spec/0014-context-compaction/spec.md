# Context accounting and auditable compaction specification

**ID:** 0014

**Status:** Implemented

**Depends on:** [Execution engine](../0010-execution-engine/spec.md),
[Telemetry and audit](../0009-telemetry-audit/spec.md)

Supporting execution records: [plan](plan.md), [tasks](tasks.md), and
[acceptance](acceptance.md).

## Purpose

The existing `ModelCapabilities.AutomaticCompaction` is descriptive provider
metadata and does not enable runner compaction. The earlier draft incorrectly
identified it as a field on `ModelRequest`. This specification introduces an
explicit request control, `ModelRequest.AutomaticCompaction`, alongside engine
context accounting; a capability advertisement alone never enables compaction.

Compaction must be auditable. A run that summarized away part of its history
and cannot say what it replaced is not replayable in any meaningful sense.

## Requirements

- **FR-CTX-001:** The engine tracks context consumption against the model's
  declared window across a workload.
- **FR-CTX-002:** When the request's `AutomaticCompaction` is set and consumption
  crosses the threshold, compaction runs. When it is unset, the workload fails on overflow
  rather than compacting silently.
- **FR-CTX-003:** Compaction records its inputs, the summary produced, the model
  that produced it, token counts, and the replaced event range — as specified in
  0001.
- **FR-CTX-004:** Replay reconstructs a compacted workload without re-running
  compaction and without contacting a provider.
- **FR-CTX-005:** Compaction never crosses a classification boundary. Content
  above the workload's egress classification is excluded from the summary
  input, and exclusion is recorded.
- **FR-CTX-006:** A workload that cannot compact within its budget terminates
  with the reason recorded rather than looping.

## Non-goals

- No summarization quality metric.
- No cross-workload context sharing or caching.
