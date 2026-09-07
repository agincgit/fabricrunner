# Execution engine specification

**ID:** 0010

**Status:** Draft

**Depends on:** [Fabric Runner core](../0001-fabric-runner-core/spec.md),
[SQLite event store](../0002-sqlite-event-store/spec.md),
[Policy contracts](../0003-policy-contracts/spec.md),
[Deterministic routing](../0004-deterministic-routing/spec.md),
[Turn and tool loop](../0005-turn-tool-loop/spec.md),
[Telemetry and audit](../0009-telemetry-audit/spec.md)

Supporting execution records: [plan](plan.md), [tasks](tasks.md), and
[acceptance](acceptance.md).

## Purpose

`docs/ARCHITECTURE.md` specifies an execution path threading policy
eligibility, placement, the provider adapter, the tool step, egress policy, and
the ordered tool result, and states that every transition is persisted before it
is exposed to a client.

No component performs that composition. `LoopRequest` carries identifiers, the
initial request, tools, middleware, a selector, a sink, a budget, and two
classifications; it carries no `ExecutionPolicy`, no `DataEgressPolicy`, no
`Router`, and no `EventStore`. The loop package references none of the four.

The consequences are concrete: policy is defined but never enforced on an
execution path, routing decisions are never bound to a model turn, and loop
progress is never persisted, so replay and projection loading can reconstruct
only a synthetic run rather than one that happened.

The Phase 1 exit condition assumes this composition exists. This specification
makes it a named deliverable.

## Requirements

- **FR-ENG-001:** An `Engine` composes `ExecutionPolicy`, `DataEgressPolicy`,
  `Router`, `EventStore`, `Provider`, and the turn/tool loop behind one entry
  point that runs a workload to a terminal state.
- **FR-ENG-002:** Every state transition is appended to the event store before
  it is observable by any caller. No client sees state the store has not
  committed.
- **FR-ENG-003:** A deny verdict from `ExecutionPolicy` prevents the model call.
  The prevented attempt is recorded as an event with the verdict and its
  reason.
- **FR-ENG-004:** A deny verdict from `DataEgressPolicy` prevents the crossing
  and is recorded. Content that policy cannot decide on is denied.
- **FR-ENG-005:** The routing decision that produced a placement is bound to the
  model turn it produced and is recoverable from the event stream, including
  candidate exclusions and the selected placement.
- **FR-ENG-006:** Replay of a completed workload contacts no provider and
  executes no tool, and reconstructs terminal state identical to the original
  run.
- **FR-ENG-007:** Cancellation at any point leaves the workload in a state the
  transition table permits. No partial transition is observable.
- **FR-ENG-008:** Budget exhaustion terminates the workload in a permitted
  terminal state with the exhausted dimension recorded.
- **FR-ENG-009:** The engine emits observer records for each path stage per
  FR-TEL-002, and runs identically with no observer attached.

## Non-goals

- No scheduling across workloads; the engine runs one workload.
- No distributed placement; remote workers are Phase 2.
- No tool implementations; the engine executes bindings it is given.

## Constraints

- Signature changes to `LoopRequest` and related types are expected and are
  recorded in [plan.md](plan.md) as they are discovered.
- The engine adopts the observer contract from 0009; it does not define one.
