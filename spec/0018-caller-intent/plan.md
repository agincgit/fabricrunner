# Caller intent implementation plan

**Specification:** [spec.md](spec.md)

**Status:** Draft

## Design

`Intent` is additive on `EngineRequest`, recorded on the workload at creation,
and threaded into every `DataEgressPolicyRequest` the engine constructs. It sits
beside `Purpose` rather than replacing it, because the two answer different
questions: `Purpose` is which direction the content is moving, `Intent` is why
the workload needs the crossing at all. Collapsing them would lose one.

The core treating intent as opaque is the property that keeps this cheap. No
routing input, no policy default, no parsing — it is recorded and handed on.

## Delivery sequence

1. Add the field and record it on the workload.
2. Thread it into every egress evaluation the engine performs.
3. Prove `Purpose` is unchanged at each of the engine's existing call sites.
4. Prove omission is a no-op for existing callers.

## Signature changes

| Type | Change | Reason |
|---|---|---|
| `EngineRequest` | `+ Intent string` | carry caller reason for the crossing |
| `DataEgressPolicyRequest` | `+ Intent string` | expose it to policy, beside `Purpose` |

Recorded in 0010's signature-change table on implementation.
