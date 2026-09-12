# Outbound personal-network model worker acceptance

**Specification:** [spec.md](spec.md)

**Status:** Draft

## Required evidence

| Observable check | Requirements | Evidence |
|---|---|---|
| A cloud `Engine.Run` dispatches to a personal model through a worker-initiated TLS/gRPC stream and consumes its normalized result | FR-WORK-001, FR-WORK-002, FR-WORK-013 | Pending implementation |
| New inbound connections to the home network are blocked; no forwarded port, UPnP rule, VPN interface or externally reachable model listener is used | FR-WORK-001 | Pending implementation |
| Unknown, expired, revoked or mismatched identities and incompatible protocol versions cannot enroll or receive work | FR-WORK-003 | Pending implementation |
| Offline, stale, draining and saturated workers are excluded before scoring | FR-WORK-004 | Pending implementation |
| Coordinator dispatch/authorization/reservation/lease commit precedes assignment send; worker acceptance commit precedes acknowledgement and model contact | FR-WORK-005 | Pending implementation |
| Denied cloud input never reaches the worker model; denied personal output never leaves the worker's network; manifests and decisions remain auditable | FR-WORK-006 | Pending implementation |
| Duplicate delivery invokes the model once; repeated event frames do not duplicate committed progress; conflicting digests fail closed | FR-WORK-007, FR-WORK-009 | Pending implementation |
| Disconnect after model completion but before terminal acknowledgement replays the durable result under the same attempt without a second model call | FR-WORK-007, FR-WORK-008 | Pending implementation |
| Expired leases and replaced sessions cannot commit new progress; safe execution retries get new attempt IDs; uncertain effects require reconciliation or approval | FR-WORK-008, FR-WORK-009 | Pending implementation |
| Cancellation, timeouts, revocation and drain meet declared bounds and release the model stream and owned resources; cleanup errors preserve the original cause | FR-WORK-010 | Pending implementation |
| Tool requests, arbitrary URLs, artifact fetching, oversized frames and outbox exhaustion are rejected or backpressured before unbounded work | FR-WORK-011 | Pending implementation |
| Independent coordinator and worker restarts preserve ownership, uncertainty, acknowledged progress and terminal results without repeating committed effects | FR-WORK-012 | Pending implementation |
| Go build, vet, race tests, static analysis, vulnerability and secret scans pass; deployment commands and tested platforms are published | FR-WORK-013 | Pending implementation |

## End-to-end deployment gate

Run separate coordinator, worker and model-service processes in a network
topology that permits worker-initiated connections to the coordinator and
established return traffic, while denying new cloud-initiated connections into
the worker/model network. A Linux network-namespace/firewall harness is suitable
for CI; also record a run with a model on actual personal hardware before
claiming the deployment capability is satisfied.

Record topology, firewall/listener checks, protocol and software versions,
redacted configuration, assignment and attempt IDs, policy/lease/event records,
normalized model result, call count, and reconnect/restart/cancellation results.
Do not commit credentials or private infrastructure identifiers. Evidence must
show that the response came from the personal model and that no inbound path
was used. An echo-only transport test is insufficient.

Inject disconnects before acknowledgement, mid-stream, and after local model
completion. Record which frames are redelivered, which attempts are reconciled,
and whether any model invocation is repeated. Test output denial with content
inspection at the worker's outbound boundary; checking only the cloud log does
not prove that private content stayed on the personal network.

Completing 0016 proves the model-worker slice. The remaining 0001 Phase 2 gate
continues to govern broader distributed execution. No acceptance evidence
exists yet; spec creation is not worker availability.
