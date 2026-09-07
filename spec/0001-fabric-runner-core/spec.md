# Fabric Runner core specification

**ID:** 0001

**Status:** In Progress

**Scope:** Open-source Fabric Runner

**Baseline:** Go 1.25.6

Supporting execution records: [plan](plan.md), [tasks](tasks.md), and
[acceptance](acceptance.md).

## Product requirement

Fabric Runner must coordinate one durable workload across:

1. managed frontier model APIs;
2. user-hosted models in cloud infrastructure; and
3. models and tools on a personal network.

Routing occurs per workload step, not once per session. A local model can
delegate reasoning to a frontier model, and a frontier model can delegate
sensitive or data-local work to a personal-network worker. Every transition
passes through the control plane.

The repository must independently complete a local-to-frontier-to-local
workload. It is not an Anthropic SDK wrapper.

## Locked decisions

- Fabric Runner owns the provider-neutral model/tool loop.
- Provider SDKs are adapters beneath that loop.
- A worker executes one bounded assignment.
- Workers never communicate directly.
- Personal-network workers connect outbound.
- Policy eligibility runs before router scoring.
- Cross-zone transfers require data classification and an egress decision.
- State changes are append-only events with optimistic sequence checks.
- Delivery is at least once; uncertain side effects are not retried.
- Approval and sandbox enforcement are independent.
- Autonomous mode never disables isolation.
- Deterministic routing works without an LLM.

## Required components

| Component | Responsibility |
|---|---|
| Workload Engine | Durable DAG, dependencies, retries, deadlines, and budgets |
| Fabric Runner | Provider-neutral turn and tool loop |
| Policy Engine | Execution eligibility, approval, and data egress |
| Router | Candidate scoring, placement, and explainability |
| Worker Gateway | Registration, heartbeat, leases, cancellation, and drain |
| Provider Adapter | Canonical/provider protocol translation |
| Tool Runtime | Validation, policy, sandbox, execution, and ordered results |
| Verifier | Deterministic or independent-model result checks |
| Event Store | Replayable causal history and projections |
| Artifact Store | Immutable classified content addressed by hash |

## Provider-neutral loop

```text
prepare canonical request
  → evaluate data and execution policy
  → route one model turn
  → normalize the provider stream
  → persist text, usage, stop, and tool-call events
  → turn tool calls into child steps
  → approve and execute tools or delegations
  → normalize and order results
  → prepare the next turn, possibly on another model
```

Adapters must preserve event order, tool-call identity, usage, finish reasons,
and unknown provider data. They must emit exactly one terminal stop or error
event.

Initial adapters:

- Anthropic Messages API
- OpenAI Responses API
- OpenAI-compatible chat endpoints used by Ollama, vLLM, llama.cpp, and LM
  Studio

## Claude Tool Runner parity

Fabric Runner must retain the useful behavior of Anthropic's Tool Runner while
placing it behind provider-neutral contracts.

| Capability | Requirement |
|---|---|
| Automatic loop | Continue until a terminal model response, budget, approval, failure, or cancellation |
| Streaming | Emit ordered deltas and a single terminal event |
| Iteration limit | Enforce through step and workload budgets |
| Typed tools | Validate tool input and output schemas before execution and return |
| Parallel calls | Execute eligible independent calls concurrently and return provider-required order |
| Error wrapping | Normalize tool errors without losing typed internal cause data |
| Result middleware | Permit policy, redaction, caching metadata, size limits, and transformation before return |
| Message mutation | Permit explicit, evented insertion or replacement between turns |
| Dynamic tools | Add or remove tools between turns through an auditable capability event |
| Tool choice | Canonicalize automatic, none, required, and named selection |
| Strict schemas | Expose strict mode and declare per-model support |
| Structured output | Carry a canonical output schema with adapter capability checks |
| Prompt caching | Express cache intent without making core behavior provider-specific |
| Compaction | Record inputs, summary, model, token counts, and replaced event range |
| Multimodal results | Return text, image, document, and artifact-reference content |
| Stateful tools | Guarantee cleanup on completion, cancellation, timeout, and panic |

Anthropic's SDK Tool Runner may be used only inside a bounded leaf assignment
with no privileged tools, a strict budget, and complete normalized events. It
cannot own a Fabric Runner session.

## Workload steps

Required step kinds:

- `model_turn`
- `tool`
- `delegate`
- `transform`
- `verify`

Required step states:

```text
pending → ready → leased → running → succeeded
                    │        │       failed
                    │        ├────── waiting_approval
                    │        └────── uncertain
                    └─────────────── ready, when safe to retry
```

Every retry creates a new attempt. Existing attempts are immutable.

## Worker protocol

v1 uses versioned protobuf over bidirectional gRPC with TLS.

Worker frames:

- hello and capabilities
- heartbeat and capacity
- assignment acknowledgement
- streaming event
- terminal result or failure
- drained acknowledgement

Control-plane frames:

- welcome and negotiated protocol version
- assignment and lease
- cancel
- drain
- reconfigure non-secret limits
- credential rotation request

Assignments use at-least-once delivery and immutable attempt IDs. The first
valid terminal result committed for an attempt wins. An uncertain
non-idempotent assignment requires reconciliation or approval.

## Routing

Eligibility filters run in this order:

1. authenticated, compatible worker;
2. healthy and not draining;
3. policy-allowed zone;
4. allowed input data;
5. required model and tool capabilities;
6. sufficient context and output limits;
7. reservable budget; and
8. available capacity before deadline.

Only eligible candidates are scored. The initial deterministic scorer considers
quality, locality, availability, cache affinity, cost, latency, and load. Every
decision records candidates, exclusions, scores, scorer version, and selected
worker. A learned scorer may contribute a value but cannot restore an excluded
candidate.

## Data egress

All content carries one classification:

- `public`
- `internal`
- `confidential`
- `secret`

Unlabeled content is treated as confidential. Secret content is never sent to
a model. It may be injected into a specifically authorized tool process without
entering prompts or event payloads.

Every cross-zone transfer records source, destination, content manifest,
classification, action, rule, reason, and approver. A transformation creates a
new immutable artifact and does not imply that egress is allowed; policy checks
the transformed output again.

## Budgets

Session, workload, step, and delegation budgets support:

- input and output tokens;
- model and tool calls;
- child steps;
- monetary cost in integer microdollars; and
- wall-clock duration.

Budget is reserved before dispatch and reconciled after reported usage. Child
budgets are carved from the parent's remainder.

## Storage

The persistent implementation uses SQLite in WAL mode. JSONL is an export
format. Large payloads live in a content-addressed artifact store.

Minimum persistent records:

- sessions, workloads, steps, dependencies, and attempts;
- events and projections;
- workers, snapshots, and leases;
- artifacts and derivation edges;
- policy, egress, and approval decisions; and
- token and cost usage.

Appending events and updating projections occurs in one transaction. Replay
must not contact a provider or execute a tool.

## Canonical projections

- **FR-CORE-001:** A workload projection reconstructs the workload and its
  steps exclusively from the workload event stream.
- **FR-CORE-002:** Projection events must match the workload aggregate and form
  an uninterrupted sequence beginning at one.
- **FR-CORE-003:** Projectors validate event integrity, schema version, payload,
  identifiers, parent relationships, and state transitions before mutation.
- **FR-CORE-004:** A rejected event leaves the projection byte-for-byte
  equivalent to its state before the apply attempt.
- **FR-CORE-005:** Unknown event types, unsupported schema versions, duplicate
  creation, and trailing or unknown payload fields fail closed.
- **FR-CORE-006:** Projection replay performs no provider, tool, network, or
  other external execution.

## Sandboxing

Initial execution backends:

- macOS Seatbelt;
- Linux bubblewrap; and
- an optional container executor.

When no required sandbox is available, side-effecting tools fail closed unless
the operator explicitly enables unsafe host execution. Unsafe execution is
visible in status, events, and audit output.

## Delivery milestones

### M0: canonical foundation

- IDs and domain types
- Workload and step transitions
- Append-only event contract
- In-memory store and deterministic replay
- Provider-neutral messages and model stream
- CI formatting, build, vet, race tests, and secret scanning

Exit: synthetic workloads can be recorded, loaded, and replayed.

### M1: single-node vertical slice

- SQLite event store
- Fabric Runner loop
- Anthropic and OpenAI-compatible adapters
- `read`, `write`, `edit`, and `bash`
- Approval policy
- Seatbelt and bubblewrap
- Context accounting and compaction

Exit: a local model delegates one reasoning step to a frontier model and
receives the result in a replayable session.

### M2: distributed workers

- Worker protocol and enrollment
- Registry, heartbeat, capacity, leases, drain, and revoke
- Outbound personal-network worker
- Egress manifests and approval
- Disconnect recovery and uncertainty handling

Exit: a coordinator routes work to a LAN worker behind NAT without an inbound
LAN port.

### M3: complete workload fabric

- Durable DAG and bidirectional delegation
- Verification policy
- Full budget hierarchy
- Routing explanations and metrics
- Provider conformance probes

Exit: one workload uses all three zones while raw confidential content remains
outside managed cloud.

## v1 acceptance gate

v1 is complete only when:

1. Anthropic, OpenAI, and a home-network model participate in one workload.
2. Placement changes safely between turns and delegated steps.
3. A personal-network worker reconnects without losing durable state.
4. Confidential data cannot cross zones without a recorded decision.
5. Tools execute under supported sandbox profiles by default.
6. Workloads survive coordinator restart.
7. Cost, token, time, and delegation budgets are enforced.
8. Replay reconstructs visible history without side effects.
9. Routing decisions and rejected alternatives are explainable.
10. Claude Tool Runner parity tests pass where an adapter declares support.
