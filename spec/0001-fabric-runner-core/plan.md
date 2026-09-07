# Fabric Runner core implementation plan

**Specification:** [spec.md](spec.md)

**Status:** In Progress

## Delivery approach

Build from deterministic, replayable contracts outward. Distributed execution
must use the same workload, event, policy, and model contracts as single-node
execution so adding workers does not create a second runtime.

## Phase 0: canonical foundation

1. Define sortable identifiers, trust zones, classifications, budgets,
   workloads, steps, and legal state transitions.
2. Define immutable event records, payload integrity, optimistic append, and
   ordered replay.
3. Define provider-neutral messages, tools, model requests, capabilities, and
   streaming events.
4. Verify the contracts with unit, replay, concurrency, and race tests.

Exit condition: synthetic workloads can be stored and deterministically
replayed without contacting a model or running a tool.

## Phase 1: single-node vertical slice

1. Add the SQLite WAL event store and rebuildable projections.
2. Implement the provider-neutral model/tool loop and budget enforcement.
3. Add Anthropic Messages and OpenAI-compatible adapters.
4. Add built-in filesystem and command tools behind approval and sandbox
   boundaries.
5. Add context accounting, explicit compaction events, and restart recovery.

Exit condition: a personal-network model delegates a bounded reasoning step to
a managed model and receives the result in one replayable workload.

## Phase 2: distributed workers

1. Define versioned protobuf and bidirectional gRPC contracts.
2. Add worker enrollment, capabilities, health, capacity, leases, cancellation,
   drain, and revocation.
3. Add outbound connections for personal-network workers.
4. Enforce cross-zone manifests, policy decisions, and uncertain-outcome
   handling.

Exit condition: a coordinator safely routes an assignment to a worker behind
NAT, survives a disconnect, and retains complete causal history.

## Phase 3: workload fabric

1. Add durable DAG scheduling and bounded child delegation.
2. Add deterministic routing scores, exclusion reasons, and placement records.
3. Add verification policies and the full hierarchical budget model.
4. Add provider conformance probes and end-to-end three-zone tests.

Exit condition: one workload uses managed, self-hosted cloud, and
personal-network models while policy prevents disallowed content movement.

## Design constraints

- Core packages must not import provider SDKs.
- Policy filters candidates before routing scores them.
- All externally visible state transitions are persisted first.
- Provider and worker retries must preserve attempt identity and causality.
- A learned router may advise scoring but cannot override policy eligibility.
- A missing required sandbox fails closed for side-effecting tools.
