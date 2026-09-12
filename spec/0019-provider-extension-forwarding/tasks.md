# Provider extension forwarding task ledger

**Specification:** [spec.md](spec.md)

**Plan:** [plan.md](plan.md)

**Acceptance:** [acceptance.md](acceptance.md)

- [ ] Write failing acceptance tests before changing any adapter.
- [ ] Add forwarding, namespace, and collision coverage to the conformance
  suite (FR-EXT-006).
- [ ] Forward extensions in the OpenAI-compatible adapter (FR-EXT-001).
- [ ] Forward extensions in the Anthropic adapter (FR-EXT-001).
- [ ] Ignore entries namespaced for other providers (FR-EXT-003).
- [ ] Reject collisions with core-owned fields (FR-EXT-004).
- [ ] Classify extension content with the request (FR-EXT-005).
- [ ] Confirm no core type changed (FR-EXT-002).
- [ ] Capture acceptance evidence.
