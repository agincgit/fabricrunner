# Anthropic Messages adapter specification

**ID:** 0008

**Status:** Implemented

**Depends on:** [Provider adapter conformance](../0006-provider-conformance/spec.md)

Supporting execution records: [plan](plan.md), [tasks](tasks.md), and
[acceptance](acceptance.md).

## Purpose

Fabric Runner needs a provider adapter for the Anthropic Messages API so the
same canonical turn and tool loop can move work between Anthropic and other
eligible model endpoints. The adapter owns wire translation only; routing,
policy, tools, budgets, and delegation remain provider-neutral.

Protocol references: [Messages API](https://platform.claude.com/docs/en/api/messages/create),
[streaming messages](https://platform.claude.com/docs/en/build-with-claude/streaming),
and [Models API](https://platform.claude.com/docs/en/api/models/list).

## Requirements

- **FR-ANT-001:** Configuration requires a canonical provider name and absolute
  base URL. Plain HTTP requires explicit opt-in. URL user information, query,
  fragment, and redirects are rejected.
- **FR-ANT-002:** Credentials are obtained per request, support bearer or
  `x-api-key` authentication, and are never retained or exposed. Every request
  carries a validated Anthropic API version.
- **FR-ANT-003:** Model discovery follows bounded `GET /models` pagination,
  rejects malformed pages, cycles, empty or duplicate IDs, and maps advertised
  input and output limits onto deeply isolated configured capabilities.
- **FR-ANT-004:** Streaming sends `POST /messages` with `stream: true`, the
  selected model, and an explicit maximum output-token limit.
- **FR-ANT-005:** Leading system text maps to the top-level system field.
  User and assistant text, assistant tool calls, and correlated tool results
  preserve canonical order. Unsupported or misplaced content fails locally.
- **FR-ANT-006:** Tool definitions preserve input schemas and descriptions.
  Automatic, none, required, and named choice map to `auto`, `none`, `any`, and
  `tool` respectively.
- **FR-ANT-007:** Structured output maps to `output_config.format` as a JSON
  schema. The canonical output name and strict flag have no Messages wire
  equivalents and are not invented as provider fields.
- **FR-ANT-008:** The bounded SSE reader verifies named-event and JSON type
  agreement and enforces the documented message/content-block state machine.
- **FR-ANT-009:** Text and indexed tool-input fragments emit ordered canonical
  deltas. Each completed tool call emits exactly once at its block stop, only
  after its assembled input is valid JSON.
- **FR-ANT-010:** Initial and cumulative usage are converted into nonnegative
  canonical increments without double counting. Cache-read and cache-write
  input tokens are preserved.
- **FR-ANT-011:** End-turn, tool-use, maximum-token, refusal, stop-sequence, and
  pause-turn reasons normalize deterministically. `message_stop` produces the
  sole terminal stop after content blocks have closed.
- **FR-ANT-012:** In-stream provider errors emit a normalized terminal model
  error. HTTP, protocol, transport, and credential failures have typed,
  bounded, redacted errors.
- **FR-ANT-013:** Cancellation interrupts discovery and active reads. Stream
  close is idempotent and releases the body exactly once.
- **FR-ANT-014:** Caller requests, maps, schemas, raw JSON, wire payloads, and
  returned events are deeply isolated.
- **FR-ANT-015:** The adapter passes the shared provider suite and adversarial
  scripted-server tests without live credentials or external model calls.

## Non-goals

- Provider-side tools, skills, citations, or computer use
- Images, documents, thinking blocks, or prompt-cache controls
- Retries, routing, failover, pricing, or the automatic tool loop
- Batch, token-counting, or Files APIs
