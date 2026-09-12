# Outbound personal-network model worker implementation plan

**Specification:** [spec.md](spec.md)

**Status:** Draft

## Delivery position

The order remains Phase 1 first, then this first model-worker slice of Phase 2.
This specification is created now to name the capability and define its
acceptance boundary. It is not implementation evidence or a delivery date.

| Predecessor | Relationship to 0016 |
|---|---|
| 0010 execution engine | Implemented foundation for cloud-side composition |
| 0011 tool suite | Phase 1 delivery predecessor; remote tools are excluded here |
| 0012 sandbox executors | Phase 1 delivery predecessor; this worker invokes an existing model service |
| 0013 approvals, budgets, lifecycle | Functional dependency for reservation, decisions, cancellation, cleanup |
| 0014 context compaction | Phase 1 delivery predecessor; bounded requests use the established context rules |
| Phase 1 acceptance | Must also prove restart continuation and the complete single-node scenario |
| 0015 eval harness | Later evaluation work; its number does not make it a prerequisite |

There is no committed calendar ETA. Any caller needing cloud-to-home model
service before this gate passes must supply its own interim connection path.
Keep that integration behind the provider boundary so the worker bridge can
replace it. The no-inbound-port/no-VPN requirement remains committed; a tag of
the single-node engine is not evidence that the worker path exists.

## Topology and ownership

```text
cloud workload -> Engine -> worker gateway/provider bridge
                                  ^
                                  | worker-initiated TLS/gRPC stream
                                  |
                          personal worker -> local model adapter/runtime
```

The coordinator is reachable from the worker. The cloud does not initiate a
connection into the personal network. Assignments, cancellations, events and
acknowledgements travel in both directions on the established stream. Worker
registration does not publish the local model address to cloud clients.

The coordinator owns workload state, policy, placement, reservation and leases.
The worker owns a bounded durable assignment journal/outbox and its configured
model connection. Model credentials remain local. The worker is an executor
under the coordinator, not a second runtime making independent workload choices.

Use 0001's protobuf frame families: hello, welcome, capabilities, heartbeat,
capacity, assignment/lease, acknowledgement, event, terminal, cancel, drain,
reconfiguration and credential rotation. Define mandatory protocol versions,
identity bindings, maximum sizes and failure codes before generating code.

## Content and effect boundaries

Input authorization and the dispatch intent commit before transmission. The
worker validates them and journals acceptance before contacting its model.
Remote output requires authorization while it is still on the personal side.
The policy exchange can send a hash/size/classification manifest first; it must
not send the prohibited content to the coordinator in order to ask permission.
Metadata itself receives an egress decision. If that cannot be authorized, the
transfer fails closed.

0010 currently checks model responses after its adapter has received them. That
boundary alone is insufficient for the personal-to-cloud egress requirement.
The implementation must add source-side enforcement and tests before exposing
the network worker. Authorization binds exact bytes and destination; blanket
session approval cannot silently authorize later tool results or model output.

Stream receipt and durable receipt are distinct. A successful gRPC send never
serves as evidence of coordinator commit. Disconnect retries only delivery of
already-journaled frames for the current attempt. Restart cannot re-run a model
merely because its result acknowledgement is missing. Worker and coordinator
reconcile uncertainty using the journal, ownership generation and committed
outcomes before a new attempt is authorized.

## Implementation sequence after the Phase 1 gate

1. Write the acceptance harness and settle the engine dispatch envelope carrying
   canonical IDs, manifest-bound authorization, reservation and lease metadata.
2. Define and version protobuf messages and the bidirectional service. Add
   compatibility, size, authentication and identity-binding tests.
3. Implement authenticated enrollment, registry, heartbeat, capacity and drain.
4. Add coordinator dispatch journaling, reservations, leases and the provider
   bridge; route only eligible registered personal models.
5. Add worker assignment journaling and configured local provider invocation.
6. Enforce input and source-side result egress, including approval and denial.
7. Add the durable outbox, acknowledgement, deduplication, reconnect, fencing,
   cancellation and uncertain-outcome reconciliation.
8. Exercise coordinator/worker restart and bounded resource use, then run the
   cloud-to-personal acceptance scenario with new inbound home connections
   blocked. Publish measured evidence and deployment instructions.

## Contract decisions required before implementation

| Area | Decision to record |
|---|---|
| Engine dispatch | Typed envelope and ownership of IDs, manifests and reservations; `Provider.Stream` alone is insufficient |
| Identity | Provisioning and rotation of mutual-TLS worker identities; revocation behavior for active sessions |
| Policy | Source-side evaluation or bounded manifest-only authorization exchange; no content-before-authorization |
| Journal | Durable schema, acknowledgement frontier, digest binding and bounded retention |
| Leases | Expiry, renewal, session generation and rejection of stale owners/results |
| Limits | Concrete heartbeat, reconnect, cancellation, frame and outbox bounds tested under failure |

These are implementation decisions within committed requirements, not reasons
to weaken the no-inbound-port/no-VPN acceptance gate. Record chosen values and
API changes here and link corresponding tests before declaring implementation.
