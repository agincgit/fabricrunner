# Execution engine implementation plan

**Specification:** [spec.md](spec.md)

**Status:** Implemented

## Design

The root `Engine` receives the existing `TurnLoop` as a dependency, avoiding an
import cycle with the loop implementation. One request runs a bounded workload
with one execution step. Its selector evaluates execution and outbound egress
policy for every candidate on every turn, persists those verdicts, then routes
and persists the validated decision before the loop may contact a provider.

Persist-before-expose is enforced structurally rather than by convention: the
engine's only means of surfacing state is a projection read, and projection
reads come from the store. There is no in-memory view a caller can observe
ahead of the commit.

Execution appends core transitions and typed `execution.recorded` events to
the same workload aggregate. SQLite applies `WorkloadProjection.Apply` inside
the append transaction. Both the returned state and replay use that same
projection reducer; replay never invokes the live execution loop. This
supersedes the draft proposal to substitute replaying providers into the loop,
which would unnecessarily depend on live policy and loop behavior.

The complete ordered model request is hashed and classified at the highest
level of its content, tool definitions, and metadata. Unlabeled data defaults
to confidential. Artifact materialization fails closed until a policy-bound
artifact resolver exists. Provider response bytes are quarantined inside the
adapter boundary while return-path egress is evaluated, before the loop,
durable content log, or tools receive them. This does not prevent bytes from
arriving at the adapter's network connection.

Tool execution receives a separately persisted execution-policy verdict.
Approval, transformation, and redaction requirements fail closed until their
respective implementations exist. Tool results are classified again when
building the next turn's request. Observation occurs at direct execution call
sites, independently of the turn-scoped sink, which only persists loop events.

The initial lifecycle transitions and final result/terminal transitions each
append atomically. Cancellation finalizes through a bounded independent store
context. Store failures never trigger effect retries or fabricated terminal
state; callers receive the last readable committed projection and the error.
Duplicate workload IDs conflict before execution. Restart continuation remains
a separate Phase 1 gate; `Replay` is read-only.

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
| `Engine`, `EngineRequest` | Add public composition entry point and dependencies | Run one bounded workload through existing contracts |
| `WorkloadProjection` | Add validated `Execution` audit records | Rebuild policy, routing, loop activity, and result from the store |
| `LoopSink`, `LoopEvent`, `LoopRequest` | Unchanged | Compose through selector, provider and tool wrappers, and the existing sink |
| `Observer`, `Record` | Additive under 0009 | Observe without influencing execution decisions |

## Sequencing note

This specification is ordered before the tool suite, executors, approval gates,
and compaction. Those four components will bind to whatever shapes the engine
settles on. Building them first and then discovering the composition needs
different shapes is the expensive ordering; the adapters already built are as
much accumulated surface as should be absorbed before this lands.

## 0013 signature additions

| Type | Change | Reason |
|---|---|---|
| `Engine` | `Approver`, `ApprovalTimeout`, `SpendEstimator` | Approval gates and conservative pre-spend admission |
| `Budget` | `MaxSteps` | Bound total steps including the root; engine requests require at least one |
| `ExecutionRecord` | Approval, budget and cleanup payloads | Durable decisions before effects and auditable cleanup |

A spend estimator is now required for live engine calls. Its supplied bounds
must cover provider defaults when `MaxOutputTokens` is unset. The engine does
not mutate the authorized request after policy hashing. This supersedes the
0010 post-report-only token/cost behavior; the low-level loop still validates
reported usage independently.

## Recovery and compaction supersession

The read-only `Replay` contract remains unchanged. Live continuation is now
`Resume`, specified in [recovery plan](recovery-plan.md). New fields are
`LoopRequest.Continuation`, `LoopEvent.Checkpoint`, `TurnSelection.Messages`,
`ModelRequest.AutomaticCompaction`, and `Engine.ContextCounter`. These supersede
the original signature table's statement that every loop data type stayed
unchanged. The `LoopSink` method itself is unchanged.

A turn checkpoint carries the ordered history and cumulative loop usage/call
counts. Compaction records carry their own usage, which the engine includes in
the final total. New reservations after a checkpoint invalidate that recovery
boundary, including reservations for compaction. Mid-call work is never retried
from an older checkpoint.
