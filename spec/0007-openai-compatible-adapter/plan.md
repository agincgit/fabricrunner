# OpenAI-compatible adapter implementation plan

**Specification:** [spec.md](spec.md)

**Status:** Implemented

## Design

Implement `provider/openaicompat` using only the Go standard HTTP stack. Keep
wire structures unexported and expose a validated immutable configuration plus
the root `Provider` interface.

Translate canonical requests into Chat Completions JSON before opening a
connection. Expand canonical tool-result batches into correlated wire messages.
Parse SSE incrementally through a bounded reader. Emit `start` immediately
after a successful response, stream text and tool fragments, then finalize
assembled calls, usage, and one stop when `[DONE]` arrives.

Use a per-request credential callback and an adapter-owned redirect policy.
Tests use `httptest.Server`; the reusable provider suite consumes a fresh
scripted fixture.

## Delivery sequence

1. Add immutable configuration, URL, credential, and model-discovery behavior.
2. Implement canonical request and message translation.
3. Implement bounded SSE parsing and stream lifecycle.
4. Normalize deltas, completed calls, usage, errors, extensions, and stops.
5. Add the shared conformance suite and adversarial HTTP tests.
6. Run local and published acceptance gates and record evidence.
