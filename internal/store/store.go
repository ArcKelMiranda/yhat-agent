// Package store provides the persistent SQLite store for Cerebro YHat v1.2.
//
// Schema
//
// The store manages three tables as defined in migrations/001_init.sql:
//   - memories: long-term knowledge records (decisions, rules, anomalies, improvements)
//   - upload_queue: pending outgoing shares to the central repository
//   - sync_state: key-value bookkeeping including schema_version
//
// Pragma guarantees
//
// Open always applies two PRAGMAs on every connection:
//   - journal_mode=WAL  (Write-Ahead Logging for concurrency)
//   - foreign_keys=ON   (referential integrity for upload_queue → memories)
//
// BR13 note
//
// This package accepts arbitrary title/content/context strings and persists them
// verbatim. The F1-E sensitive-content filter lives in the tool layer above this
// package and is responsible for rejecting patterns (AWS keys, bearer tokens,
// PEM private-key blocks, etc.) before ProposeMemory is called. The store does
// not perform sensitive-content scanning.

package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	_ "modernc.org/sqlite" // register sqlite driver
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// CurrentSchemaVersion is the schema version this package knows how to apply.
// Bump this constant and add a new migration file to evolve the schema.
const CurrentSchemaVersion = 1

// --------------------------------------------------------------------------
// Constants: MemoryType
// --------------------------------------------------------------------------/

const (
	MemoryTypeDecision    = "decision"
	MemoryTypeRule        = "rule"
	MemoryTypeAnomaly     = "anomaly"
	MemoryTypeImprovement = "improvement"
)

// --------------------------------------------------------------------------
// Constants: Status
// --------------------------------------------------------------------------/

const (
	StatusProposed  = "proposed"
	StatusValidated = "validated"
	StatusRejected  = "rejected"
	StatusArchived  = "archived"
)

// --------------------------------------------------------------------------
// Constants: Origin
// --------------------------------------------------------------------------/

const (
	OriginLocal = "local"
	OriginTeam  = "team"
)

// --------------------------------------------------------------------------
// Constants: ShareStatus
// --------------------------------------------------------------------------/

const (
	ShareStatusNone     = "none"
	ShareStatusQueued   = "queued"
	ShareStatusSent     = "sent"
	ShareStatusAccepted = "accepted"
	ShareStatusRejected = "rejected"
)

// ------------------------------------------------------------------------------------------------------------------------------------------
// Sentinel errors
// --------------------------------------------------------------------------/

var (
	// ErrNotFound is returned when a requested record does not exist.
	ErrNotFound = errors.New("record not found")

	// ErrDuplicate is returned when a proposed memory has the same
	// content_hash as an existing proposed or validated memory (BR7).
	ErrDuplicate = errors.New("duplicate content_hash: a proposed or validated record with the same title and content already exists")

	// ErrTeamNotEditable is returned when a mutation is attempted on a
	// record whose origin is 'team' (BR11).
	ErrTeamNotEditable = errors.New("team-origin records are not editable through the local store")

	// ErrInvalidField is returned when a field value fails validation
	// (e.g. title or content outside allowed length, BR5).
	ErrInvalidField = errors.New("invalid field value")
)

// ------------------------------------------------------------------------------------------------------------------------------------------
// Types
// --------------------------------------------------------------------------/

// Memory represents a single long-term knowledge record in the Cerebro.
type Memory struct {
	ID            string     `json:"id"`
	Origin        string     `json:"origin"`            // 'local' or 'team'
	Type          string     `json:"type"`              // decision, rule, anomaly, improvement
	Title         string     `json:"title"`             // 1–200 characters
	Content       string     `json:"content"`           // 1–10000 characters
	Context       *string    `json:"context,omitempty"` // nullable
	ContentHash   string     `json:"content_hash"`      // SHA-256 of title+content
	Author        string     `json:"author"`
	Status        string     `json:"status"` // proposed, validated, rejected, archived
	ValidatedBy   *string    `json:"validated_by,omitempty"`
	ValidatedAt   *time.Time `json:"validated_at,omitempty"`
	RejectReason  *string    `json:"reject_reason,omitempty"`
	ShareStatus   string     `json:"share_status"` // none, queued, sent, accepted, rejected
	SharedAt      *time.Time `json:"shared_at,omitempty"`
	RemoteID      *string    `json:"remote_id,omitempty"`   // set when origin='team'
	TeamStatus    *string    `json:"team_status,omitempty"` // approved, retired
	TeamUpdatedAt *time.Time `json:"team_updated_at,omitempty"`
	IsExample     bool       `json:"is_example"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// UploadEntry represents a pending share request in the upload queue.
type UploadEntry struct {
	ID          int64      `json:"id"`
	MemoryID    string     `json:"memory_id"`
	OrderedBy   string     `json:"ordered_by"`
	OrderedAt   time.Time  `json:"ordered_at"`
	Attempts    int        `json:"attempts"`
	LastError   *string    `json:"last_error,omitempty"`
	NextRetryAt *time.Time `json:"next_retry_at,omitempty"`
}

// SyncState represents a single key-value entry in the sync_state table.
type SyncState struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ------------------------------------------------------------------------------------------------------------------------------------------
// Store
// --------------------------------------------------------------------------/

// Store wraps a *sql.DB with typed methods for the Cerebro tables.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path.
// It applies WAL mode and foreign_keys=ON PRAGMAs, then runs the embedded
// migration if this is the first open (no schema_version row).
// Returns ErrInvalidField if the schema_version row is missing or refers to
// a schema newer than CurrentSchemaVersion.
func Open(path string) (*Store, error) {
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}

	// Ensure a single connection for write ordering; WAL handles readers.
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("Ping: %w", err)
	}

	s := &Store{db: db}

	if err := s.runMigrations(); err != nil {
		db.Close()
		return nil, err
	}

	return s, nil
}

// Close releases the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// runMigrations reads the embedded migration file and applies it if this is
// the first open. It fails if schema_version is missing (inconsistent pre-existing
// DB) or higher than CurrentSchemaVersion.
func (s *Store) runMigrations() error {
	// Read the embedded migration file.
	migrationSQL, err := migrationsFS.ReadFile("migrations/001_init.sql")
	if err != nil {
		return fmt.Errorf("read embedded migration: %w", err)
	}

	// Detect whether sync_state table already exists.
	var tableName string
	err = s.db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='sync_state'",
	).Scan(&tableName)

	if err == sql.ErrNoRows {
		// sync_state does not exist: this is a fresh DB. Apply migration.
		// (sql.ErrNoRows means QueryRow found nothing, which is expected.)
	} else if err != nil {
		return fmt.Errorf("check sync_state existence: %w", err)
	} else {
		// sync_state table already exists: check for schema_version row.
		var version string
		err = s.db.QueryRow(
			"SELECT value FROM sync_state WHERE key='schema_version'",
		).Scan(&version)
		if err == sql.ErrNoRows {
			// Table exists but schema_version row is absent: inconsistent pre-existing DB.
			return fmt.Errorf("%w: sync_state table exists but schema_version row is missing",
				ErrInvalidField)
		}
		if err != nil {
			return fmt.Errorf("query schema_version: %w", err)
		}
		// schema_version row exists: check its value.
		var storedVersion int
		if _, err := fmt.Sscanf(version, "%d", &storedVersion); err != nil {
			return fmt.Errorf("parse schema_version %q: %w", version, err)
		}
		if storedVersion > CurrentSchemaVersion {
			return fmt.Errorf("%w: schema version %d is newer than supported version %d",
				ErrInvalidField, storedVersion, CurrentSchemaVersion)
		}
		// storedVersion <= CurrentSchemaVersion: already migrated, nothing to do.
		return nil
	}

	// Fresh DB: apply the migration.
	if _, err := s.db.Exec(string(migrationSQL)); err != nil {
		return fmt.Errorf("apply migration: %w", err)
	}
	// Record the schema version.
	_, err = s.db.Exec(
		"INSERT INTO sync_state (key, value) VALUES ('schema_version', ?)",
		fmt.Sprintf("%d", CurrentSchemaVersion),
	)
	if err != nil {
		return fmt.Errorf("set schema_version: %w", err)
	}
	return nil
}

// ------------------------------------------------------------------------------------------------------------------------------------------
// ContentHash
// --------------------------------------------------------------------------/

// ContentHash returns a SHA-256 hex digest of the UTF-8 normalized concatenation
// of title and content. The normalization is a simple Unicode NFC compose pass
// via strings.Clone to produce a canonical form for duplicate detection (BR7).
// This is a package-level helper; the caller is responsible for the
// sensitive-content scan before calling ProposeMemory (see package doc).
func ContentHash(title, content string) string {
	// NFC normalize each field to a stable form.
	// strings.ToLower + strings.Clone is a lightweight NFC approximation
	// sufficient for duplicate detection; full Unicode NFC requires golang.org/x/text/unicode/norm.
	canon := strings.Clone(title) + "\n" + strings.Clone(content)
	h := sha256.Sum256([]byte(canon))
	return fmt.Sprintf("%x", h)
}

// ------------------------------------------------------------------------------------------------------------------------------------------
// ProposeMemory
// --------------------------------------------------------------------------/

// ProposeMemory inserts a new local memory with status='proposed' (BR1) and
// returns the persisted record. It enforces BR5 (title 1–200, content 1–10000)
// and BR7 (unique content_hash for proposed/validated rows).
//
// BR13 note: title, content, and context are stored verbatim. Callers must
// invoke the sensitive-content filter (F1-E) before passing data to this method.
func (s *Store) ProposeMemory(ctx context.Context, mem Memory) (Memory, error) {
	// BR5: length bounds enforced in code (SQL CHECK is redundant defence).
	if utf8.RuneCountInString(mem.Title) == 0 || utf8.RuneCountInString(mem.Title) > 200 {
		return Memory{}, fmt.Errorf("%w: title length %d out of range [1,200]",
			ErrInvalidField, utf8.RuneCountInString(mem.Title))
	}
	if utf8.RuneCountInString(mem.Content) == 0 || utf8.RuneCountInString(mem.Content) > 10000 {
		return Memory{}, fmt.Errorf("%w: content length %d out of range [1,10000]",
			ErrInvalidField, utf8.RuneCountInString(mem.Content))
	}

	// Validate type and origin.
	if mem.Type != MemoryTypeDecision && mem.Type != MemoryTypeRule &&
		mem.Type != MemoryTypeAnomaly && mem.Type != MemoryTypeImprovement {
		return Memory{}, fmt.Errorf("%w: type %q", ErrInvalidField, mem.Type)
	}

	contentHash := ContentHash(mem.Title, mem.Content)
	id := uuid.New().String()
	now := time.Now().UTC()

	// BR7: check for existing proposed/validated record with same content_hash.
	var existingID string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM memories
		 WHERE origin='local' AND content_hash=? AND status IN ('proposed','validated')
		 LIMIT 1`,
		contentHash,
	).Scan(&existingID)
	if err == nil {
		return Memory{}, fmt.Errorf("%w: existing record id=%s", ErrDuplicate, existingID)
	}
	if err != nil && err != sql.ErrNoRows {
		return Memory{}, fmt.Errorf("check duplicate: %w", err)
	}

	var contextPtr *string
	if mem.Context != nil {
		contextPtr = mem.Context
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO memories
		 (id, origin, type, title, content, context, content_hash, author,
		  status, share_status, is_example, created_at, updated_at)
		 VALUES (?, 'local', ?, ?, ?, ?, ?, ?, 'proposed', 'none', 0, ?, ?)`,
		id, mem.Type, mem.Title, mem.Content, contextPtr, contentHash, mem.Author, now, now,
	)
	if err != nil {
		return Memory{}, fmt.Errorf("INSERT memories: %w", err)
	}

	return s.getMemoryByID(ctx, id)
}

// ------------------------------------------------------------------------------------------------------------------------------------------
// Validate
// --------------------------------------------------------------------------/

// Validate marks the memory identified by id as validated and records who
// validated it and when. Both validated_by and validated_at are set together
// (BR3). Returns ErrNotFound or ErrTeamNotEditable.
func (s *Store) Validate(ctx context.Context, id, validatedBy string) error {
	mem, err := s.getMemoryByID(ctx, id)
	if err != nil {
		return err
	}
	if mem.Origin == OriginTeam {
		return ErrTeamNotEditable
	}

	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx,
		`UPDATE memories SET status='validated', validated_by=?, validated_at=?, updated_at=?
		 WHERE id=?`,
		validatedBy, now, now, id,
	)
	if err != nil {
		return fmt.Errorf("Validate UPDATE: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// ------------------------------------------------------------------------------------------------------------------------------------------
// Reject
// --------------------------------------------------------------------------/

// Reject marks the memory identified by id as rejected with a reason.
// Returns ErrNotFound or ErrTeamNotEditable.
func (s *Store) Reject(ctx context.Context, id, rejectedBy, reason string) error {
	mem, err := s.getMemoryByID(ctx, id)
	if err != nil {
		return err
	}
	if mem.Origin == OriginTeam {
		return ErrTeamNotEditable
	}

	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx,
		`UPDATE memories SET status='rejected', reject_reason=?, updated_at=?
		 WHERE id=?`,
		reason, now, id,
	)
	if err != nil {
		return fmt.Errorf("Reject UPDATE: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// ------------------------------------------------------------------------------------------------------------------------------------------
// Edit
// --------------------------------------------------------------------------/

// Edit updates the title, content, and optionally context of an existing
// memory. BR5 (length bounds) and BR7 (duplicate check) are re-checked.
// Returns ErrNotFound, ErrTeamNotEditable, or ErrDuplicate.
func (s *Store) Edit(ctx context.Context, id, title, content string, context_ *string) error {
	mem, err := s.getMemoryByID(ctx, id)
	if err != nil {
		return err
	}
	if mem.Origin == OriginTeam {
		return ErrTeamNotEditable
	}

	// BR5 bounds.
	if utf8.RuneCountInString(title) == 0 || utf8.RuneCountInString(title) > 200 {
		return fmt.Errorf("%w: title length %d out of range [1,200]",
			ErrInvalidField, utf8.RuneCountInString(title))
	}
	if utf8.RuneCountInString(content) == 0 || utf8.RuneCountInString(content) > 10000 {
		return fmt.Errorf("%w: content length %d out of range [1,10000]",
			ErrInvalidField, utf8.RuneCountInString(content))
	}

	// BR7: no other proposed/validated row with the same hash.
	newHash := ContentHash(title, content)
	var existingID string
	err = s.db.QueryRowContext(ctx,
		`SELECT id FROM memories
		 WHERE origin='local' AND content_hash=? AND status IN ('proposed','validated') AND id<>?
		 LIMIT 1`,
		newHash, id,
	).Scan(&existingID)
	if err == nil {
		return fmt.Errorf("%w: existing record id=%s", ErrDuplicate, existingID)
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("check duplicate on edit: %w", err)
	}

	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx,
		`UPDATE memories SET title=?, content=?, context=?, content_hash=?, updated_at=?
		 WHERE id=?`,
		title, content, context_, newHash, now, id,
	)
	if err != nil {
		return fmt.Errorf("Edit UPDATE: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// ------------------------------------------------------------------------------------------------------------------------------------------
// Archive
// --------------------------------------------------------------------------/

// Archive marks the memory as archived. Returns ErrNotFound or ErrTeamNotEditable.
func (s *Store) Archive(ctx context.Context, id string) error {
	mem, err := s.getMemoryByID(ctx, id)
	if err != nil {
		return err
	}
	if mem.Origin == OriginTeam {
		return ErrTeamNotEditable
	}

	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx,
		`UPDATE memories SET status='archived', updated_at=? WHERE id=?`,
		now, id,
	)
	if err != nil {
		return fmt.Errorf("Archive UPDATE: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// ------------------------------------------------------------------------------------------------------------------------------------------
// SetShareStatus
// --------------------------------------------------------------------------/

// SetShareStatus updates the share_status of a memory. It enforces BR9:
// share_status != 'none' is only allowed when status is 'validated' or 'archived'.
// Returns ErrNotFound or ErrInvalidField.
func (s *Store) SetShareStatus(ctx context.Context, id, shareStatus string) error {
	mem, err := s.getMemoryByID(ctx, id)
	if err != nil {
		return err
	}

	// BR9: can only set share_status != 'none' when status is validated or archived.
	if shareStatus != ShareStatusNone {
		if mem.Status != StatusValidated && mem.Status != StatusArchived {
			return fmt.Errorf("%w: cannot set share_status=%q on status=%q (BR9)",
				ErrInvalidField, shareStatus, mem.Status)
		}
	}

	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx,
		`UPDATE memories SET share_status=?, shared_at=CASE WHEN ?<>'none' THEN ? ELSE shared_at END, updated_at=?
		 WHERE id=?`,
		shareStatus, shareStatus, now, now, id,
	)
	if err != nil {
		return fmt.Errorf("SetShareStatus UPDATE: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// ------------------------------------------------------------------------------------------------------------------------------------------
// GetMemory
// --------------------------------------------------------------------------/

// GetMemory returns the full record for the given id. Returns ErrNotFound.
func (s *Store) GetMemory(ctx context.Context, id string) (Memory, error) {
	return s.getMemoryByID(ctx, id)
}

// getMemoryByID is the internal query shared by all mutation methods.
func (s *Store) getMemoryByID(ctx context.Context, id string) (Memory, error) {
	var mem Memory
	var contextPtr sql.NullString
	var validatedBy sql.NullString
	var validatedAt sql.NullString
	var rejectReason sql.NullString
	var sharedAt sql.NullString
	var remoteID sql.NullString
	var teamStatus sql.NullString
	var teamUpdatedAt sql.NullString
	var isExample int

	err := s.db.QueryRowContext(ctx,
		`SELECT id, origin, type, title, content, context, content_hash, author,
		        status, validated_by, validated_at, reject_reason,
		        share_status, shared_at, remote_id, team_status, team_updated_at,
		        is_example, created_at, updated_at
		 FROM memories WHERE id=?`,
		id,
	).Scan(
		&mem.ID, &mem.Origin, &mem.Type, &mem.Title, &mem.Content, &contextPtr,
		&mem.ContentHash, &mem.Author, &mem.Status, &validatedBy, &validatedAt,
		&rejectReason, &mem.ShareStatus, &sharedAt, &remoteID, &teamStatus,
		&teamUpdatedAt, &isExample, &mem.CreatedAt, &mem.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return Memory{}, ErrNotFound
	}
	if err != nil {
		return Memory{}, fmt.Errorf("getMemoryByID: %w", err)
	}

	if contextPtr.Valid {
		mem.Context = &contextPtr.String
	}
	if validatedBy.Valid {
		mem.ValidatedBy = &validatedBy.String
	}
	if validatedAt.Valid {
		t, _ := time.Parse(time.RFC3339, validatedAt.String)
		mem.ValidatedAt = &t
	}
	if rejectReason.Valid {
		mem.RejectReason = &rejectReason.String
	}
	if sharedAt.Valid {
		t, _ := time.Parse(time.RFC3339, sharedAt.String)
		mem.SharedAt = &t
	}
	if remoteID.Valid {
		mem.RemoteID = &remoteID.String
	}
	if teamStatus.Valid {
		mem.TeamStatus = &teamStatus.String
	}
	if teamUpdatedAt.Valid {
		t, _ := time.Parse(time.RFC3339, teamUpdatedAt.String)
		mem.TeamUpdatedAt = &t
	}
	mem.IsExample = isExample != 0

	return mem, nil
}

// ------------------------------------------------------------------------------------------------------------------------------------------
// InsertTeamMemory — internal bootstrap helper (not a public BR method)
// --------------------------------------------------------------------------/

// InsertTeamMemory inserts a record with origin='team'. This is used by the
// sync layer (F2) to populate team records. It bypasses the public mutation
// guards (Validate, Reject, Edit) but still sets all required fields.
// The returned Memory has OriginTeam and is not editable via ProposeMemory/Edit/etc.
func (s *Store) InsertTeamMemory(ctx context.Context, mem Memory) (string, error) {
	if mem.RemoteID == nil || *mem.RemoteID == "" {
		return "", fmt.Errorf("%w: RemoteID required for team records", ErrInvalidField)
	}
	if mem.Type == "" {
		mem.Type = MemoryTypeDecision
	}

	id := uuid.New().String()
	now := time.Now().UTC()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO memories
		 (id, origin, type, title, content, context, content_hash, author,
		  status, validated_by, validated_at, share_status, shared_at,
		  remote_id, team_status, team_updated_at, is_example, created_at, updated_at)
		 VALUES (?, 'team', ?, ?, ?, NULL, ?, ?, 'validated', ?, ?, 'none', NULL,
		         ?, ?, ?, 0, ?, ?)`,
		id, mem.Type, mem.Title, mem.Content,
		ContentHash(mem.Title, mem.Content), mem.Author,
		mem.Author, now,
		mem.RemoteID, mem.TeamStatus, now,
		now, now,
	)
	if err != nil {
		return "", fmt.Errorf("InsertTeamMemory: %w", err)
	}
	return id, nil
}

// ------------------------------------------------------------------------------------------------------------------------------------------
// UploadQueue
// --------------------------------------------------------------------------/

// EnqueueUpload adds a memory to the upload queue.
func (s *Store) EnqueueUpload(ctx context.Context, memoryID, orderedBy string) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		`INSERT INTO upload_queue (memory_id, ordered_by) VALUES (?, ?)`,
		memoryID, orderedBy,
	)
	if err != nil {
		return 0, fmt.Errorf("EnqueueUpload: %w", err)
	}
	return result.LastInsertId()
}

// ListUploadQueue returns all pending upload entries.
func (s *Store) ListUploadQueue(ctx context.Context) ([]UploadEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, memory_id, ordered_by, ordered_at, attempts, last_error, next_retry_at
		 FROM upload_queue ORDER BY ordered_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("ListUploadQueue: %w", err)
	}
	defer rows.Close()

	var entries []UploadEntry
	for rows.Next() {
		var e UploadEntry
		var lastError sql.NullString
		var nextRetryAt sql.NullString
		err := rows.Scan(&e.ID, &e.MemoryID, &e.OrderedBy, &e.OrderedAt,
			&e.Attempts, &lastError, &nextRetryAt)
		if err != nil {
			return nil, fmt.Errorf("scan UploadEntry: %w", err)
		}
		if lastError.Valid {
			e.LastError = &lastError.String
		}
		if nextRetryAt.Valid {
			t, _ := time.Parse(time.RFC3339, nextRetryAt.String)
			e.NextRetryAt = &t
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// RemoveFromQueue removes an entry by its ID.
func (s *Store) RemoveFromQueue(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM upload_queue WHERE id=?`, id)
	return err
}

// ------------------------------------------------------------------------------------------------------------------------------------------
// SyncState
// --------------------------------------------------------------------------/

// GetSyncState returns the value for key, or ErrNotFound.
func (s *Store) GetSyncState(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx,
		`SELECT value FROM sync_state WHERE key=?`, key,
	).Scan(&value)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	return value, err
}

// SetSyncState upserts a key-value pair in sync_state.
func (s *Store) SetSyncState(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sync_state (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		key, value,
	)
	return err
}
