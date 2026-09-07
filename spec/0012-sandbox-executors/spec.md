# Sandbox executors specification

**ID:** 0012

**Status:** Draft

**Depends on:** [Policy contracts](../0003-policy-contracts/spec.md),
[Execution engine](../0010-execution-engine/spec.md)

Supporting execution records: [plan](plan.md), [tasks](tasks.md), and
[acceptance](acceptance.md).

## Purpose

`SECURITY.md` states that side-effecting tools are denied when a required
sandbox cannot be established. No code implements this. There is no sandbox
contract, and `policy.go` records that sandbox availability is deliberately
absent from `ExecutionTarget`, so no policy implementation could make that
determination even if it wanted to.

This specification defines the sandbox contract and its two executors, and is
the work that makes the existing `SECURITY.md` claim true.

## Requirements

- **FR-BOX-001:** The module exposes a `Sandbox` contract describing an
  established confinement, and an `Executor` that runs an argument vector
  inside one.
- **FR-BOX-002:** Failure to establish a sandbox denies the execution. Absence
  of a sandbox implementation is a denial, never a bypass.
- **FR-BOX-003:** `ExecutionTarget` gains sandbox capability so policy can
  evaluate it. This supersedes the deliberate omission recorded in `policy.go`.
- **FR-BOX-004:** A macOS executor confines via Seatbelt.
- **FR-BOX-005:** A Linux executor confines via bubblewrap.
- **FR-BOX-006:** On a platform with no supported executor, side-effecting
  execution is denied and the denial names the unsupported platform.
- **FR-BOX-007:** Confinement is asserted by observed behavior — a denied write
  outside the root, a denied network call, a denied process escape — not by the
  presence of a flag in a constructed command line.
- **FR-BOX-008:** Sandbox establishment, denial, and teardown are recorded as
  events and observer records.

## Non-goals

- No container runtime, VM, or hypervisor isolation.
- No seccomp profile authoring beyond what bubblewrap provides.

## Constraints

- FR-BOX-003 changes a published type. The change is recorded in 0003 as a
  supersession rather than an unremarked edit.
