# Telemetry and audit implementation plan

**Specification:** [spec.md](spec.md)

**Status:** Implemented

## Design

Define `Observer` in the root package as a narrow interface over a single
`Record` envelope. The envelope carries a kind, the identifier triple
(workload, step, attempt), a classification, a monotonic sequence, and a closed
attribute set. Kinds enumerate the path stages named in FR-TEL-002.

Isolation is the load-bearing property. Every call site wraps observation so
that a panic is recovered, an error is discarded, and a deadline is bounded.
The engine treats observation as fire-and-forget: it never reads a value back
from an observer, so no observer can influence a decision.

Classification filtering happens at construction, not at export. A record is
built from already-classified material and drops content above the threshold
before it exists, so an exporter cannot be the thing that leaks.

## Delivery sequence

1. Define `Record`, `RecordKind`, and the closed attribute key set.
2. Define `Observer` and the no-op behavior for a nil observer.
3. Add the isolation wrapper: panic recovery, error discard, deadline bound.
4. Add classification filtering and its tests.
5. Add a `observertest` conformance suite third-party observers can run.
6. Wire observation into the engine under 0010, not here.

## Sequencing note

This specification is deliberately ordered before
[0010](../0010-execution-engine/spec.md). The engine is what observes the full
path, so building it first and retrofitting observation would force the seam
through whatever shape the engine happened to take. The contract is defined
first; the engine adopts it.

## Open questions

Resolved: exporters assign receipt timestamps; the record includes a
per-execution monotonic sequence. Concurrent delivery may arrive out of order.

The isolation boundary starts callbacks asynchronously, sets a bounded context,
recovers panics, and ignores errors. A process-wide limit of 64 in-flight
callbacks bounds resource use even when an exporter ignores cancellation.
Saturation drops operational observations, never durable events. Go cannot
forcibly terminate arbitrary observer code; such callbacks occupy a slot until
they return. A nil observer allocates no observation dispatcher.

Record construction removes content and all free-form attribute values at or
above confidential. Identifier, byte count and SHA-256 remain. The emission
boundary repeats construction so hand-built records cannot bypass filtering.

## Lifecycle observation clarification

0012 lifecycle events are emitted with the observer's own bounded timeout,
independent of a just-completed cleanup context. Otherwise the cleanup context
could be canceled before the exporter goroutine is scheduled. Durable audit
still commits first, and observation remains best effort with the same global
in-flight cap. The sandbox acceptance test checks both the committed events and
cooperative observer delivery.
