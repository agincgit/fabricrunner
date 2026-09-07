# Eval harness implementation plan

**Specification:** [spec.md](spec.md)

**Status:** Draft

## Design

The harness is a thin layer over the engine. A case is a workload description
supplied by the caller; the runner executes it through the ordinary engine path
so that FR-EVAL-005 holds by construction rather than by discipline — there is
no separate execution path that could skip a control.

Re-scoring uses replay. Because replay contacts no provider and executes no
tool, a scorer can be changed and the whole suite re-scored against recorded
streams at no model cost. This is the property that justifies building the
harness here rather than downstream.

The report is a versioned struct with a stable field order, serialized so two
runs diff cleanly.

## Delivery sequence

1. Define `Case`, `Scorer`, and the versioned `Report`.
2. Implement the runner over the ordinary engine path.
3. Implement re-scoring from recorded streams.
4. Add failure isolation so one failing case does not abort a run.
5. Add a `evaltest` suite proving no case content lives in this module.

## Sequencing note

Ordered after Phase 2. The harness depends on replay and the engine being
settled, and nothing in Phase 1 or 2 depends on it. Building it earlier would
mean re-working it as the engine shapes move.
