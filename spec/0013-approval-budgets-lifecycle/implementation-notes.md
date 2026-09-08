# Implementation decisions

The engine requires a supplied `SpendEstimator` returning a conservative upper
bound for the exact model request and selected descriptor. Missing or invalid
bounds fail closed before provider I/O. There is no built-in pricing model or
implicit assumption that a missing price means free. The requested output cap
must fit the bound. Reservations are durably recorded before turn-start and
before the external call. Bounds remain charged conservatively, including
uncertain calls; releasing unused reservations is a later optimization.

Approval requests contain the policy scope and content manifest, never raw
prompts, arguments or model output. Secret-bearing requests cannot be approved
for model use. A configured approver gates the initial step and resolves
`require_approval` verdicts; no approver leaves an ordinary allowed step alone
but cannot override a policy-required approval. Decisions precede effects in
the durable stream. Replay remains read-only.

The single-node engine has one root step. `MaxChildSteps` bounds child creation;
child creation remains unsupported here and cannot spend that budget. A new
`MaxSteps` field bounds all steps, including the root, and must be positive for
engine runs. This makes total-step exhaustion observable without inventing a
child step.

Cleanup of tools is still owned by the loop after handoff; the engine owns
cleanup on approval or admission failure before handoff. Sandbox teardown and
cleanup failure records use a bounded context detached from caller cancellation.
Collaborator implementations must honor cancellation; arbitrary in-process Go
code cannot be forcibly terminated. OS tool processes receive forced teardown.
