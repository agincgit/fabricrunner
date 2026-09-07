# Deterministic routing acceptance

**Specification:** [spec.md](spec.md)

**Status:** Accepted

The implementation is accepted when:

- malformed requests, weights, capabilities, observations, policy verdicts,
  timestamps, and duplicate candidate IDs are rejected;
- candidates are filtered in FR-ROUTE-001 order and record only the first stable
  exclusion;
- non-allow, manifest-mismatched, or scope-mismatched policy verdicts exclude a
  candidate;
- a cross-zone candidate without an allowing egress verdict is excluded while a
  same-zone candidate does not require one;
- required modalities, tools, native tool use, structured output, context, and
  output capacity are enforced;
- cost budget and completion-before-deadline capacity are enforced;
- excluded candidates have no score;
- score components and totals match the versioned integer formula;
- ties select the lexically smallest candidate ID regardless of request order;
- repeated requests produce byte-for-byte equivalent decisions;
- no eligible route returns the complete assessment and
  `ErrNoEligibleCandidates`;
- router code cannot mutate caller-owned request data;
- canceled evaluations return the context error and no decision; and
- formatting, build, vet, static analysis, race tests, vulnerability scanning,
  CI, leak scanning, and repository disclosure scanning pass.

## Evidence

| Check | Evidence | Result |
|---|---|---|
| Format | `gofmt -l .` | Pass (no output) |
| Dependency integrity | `go mod verify` | Pass |
| Build | `go build ./...` | Pass |
| Vet | `go vet ./...` | Pass |
| Race and repetition | `go test ./... -race -shuffle=on -count=20` | Pass |
| Static analysis | `staticcheck ./...` with v0.6.1 | Pass |
| Vulnerability scan | `govulncheck ./...` | Pass (0 reachable vulnerabilities) |
| Repository reference scan | prohibited-reference scan excluding license and Git metadata | Pass |

Published CI evidence:

- [build](https://github.com/agincgit/fabricrunner/actions/runs/34096099977/job/101660000283): pass;
- [gitleaks](https://github.com/agincgit/fabricrunner/actions/runs/34096099977/job/101660000514): pass.
