# Provider adapter conformance acceptance

**Specification:** [spec.md](spec.md)

**Status:** Accepted

The implementation is accepted when:

- invalid capability values, tool-use modes, descriptor limits, labels, and
  references fail closed;
- provider discovery rejects an empty identity, mismatched ownership, and
  duplicate models;
- discovery cancellation is returned unchanged;
- discovered labels and capabilities cannot alias provider-owned data;
- missing, repeated, late, out-of-order, post-terminal, and unterminated start
  or terminal events fail;
- event types reject payload fields owned by another event type;
- tool-call deltas and completed calls validate their canonical fields;
- extensions survive deep cloning but cannot control stream state;
- stream cancellation is returned unchanged and every accepted stream closes
  exactly once;
- a close failure is joined with, and does not replace, a receive failure;
- the turn loop consumes the shared validator without changing its accepted
  provider-neutral behavior;
- the reusable fixture suite detects catalog, stream, close, and request
  isolation violations; and
- formatting, build, vet, static analysis, repeated race tests, vulnerability
  scanning, CI, leak scanning, and repository disclosure scanning pass.

## Evidence

Additional local verification on 2026-09-08 UTC for FR-PROVIDER-011:

- `TestAutomaticCompactionCapabilityDoesNotEnableRunnerCompaction` preserves
  the capability through discovery and proves that enabling it changes neither
  the original request messages nor the real turn loop's call count or result.
- `TestRetryableHintDoesNotRetryFailedTurn` proves that both hint values produce
  one provider call, one stream close and the same failed-turn outcome despite
  a budget allowing additional calls.
- `go test ./...`, `go build ./...`, and `go vet ./...` pass on Windows;
  `GOTOOLCHAIN=go1.25.13 go test ./... -race -count=1` passes on Ubuntu/WSL.
  Staticcheck v0.8.1 passes. No production behavior or signature changed.
- [Published release preparation CI](https://github.com/agincgit/fabricrunner/actions/runs/34185189210)
  passes, including the new hint regression tests and the full race suite.

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

Published CI evidence:

- [build and vulnerability scan](https://github.com/agincgit/fabricrunner/actions/runs/34100416088/job/101673335211): pass;
- [gitleaks](https://github.com/agincgit/fabricrunner/actions/runs/34100416088/job/101673334981): pass.
