# OpenAI-compatible adapter specification

**ID:** 0007

**Status:** In Progress

**Depends on:** [Provider adapter conformance](../0006-provider-conformance/spec.md)

Supporting execution records: [plan](plan.md), [tasks](tasks.md), and
[acceptance](acceptance.md).

## Purpose

Fabric Runner needs one credential-safe adapter for OpenAI Chat Completions and
compatible endpoints exposed by hosted services and personal-network runtimes.
The adapter translates exactly one canonical model turn; Fabric Runner retains
ownership of routing, tools, policy, budgets, and the automatic loop.

Protocol reference: [official OpenAI Chat Completions API](https://developers.openai.com/api/reference/cli/resources/chat).

## Requirements

- **FR-OAI-001:** Configuration requires a canonical provider name and absolute
  base URL. Plain HTTP requires an explicit opt-in so personal-network endpoints
  are supported without weakening the default for remote endpoints.
- **FR-OAI-002:** Credentials are obtained per request from a callback, sent
  only as a bearer authorization header, and never retained in events, model
  extensions, response errors, or formatted configuration.
- **FR-OAI-003:** Model discovery calls `GET /models`, rejects non-success HTTP
  status, malformed bodies, empty or duplicate IDs, and returns configured
  default capabilities and deeply cloned labels.
- **FR-OAI-004:** Streaming calls `POST /chat/completions` with `stream: true`
  and usage inclusion. The selected canonical provider must match the adapter.
- **FR-OAI-005:** System and user text, assistant text and function calls, and
  correlated tool results are translated without changing message or call
  order. Unsupported canonical content fails before any HTTP request.
- **FR-OAI-006:** Function definitions preserve input schemas and strict mode.
  Automatic, none, required, and named tool choice map to their canonical Chat
  Completions forms.
- **FR-OAI-007:** Structured JSON output maps to `response_format` with the
  canonical name, schema, and strict flag. No output constraint is invented.
- **FR-OAI-008:** The token-limit wire field is an explicit configuration
  choice between `max_tokens` and `max_completion_tokens`; the adapter never
  sends both.
- **FR-OAI-009:** The SSE reader accepts comments and multiline data, bounds
  event and HTTP error sizes, requires `[DONE]`, and rejects malformed JSON,
  unexpected event fields needed for control, and multiple completion choices.
- **FR-OAI-010:** Text and indexed function-call fragments emit ordered
  canonical deltas. Completed function calls emit once, in call-index order,
  only after their assembled arguments form valid JSON.
- **FR-OAI-011:** Usage is emitted before the terminal stop. Prompt,
  completion, cache-read, and cache-write token counts are nonnegative; unknown
  monetary cost remains zero rather than being estimated.
- **FR-OAI-012:** Finish reasons normalize to end-turn, tool-use, maximum-token,
  refusal, or unknown. Provider response identifiers, model identity, system
  fingerprint, and unknown finish reason may be preserved only as extensions.
- **FR-OAI-013:** An in-stream provider error emits one normalized terminal
  model error. HTTP, protocol, and transport failures retain typed causes and
  bounded redacted messages.
- **FR-OAI-014:** Context cancellation interrupts discovery, request creation,
  and active body reads. Stream close is idempotent and releases the body
  exactly once.
- **FR-OAI-015:** Caller requests, configuration maps, schemas, raw JSON,
  provider chunks, and returned events are deeply isolated.
- **FR-OAI-016:** The adapter passes the shared `providertest` suite and local
  scripted-server tests with no live credentials or external model calls.

## Security constraints

- URL user information, query strings, and fragments are rejected.
- Redirects are not followed by the adapter-owned default client.
- Authorization values and response bodies are never included in errors.
- Provider error messages are length-bounded.
- Canonical extension maps never alter translation behavior.

## Non-goals

- OpenAI Responses API
- Audio, image, file, or deprecated function-message translation
- Provider-side built-in tools
- Retries, backoff, routing, or failover
- Price estimation
- Live model capability inference
