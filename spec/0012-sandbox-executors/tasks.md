# Sandbox executors task ledger

**Specification:** [spec.md](spec.md)

**Plan:** [plan.md](plan.md)

**Acceptance:** [acceptance.md](acceptance.md)

- [x] Write failing acceptance tests before declaring any sandbox type.
- [x] Define `Sandbox` and `Executor` with a denying default (FR-BOX-001,
  FR-BOX-002).
- [x] Extend `ExecutionTarget` with sandbox capability and supersede the 0003
  note (FR-BOX-003).
- [ ] Add the Seatbelt executor (FR-BOX-004).
- [x] Add the bubblewrap executor (FR-BOX-005).
- [x] Deny on unsupported platforms with a named reason (FR-BOX-006).
- [ ] Add behavioral confinement tests (FR-BOX-007).
- [x] Record establishment, denial, and teardown (FR-BOX-008).
- [x] Reword `SECURITY.md` from planned to current.
- [ ] Capture acceptance evidence.
