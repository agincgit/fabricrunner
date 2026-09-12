# Postgres event store acceptance

**Specification:** [spec.md](spec.md)

**Status:** Draft

Each criterion below is the name of a test to be written before the
implementation exists.

The implementation is accepted when:

- `TestAppendAtExpectedSequenceSucceeds` — an append at the expected sequence
  commits;
- `TestStaleSequenceConflictsAndWritesNothing` — a stale expected sequence
  returns `ErrVersionConflict` and leaves the stream unchanged;
- `TestConcurrentWritersSerialise` — two writers racing on one aggregate produce
  one success and one conflict;
- `TestPayloadRoundTripsByteForByte` — deliberately unnormalised JSON returns
  exactly as written and its digest verifies;
- `TestAbsentOptionalReferenceIsZeroID` — an event with no causation,
  correlation, or attempt reference loads without error;
- `TestReplayReconstructsWorkloadProjection` — `Replay` rebuilds the projection
  through the library's own path;
- `TestMigrateIsIdempotent` — re-running `Migrate` makes no change;
- `TestServingPathCannotMigrate` — the serving path exposes no schema-altering
  call;
- `TestAllStoresPassConformanceSuite` — in-memory, SQLite, and Postgres each
  pass `storetest`;
- `TestSkipsWithoutDatabaseGate` — the suite skips cleanly with
  `FABRICRUNNER_TEST_POSTGRES` unset; and
- build, vet, static analysis, race tests, and leak scanning pass.

## Evidence

Pending implementation.
