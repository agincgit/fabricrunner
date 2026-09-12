# Postgres event store task ledger

**Specification:** [spec.md](spec.md)

**Plan:** [plan.md](plan.md)

**Acceptance:** [acceptance.md](acceptance.md)

- [ ] Write failing acceptance tests before declaring any Postgres type.
- [ ] Extract the shared `storetest` conformance suite and run the existing two
  stores against it (FR-PG-008).
- [ ] Add the `BYTEA` migration with its rationale comment (FR-PG-004).
- [ ] Implement idempotent `Migrate`, separate from the serving path
  (FR-PG-002, FR-PG-003).
- [ ] Implement append and load with `FOR UPDATE` concurrency (FR-PG-001,
  FR-PG-006).
- [ ] Treat absent optional references as the zero ID (FR-PG-005).
- [ ] Gate live-database tests on `FABRICRUNNER_TEST_POSTGRES` (FR-PG-007).
- [ ] Add `github.com/jackc/pgx/v5` to `go.mod`.
- [ ] Capture acceptance evidence.
