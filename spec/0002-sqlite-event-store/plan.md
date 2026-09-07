# SQLite event store implementation plan

**Specification:** [spec.md](spec.md)

**Status:** In Progress

## Design

Use Go's `database/sql` API with the pure-Go `modernc.org/sqlite` driver. Keep
the driver inside `store/sqlite`; the root package exposes no SQLite-specific
types.

The append transaction performs this sequence:

1. validate the aggregate and every draft;
2. create the aggregate-version row if needed;
3. compare-and-swap the expected aggregate version;
4. materialize and insert immutable events;
5. apply workload events to the prior projection;
6. persist the updated projection at the new version; and
7. commit.

Any error rolls back the complete transaction.

## Delivery sequence

1. Add schema initialization and compatibility checks.
2. Implement append and load with optimistic concurrency.
3. Add transactional workload projection persistence and loading.
4. Add restart, cross-handle race, rollback, ordering, and cancellation tests.
5. Run formatting, build, vet, static analysis, race tests, and CI.

## Schema evolution

Schema version 1 is installed in one transaction and recorded with
`PRAGMA user_version`. Future versions add ordered migration functions. A
database with a greater version is rejected without modification.
