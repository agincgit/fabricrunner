# SQLite event store specification

**ID:** 0002

**Status:** Implemented

**Depends on:** [Fabric Runner core](../0001-fabric-runner-core/spec.md)

Supporting execution records: [plan](plan.md), [tasks](tasks.md), and
[acceptance](acceptance.md).

## Purpose

Fabric Runner needs a durable local store before it can safely execute model
turns or tools. SQLite is the first persistent implementation of the public
event-store contract and the source of truth for single-node execution.

## Requirements

- **FR-SQL-001:** Opening a file-backed store initializes a versioned schema,
  enables WAL mode, enables foreign-key enforcement, and configures a bounded
  busy timeout.
- **FR-SQL-002:** The store implements `EventStore` with the same validation,
  optimistic sequence, ordering, and copy-isolation behavior as the in-memory
  implementation.
- **FR-SQL-003:** Appending workload events and updating the corresponding
  workload projection occur in one database transaction. Either both become
  visible or neither does.
- **FR-SQL-004:** Concurrent writers, including writers using separate database
  handles, cannot commit the same aggregate sequence. A stale writer receives
  `ErrVersionConflict` with the observed version.
- **FR-SQL-005:** Closing and reopening the database preserves events,
  projections, causal identifiers, occurrence times, payload bytes, and hashes.
- **FR-SQL-006:** Loads return validated events in ascending sequence order and
  honor `afterSequence` without exposing mutable database-owned buffers.
- **FR-SQL-007:** Cancellation and deadlines propagate through open, append,
  load, projection load, and schema initialization.
- **FR-SQL-008:** A store refuses a schema version newer than the implementation
  understands. Migrations are transactional and forward-only.
- **FR-SQL-009:** Rebuilding or loading a projection never contacts a provider,
  executes a tool, or performs application-defined callbacks.
- **FR-SQL-010:** A newly created database file uses owner-only permissions on
  platforms that support POSIX file modes.
- **FR-SQL-011:** Absolute Windows drive paths are encoded as local file URIs,
  not URI authorities, so file-backed storage opens on Windows as well as Unix.

## Storage model

The first schema contains:

- an aggregate-version row used for compare-and-swap appends;
- immutable event rows keyed by aggregate and sequence; and
- a rebuildable workload-projection document keyed by aggregate.

Event payloads remain JSON bytes and retain their SHA-256 digest. Timestamps are
stored as UTC Unix nanoseconds. Empty optional causal identifiers are stored as
empty text and reconstructed as zero IDs.

## Failure behavior

- Invalid aggregates or drafts fail before a transaction begins.
- Projection rejection rolls back the version claim and all inserted events.
- Database lock exhaustion returns a storage error and never masquerades as a
  successful append.
- Version conflicts remain distinguishable with `errors.Is` and
  `errors.As`.
- Corrupt stored events or projections fail closed.

## Non-goals

- Database encryption and key management
- Multi-node database replication
- Backup scheduling or retention policy
- Provider, tool, routing, or worker execution
