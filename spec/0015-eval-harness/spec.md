# Eval harness specification

**ID:** 0015

**Status:** Draft

**Depends on:** [Execution engine](../0010-execution-engine/spec.md),
[Telemetry and audit](../0009-telemetry-audit/spec.md)

Supporting execution records: [plan](plan.md), [tasks](tasks.md), and
[acceptance](acceptance.md).

## Purpose

Whether an eval harness belongs in this module was an open scope question. The
decision is that it does, and this specification records it so the question is
not reopened.

The reasoning is that the pieces already built make a harness unusually cheap
here. Deterministic replay means a case can be re-scored without re-running a
model, and the event store already holds the record to score against. That
combination is not typical, and it is a genuine strength rather than scope
creep.

The alternative — leaving it out — was rejected because every embedder needing
one would build a private variant against these contracts, and those variants
would diverge in exactly the place where a shared report format helps most.

## Boundary

The harness is generic over what is evaluated. It supplies a runner, a scoring
interface, and a stable report format. **It supplies no cases.** Cases encode
product behavior and are the embedder's asset; they do not appear in this
repository in any form, including as examples or fixtures.

## Requirements

- **FR-EVAL-001:** A `Runner` executes a supplied set of cases against the
  engine and produces a report.
- **FR-EVAL-002:** A `Scorer` contract scores a completed workload. The module
  supplies the interface and no scoring policy.
- **FR-EVAL-003:** The report format is versioned and stable. A report is
  machine-readable and diffable across runs.
- **FR-EVAL-004:** A case can be re-scored from a recorded event stream without
  contacting a provider or executing a tool.
- **FR-EVAL-005:** Running a case is subject to the same policy, budget, and
  sandbox controls as any other workload. Evaluation is not a privileged path.
- **FR-EVAL-006:** Case content is supplied by the caller and never persisted
  into this repository. No fixture in this module contains a case.
- **FR-EVAL-007:** A failed case does not abort the run. The report distinguishes
  a failing case from an erroring harness.

## Non-goals

- No cases, no calibration data, no benchmark suite.
- No scoring policy, rubric, or judge model.
- No leaderboard, storage service, or reporting UI.
