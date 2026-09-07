# Deterministic routing specification

**ID:** 0004

**Status:** In Progress

**Depends on:** [Fabric Runner core](../0001-fabric-runner-core/spec.md),
[Policy contracts](../0003-policy-contracts/spec.md)

Supporting execution records: [plan](plan.md), [tasks](tasks.md), and
[acceptance](acceptance.md).

## Purpose

Fabric Runner needs one explainable placement decision for managed-cloud,
self-hosted-cloud, and personal-network model candidates. Policy establishes
authorization; routing may optimize only among candidates that remain eligible.

The initial router is deterministic and performs no I/O. Health, capability,
cost, latency, load, and policy observations enter as an immutable snapshot so
the same request always produces the same decision.

## Requirements

- **FR-ROUTE-001:** Eligibility filters run before scoring in this order:
  authenticated and compatible; healthy and not draining; execution-policy
  authorization and allowed zone; input-data egress authorization; required
  model and tool capabilities; context and output limits; reservable cost
  budget; and capacity before the deadline.
- **FR-ROUTE-002:** An excluded candidate is never scored and cannot be restored
  by a scorer, including an advisory learned scorer. The public evaluation
  boundary recomputes mandatory eligibility when validating router output.
- **FR-ROUTE-003:** Execution and egress verdicts must be valid, allow the
  operation, and bind the request's exact content-manifest digest and typed
  scope. Missing, approval-required, transformation, redaction, denial,
  invalid, or mismatched verdicts exclude the candidate.
- **FR-ROUTE-004:** Same-zone placement does not require an egress verdict.
  Every cross-zone candidate requires one.
- **FR-ROUTE-005:** The deterministic scorer considers normalized quality,
  locality, availability, cache affinity, cost, latency, and inverse load. It
  uses integer permille arithmetic and versioned weights; floating point is not
  used.
- **FR-ROUTE-006:** A decision records router name and version, request and
  manifest identity, the fixed routing timestamps and authorization-sensitive
  request snapshot, every candidate ID, zone, and model in request order, the
  first stable exclusion code and reason, every eligible candidate's normalized
  components and total score, and the selected candidate.
- **FR-ROUTE-007:** Highest score wins. Equal scores use the lexically smallest
  canonical candidate ID, independent of input order.
- **FR-ROUTE-008:** No eligible candidate returns a complete decision plus the
  sentinel `ErrNoEligibleCandidates`.
- **FR-ROUTE-009:** Invalid input fails before any partial decision is returned.
  Duplicate candidate IDs and malformed or out-of-range observations are
  invalid.
- **FR-ROUTE-010:** Cancellation returns `context.Canceled` or
  `context.DeadlineExceeded` and no decision.
- **FR-ROUTE-011:** Router implementations receive an isolated copy and cannot
  mutate caller-owned slices or maps.
- **FR-ROUTE-012:** Routing uses only canonical fields and never provider
  extension data.

## Canonical observations

Candidate snapshots contain:

- canonical candidate ID, trust zone, and model descriptor;
- authentication, compatibility, health, and drain state;
- execution-policy verdict and, when crossing zones, egress verdict;
- available tool names and declared model capabilities;
- available slots and earliest availability;
- expected cost and completion latency;
- quality, cache affinity, and load as integers from 0 through 1000.

The request also carries autonomous mode and, for cross-zone candidates, the
egress purpose and optional derived-from digest. These fields let routing verify
the complete authorization-sensitive policy scope without reevaluating policy.

The `Compatible` observation includes protocol compatibility and any
deployment-defined quality-class requirement. Routing never reads open-ended
model labels or provider extension fields.

The caller supplies a fixed `ObservedAt` and `Deadline`. The router never reads
the system clock. A candidate has capacity before the deadline only when it has
at least one available slot and `AvailableAt + ExpectedLatency` is not after the
deadline.

## Default score

All components are normalized to 0 through 1000. Default weights total 1000:

| Component | Weight |
|---|---:|
| Quality | 250 |
| Locality | 200 |
| Availability | 100 |
| Cache affinity | 100 |
| Cost | 150 |
| Latency | 150 |
| Inverse load | 50 |

Locality is 1000 in the source zone and 0 otherwise. Availability is 100 points
per available slot, capped at 1000. Cost is normalized against the step's
maximum cost. Latency includes queue wait and expected execution time and is
normalized against the fixed observation-to-deadline window.

## Non-goals

- Network discovery or active health probing
- Worker authentication or lease reservation
- Provider-specific model fields
- Learned scoring or feedback training
- Budget mutation or persistence
- Dispatching the selected candidate
