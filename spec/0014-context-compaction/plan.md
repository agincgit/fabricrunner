# Context accounting and auditable compaction implementation plan

**Specification:** [spec.md](spec.md)

**Status:** Draft

## Design

Accounting is derived from the event stream rather than tracked alongside it,
so the count cannot drift from the record. Compaction appends a compaction
event carrying inputs, summary, model, token counts, and the replaced range;
replay reads that event and applies the summary instead of re-summarizing,
which is what makes FR-CTX-004 hold without a second code path.

Compaction is itself a model call, so it is subject to budget and policy like
any other. A workload that cannot afford to compact terminates rather than
retrying, per FR-CTX-006.

## Delivery sequence

1. Derive context accounting from the event stream.
2. Give `AutomaticCompaction` behavior, and fail on overflow when unset.
3. Append the auditable compaction event.
4. Make replay apply the recorded summary.
5. Add classification exclusion and its recording.
