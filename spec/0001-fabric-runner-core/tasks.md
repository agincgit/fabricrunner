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
- [x] Document licensing terminology, release dating, and inbound contribution
  terms (FR-CORE-009 and FR-CORE-010).
- [ ] Complete and record the v1.0 license review (FR-CORE-011).
- [x] Capture Phase 0 acceptance evidence.

## Phase 1: single-node vertical slice

- [x] Implement a SQLite WAL event store with transactional projections.
- [x] Define public execution-policy and data-egress-policy interfaces.
- [x] Implement the deterministic eligibility and routing contracts.
- [x] Implement the provider-neutral turn and tool loop.
- [x] Add Anthropic Messages adapter and conformance tests.
- [x] Add OpenAI-compatible adapter and conformance tests.
- [x] Reword the `SECURITY.md` sandbox sentence to state planned rather than
  current behavior.
- [x] Document `ModelCapabilities.AutomaticCompaction` and `ModelError.Retryable`
  as informational metadata, and assert in `providertest` that neither enables
  compaction or retries. These are the actual declaring types; earlier entries
  incorrectly named `ModelRequest` and `ModelEvent`.
- [x] Tag `v0.1.0` once the two items above have landed. Published on 2026-09-08
  UTC at `579ded98dd18e85610b606b0d3406c144413e313`; see the
  [release record](../../RELEASES.md).
- [x] Note the patched-toolchain requirement in the README so the `go` directive
  is not mistaken for a misconfiguration.
- [x] Add the telemetry and audit contract
  ([0009](../0009-telemetry-audit/spec.md)).
- [x] Add the execution engine that composes policy, routing, the event store,
  and the loop ([0010](../0010-execution-engine/spec.md)).
- [x] Add read, write, edit, and command tools
  ([0011](../0011-tool-suite/spec.md)).
- [ ] Add macOS Seatbelt and Linux bubblewrap executors
  ([0012](../0012-sandbox-executors/spec.md)).
- [x] Add approval gates, budgets, cancellation, and cleanup
  ([0013](../0013-approval-budgets-lifecycle/spec.md)).
- [x] Add context accounting and auditable compaction
  ([0014](../0014-context-compaction/spec.md)).
- [ ] Pass the Phase 1 end-to-end acceptance scenario.

## Phase 2: distributed workers

- [x] Carve the outbound personal-network model-worker slice into
  [0016](../0016-outbound-personal-worker/spec.md), preserving the no-inbound-port
  and no-VPN gate and explicitly sequencing implementation after 0011–0014 and
  Phase 1 acceptance. Only spec creation moved earlier; implementation did not.
- [ ] Define and version the protobuf worker protocol.
- [ ] Implement enrollment, authentication, capabilities, and heartbeats.
- [ ] Implement capacity, leasing, acknowledgement, cancellation, and drain.
- [ ] Implement outbound personal-network model-worker connections and pass
  [0016 acceptance](../0016-outbound-personal-worker/acceptance.md).
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

## Phase 4: evaluation

- [ ] Add the eval harness runner, scoring interface, and report format
  ([0015](../0015-eval-harness/spec.md)).
