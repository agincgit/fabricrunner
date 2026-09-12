# Postgres event store specification

**ID:** 0017

**Status:** Draft

**Depends on:** [Fabric Runner core](../0001-fabric-runner-core/spec.md),
[SQLite event store](../0002-sqlite-event-store/spec.md)

Supporting execution records: [plan](plan.md), [tasks](tasks.md), and
[acceptance](acceptance.md).

## Purpose

The module ships an in-memory `EventStore` and a SQLite one. Both are correct,
and neither survives a restart on an ephemeral disk. That is the wrong property
for a store whose purpose is answering "why did it do that" a month later.

This specification adds a Postgres implementation, and a shared conformance
suite so the three stores stop carrying three copies of the same tests.

## Requirements

- **FR-PG-001:** `store/postgres` implements `EventStore` with the same
  validation, optimistic sequencing, ordering, and copy-isolation behavior as
  the existing implementations.
- **FR-PG-002:** Schema is applied by an explicit `Migrate` step, never by the
  serving process. A process that appends to the table cannot also reshape it.
- **FR-PG-003:** `Migrate` is idempotent. Re-running it against a current
  database makes no change.
- **FR-PG-004:** Event payloads are stored as `BYTEA`, not `JSONB`. Every event
  carries a SHA-256 of its payload bytes; `JSONB` reorders keys and drops
  insignificant whitespace, so a payload stored that way returns different bytes
  and its recorded digest stops verifying. The migration carries a comment
  stating this, because the choice looks wrong to a reader who does not know it.
- **FR-PG-005:** Optional references — causation, correlation, and attempt — are
  absent as the zero ID, never as a parse failure. Treating an absent optional
  reference as corruption makes every workload that omits one unreplayable while
  appearing to be data loss.
- **FR-PG-006:** Concurrent writers on one aggregate serialise. The aggregate
  version row is locked for update, and a stale expected sequence returns
  `ErrVersionConflict` having written nothing.
- **FR-PG-007:** Tests requiring a live database are gated on
  `FABRICRUNNER_TEST_POSTGRES` and skip without it, so CI passes with no
  database available.
- **FR-PG-008:** A shared `storetest` conformance suite is published, and the
  in-memory, SQLite, and Postgres stores each run it. A third-party store can
  run the same suite.

## Non-goals

- No connection pooling policy, retention policy, or partitioning scheme.
- No migration framework beyond ordered forward-only steps.

## Constraints

- Adds `github.com/jackc/pgx/v5` (stdlib driver) to `go.mod`. The root package
  gains no new dependency.
