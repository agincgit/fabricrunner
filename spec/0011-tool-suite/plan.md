# Tool suite implementation plan

**Specification:** [spec.md](spec.md)

**Status:** Implemented

## Design

Each tool lives in `tool/` as a separate handler with an explicit root and an
injected executor. Path resolution is a single shared function used by every
filesystem tool: resolve, then verify containment against the root after
symlink evaluation. Containment is checked on the resolved path, never on the
input string, because string-level checks are defeated by symlinks.

The command tool never sees a shell. It takes an argument vector and hands it
to the 0012 executor, which is the only component that can start a process.

## Delivery sequence

1. Add the shared root-containment resolver and its adversarial tests.
2. Add read, then write, then edit, each with containment and classification
   tests.
3. Add the command tool against the 0012 executor contract.
4. Add cancellation and truncation behavior across all four.

## Sequencing note

Ordered after 0010 so the tools bind to settled engine shapes, and paired with
0012 because FR-TOOL-004 cannot be satisfied without the sandbox contract.

## Local execution design

See [0012 implementation decisions](../0012-sandbox-executors/implementation-notes.md).
Read uses traversal-resistant `os.Root`; writes and edits use the same rooted
operations inside the trusted `fabricrunner-tool` helper under confinement.
Commands pass argument vectors unchanged. Outputs are bounded and carry an
explicit truncation flag and conservative partial-effect metadata. The engine
records sandbox lifecycle and completed/failed execution metadata durably,
including after cancellation using a bounded finalization context.
