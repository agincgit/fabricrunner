# SQLite event store acceptance

**Specification:** [spec.md](spec.md)

**Status:** Accepted

The implementation is accepted when:

- a new database reports WAL journal mode, foreign keys enabled on every used
  connection, the configured busy timeout, and schema version 1;
- a database created on a POSIX filesystem is not readable or writable by group
  or other users;
- append and load pass the shared event-store contract suite;
- two handles racing with the same expected sequence produce one success and
  one `ErrVersionConflict`;
- an invalid workload transition leaves the event stream, aggregate version,
  and persisted projection unchanged;
- closing and reopening produces an identical event stream and workload
  projection;
- `afterSequence` returns only later events in ascending order;
- caller mutation of append or load results cannot change persisted bytes;
- cancellation is returned without a partial commit;
- a simulated future schema version is rejected without modification; and
- build, vet, static analysis, race tests, and leak scanning pass.

## Evidence

Local verification on 2026-09-07:

| Gate | Evidence | Result |
|---|---|---|
| Dependency integrity | `go mod verify` | Pass |
| Formatting | `test -z "$(gofmt -l .)"` | Pass |
| Build | `go build ./...` | Pass |
| Vet | `go vet ./...` | Pass |
| Static analysis | `go run honnef.co/go/tools/cmd/staticcheck@v0.6.1 ./...` | Pass |
| Race and repeatability | `go test ./... -race -count=20` | Pass |
| Vulnerability scan | `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` using Go 1.25.6 | Pass; zero reachable vulnerabilities |
| Published CI | [Build and secret-scanning run](https://github.com/agincgit/fabricrunner/actions/runs/34092848259) | Pass |
