# Policy contracts acceptance

**Specification:** [spec.md](spec.md)

**Status:** Accepted

The implementation is accepted when:

- manifest validation rejects empty manifests, malformed SHA-256 digests,
  negative sizes, and unknown classifications;
- an unlabeled item has an effective confidential classification;
- a manifest reports its most restrictive effective classification;
- manifest digests are stable and bind item order and canonical fields;
- execution requests reject malformed identifiers, step kinds, targets, and
  manifests;
- egress requests reject invalid zones and malformed manifests;
- verdict validation rejects missing provenance, unknown actions, and missing
  transformation instructions;
- verdicts with missing or mismatched manifest digests fail closed;
- the core evaluation gate denies secret content for every model target without
  invoking a pluggable policy;
- baseline model execution denies every manifest containing secret content;
- baseline tool execution requires approval and never grants secret access;
- baseline cross-zone behavior matches FR-POL-010 exactly;
- canceled evaluations return `context.Canceled` without a verdict;
- repeated baseline evaluation returns identical verdicts; and
- build, vet, static analysis, race tests, vulnerability scanning, and leak
  scanning pass.

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

- [build](https://github.com/agincgit/fabricrunner/actions/runs/34094118252/job/101653762392): pass;
- [gitleaks](https://github.com/agincgit/fabricrunner/actions/runs/34094118252/job/101653762673): pass.
