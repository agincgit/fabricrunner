# Anthropic Messages adapter acceptance

**Specification:** [spec.md](spec.md)

**Status:** Accepted

The implementation is accepted when:

- configuration, URL, API-version, authentication, and data-isolation checks
  fail closed without disclosing credentials or response bodies;
- bounded model pagination maps advertised token limits and rejects malformed,
  duplicate, empty-ID, cursorless, oversized, and HTTP-error responses;
- system, user, assistant, tool-call, and tool-result content produces the
  documented Messages wire order before a connection is opened;
- all canonical tool choices, strict schemas, structured output, and output
  limits produce exact request JSON;
- named SSE events follow the message and content-block lifecycle;
- text, tool-input fragments, completed calls, sparse cumulative usage, cache
  counters, stop reasons, and in-stream errors normalize in order;
- malformed, mismatched, unsupported, incomplete, duplicate-call, negative-
  usage, and unterminated streams fail closed;
- cancellation unblocks reads and close releases the body exactly once;
- the shared conformance suite and a complete two-turn tool-loop test pass; and
- formatting, build, vet, static analysis, repeated race tests, vulnerability,
  leak, and prohibited-reference scans pass.

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

Published CI evidence is recorded after the reviewed change runs remotely.
