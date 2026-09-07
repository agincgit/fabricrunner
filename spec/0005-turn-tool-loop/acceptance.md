# Provider-neutral turn and tool loop acceptance

**Specification:** [spec.md](spec.md)

**Status:** Accepted

The implementation is accepted when:

- invalid requests invoke no collaborator;
- every turn invokes the selector and the selected model reaches only that
  provider;
- selection can change between turns after ordered tool results;
- malformed, out-of-order, unterminated, and post-terminal streams fail;
- duplicate or undeclared calls and schema-invalid inputs never execute a tool;
- schema-invalid successful output is not returned to a model;
- normalized tool failures preserve public error data and typed internal causes;
- eligible parallel calls overlap while unsafe batches execute sequentially;
- parallel results, completion events, and tool-result messages retain call
  order;
- middleware order is deterministic and call IDs cannot be changed;
- every supported budget stops before the next side effect;
- cancellation closes the stream, cancels active tools, and returns only the
  completed prefix;
- every tool closes exactly once in reverse order across success, failure,
  cancellation, and panic;
- sink sequence numbers are contiguous and sink failure stops before the next
  side effect;
- collaborators cannot mutate caller-owned input;
- race tests pass under repeated parallel execution; and
- formatting, build, vet, static analysis, vulnerability scanning, CI, leak
  scanning, and repository disclosure scanning pass.

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

Published CI evidence:

- [build and vulnerability scan](https://github.com/agincgit/fabricrunner/actions/runs/34098757598/job/101668139450): pass;
- [gitleaks](https://github.com/agincgit/fabricrunner/actions/runs/34098757598/job/101668139403): pass.
