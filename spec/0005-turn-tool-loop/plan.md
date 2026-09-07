# Provider-neutral turn and tool loop implementation plan

**Specification:** [spec.md](spec.md)

**Status:** In Progress

## Design

Put canonical loop, selection, tool, middleware, event, result, and error types
in the root module. Put the deterministic orchestration implementation in
`loop` and JSON Schema compilation in an internal helper.

Treat each turn as a fresh selection plus one bounded `Provider.Stream` call.
Accumulate only normalized events. Assemble canonical assistant and tool-result
messages without retaining provider-owned buffers.

Compile tool schemas once per loop invocation. Execute a fully parallel-safe
batch with one goroutine per call and an isolated result slot; otherwise execute
the complete batch sequentially. Emit starts before execution and completions
in original call order.

Use a single deferred cleanup path with panic recovery and reverse-order close.

## Delivery sequence

1. Define validated and cloneable loop, tool, selection, event, result, and
   error contracts.
2. Add Draft 2020-12-compatible input and output schema validation.
3. Implement per-turn selection and normalized stream consumption.
4. Implement sequential and parallel-safe tool batches with ordered results.
5. Implement result middleware, budgets, events, cancellation, panic recovery,
   and deterministic cleanup.
6. Add conformance-style fakes covering success and every failure boundary.
7. Run local release checks and published CI, then capture acceptance evidence.
