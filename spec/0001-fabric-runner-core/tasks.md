# Fabric Runner core task ledger

**Specification:** [spec.md](spec.md)

**Plan:** [plan.md](plan.md)

**Acceptance:** [acceptance.md](acceptance.md)

## Phase 0: canonical foundation

- [x] Define UUIDv7 identifiers and validation.
- [x] Define zones, classifications, budgets, workloads, steps, and legal state
  transitions.
- [x] Define immutable events with schema versions, causal identifiers, and
  payload hashes.
- [x] Define optimistic append and ordered replay through the event-store
  contract.
- [x] Add a concurrency-safe in-memory event store.
- [x] Define provider-neutral messages, content, tools, model requests,
  capabilities, and model stream events.
- [x] Add formatting, build, vet, race-test, and secret-scanning CI gates.
- [x] Add aggregate projections and deterministic projection replay
  (FR-CORE-001 through FR-CORE-006).
- [x] Require a patched Go toolchain and a zero-reachable-vulnerability release
  gate (FR-CORE-007).
- [x] Apply FSL-1.1-MIT with the per-version two-year MIT conversion
  (FR-CORE-008).
- [x] Capture Phase 0 acceptance evidence.

## Phase 1: single-node vertical slice

- [x] Implement a SQLite WAL event store with transactional projections.
- [x] Define public execution-policy and data-egress-policy interfaces.
- [x] Implement the deterministic eligibility and routing contracts.
- [x] Implement the provider-neutral turn and tool loop.
- [ ] Add Anthropic Messages adapter and conformance tests.
- [ ] Add OpenAI-compatible adapter and conformance tests.
- [ ] Add read, write, edit, and command tools.
- [ ] Add macOS Seatbelt and Linux bubblewrap executors.
- [ ] Add approval gates, budgets, cancellation, and cleanup.
- [ ] Add context accounting and auditable compaction.
- [ ] Pass the Phase 1 end-to-end acceptance scenario.

## Phase 2: distributed workers

- [ ] Define and version the protobuf worker protocol.
- [ ] Implement enrollment, authentication, capabilities, and heartbeats.
- [ ] Implement capacity, leasing, acknowledgement, cancellation, and drain.
- [ ] Implement outbound personal-network worker connections.
- [ ] Implement cross-zone content manifests and egress decisions.
- [ ] Implement reconnect, lease expiry, and uncertain-outcome reconciliation.
- [ ] Pass the Phase 2 end-to-end acceptance scenario.

## Phase 3: workload fabric

- [ ] Implement durable DAG scheduling and child-step budgets.
- [ ] Implement bidirectional, control-plane-mediated delegation.
- [ ] Record candidate exclusions, routing scores, and selected placement.
- [ ] Add deterministic and independent-model verification policies.
- [ ] Enforce workload, step, delegation, token, cost, call, and time budgets.
- [ ] Add provider capability probes and compatibility reporting.
- [ ] Pass the three-zone v1 acceptance suite.
