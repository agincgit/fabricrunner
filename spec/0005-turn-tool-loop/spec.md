# Provider-neutral turn and tool loop specification

**ID:** 0005

**Status:** Implemented

**Depends on:** [Fabric Runner core](../0001-fabric-runner-core/spec.md)

Supporting execution records: [plan](plan.md), [tasks](tasks.md), and
[acceptance](acceptance.md).

## Purpose

Fabric Runner, rather than a provider SDK, owns the automatic model/tool loop.
Every model turn is independently selected so one bounded loop may move between
personal-network, self-hosted-cloud, and managed-cloud models without changing
message or tool semantics.

This milestone establishes the in-process orchestration boundary. Durable
workload persistence, approval queues, sandboxes, and concrete provider adapters
remain separate components.

## Requirements

- **FR-LOOP-001:** Before every model call, a `TurnSelector` returns one bounded
  provider and canonical model reference. Selection is repeated after tool
  results, permitting a different provider or zone on the next turn.
- **FR-LOOP-002:** The loop clones caller-owned messages, schemas, JSON values,
  maps, and results. Selectors, providers, tools, middleware, and sinks cannot
  mutate caller input.
- **FR-LOOP-003:** Provider streams are consumed in strict sequence and must
  contain exactly one terminal stop or error. Normalized model events are sent
  to the loop sink in the same order.
- **FR-LOOP-004:** Text deltas and completed tool calls form one canonical
  assistant message. Tool-call IDs are nonempty and unique across the loop.
- **FR-LOOP-005:** Every requested tool must be declared. Inputs and successful
  JSON outputs are validated against Draft 2020-12-compatible schemas before
  execution and before return to the next model.
- **FR-LOOP-006:** Tool failures become canonical error results carrying a
  stable public code and message while the final result retains the typed
  internal cause for observation. A tool failure does not silently become a
  successful output.
- **FR-LOOP-007:** Parallel execution is opt-in. A batch runs concurrently only
  when parallel execution is enabled and every invoked binding declares itself
  parallel-safe. Results and completion events remain in provider call order.
- **FR-LOOP-008:** Result middleware runs in declaration order before output
  schema validation and before the result is appended to messages. Middleware
  may redact, transform, limit, or annotate results but cannot change call ID.
- **FR-LOOP-009:** Model-call, tool-call, input-token, output-token, cost, and
  wall-time budgets are checked at deterministic boundaries. Exhaustion returns
  a partial result plus `ErrLoopBudgetExceeded`; no additional side effect is
  started.
- **FR-LOOP-010:** End-turn, refusal, and maximum-token stops terminate with a
  result. Tool-use stops require at least one completed tool call and continue
  only after ordered results are appended.
- **FR-LOOP-011:** Cancellation closes the active stream, reaches active tools,
  and returns the context error with the completed prefix only.
- **FR-LOOP-012:** Every declared tool is closed exactly once in reverse
  declaration order on success, failure, cancellation, timeout, or recovered
  panic. Cleanup errors are joined without replacing the primary cause.
- **FR-LOOP-013:** The sink receives globally increasing sequence numbers for
  turn start, normalized model events, tool start, tool completion, appended
  messages, and terminal loop state. A sink error stops the loop before the next
  provider or tool side effect.
- **FR-LOOP-014:** Provider-specific extensions may be preserved in normalized
  model events but never control loop, tool, policy, or budget behavior.

## Contracts

- `TurnSelector` selects a `Provider` and `ModelRef` for one turn.
- `ToolHandler` executes one schema-validated invocation and owns cleanup.
- `ToolResultMiddleware` transforms a canonical result before return.
- `LoopSink` synchronously records ordered canonical loop events.
- `TurnLoop` runs a bounded request and returns the complete canonical message
  prefix, usage, stop reason, counters, and normalized tool failures.

Tool bindings are an ordered slice, never a map. Tool names must be unique.
Cleanup order is therefore deterministic.

## Error model

Invalid requests fail before any selector, provider, tool, or sink call.
Provider and sink failures stop immediately. Tool execution failures are
represented both as tool-result content for model-visible recovery and as typed
failure records in the final result. The loop may continue after such a failure
while budget remains.

## Non-goals

- Tool approval persistence or user interfaces
- Sandbox implementation
- Built-in filesystem or command tools
- Message mutation and dynamic-tool hooks
- Context compaction
- Provider-specific retry logic
- Durable event-store integration
