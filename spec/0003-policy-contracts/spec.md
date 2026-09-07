# Policy contracts specification

**ID:** 0003

**Status:** In Progress

**Depends on:** [Fabric Runner core](../0001-fabric-runner-core/spec.md)

Supporting execution records: [plan](plan.md), [tasks](tasks.md), and
[acceptance](acceptance.md).

## Purpose

Routing cannot score a worker, model, or tool until policy establishes that the
placement is eligible. Cross-zone content movement also requires a separate,
recordable egress verdict. These contracts must remain independent of any
provider SDK or application policy pack.

## Requirements

- **FR-POL-001:** Execution eligibility is evaluated before router scoring. A
  policy error or invalid verdict fails closed.
- **FR-POL-002:** Every cross-zone transfer is evaluated independently from
  execution eligibility and produces an allow, deny, require-approval,
  transform, or redact verdict.
- **FR-POL-003:** A content manifest identifies each immutable item by SHA-256,
  media type, byte size, and classification. Unlabeled content is treated as
  confidential.
- **FR-POL-004:** Secret content is never eligible for a model target in any
  zone. Tool access to secret content requires an explicit policy supplied by
  an operator; the baseline policy does not grant it.
- **FR-POL-005:** Verdicts carry a policy name, policy version, stable rule ID,
  action, human-readable reason, and the SHA-256 digest of the exact manifest
  evaluated, suitable for an append-only decision event.
- **FR-POL-006:** Approval and sandboxing are independent. An approval verdict
  does not assert that a sandbox is present, and sandbox availability is not an
  input to policy authorization.
- **FR-POL-007:** Core policy decisions use only canonical request fields, never
  opaque provider extensions.
- **FR-POL-008:** Cancellation propagates through policy interfaces.
- **FR-POL-009:** A transformation or redaction produces a new manifest that is
  evaluated again; manifest-bound verdicts cannot authorize different content
  or the original pre-transformation manifest.
- **FR-POL-010:** The built-in baseline policy is deterministic: model execution
  without secret content is eligible, tool execution requires approval,
  same-zone transfer is allowed, public cross-zone transfer is allowed,
  internal or confidential cross-zone transfer requires approval, and secret
  cross-zone transfer is denied.

## Contracts

Two interfaces remain separate:

- `ExecutionPolicy` evaluates a bounded model-turn or tool target.
- `DataEgressPolicy` evaluates movement from one trust zone to another.

Both return a canonical `PolicyVerdict`. The workload engine persists the
verdict before dispatch or transfer. The interfaces do not execute approvals,
transformations, tools, or network requests.

Every verdict is bound to `ContentManifest.Digest()`. The digest uses a
domain-separated, length-prefixed canonical encoding and preserves manifest
item order. A verdict with a missing or mismatched manifest digest fails closed.

## Classification ordering

From least to most restrictive:

```text
public < internal < confidential < secret
```

The effective classification of a manifest is its most restrictive item.
An empty manifest is invalid.

## Non-goals

- Application-specific rules or policy languages
- Identity-provider integration
- Approval queue persistence or user interfaces
- Sandbox selection or enforcement
- Router scoring
