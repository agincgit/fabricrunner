# Tool suite acceptance

**Specification:** [spec.md](spec.md)

**Status:** Implemented

Each criterion below is the name of a test to be written before the
implementation exists.

The implementation is accepted when:

- `TestPathEscapeRefused` — traversal, absolute paths, and symlinks pointing
  outside the root are each refused, asserted on the resolved path;
- `TestSideEffectingToolsDeclared` — every writing, editing, or executing tool
  reports itself as side-effecting;
- `TestSideEffectingToolFailsClosedWithoutSandbox` — each side-effecting tool
  is denied when no sandbox can be established;
- `TestUnknownProvenanceClassifiedConfidential` — tool output of unknown
  provenance is not classified public;
- `TestOversizedOutputTruncatedAndRecorded` — output past the bound is
  truncated with the truncation recorded;
- `TestCommandToolTakesArgumentVector` — the command tool exposes no shell
  string path, asserted by the absence of a shell in the executed invocation;
- `TestToolCancellationRecordsPartialEffects` — a cancelled tool's partial
  effects are recorded; and
- build, vet, static analysis, race tests, and leak scanning pass.

## Evidence

See the dated local acceptance evidence below.

## Local acceptance evidence — 2026-09-08 UTC

Implemented on Linux; Windows explicitly denies side effects. No GitHub push has
been made. macOS executor acceptance is tracked separately in 0012.

| Requirement | Observable evidence | Result |
|---|---|---|
| FR-TOOL-001, FR-TOOL-002 | `TestPathEscapeRefused`; confined write/edit/read round trip | Pass on Linux |
| FR-TOOL-003, FR-TOOL-004 | Declaration and missing-sandbox denial tests | Pass |
| FR-TOOL-005, FR-TOOL-006 | Unknown provenance and explicit output truncation tests | Pass |
| FR-TOOL-007 | `TestCommandToolTakesArgumentVector` sends shell metacharacters literally | Pass on Linux |
| FR-TOOL-008 | `TestToolCancellationRecordsPartialEffects`; durable sandbox execution outcome | Pass on Linux |
| Checks | Windows tests/vet; Linux full race suite; staticcheck; patched-toolchain vulnerability scan; redacted gitleaks directory scan | Pass |

Write/edit replace regular files atomically and refuse symlink targets. Read
allows symlinks contained by `os.Root`. FIFO/device reads are refused; Unix opens
are nonblocking so FIFO replacement cannot hang before file-type validation.
