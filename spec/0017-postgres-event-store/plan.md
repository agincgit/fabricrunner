# Postgres event store implementation plan

**Specification:** [spec.md](spec.md)

**Status:** Draft

## Design

Four files: `store.go`, `migrate.go`, `store_test.go`, and
`migrations/0001_events.sql`. The driver stays inside `store/postgres`; the root
package exposes no Postgres-specific type, matching how `store/sqlite` is
bounded.

Append takes the aggregate version row `FOR UPDATE`, compares the expected
sequence, inserts the events, applies workload events to the prior projection,
persists it, and commits. Serialisation is therefore the database's job rather
than the process's, which is what makes two independent writers safe.

`BYTEA` over `JSONB` is the load-bearing schema decision and the one most likely
to be "corrected" in review. The digest is computed over payload bytes, so any
storage layer that normalises those bytes breaks verification. The migration
comment states this inline.

## Delivery sequence

1. Extract the existing in-memory and SQLite tests into a shared `storetest`
   suite, and prove both still pass against it.
2. Add the migration and `Migrate`, with idempotence tests.
3. Implement append and load with `FOR UPDATE` optimistic concurrency.
4. Add transactional projection persistence.
5. Run the conformance suite against Postgres.

## Sequencing note

The conformance suite comes first. Written afterwards it tends to encode
whatever the newest implementation happens to do; written first it is derived
from the two implementations already known to be correct.
