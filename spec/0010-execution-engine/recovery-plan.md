# Phase 1 restart continuation

- **FR-ENGINE-011:** `Engine.Resume` continues an interrupted running workload
  from a durable turn checkpoint, preserving IDs, usage, reservations, approval
  outcomes, compacted history, and the original wall-time deadline. Its request
  must match the digest recorded on creation. `Replay` remains read-only.
- **FR-ENGINE-012:** A checkpoint is committed only after all tool results and
  the ordered tool-result message are committed. An interruption after a newer
  effect intent cannot reuse an older checkpoint; it returns an explicit
  uncertain-outcome error and performs no external effect.
- **FR-ENGINE-013:** A resumer claims the recorded aggregate version before any
  new effect. Concurrent claimants cannot both progress from the same version.
  An active original runner loses its next append before another external call.

This adds `LoopRequest.Continuation`, `LoopEvent.Checkpoint`, and
`LoopEventCheckpoint`. The low-level loop starts at the next turn and restores
the loop sequence, counts, usage and messages. It does not simulate old calls.
Initially only committed turn boundaries are resumable. Mid-call recovery
requires provider/worker reconciliation (Phase 2), not automatic call replay.

Acceptance: close and reopen SQLite after injecting a storage failure directly
after a checkpoint, resume with the same request, and assert the committed tool
effect occurs exactly once. Resume after an unacknowledged call intent must make
zero calls. Request changes and stale/concurrent claims must be refused.
