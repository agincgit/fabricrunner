# Approval gates, budgets, and lifecycle specification

**ID:** 0013

**Status:** Implemented

**Depends on:** [Execution engine](../0010-execution-engine/spec.md),
[Tool suite](../0011-tool-suite/spec.md)

Supporting execution records: [plan](plan.md), [tasks](tasks.md), and
[acceptance](acceptance.md).

## Purpose

The engine can run a workload to completion, but nothing pauses it for human
judgement, enforces its budget in every dimension, or guarantees that a
cancelled or failed run leaves nothing behind. This specification covers the
three together because they share one property: each is a way a workload stops
early, and each must leave a state the transition table permits.

## Requirements

- **FR-GATE-001:** An `Approver` contract can gate a step before it executes.
  A gate that is not configured does not pause execution.
- **FR-GATE-002:** An approval request carries what is being approved, its
  classification, and its target. It never carries content above
  `ClassConfidential`.
- **FR-GATE-003:** A denied or timed-out approval terminates the workload in a
  permitted terminal state and records the decision and who or what made it.
- **FR-GATE-004:** Approval outcomes are recorded such that replay does not
  re-request approval.
- **FR-BUD-001:** Every budget dimension — token, cost, call, time, step — is
  enforced, and exhaustion of any one terminates the workload with that
  dimension recorded.
- **FR-BUD-002:** A budget is checked before the spend, not after. A call that
  would exceed the budget is not made.
- **FR-LIFE-001:** Cancellation propagates to in-flight provider calls and tool
  executions, and completes within a bounded interval.
- **FR-LIFE-002:** Cleanup runs on every termination path — success, failure,
  cancellation, budget exhaustion, and approval denial. Temporary files,
  sandboxes, and processes are released.
- **FR-LIFE-003:** Cleanup failure is recorded and never masks the original
  termination reason.

## Non-goals

- No approval user interface, notification transport, or identity system.
- No cost estimation model; the engine enforces a supplied budget.
