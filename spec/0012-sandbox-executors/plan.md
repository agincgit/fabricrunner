# Sandbox executors implementation plan

## Ubuntu CI host prerequisite

Ubuntu 24.04 restricts capabilities in unprivileged user namespaces unless the
application has an appropriate AppArmor profile. The CI host installs a profile
scoped to `/usr/bin/bwrap` that permits its user namespaces. Global restrictions
remain enabled. The runtime still creates isolated mount, PID and network
namespaces, and the behavioral confinement tests remain mandatory.

This follows [Ubuntu's per-application user namespace guidance](https://ubuntu.com/blog/ubuntu-23-10-restricted-unprivileged-user-namespaces).
The first published run of PR #15 failed closed during namespace setup;
this host prerequisite addresses that failure without skipping sandbox tests.

**Specification:** [spec.md](spec.md)

**Status:** In Progress

## Design

`Sandbox` describes an established confinement; `Executor` establishes one and
runs an argument vector inside it. The default executor is the denying one, so
a caller who configures nothing gets refusal rather than an unconfined process.
This is the structural expression of FR-BOX-002: the permissive path requires
an explicit choice.

Verification is behavioral. Tests assert that a confined process cannot write
outside its root, cannot open a network connection, and cannot escape its
process group — by attempting each and requiring failure. Asserting on
generated command-line flags would pass while confining nothing.

## Delivery sequence

1. Define `Sandbox` and `Executor` with the denying default.
2. Extend `ExecutionTarget` with sandbox capability and supersede the note in
   0003.
3. Add the Seatbelt executor, gated to darwin.
4. Add the bubblewrap executor, gated to linux.
5. Add behavioral confinement tests per platform, skipped with an explicit
   reason where the platform is unavailable.
6. Reword `SECURITY.md` from planned to current once evidence exists.

## Sequencing note

The `SECURITY.md` wording fix is separated from this work and ships first, as
part of Phase 1a. The documentation is wrong today regardless of when the
executors land, and correcting it should not wait on them.

## Local implementation evidence (2026-09-08 UTC)

The tools and Linux executor are implemented locally on `codex/phase-0-through-2`.
`go test ./...` passes on Windows. `GOTOOLCHAIN=go1.25.13 go test ./tool
./sandbox -race -count=1` passes under WSL Ubuntu, including actual confined
write/edit/read, literal argument-vector execution, cancellation effect recording,
blocked outside writes, denied network connections, and session-escaping child
termination. No macOS runtime evidence is available yet; 0012 and the Phase 1
acceptance gate remain open. This is not Phase 2 completion evidence.

Public additions: `ToolDefinition.SideEffecting`, `ExecutionTarget.Sandbox`,
`SandboxedTool`, and the `sandbox` execution-record payload. Existing execution
policy scope equality now binds sandbox capability. See
[implementation decisions](implementation-notes.md).
