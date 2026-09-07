# Architecture

This document explains the architecture. The authoritative requirements and
acceptance gates are in the
[`Fabric Runner core specification`](../spec/0001-fabric-runner-core/spec.md).

## Core rule

Fabric Runner owns the provider-neutral model/tool loop. Provider SDKs translate
requests and events; they do not own sessions, execute tools, select providers,
or hide retries and compaction.

```text
client
  │
  ▼
control plane
  ├── workload engine
  ├── Fabric Runner
  ├── policy and egress gate
  ├── deterministic router
  └── append-only event store
          │
          ├── managed-cloud connector
          ├── self-hosted-cloud worker
          └── personal-network worker
```

A worker executes one bounded assignment: a model turn, tool action,
transformation, or verification. Workers never communicate directly. A model
may propose delegation, but the control plane creates, authorizes, budgets, and
places the child step.

## Trust zones

| Zone | Examples | Default posture |
|---|---|---|
| `personal` | Laptop, LAN GPU server, homelab | Sensitive work preferred here |
| `self_cloud` | User VPS, Kubernetes, rented GPU | User-controlled but remote |
| `managed_cloud` | Anthropic, OpenAI, Google | Non-public data requires an explicit egress rule |

Personal-network workers connect outbound. Running a worker must not require an
inbound home-network port.

## Public contracts

The public module will stabilize these contracts before implementation details:

- Canonical IDs, messages, model events, tools, workloads, and steps
- Append-only events and optimistic sequence checks
- Provider adapter interface
- Policy decision interface
- Worker capability and assignment protocol
- Router eligibility and scoring interface
- Artifact classification and egress decisions

Unknown provider fields may be retained as namespaced extensions. Core policy
and routing must not depend on opaque provider data.

## Execution path

```text
model request
  → policy eligibility
  → worker placement
  → provider adapter
  → normalized stream
  → tool/delegation step
  → execution and egress policy
  → sandboxed worker
  → ordered tool result
  → next model turn, which may use a different model
```

Every transition is persisted before it is exposed to a client. Replay never
contacts a model or reruns a tool.

## Safety invariants

1. Model output is untrusted input.
2. Approval policy and sandbox enforcement are separate.
3. Autonomous mode never disables isolation.
4. Secret content is never sent to a model or persisted as an artifact.
5. Cross-zone transfers have a content manifest and policy decision.
6. A worker may propose delegation but cannot choose its destination.
7. Non-idempotent work with an unknown outcome is not retried automatically.
8. Cancellation reaches model streams, tools, and complete process groups.

## Initial delivery

Milestone 0 establishes canonical types, append-only events, replay, and tests.
The first vertical slice then adds Anthropic and OpenAI-compatible adapters,
four built-in tools, approvals, and platform sandboxes. Distributed workers are
added only after the single-node execution model is deterministic.
