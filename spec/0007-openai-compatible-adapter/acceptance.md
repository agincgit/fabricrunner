# OpenAI-compatible adapter acceptance

**Specification:** [spec.md](spec.md)

**Status:** Pending

The implementation is accepted when:

- invalid, credential-bearing, ambiguous, and insecure-without-opt-in base URLs
  fail before transport;
- credentials are resolved per request, transmitted only in authorization, and
  absent from all returned errors and events;
- discovery validates status, size, JSON, IDs, ownership, capabilities, and
  isolation;
- every supported canonical role and content type produces the expected wire
  message order;
- unsupported content fails before the scripted server observes a request;
- all tool choices, strict function schemas, structured output, and both token
  field modes produce exact request JSON;
- SSE comments, multiline data, text, multiple indexed tool calls, usage-only
  chunks, finish reasons, and `[DONE]` normalize in order;
- malformed, oversized, unterminated, multi-choice, invalid-call, negative-usage,
  HTTP-error, and in-stream-error cases fail closed as specified;
- cancellation unblocks active reads and close releases the body exactly once;
- caller and provider-owned data remain isolated;
- the shared provider conformance suite passes against the adapter; and
- formatting, build, vet, static analysis, repeated race tests, vulnerability
  scanning, CI, leak scanning, and repository disclosure scanning pass.

## Evidence

Local verification on 2026-09-07:

| Check | Evidence | Result |
|---|---|---|
| Toolchain | `go version` | Pass (Go 1.25.13) |
| Formatting | `gofmt -l .` | Pass (no output) |
| Dependency integrity | `go mod verify` | Pass |
| Build | `go build ./...` | Pass |
| Vet | `go vet ./...` | Pass |
| Race and repetition | `go test ./... -race -shuffle=on -count=20` | Pass |
| Static analysis | `staticcheck ./...` with v0.6.1 | Pass |
| Vulnerability scan | `govulncheck ./...` | Pass (0 reachable vulnerabilities) |
| Secret-history scan | `gitleaks detect --redact` with v8.30.1 | Pass |
| Repository reference scan | prohibited-reference scan excluding license and Git metadata | Pass |

Published CI evidence is pending.
