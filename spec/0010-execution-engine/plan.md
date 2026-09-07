# Execution engine implementation plan

**Specification:** [spec.md](spec.md)

**Status:** Draft

## Design

The engine is a state machine over the workload transition table. Each
iteration resolves the next step, evaluates execution policy, routes to a
placement, runs one turn through the loop, applies egress policy to any
crossing, and appends the resulting events — in that order, with the append
preceding any observable effect.

Persist-before-expose is enforced structurally rather than by convention: the
engine's only means of surfacing state is a projection read, and projection
reads come from the store. There is no in-memory view a caller can observe
ahead of the commit.

Replay is the same state machine with the provider and tool executor replaced
by readers over the recorded stream. That the two paths share one machine is
what makes FR-ENG-006 meaningful rather than a parallel implementation that
drifts.

## Delivery sequence

1. Write the acceptance tests. They will not compile, because the types they
   name do not exist. That is the intended starting state.
2. Define the `Engine` type and its dependency set.
3. Implement the transition loop with persist-before-expose.
4. Bind policy evaluation and record prevented attempts.
5. Bind routing and persist the decision against the turn.
6. Implement replay over the recorded stream.
7. Implement cancellation and budget termination.
8. Adopt the 0009 observer.

## Expected signature changes

This is the first component to hold all four contracts together, so the
contracts' sufficiency is untested. Changes discovered during implementation
are recorded here rather than made silently:

| Type | Change | Reason |
|---|---|---|
| _(to be filled during implementation)_ | | |

## Sequencing note

This specification is ordered before the tool suite, executors, approval gates,
and compaction. Those four components will bind to whatever shapes the engine
settles on. Building them first and then discovering the composition needs
different shapes is the expensive ordering; the adapters already built are as
much accumulated surface as should be absorbed before this lands.
