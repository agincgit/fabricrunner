# Provider extension forwarding specification

**ID:** 0019

**Status:** Draft

**Depends on:** [Provider conformance](../0006-provider-conformance/spec.md),
[OpenAI-compatible adapter](../0007-openai-compatible-adapter/spec.md),
[Anthropic Messages adapter](../0008-anthropic-messages-adapter/spec.md)

Supporting execution records: [plan](plan.md), [tasks](tasks.md), and
[acceptance](acceptance.md).

## Purpose

`ModelRequest.Extension` exists and neither adapter reads it. A caller therefore
has no way to send a single provider-specific field, and every such field
becomes a core release — a new typed member on `ModelRequest`, a conformance
change, and a version bump, for something only one provider understands.

Forwarding the extension under its namespaced keys, with core types unchanged,
is the cheapest fix with the widest effect.

## Requirements

- **FR-EXT-001:** Each adapter forwards `ModelRequest.Extension` entries under
  its own namespace to the provider request.
- **FR-EXT-002:** Core types do not change. No new typed field is added to
  `ModelRequest` for any provider-specific control.
- **FR-EXT-003:** An entry namespaced for another provider is ignored, not
  forwarded and not an error.
- **FR-EXT-004:** An extension entry never overrides a field the core owns.
  A collision with a core-controlled field is rejected rather than silently
  winning.
- **FR-EXT-005:** Extension content is caller-supplied data. It is never derived
  from model output, and it carries the request's classification.
- **FR-EXT-006:** The conformance suite covers forwarding, namespace isolation,
  and core-field protection, so a third-party adapter is held to the same rule.

## Non-goals

- No registry, schema, or validation of extension keys.
- No translation of an extension between providers.

## Note

Reasoning-effort control is the immediate use for this. It is an example, not
part of the contract — the point of the extension is that such controls do not
require a core release.
