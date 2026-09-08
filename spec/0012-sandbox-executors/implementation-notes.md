# Implementation decisions

The local Phase 0–2 implementation run begins from `06fb121`. All development
commits remain local until Phase 2 acceptance; publication will be one squashed
commit. The v1.0 license review remains a v1 release prerequisite, not a claim
that can be completed by implementing Phase 0.

0011 and 0012 are delivered together. Filesystem operations use Go `os.Root`
for traversal-resistant access (including races), in addition to rejecting
absolute input paths. Writes and edits execute in a dedicated helper inside
the OS sandbox, rather than checking a capability flag and writing in the
unconfined coordinator. The helper is an explicitly configured, trusted binary;
model output cannot select it. Commands use an argument vector and no shell
wrapper. Linux programs may invoke subprocesses under the PID namespace; the
initial macOS profile denies child creation pending native acceptance.

Linux uses a fresh bubblewrap PID, network, user, IPC, UTS and mount namespace
per execution, a read-only system runtime, empty environment, a writable
workspace and ephemeral `/tmp`. Cancellation kills the process group and the
namespace init; session changes cannot survive namespace teardown. macOS uses
Seatbelt with deny-default policy and process-group cleanup. Platform-specific
behavioral evidence is required; cross compilation is not macOS acceptance.

The engine receives sandbox lifecycle records through the tool invocation
context. Recording failure denies establishment/execution. Observer delivery
is best effort as in 0009. An established capability describes a successful
probe; every subsequent operation still launches through confinement.
