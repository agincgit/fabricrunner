// Package sqlite provides the durable SQLite event store for Fabric Runner.
package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/agincgit/fabricrunner"
	_ "modernc.org/sqlite"
)

const currentSchemaVersion = 1

const schemaV1 = `
CREATE TABLE aggregate_versions (
    aggregate_type TEXT NOT NULL,
    aggregate_id   TEXT NOT NULL,
    version        INTEGER NOT NULL CHECK (version >= 0),
    PRIMARY KEY (aggregate_type, aggregate_id)
) WITHOUT ROWID;

CREATE TABLE events (
    aggregate_type TEXT NOT NULL,
    aggregate_id   TEXT NOT NULL,
    sequence       INTEGER NOT NULL CHECK (sequence > 0),
    event_id       TEXT NOT NULL UNIQUE,
    event_type     TEXT NOT NULL,
    schema_version INTEGER NOT NULL CHECK (schema_version > 0),
    causation_id   TEXT NOT NULL,
    correlation_id TEXT NOT NULL,
    attempt_id     TEXT NOT NULL,
    occurred_at_ns INTEGER NOT NULL,
    payload        BLOB NOT NULL,
    payload_sha256 TEXT NOT NULL,
    PRIMARY KEY (aggregate_type, aggregate_id, sequence),
    FOREIGN KEY (aggregate_type, aggregate_id)
        REFERENCES aggregate_versions (aggregate_type, aggregate_id)
) WITHOUT ROWID;

CREATE TABLE workload_projections (
    aggregate_type   TEXT NOT NULL,
    aggregate_id     TEXT NOT NULL,
    sequence         INTEGER NOT NULL CHECK (sequence > 0),
    document         BLOB NOT NULL,
    document_sha256  TEXT NOT NULL,
    PRIMARY KEY (aggregate_type, aggregate_id),
    FOREIGN KEY (aggregate_type, aggregate_id)
        REFERENCES aggregate_versions (aggregate_type, aggregate_id)
) WITHOUT ROWID;
`

var (
	// ErrFutureSchema means the database was created by a newer, incompatible build.
	ErrFutureSchema = errors.New("sqlite store schema is newer than this build")
	// ErrCorruptStore means persisted data failed an integrity or structural check.
	ErrCorruptStore = errors.New("sqlite store is corrupt")
)

// Options controls SQLite connection behavior. Zero values select safe
// defaults.
type Options struct {
	BusyTimeout        time.Duration
	MaxOpenConnections int
}

// Diagnostics reports effective settings from a live database connection.
type Diagnostics struct {
	JournalMode   string
	ForeignKeys   bool
	BusyTimeout   time.Duration
	SchemaVersion int
}

// Store is a durable, concurrency-safe EventStore backed by SQLite.
type Store struct {
	db         *sql.DB
	busyMillis int64
}

// Open opens or creates a file-backed store with safe defaults.
func Open(ctx context.Context, path string) (*Store, error) {
	return OpenWithOptions(ctx, path, Options{})
}

// OpenWithOptions opens or creates a file-backed store with explicit options.
func OpenWithOptions(ctx context.Context, path string, options Options) (*Store, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("SQLite path is required")
	}
	if options.BusyTimeout < 0 {
		return nil, errors.New("busy timeout cannot be negative")
	}
	if options.MaxOpenConnections < 0 {
		return nil, errors.New("maximum open connections cannot be negative")
	}
	if options.BusyTimeout == 0 {
		options.BusyTimeout = 5 * time.Second
	}
	if options.MaxOpenConnections == 0 {
		options.MaxOpenConnections = 4
	}
	busyMillis := options.BusyTimeout.Milliseconds()
	if busyMillis < 1 || busyMillis > math.MaxInt32 {
		return nil, errors.New("busy timeout must be between 1ms and 2147483647ms")
	}

	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve SQLite path: %w", err)
	}
	created, err := ensureDatabaseFile(absolutePath)
	if err != nil {
		return nil, err
	}
	existingVersion, err := inspectSchemaVersion(ctx, absolutePath)
	if err != nil {
		cleanupNewDatabase(absolutePath, created)
		return nil, err
	}
	if existingVersion > currentSchemaVersion {
		return nil, fmt.Errorf(
			"%w: database is %d, supported is %d",
			ErrFutureSchema,
			existingVersion,
			currentSchemaVersion,
		)
	}

	db, err := sql.Open("sqlite", sqliteDSN(absolutePath, busyMillis))
	if err != nil {
		cleanupNewDatabase(absolutePath, created)
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	db.SetMaxOpenConns(options.MaxOpenConnections)
	db.SetMaxIdleConns(options.MaxOpenConnections)

	store := &Store{db: db, busyMillis: busyMillis}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		cleanupNewDatabase(absolutePath, created)
		return nil, fmt.Errorf("connect to SQLite database: %w", err)
	}
	if err := store.initialize(ctx); err != nil {
		db.Close()
		cleanupNewDatabase(absolutePath, created)
		return nil, err
	}
	return store, nil
}

// Close releases database connections owned by the store.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Append atomically appends drafts at the expected aggregate sequence. For a
// workload stream, its persisted projection is updated in the same transaction.
func (s *Store) Append(
	ctx context.Context,
	aggregate fabricrunner.AggregateRef,
	expectedSequence uint64,
	drafts ...fabricrunner.EventDraft,
) ([]fabricrunner.Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.ready(); err != nil {
		return nil, err
	}
	if err := aggregate.Validate(); err != nil {
		return nil, err
	}
	for index, draft := range drafts {
		if err := draft.Validate(); err != nil {
			return nil, fmt.Errorf("draft %d: %w", index, err)
		}
	}
	if len(drafts) == 0 {
		return []fabricrunner.Event{}, nil
	}
	if expectedSequence > math.MaxInt64 || uint64(len(drafts)) > math.MaxInt64-expectedSequence {
		return nil, errors.New("event sequence exceeds SQLite integer range")
	}

	connection, err := s.db.Conn(ctx)
	if err != nil {
		return nil, mapContextError(ctx, fmt.Errorf("acquire SQLite append connection: %w", err))
	}
	defer connection.Close()
	restoreBusyTimeout, err := s.boundBusyTimeout(ctx, connection)
	if err != nil {
		return nil, err
	}
	defer restoreBusyTimeout()

	tx, err := connection.BeginTx(ctx, nil)
	if err != nil {
		return nil, mapContextError(ctx, fmt.Errorf("begin SQLite append: %w", err))
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO aggregate_versions (aggregate_type, aggregate_id, version)
		 VALUES (?, ?, 0)
		 ON CONFLICT (aggregate_type, aggregate_id) DO NOTHING`,
		aggregate.Type,
		aggregate.ID.String(),
	); err != nil {
		return nil, fmt.Errorf("ensure aggregate version: %w", err)
	}

	result, err := tx.ExecContext(
		ctx,
		`UPDATE aggregate_versions
		 SET version = version + ?
		 WHERE aggregate_type = ? AND aggregate_id = ? AND version = ?`,
		len(drafts),
		aggregate.Type,
		aggregate.ID.String(),
		expectedSequence,
	)
	if err != nil {
		return nil, fmt.Errorf("claim aggregate version: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("read version claim result: %w", err)
	}
	if rowsAffected != 1 {
		actual, err := aggregateVersion(ctx, tx, aggregate)
		if err != nil {
			return nil, err
		}
		return nil, &fabricrunner.VersionConflictError{
			Aggregate: aggregate,
			Expected:  expectedSequence,
			Actual:    actual,
		}
	}

	created := make([]fabricrunner.Event, 0, len(drafts))
	for index, draft := range drafts {
		event, err := fabricrunner.NewEvent(
			aggregate,
			expectedSequence+uint64(index)+1,
			draft,
			time.Now().UTC(),
		)
		if err != nil {
			return nil, fmt.Errorf("materialize draft %d: %w", index, err)
		}
		if err := insertEvent(ctx, tx, event); err != nil {
			return nil, err
		}
		created = append(created, event)
	}

	if aggregate.Type == fabricrunner.WorkloadAggregateType {
		if err := updateWorkloadProjection(ctx, tx, aggregate, expectedSequence, created); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit SQLite append: %w", err)
	}
	return cloneEvents(created), nil
}

// Load returns validated events after the supplied sequence in ascending order.
func (s *Store) Load(
	ctx context.Context,
	aggregate fabricrunner.AggregateRef,
	afterSequence uint64,
) ([]fabricrunner.Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.ready(); err != nil {
		return nil, err
	}
	if err := aggregate.Validate(); err != nil {
		return nil, err
	}
	if afterSequence > math.MaxInt64 {
		return []fabricrunner.Event{}, nil
	}

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT sequence, event_id, event_type, schema_version,
		        causation_id, correlation_id, attempt_id, occurred_at_ns,
		        payload, payload_sha256
		 FROM events
		 WHERE aggregate_type = ? AND aggregate_id = ? AND sequence > ?
		 ORDER BY sequence ASC`,
		aggregate.Type,
		aggregate.ID.String(),
		afterSequence,
	)
	if err != nil {
		return nil, fmt.Errorf("load SQLite events: %w", err)
	}
	defer rows.Close()

	events := make([]fabricrunner.Event, 0)
	expected := afterSequence + 1
	for rows.Next() {
		event, err := scanEvent(rows, aggregate)
		if err != nil {
			return nil, err
		}
		if event.Sequence != expected {
			return nil, fmt.Errorf(
				"%w: event sequence %d, want %d",
				ErrCorruptStore,
				event.Sequence,
				expected,
			)
		}
		expected++
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate SQLite events: %w", err)
	}
	return cloneEvents(events), nil
}

// LoadWorkloadProjection returns an isolated copy of the transactionally
// persisted workload projection.
func (s *Store) LoadWorkloadProjection(
	ctx context.Context,
	aggregate fabricrunner.AggregateRef,
) (*fabricrunner.WorkloadProjection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.ready(); err != nil {
		return nil, err
	}
	if err := aggregate.Validate(); err != nil {
		return nil, err
	}
	if aggregate.Type != fabricrunner.WorkloadAggregateType {
		return nil, fmt.Errorf("workload projection requires aggregate type %q", fabricrunner.WorkloadAggregateType)
	}

	var sequence int64
	var document []byte
	var digest string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT sequence, document, document_sha256
		 FROM workload_projections
		 WHERE aggregate_type = ? AND aggregate_id = ?`,
		aggregate.Type,
		aggregate.ID.String(),
	).Scan(&sequence, &document, &digest)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s %s", fabricrunner.ErrAggregateNotFound, aggregate.Type, aggregate.ID)
	}
	if err != nil {
		return nil, fmt.Errorf("load SQLite workload projection: %w", err)
	}
	if sequence < 1 || sha256Hex(document) != digest {
		return nil, fmt.Errorf("%w: workload projection integrity check failed", ErrCorruptStore)
	}

	var projection fabricrunner.WorkloadProjection
	if err := decodeStrict(document, &projection); err != nil {
		return nil, fmt.Errorf("%w: workload projection: %v", ErrCorruptStore, err)
	}
	if projection.Aggregate != aggregate || projection.Version != uint64(sequence) ||
		projection.Workload.ID != aggregate.ID {
		return nil, fmt.Errorf("%w: workload projection identity mismatch", ErrCorruptStore)
	}
	return cloneProjection(&projection), nil
}

// Diagnostics reads the effective settings of one pooled connection.
func (s *Store) Diagnostics(ctx context.Context) (Diagnostics, error) {
	if err := ctx.Err(); err != nil {
		return Diagnostics{}, err
	}
	if err := s.ready(); err != nil {
		return Diagnostics{}, err
	}
	connection, err := s.db.Conn(ctx)
	if err != nil {
		return Diagnostics{}, err
	}
	defer connection.Close()

	var result Diagnostics
	var foreignKeys int
	var busyMillis int64
	if err := connection.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&result.JournalMode); err != nil {
		return Diagnostics{}, err
	}
	if err := connection.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		return Diagnostics{}, err
	}
	if err := connection.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&busyMillis); err != nil {
		return Diagnostics{}, err
	}
	if err := connection.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&result.SchemaVersion); err != nil {
		return Diagnostics{}, err
	}
	result.JournalMode = strings.ToLower(result.JournalMode)
	result.ForeignKeys = foreignKeys == 1
	result.BusyTimeout = time.Duration(busyMillis) * time.Millisecond
	return result, nil
}

func (s *Store) initialize(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin SQLite schema initialization: %w", err)
	}
	defer tx.Rollback()

	var version int
	if err := tx.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read SQLite schema version: %w", err)
	}
	if version > currentSchemaVersion {
		return fmt.Errorf("%w: database is %d, supported is %d", ErrFutureSchema, version, currentSchemaVersion)
	}
	if version == 0 {
		if _, err := tx.ExecContext(ctx, schemaV1); err != nil {
			return fmt.Errorf("install SQLite schema version 1: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `PRAGMA user_version = 1`); err != nil {
			return fmt.Errorf("record SQLite schema version 1: %w", err)
		}
		version = 1
	}
	if version != currentSchemaVersion {
		return fmt.Errorf("unsupported SQLite schema version %d", version)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit SQLite schema initialization: %w", err)
	}

	diagnostics, err := s.Diagnostics(ctx)
	if err != nil {
		return fmt.Errorf("verify SQLite settings: %w", err)
	}
	if diagnostics.JournalMode != "wal" || !diagnostics.ForeignKeys ||
		diagnostics.SchemaVersion != currentSchemaVersion {
		return fmt.Errorf(
			"SQLite settings invalid: journal=%s foreign_keys=%t schema=%d",
			diagnostics.JournalMode,
			diagnostics.ForeignKeys,
			diagnostics.SchemaVersion,
		)
	}
	return nil
}

func (s *Store) ready() error {
	if s == nil || s.db == nil {
		return errors.New("SQLite store is nil")
	}
	return nil
}

func (s *Store) boundBusyTimeout(ctx context.Context, connection *sql.Conn) (func(), error) {
	operationMillis := s.busyMillis
	if deadline, exists := ctx.Deadline(); exists {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, context.DeadlineExceeded
		}
		deadlineMillis := int64((remaining + time.Millisecond - 1) / time.Millisecond)
		if deadlineMillis < operationMillis {
			operationMillis = deadlineMillis
		}
	}
	if operationMillis == s.busyMillis {
		return func() {}, nil
	}
	if _, err := connection.ExecContext(
		ctx,
		`PRAGMA busy_timeout = `+strconv.FormatInt(operationMillis, 10),
	); err != nil {
		return nil, mapContextError(ctx, fmt.Errorf("bound SQLite busy timeout: %w", err))
	}
	return func() {
		restoreContext, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = connection.ExecContext(
			restoreContext,
			`PRAGMA busy_timeout = `+strconv.FormatInt(s.busyMillis, 10),
		)
	}, nil
}

func mapContextError(ctx context.Context, err error) error {
	if contextError := ctx.Err(); contextError != nil {
		return contextError
	}
	if deadline, exists := ctx.Deadline(); exists && !time.Now().Before(deadline) {
		return context.DeadlineExceeded
	}
	return err
}

func sqliteDSN(path string, busyMillis int64) string {
	databaseURL := databaseURL(path)
	query := databaseURL.Query()
	query.Set("_busy_timeout", strconv.FormatInt(busyMillis, 10))
	query.Set("_defensive", "1")
	query.Set("_dqs", "0")
	query.Set("_foreign_keys", "on")
	query.Set("_journal_mode", "wal")
	query.Set("_synchronous", "normal")
	query.Set("_txlock", "immediate")
	databaseURL.RawQuery = query.Encode()
	return databaseURL.String()
}

func inspectSchemaVersion(ctx context.Context, path string) (int, error) {
	databaseURI := databaseURL(path)
	query := databaseURI.Query()
	query.Set("mode", "ro")
	query.Set("_query_only", "1")
	databaseURI.RawQuery = query.Encode()

	database, err := sql.Open("sqlite", databaseURI.String())
	if err != nil {
		return 0, fmt.Errorf("open SQLite schema probe: %w", err)
	}
	database.SetMaxOpenConns(1)
	defer database.Close()

	var version int
	if err := database.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return 0, fmt.Errorf("read SQLite schema version: %w", err)
	}
	return version, nil
}

func databaseURL(path string) *url.URL {
	uriPath := filepath.ToSlash(path)
	if len(uriPath) >= 3 && uriPath[1] == ':' && uriPath[2] == '/' {
		uriPath = "/" + uriPath
	}
	return &url.URL{Scheme: "file", Path: uriPath}
}

func ensureDatabaseFile(path string) (bool, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create SQLite database: %w", err)
	}
	if err := file.Close(); err != nil {
		return false, fmt.Errorf("close new SQLite database: %w", err)
	}
	return true, nil
}

func cleanupNewDatabase(path string, created bool) {
	if !created {
		return
	}
	_ = os.Remove(path)
	_ = os.Remove(path + "-shm")
	_ = os.Remove(path + "-wal")
}

func aggregateVersion(ctx context.Context, tx *sql.Tx, aggregate fabricrunner.AggregateRef) (uint64, error) {
	var version int64
	if err := tx.QueryRowContext(
		ctx,
		`SELECT version FROM aggregate_versions
		 WHERE aggregate_type = ? AND aggregate_id = ?`,
		aggregate.Type,
		aggregate.ID.String(),
	).Scan(&version); err != nil {
		return 0, fmt.Errorf("read aggregate version: %w", err)
	}
	if version < 0 {
		return 0, fmt.Errorf("%w: negative aggregate version", ErrCorruptStore)
	}
	return uint64(version), nil
}

func insertEvent(ctx context.Context, tx *sql.Tx, event fabricrunner.Event) error {
	_, err := tx.ExecContext(
		ctx,
		`INSERT INTO events (
		    aggregate_type, aggregate_id, sequence, event_id, event_type,
		    schema_version, causation_id, correlation_id, attempt_id,
		    occurred_at_ns, payload, payload_sha256
		 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.Aggregate.Type,
		event.Aggregate.ID.String(),
		event.Sequence,
		event.ID.String(),
		event.Type,
		event.SchemaVersion,
		event.CausationID.String(),
		event.CorrelationID.String(),
		event.AttemptID.String(),
		event.OccurredAt.UnixNano(),
		[]byte(event.Payload),
		event.PayloadSHA256,
	)
	if err != nil {
		return fmt.Errorf("insert event sequence %d: %w", event.Sequence, err)
	}
	return nil
}

func updateWorkloadProjection(
	ctx context.Context,
	tx *sql.Tx,
	aggregate fabricrunner.AggregateRef,
	expectedSequence uint64,
	events []fabricrunner.Event,
) error {
	projection, err := loadWorkloadProjectionTx(ctx, tx, aggregate, expectedSequence)
	if err != nil {
		return err
	}
	for _, event := range events {
		if err := projection.Apply(event); err != nil {
			return fmt.Errorf("update workload projection: %w", err)
		}
	}
	document, err := json.Marshal(projection)
	if err != nil {
		return fmt.Errorf("encode workload projection: %w", err)
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO workload_projections (
		    aggregate_type, aggregate_id, sequence, document, document_sha256
		 ) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (aggregate_type, aggregate_id) DO UPDATE SET
		    sequence = excluded.sequence,
		    document = excluded.document,
		    document_sha256 = excluded.document_sha256`,
		aggregate.Type,
		aggregate.ID.String(),
		projection.Version,
		document,
		sha256Hex(document),
	); err != nil {
		return fmt.Errorf("persist workload projection: %w", err)
	}
	return nil
}

func loadWorkloadProjectionTx(
	ctx context.Context,
	tx *sql.Tx,
	aggregate fabricrunner.AggregateRef,
	expectedSequence uint64,
) (*fabricrunner.WorkloadProjection, error) {
	var sequence int64
	var document []byte
	var digest string
	err := tx.QueryRowContext(
		ctx,
		`SELECT sequence, document, document_sha256
		 FROM workload_projections
		 WHERE aggregate_type = ? AND aggregate_id = ?`,
		aggregate.Type,
		aggregate.ID.String(),
	).Scan(&sequence, &document, &digest)
	if errors.Is(err, sql.ErrNoRows) {
		if expectedSequence != 0 {
			return nil, fmt.Errorf("%w: workload projection missing at sequence %d", ErrCorruptStore, expectedSequence)
		}
		return fabricrunner.NewWorkloadProjection(aggregate)
	}
	if err != nil {
		return nil, fmt.Errorf("load workload projection for update: %w", err)
	}
	if sequence < 1 || uint64(sequence) != expectedSequence || sha256Hex(document) != digest {
		return nil, fmt.Errorf("%w: workload projection version or digest mismatch", ErrCorruptStore)
	}
	var projection fabricrunner.WorkloadProjection
	if err := decodeStrict(document, &projection); err != nil {
		return nil, fmt.Errorf("%w: workload projection: %v", ErrCorruptStore, err)
	}
	if projection.Aggregate != aggregate || projection.Version != expectedSequence {
		return nil, fmt.Errorf("%w: workload projection identity mismatch", ErrCorruptStore)
	}
	return &projection, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanEvent(row rowScanner, aggregate fabricrunner.AggregateRef) (fabricrunner.Event, error) {
	var sequence int64
	var schemaVersion int64
	var occurredAtNS int64
	var eventID, eventType string
	var causationID, correlationID, attemptID string
	var payload []byte
	var payloadHash string
	if err := row.Scan(
		&sequence,
		&eventID,
		&eventType,
		&schemaVersion,
		&causationID,
		&correlationID,
		&attemptID,
		&occurredAtNS,
		&payload,
		&payloadHash,
	); err != nil {
		return fabricrunner.Event{}, fmt.Errorf("scan SQLite event: %w", err)
	}
	if sequence < 1 || schemaVersion < 1 || schemaVersion > math.MaxUint32 {
		return fabricrunner.Event{}, fmt.Errorf("%w: invalid event sequence or schema version", ErrCorruptStore)
	}

	parsedEventID, err := fabricrunner.ParseID(eventID)
	if err != nil {
		return fabricrunner.Event{}, fmt.Errorf("%w: event ID: %v", ErrCorruptStore, err)
	}
	parsedCausationID, err := parseOptionalID(causationID)
	if err != nil {
		return fabricrunner.Event{}, fmt.Errorf("%w: causation ID: %v", ErrCorruptStore, err)
	}
	parsedCorrelationID, err := parseOptionalID(correlationID)
	if err != nil {
		return fabricrunner.Event{}, fmt.Errorf("%w: correlation ID: %v", ErrCorruptStore, err)
	}
	parsedAttemptID, err := parseOptionalID(attemptID)
	if err != nil {
		return fabricrunner.Event{}, fmt.Errorf("%w: attempt ID: %v", ErrCorruptStore, err)
	}

	event := fabricrunner.Event{
		ID:            parsedEventID,
		Aggregate:     aggregate,
		Sequence:      uint64(sequence),
		Type:          eventType,
		SchemaVersion: uint32(schemaVersion),
		CausationID:   parsedCausationID,
		CorrelationID: parsedCorrelationID,
		AttemptID:     parsedAttemptID,
		OccurredAt:    time.Unix(0, occurredAtNS).UTC(),
		Payload:       append(json.RawMessage(nil), payload...),
		PayloadSHA256: payloadHash,
	}
	if err := event.Validate(); err != nil {
		return fabricrunner.Event{}, fmt.Errorf("%w: event validation: %v", ErrCorruptStore, err)
	}
	return event, nil
}

func parseOptionalID(value string) (fabricrunner.ID, error) {
	if value == "" {
		return "", nil
	}
	return fabricrunner.ParseID(value)
}

func decodeStrict(document []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func cloneEvents(events []fabricrunner.Event) []fabricrunner.Event {
	cloned := make([]fabricrunner.Event, len(events))
	copy(cloned, events)
	for index := range cloned {
		cloned[index].Payload = append(json.RawMessage(nil), events[index].Payload...)
	}
	return cloned
}

func cloneProjection(source *fabricrunner.WorkloadProjection) *fabricrunner.WorkloadProjection {
	clone := *source
	clone.Steps = make(map[fabricrunner.ID]fabricrunner.Step, len(source.Steps))
	for id, step := range source.Steps {
		step.Requirements.Modalities = append([]string(nil), step.Requirements.Modalities...)
		step.Requirements.RequiredTools = append([]string(nil), step.Requirements.RequiredTools...)
		step.Requirements.AllowedZones = append([]fabricrunner.Zone(nil), step.Requirements.AllowedZones...)
		clone.Steps[id] = step
	}
	return &clone
}

var _ fabricrunner.EventStore = (*Store)(nil)
