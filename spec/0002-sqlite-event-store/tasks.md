# SQLite event store task ledger

**Specification:** [spec.md](spec.md)

**Plan:** [plan.md](plan.md)

**Acceptance:** [acceptance.md](acceptance.md)

- [x] Add the pure-Go SQLite dependency.
- [x] Initialize schema version 1, WAL, foreign keys, busy timeout, and secure
  file permissions (FR-SQL-001, FR-SQL-008, FR-SQL-010).
- [x] Implement transactional optimistic append (FR-SQL-002 through
  FR-SQL-004).
- [x] Implement ordered isolated loads (FR-SQL-002, FR-SQL-005,
  FR-SQL-006).
- [x] Persist workload projections in the append transaction (FR-SQL-003 and
  FR-SQL-009).
- [x] Add restart, concurrency, rollback, corruption, and cancellation tests.
- [ ] Capture local and published acceptance evidence.
