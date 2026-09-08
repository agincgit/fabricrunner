# Fabric Runner core acceptance

**Specification:** [spec.md](spec.md)

**Status:** In Progress

Acceptance is based on externally observable behavior. A checked task is not
completion evidence by itself.

## Phase 0 gate

The canonical foundation is accepted when all of the following hold:

- `go test ./... -race -count=1` passes.
- Generated identifiers validate as UUIDv7 and preserve generation-time order.
- Invalid and terminal workload or step transitions are rejected without
  mutating state.
- Concurrent appends to one aggregate cannot commit the same sequence number.
- An append with a stale expected sequence returns a conflict.
- Stored event payloads cannot be mutated through caller-owned byte slices.
- Replay returns events in sequence order and performs no external side effect.
- Workload replay reconstructs identical workload and step state on repeated
  runs (FR-CORE-001 and FR-CORE-006).
- Sequence gaps, aggregate mismatches, unsupported schemas, unknown events,
  malformed relationships, and invalid transitions are rejected without
  changing projected state (FR-CORE-002 through FR-CORE-005).
- Invalid messages, tool schemas, named tool choices, and malformed stream
  events are rejected.
- A model stream preserves event order and terminates exactly once with stop or
  error.
- Repository formatting, build, vet, race-test, and secret-scanning checks pass.
- Builds use the specified patched Go release and vulnerability scanning finds
  no reachable known vulnerability (FR-CORE-007).
- Repository licensing identifies FSL-1.1-MIT and its per-version two-year MIT
  conversion (FR-CORE-008).
- Repository language consistently identifies current versions as Fair Source
  or source-available, release records expose future-license dates, and
  contribution terms preserve the future MIT grant (FR-CORE-009 and
  FR-CORE-010).

### Phase 0 evidence

Local verification on 2026-09-07:

| Gate | Evidence | Result |
|---|---|---|
| Formatting | `test -z "$(gofmt -l .)"` | Pass |
| Build | `go build ./...` | Pass |
| Vet | `go vet ./...` | Pass |
| Static analysis | `staticcheck ./...` | Pass |
| Race and repeatability | `go test ./... -race -count=20` | Pass |
| Disclosure scan | Prohibited repository markers absent from the working tree | Pass |
| License governance | Fair Source terminology, release dating, contribution terms, and v1 review recorded | Pass |
| Published CI | [Build and secret-scanning run](https://github.com/agincgit/fabricrunner/actions/runs/34091300323) | Pass |

## Phase 1 gate

- A stopped process can reconstruct an in-flight workload from SQLite and
  continue without repeating a committed side effect.
- A personal-network model can delegate one bounded reasoning step to a managed
  model and consume the normalized result.
- Anthropic and OpenAI-compatible adapters pass the same model and tool-loop
  conformance suite.
- Tool input and output are schema-validated, approved when required, executed
  in an available sandbox, and returned in provider-required order.
- Cancellation stops the active model stream, tool process group, and owned
  resources.
- Every compaction identifies the replaced event range, summary inputs, model,
  and token counts.

## Phase 2 gate

The outbound model-worker slice is specified by
[0016](../0016-outbound-personal-worker/acceptance.md). Its acceptance evidence
must include the cloud-to-personal deployment scenario; publishing the spec or
tagging the single-node engine does not satisfy this gate.

- A personal-network worker connects outbound, receives a lease, streams
  events, and completes an assignment using a model on the personal network
  without an inbound home network port or a VPN into the personal network.
- Worker disconnect and reconnect preserve attempt identity and durable state.
- An expired idempotent assignment can be retried; an uncertain non-idempotent
  assignment cannot be retried without reconciliation or approval.
- Every cross-zone transfer has a content manifest and an allow, transform,
  redact, require-approval, or deny decision.

## Phase 3 and v1 gate

- One workload uses managed-cloud, self-hosted-cloud, and personal-network
  models.
- Placement can change safely between model turns and delegated steps.
- Policy exclusions occur before scoring and cannot be restored by a learned
  scorer.
- Routing records include candidates, exclusion reasons, normalized scores,
  scorer version, and selected worker.
- Confidential content never enters managed-cloud requests unless an explicit
  egress decision allows it; secret content never enters any model request.
- Session, workload, step, and delegation budgets are enforced and reconciled.
- Replay reconstructs the visible history without contacting providers or
  executing tools.
- Supported side-effecting tools run in a sandbox by default and fail closed
  when the required sandbox is unavailable.
- Claude Tool Runner parity tests pass for every capability an adapter declares.
