# Provider adapter conformance specification

**ID:** 0006

**Status:** Implemented

**Depends on:** [Fabric Runner core](../0001-fabric-runner-core/spec.md),
[Provider-neutral turn and tool loop](../0005-turn-tool-loop/spec.md)

Supporting execution records: [plan](plan.md), [tasks](tasks.md), and
[acceptance](acceptance.md).

## Purpose

Every model adapter must present the same canonical catalog, request, stream,
and cancellation behavior regardless of its provider SDK or wire protocol.
This milestone defines that boundary and a reusable Go test suite so concrete
adapters are tested against one contract.

## Requirements

- **FR-PROVIDER-001:** Provider names, model references, tool-use modes,
  context limits, output limits, capability combinations, and labels are
  validated before discovery data can enter routing.
- **FR-PROVIDER-002:** Canonical discovery rejects duplicate models and model
  references whose provider does not match the provider being inspected.
  Returned descriptors are deeply isolated from provider-owned memory.
- **FR-PROVIDER-003:** A canonical stream begins with exactly one `start`
  event at sequence one, advances contiguously, and ends with exactly one
  `stop` or `error` event followed by EOF.
- **FR-PROVIDER-004:** Every event carries only the canonical payload allowed
  for its event type. Unknown provider data may be retained only in the
  extension map and cannot alter validation or terminal behavior.
- **FR-PROVIDER-005:** Tool-call deltas carry a nonnegative provider-neutral
  call index and at least one identity, name, or JSON-fragment field. Completed
  calls carry valid JSON and stable nonempty identity and name.
- **FR-PROVIDER-006:** Usage values and stop reasons are validated before they
  are exposed to the loop. Negative counts, negative cost, and unknown stop
  reasons fail closed.
- **FR-PROVIDER-007:** Discovery and stream helpers return context cancellation
  unchanged, close every accepted stream exactly once, and join close failures
  without hiding the primary failure.
- **FR-PROVIDER-008:** Provider and stream inputs and outputs are deeply cloned
  at the canonical boundary. Adapter or test-observer mutation cannot modify
  caller-owned requests or previously returned data.
- **FR-PROVIDER-009:** The reusable `providertest` suite verifies identity,
  discovery, stream ordering, terminal state, close behavior, extension
  preservation, and request isolation for any adapter fixture without network
  credentials.
- **FR-PROVIDER-010:** The provider conformance layer performs no routing,
  policy, retries, tool execution, message mutation, or automatic looping.
- **FR-PROVIDER-011:** `ModelCapabilities.AutomaticCompaction` and
  `ModelError.Retryable` are preserved as informational metadata. Neither
  triggers runner compaction, retries, or rerouting. Regression tests exercise
  the real turn loop with each hint enabled and disabled; behavior remains
  unchanged apart from the descriptive metadata. This documents the v0.1.0
  boundary pending explicit compaction and retry policy implementations.

## Contracts

- `DiscoverModels` validates one provider catalog and returns a deep clone.
- `ModelStreamValidator` validates one normalized stream incrementally.
- `DrainStream` owns accepted-stream cleanup and returns the validated event
  prefix.
- `providertest.Run` executes the same observable suite against an adapter
  fixture supplied by the adapter package.

## Non-goals

- Anthropic or OpenAI wire translation
- Live-provider probes or credentials
- Retry, backoff, quota, or failover policy
- Model selection or routing
- Tool execution
