# Anthropic Messages adapter implementation plan

**Specification:** [spec.md](spec.md)

**Status:** Implemented

## Design

Implement `provider/anthropic` with the Go standard HTTP stack. Validate and
clone configuration at construction, fetch credentials for each request, and
keep all wire structures internal.

Translate a canonical turn completely before opening a connection. Parse the
documented SSE sequence through a bounded state machine: message start,
ordered content-block start/deltas/stop, message delta, and message stop.
Convert cumulative provider usage to canonical increments and complete tool
inputs at block boundaries.

Tests use scripted local servers and the reusable provider conformance suite.

## Delivery sequence

1. Add validated configuration, authentication, and paginated discovery.
2. Add exact message, tool, choice, output, and token-limit translation.
3. Add bounded SSE parsing and strict lifecycle enforcement.
4. Add normalized calls, usage, stop reasons, and errors.
5. Run the conformance suite, adversarial tests, and repository gates.
6. Publish through reviewed CI and record acceptance evidence.
