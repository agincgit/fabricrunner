# Tool suite acceptance

**Specification:** [spec.md](spec.md)

**Status:** Draft

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

Pending implementation.
