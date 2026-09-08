# Outbound personal-network model worker task ledger

**Specification:** [spec.md](spec.md)

**Plan:** [plan.md](plan.md)

**Acceptance:** [acceptance.md](acceptance.md)

- [x] Name the outbound model-worker dependency and record its position after
  0011–0014 and the full Phase 1 gate. This is planning completion only.
- [ ] Verify the Phase 1 gate, including 0013 and restart continuation, before
  beginning this specification's implementation.
- [ ] Write failing behavioral acceptance tests for FR-WORK-001–013.
- [ ] Settle and document the engine dispatch contract; record changes in 0010
  before implementation (FR-WORK-002, FR-WORK-005).
- [ ] Define versioned protobuf/gRPC frames, negotiation, authentication and
  bounded limits (FR-WORK-001, FR-WORK-003, FR-WORK-011).
- [ ] Implement identity provisioning, rotation, revocation, registration,
  validated capabilities, heartbeat and capacity (FR-WORK-003, FR-WORK-004).
- [ ] Implement persisted dispatch, reservations, leases, fencing and worker
  acceptance journaling (FR-WORK-005, FR-WORK-008).
- [ ] Implement the cloud provider bridge and local model adapter invocation;
  reject remote tools and arbitrary endpoints (FR-WORK-002, FR-WORK-011).
- [ ] Enforce both input and source-side output egress before content crosses;
  bind approvals and transformations to exact manifests (FR-WORK-006).
- [ ] Implement contiguous event validation, durable acknowledgements, bounded
  outbox, deduplication and terminal-result commitment (FR-WORK-007).
- [ ] Implement reconnect and restart without implicit re-execution, explicit
  new-attempt retries, and uncertainty reconciliation (FR-WORK-008,
  FR-WORK-009, FR-WORK-012).
- [ ] Implement bounded cancellation, timeout, drain, revocation and cleanup
  with durable outcomes and classified observation (FR-WORK-010, FR-WORK-012).
- [ ] Add runnable deployment examples and capture the cloud-to-home scenario
  with no inbound home port or VPN (FR-WORK-001, FR-WORK-013).
- [ ] Fill the acceptance evidence table; do not mark 0016 or the full Phase 2
  gate complete from protocol definitions or an in-process mock alone.
