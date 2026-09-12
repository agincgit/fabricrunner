# Provider extension forwarding implementation plan

**Specification:** [spec.md](spec.md)

**Status:** Draft

## Design

Each adapter reads only its own namespace from `Extension` and merges those
entries into the provider request it is already building. Everything else is
ignored, so a caller can populate extensions for several providers on one
request and let routing decide which adapter sees which.

Core-field protection is the one place this must not be permissive. If an
extension key maps onto something the core owns — the model, the message list,
the tool set, the budget — forwarding it would let a caller bypass policy and
budget by writing directly to the provider payload. That collision is an error,
not an override.

## Delivery sequence

1. Add namespace extraction and core-field collision detection to the shared
   conformance suite first.
2. Implement forwarding in the OpenAI-compatible adapter.
3. Implement forwarding in the Anthropic adapter.
4. Prove cross-namespace entries are ignored by both.
