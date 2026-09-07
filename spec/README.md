# Specifications

This directory is the authoritative source for Fabric Runner behavior and
delivery. Files under `docs/` explain the system; they do not replace an
accepted specification.

## Workflow

Every material feature starts in a numbered directory:

```text
spec/NNNN-short-name/
  spec.md
  plan.md
  tasks.md
  acceptance.md
```

- `spec.md` defines requirements, constraints, invariants, and non-goals.
- `plan.md` records the design, delivery sequence, and technical decisions.
- `tasks.md` is the traceable execution ledger.
- `acceptance.md` defines observable completion criteria and verification.

Specification status is one of `Draft`, `Accepted`, `In Progress`,
`Implemented`, or `Superseded`. A superseded specification links to its
replacement.

## Change rules

1. Record a behavior change in the relevant specification before changing the
   implementation.
2. Give each new requirement a stable identifier and reference it from its
   tasks and acceptance checks.
3. Update the plan when implementation constraints or sequencing change.
4. Mark tasks complete only when their acceptance evidence exists.
5. Keep historical decisions; supersede them instead of silently rewriting
   implemented behavior.

## Active specifications

| ID | Title | Status |
|---|---|---|
| [0001](0001-fabric-runner-core/spec.md) | Fabric Runner core | In Progress |
| [0002](0002-sqlite-event-store/spec.md) | SQLite event store | Implemented |
| [0003](0003-policy-contracts/spec.md) | Policy contracts | Implemented |
| [0004](0004-deterministic-routing/spec.md) | Deterministic routing | Implemented |
| [0005](0005-turn-tool-loop/spec.md) | Provider-neutral turn/tool loop | Implemented |
| [0006](0006-provider-conformance/spec.md) | Provider adapter conformance | Implemented |
| [0007](0007-openai-compatible-adapter/spec.md) | OpenAI-compatible adapter | Implemented |
| [0008](0008-anthropic-messages-adapter/spec.md) | Anthropic Messages adapter | Implemented |
