# Tool suite specification

**ID:** 0011

**Status:** Draft

**Depends on:** [Execution engine](../0010-execution-engine/spec.md),
[Sandbox executors](../0012-sandbox-executors/spec.md)

Supporting execution records: [plan](plan.md), [tasks](tasks.md), and
[acceptance](acceptance.md).

## Purpose

The turn loop can execute tool bindings but the module ships none. Without
tools there is nothing for a model turn to do beyond produce text, so the
Phase 1 end-to-end scenario cannot be exercised.

This specification defines the four baseline tools: read, write, edit, and
command.

## Requirements

- **FR-TOOL-001:** Each tool is a `ToolHandler` with a declared schema, and is
  usable without any application-specific knowledge.
- **FR-TOOL-002:** Every filesystem tool operates within an explicit root. A
  path resolving outside the root is refused, including through symlinks,
  `..` traversal, and absolute paths.
- **FR-TOOL-003:** Side-effecting tools declare themselves as such. A tool that
  writes, edits, or executes is never treated as read-only.
- **FR-TOOL-004:** Every side-effecting tool requires an established sandbox
  per 0012 and fails closed when one cannot be established.
- **FR-TOOL-005:** Tool output carries a classification. Output of unknown
  provenance is classified confidential rather than public.
- **FR-TOOL-006:** Tool results are size-bounded. Output exceeding the bound is
  truncated with the truncation recorded, never silently dropped.
- **FR-TOOL-007:** The command tool takes an argument vector, never a shell
  string. No tool constructs a shell invocation from model output.
- **FR-TOOL-008:** Cancellation propagates to a running tool, and a cancelled
  tool's partial effects are recorded.

## Non-goals

- No network tool. Egress is a policy concern and is not introduced as a tool.
- No package-manager, version-control, or language-specific tooling.

## Constraints

- Tools depend on the sandbox contract from 0012 and do not define their own.
