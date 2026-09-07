# Telemetry and audit contract specification

**ID:** 0009

**Status:** Draft

**Depends on:** [Fabric Runner core](../0001-fabric-runner-core/spec.md)

Supporting execution records: [plan](plan.md), [tasks](tasks.md), and
[acceptance](acceptance.md).

## Purpose

`README.md` states that applications extend this module through public
contracts for providers, policies, tools, routing, and telemetry. Four of those
five exist. This specification defines the fifth.

Telemetry is separate from the event store on purpose. The event store is
durable history and the source of truth for replay; telemetry is operational
observation. They have different retention, volume, and consistency
requirements, and forcing both through one interface makes each worse.

## Requirements

- **FR-TEL-001:** The module exposes an exporter-neutral `Observer` contract.
  No implementation in this repository binds to a vendor, wire format, or
  collector.
- **FR-TEL-002:** The contract observes the whole execution path: workload and
  step lifecycle, attempt boundaries, routing decisions, policy verdicts,
  provider calls, and tool calls. It is not scoped to a single turn loop.
- **FR-TEL-003:** Observation never changes execution outcome. An observer that
  panics, blocks past its deadline, or returns an error does not fail the
  workload and does not alter any recorded event.
- **FR-TEL-004:** A nil observer is valid and costs nothing beyond a nil check.
  No caller is required to supply one.
- **FR-TEL-005:** Emitted records carry the classification of the data they
  describe. Content at or above `ClassConfidential` is never placed in a
  telemetry record; only its identifier, size, and hash may appear.
- **FR-TEL-006:** Record attribute keys are drawn from a closed, versioned set.
  Unbounded-cardinality values are not admitted as keys.
- **FR-TEL-007:** Observation is context-propagated. Cancellation of the
  execution context cancels in-flight observation without blocking the caller.
- **FR-TEL-008:** `LoopSink` remains turn-scoped and is not widened. Where both
  are supplied, loop events and observer records describe the same activity
  without either being derived from the other.
- **FR-TEL-009:** An audit view is derivable from the event store alone, with no
  observer attached. Telemetry improves operability; it is never the only record
  of what happened.

## Non-goals

- No exporter, collector, or transport implementation.
- No metric aggregation, sampling policy, or retention policy.
- No replacement for the event store as the audit source of truth.

## Constraints

- The contract is additive. No existing exported signature changes to
  accommodate it.
- The root package gains no dependency outside the standard library.
