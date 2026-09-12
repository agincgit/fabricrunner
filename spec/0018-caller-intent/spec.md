# Caller intent specification

**ID:** 0018

**Status:** Draft

**Depends on:** [Policy contracts](../0003-policy-contracts/spec.md),
[Execution engine](../0010-execution-engine/spec.md)

Supporting execution records: [plan](plan.md), [tasks](tasks.md), and
[acceptance](acceptance.md).

## Purpose

The engine sets `EgressPurpose` itself — `model_request` for the outbound turn,
`model_result` for the reply. That encodes *direction*, and it is useful: an
egress rule can permit asking a model while refusing to keep its answer.

What has no home is *why this turn needs to leave its zone at all*. Only the
caller knows that, and there is no field to carry it. A caller whose policy
requires a stated reason for moving non-public content must therefore enforce
it before the engine sees the request, after which the durable record holds the
real reason in one place and an engine-assigned constant in another.

## Requirements

- **FR-INT-001:** `EngineRequest` gains `Intent string`, free text and optional.
- **FR-INT-002:** Intent is recorded on the workload alongside `Goal` and is
  recoverable from the event stream.
- **FR-INT-003:** Intent is carried into `DataEgressPolicyRequest` as a field
  distinct from `Purpose`. A policy receives both axes and may rule on either.
- **FR-INT-004:** The core never interprets intent. Routing and the baseline
  policy ignore it entirely.
- **FR-INT-005:** Omitting intent changes nothing for existing callers. The
  field is additive and its zero value is valid.
- **FR-INT-006:** Intent is caller-supplied text and is treated as data. It is
  subject to the same classification rules as any other recorded content and is
  never interpreted as an instruction.

## Non-goals

- No intent vocabulary, taxonomy, or validation.
- No inference of intent from goal, content, or model output.
