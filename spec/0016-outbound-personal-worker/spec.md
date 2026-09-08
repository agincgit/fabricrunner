# Outbound personal-network model worker specification

**ID:** 0016

**Status:** Draft

**Depends on:** [Fabric Runner core](../0001-fabric-runner-core/spec.md),
[SQLite event store](../0002-sqlite-event-store/spec.md),
[Policy contracts](../0003-policy-contracts/spec.md),
[Deterministic routing](../0004-deterministic-routing/spec.md),
[Provider conformance](../0006-provider-conformance/spec.md),
[OpenAI-compatible adapter](../0007-openai-compatible-adapter/spec.md),
[Telemetry and audit](../0009-telemetry-audit/spec.md),
[Execution engine](../0010-execution-engine/spec.md), and
[Approval gates, budgets, and lifecycle](../0013-approval-budgets-lifecycle/spec.md).

**Delivery gate:** After 0011–0014 and the complete Phase 1 acceptance gate.

Supporting execution records: [plan](plan.md), [tasks](tasks.md), and
[acceptance](acceptance.md).

## Purpose

A cloud workload must be able to receive model output from personal hardware
without dialing into the home network. The personal worker establishes the
connection to a reachable coordinator, receives one bounded model assignment,
calls an operator-configured personal-network model, and returns normalized
events and a durable terminal result over that connection.

This is the concrete outbound-worker slice of 0001's Phase 2 gate. It is a
planned dependency for downstream cloud workloads, not a capability of v0.1.0.
Creating this specification now does not advance Phase 2 implementation ahead
of the previously agreed Phase 1-first sequence.

## Requirements

- **FR-WORK-001:** The worker initiates a TLS-protected, versioned bidirectional
  gRPC connection to the coordinator. Assignment delivery and result streaming
  use that worker-initiated connection. The home deployment requires no inbound
  listener reachable from the cloud, port forwarding, UPnP mapping, VPN, or
  publicly exposed model endpoint. Normal return traffic on established
  outbound connections is permitted.
- **FR-WORK-002:** A cloud-side provider bridge integrates with `Engine.Run` and
  the existing routing, policy, budget, and event-store contracts. The worker
  executes one bounded model turn through a local provider adapter; it does not
  start an independent workload loop or perform peer-to-peer delegation.
- **FR-WORK-003:** Operators provision a worker identity and trusted coordinator
  endpoint. Mutual TLS authenticates both ends; identity is bound to the
  registered worker, permitted models, and assignments. Expired, unknown,
  revoked, or mismatched identities and incompatible protocol versions fail
  closed. Secrets remain in operator configuration and never enter protocol
  event payloads, telemetry, or durable content logs.
- **FR-WORK-004:** Registration reports validated model capabilities, health,
  capacity, and protocol version. An offline, stale, incompatible, draining,
  revoked, or capacity-exhausted worker is excluded before scoring. A claimed
  capability never grants authorization to receive a workload.
- **FR-WORK-005:** Before assignment dispatch, the coordinator commits the
  placement, manifest-bound authorization, budget reservation, lease identity,
  workload/step/attempt IDs, expiry, and assignment digest. The worker durably
  records accepted assignment identity before acknowledgement or model contact.
  Model endpoints are locally configured allowlisted mappings; an assignment
  cannot supply an arbitrary URL or credential.
- **FR-WORK-006:** Every content crossing is evaluated before bytes leave its
  source trust zone. The receiver validates its manifest, digest, destination,
  scope, and authorization. Personal-model output is held at the worker until
  its return-path policy permits transmission; only classified metadata needed
  for that decision may precede it. Unknown classification is confidential;
  secret content never enters a model request. Approval, redaction, or
  transformation requirements must be fulfilled and re-evaluated before
  transmission; otherwise the transfer is denied.
- **FR-WORK-007:** Events carry attempt identity, contiguous sequence numbers,
  payload digests, and explicit acknowledgements. The coordinator commits each
  accepted event before exposing it or acknowledging durable receipt. The
  worker retains unacknowledged events in a bounded durable outbox. Duplicate
  frames are deduplicated; conflicting content for the same identity or
  sequence is rejected. The first valid terminal result committed for an
  attempt wins.
- **FR-WORK-008:** Reconnect resumes delivery for the same attempt from the last
  durable acknowledgement. It never silently invokes the model again. Lease
  ownership and a fencing generation prevent a replaced session from committing
  new progress or receiving new assignments. A disconnected worker cannot run
  beyond its configured lease validity or assignment deadline.
- **FR-WORK-009:** Transport redelivery preserves attempt identity. An explicit
  execution retry creates a new attempt linked to the previous one. Expired
  work with an unknown outcome enters reconciliation; `Retryable` metadata is
  insufficient authorization to replay an effect. Only an explicitly
  retry-authorized idempotent assignment or a recorded reconciliation/approval
  decision may be retried. Duplicate execution and charges must not be hidden
  by a successful reconnect.
- **FR-WORK-010:** Cancellation, timeout, revocation, and drain have bounded,
  recorded behavior. Cancellation stops the worker's model stream and releases
  owned resources; drain refuses new leases while resolving current work.
  Cleanup failure is recorded without replacing the primary result. Reconnect
  backoff, heartbeat, acknowledgement, lease and shutdown deadlines have
  explicit configurable bounds.
- **FR-WORK-011:** The first slice is model-only. Requests for remote tool
  execution, child workloads, arbitrary artifact fetching, or shell execution
  are rejected before dispatch. Unsolicited model tool calls fail the assignment
  without executing a tool. Streaming buffers, frame sizes, concurrent leases,
  model input/output limits, and unacknowledged outbox storage are bounded.
- **FR-WORK-012:** Coordinator and worker restart reconstruct durable ownership,
  progress, terminal outcomes, and uncertainty without replaying committed
  effects. The audit comes from the event stores alone. Observer records cover
  enrollment, connection, lease, cancellation, drain and reconciliation while
  preserving 0009's classification and isolation guarantees.
- **FR-WORK-013:** Supply a runnable coordinator/worker example and deployment
  instructions for a cloud coordinator and a personal-network model. Document
  configuration, credential provisioning/rotation/revocation, outbound network
  requirements, failure diagnostics, and tested platforms. The example must
  demonstrate the cloud request reaching the personal model and returning its
  result through the existing engine.

## Non-goals

- General remote tool execution or completing every distributed-worker feature
  in 0001 solely by completing this specification.
- Hosting, launching, downloading, or managing model runtimes.
- Making arbitrary home services reachable, peer-to-peer hole punching, or
  operating a general-purpose tunnel or VPN.
- Claiming that every firewall permits gRPC. The deployment requires outbound
  reachability to an operator-controlled coordinator with compatible TLS/HTTP2
  transport; restrictive proxies must be tested and documented.
- Treating model-only scope as evidence that a model call is free of cost or
  safe to repeat.

## Sequencing and compatibility

0013 is a functional dependency for reservations, approval outcomes, and
bounded cleanup. The first model-only slice does not intrinsically need 0011's
tools, 0012's sandbox executors, or 0014's compaction on its wire path. All four
remain delivery predecessors through the Phase 1 gate. This distinction is
intentional; narrowing scope does not silently reorder the roadmap.

The integration plan must settle identity, cancellation and acknowledgement
ownership before publishing protocol v1. A raw `Provider.Stream` call currently
lacks workload/step/attempt IDs, manifests, leases and reservations, so the
bridge cannot infer those from model text or untrusted metadata. Any required
engine dispatch contract change must be recorded in this specification and
0010's signature-change table before implementation.
