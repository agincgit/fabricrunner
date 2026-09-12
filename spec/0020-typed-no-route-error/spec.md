# Typed no-route error specification

**ID:** 0020

**Status:** Draft

**Depends on:** [Deterministic routing](../0004-deterministic-routing/spec.md),
[Execution engine](../0010-execution-engine/spec.md)

Supporting execution records: [plan](plan.md), [tasks](tasks.md), and
[acceptance](acceptance.md).

## Purpose

`EvaluateRouter` returns `ErrNoEligibleCandidates` whether every candidate was
excluded by policy or every candidate was merely unhealthy. To a caller those
are opposite situations — "you may not" versus "not right now" — and one is
permanent while the other resolves itself.

A caller that needs the distinction has to re-derive it by inspecting candidates
before the engine runs, duplicating the routing logic it just invoked. The
engine already records the exclusions in the routing decision. The error simply
discards them.

## Requirements

- **FR-ROUTE-001:** A no-route error type wraps `ErrNoEligibleCandidates` and
  carries the `RoutingDecision` that produced it.
- **FR-ROUTE-002:** `errors.As` yields the decision, exposing each candidate's
  assessment and exclusion code.
- **FR-ROUTE-003:** `errors.Is(err, ErrNoEligibleCandidates)` continues to
  report true. Existing checks are unaffected.
- **FR-ROUTE-004:** The carried decision is the same one recorded in the event
  stream. The error and the durable record cannot disagree.
- **FR-ROUTE-005:** Exclusion codes are a closed, documented set, so a caller
  can map them to its own outcomes without string matching.

## Non-goals

- No caller-facing classification of codes into retryable and permanent. The
  codes are exposed; the interpretation is the caller's.
- No change to routing behavior or to which candidates are excluded.
