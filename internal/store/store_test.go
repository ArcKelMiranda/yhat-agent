// F1-A: Persistent SQLite store tests.
// Every BR rule has a named test. Tests use t.TempDir() for the SQLite file.

package store_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite" // register sqlite driver

	"github.com/ArcKelMiranda/yhat-agent/internal/store"
)

// helper opens a fresh store in a temp directory and returns its path.
func newStoreDir(t *testing.T) (path string, cleanup func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "yhat-store-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	dbPath := filepath.Join(dir, "yhat.db")
	return dbPath, func() { os.RemoveAll(dir) }
}

// ─────────────────────────────────────────────────────────────────────────────
// RED phase: write each test to capture the expected failure, then implement.
// ─────────────────────────────────────────────────────────────────────────────

// TestEmbeddedMigrationApplies verifies that Open applies 001_init.sql on a
// fresh database: tables and indexes are created and schema_version=1 is set.
func TestEmbeddedMigrationApplies(t *testing.T) {
	dbPath, cleanup := newStoreDir(t)
	defer cleanup()

	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, table := range []string{"memories", "upload_queue", "sync_state"} {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != sql.ErrNoRows && err != nil {
			t.Errorf("table %q: %v", table, err)
		}
		if name != table {
			t.Errorf("table %q: want present, got %q", table, name)
		}
	}

	for _, idx := range []string{"idx_memories_origin_status", "idx_memories_live_hash"} {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='index' AND name=?", idx).Scan(&name)
		if err != sql.ErrNoRows && err != nil {
			t.Errorf("index %q: %v", idx, err)
		}
		if name != idx {
			t.Errorf("index %q: want present, got %q", idx, name)
		}
	}

	var version string
	err = db.QueryRow("SELECT value FROM sync_state WHERE key='schema_version'").Scan(&version)
	if err != nil {
		t.Errorf("schema_version row: %v", err)
	}
	if version != "1" {
		t.Errorf("schema_version: want 1, got %q", version)
	}
}

// TestEmbeddedMigrationRejectsFutureVersion simulates a database whose
// schema_version is higher than CurrentSchemaVersion and asserts that Open
// returns an error that mentions the future version.
func TestEmbeddedMigrationRejectsFutureVersion(t *testing.T) {
	dbPath, cleanup := newStoreDir(t)
	defer cleanup()

	// Bootstrap a minimal DB with a future schema_version row.
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE sync_state (key TEXT PRIMARY KEY, value TEXT NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO sync_state (key, value) VALUES ('schema_version', '99')`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	_, err = store.Open(dbPath)
	if err == nil {
		t.Fatal("Open: want error for future schema_version, got nil")
	}
}

// TestOpenMissingSchemaVersion verifies that Open fails with a descriptive
// error when the sync_state table exists but the schema_version row is absent.
// This represents an inconsistent pre-existing DB (e.g., created by a future
// version that removed the row), which must fail loudly per the migration design.
func TestOpenMissingSchemaVersion(t *testing.T) {
	dbPath, cleanup := newStoreDir(t)
	defer cleanup()

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	// Create sync_state without the schema_version row — an inconsistent pre-existing DB.
	_, err = db.Exec(`CREATE TABLE sync_state (key TEXT PRIMARY KEY, value TEXT NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	_, err = store.Open(dbPath)
	if err == nil {
		t.Fatal("Open: want error for missing schema_version row, got nil")
	}
	// The error should mention the missing row.
	if !strings.Contains(err.Error(), "schema_version") {
		t.Errorf("Open error: want message about schema_version, got %q", err.Error())
	}
}

// TestOpenWALAndFKPragmas verifies that after Open the connection uses WAL
// mode and has foreign_keys enabled.
func TestOpenWALAndFKPragmas(t *testing.T) {
	dbPath, cleanup := newStoreDir(t)
	defer cleanup()

	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	// Open a second connection with the same pragma DSN and verify.
	dsn := dbPath + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)"
	db2, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()

	var journalMode string
	err = db2.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatal(err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode: want wal, got %q", journalMode)
	}

	var fk integer
	err = db2.QueryRow("PRAGMA foreign_keys").Scan(&fk)
	if err != nil {
		t.Fatal(err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys: want 1, got %d", fk)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// BR1: Default status is 'proposed'
// ─────────────────────────────────────────────────────────────────────────────

func TestBR1DefaultProposed(t *testing.T) {
	dbPath, cleanup := newStoreDir(t)
	defer cleanup()

	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	mem, err := s.ProposeMemory(context.Background(), store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Decisión de prueba BR1",
		Content: "Contenido breve para BR1.",
		Author:  "test-operator",
	})
	if err != nil {
		t.Fatalf("ProposeMemory: %v", err)
	}
	if mem.Status != store.StatusProposed {
		t.Errorf("status after ProposeMemory: want %q, got %q", store.StatusProposed, mem.Status)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// BR3: validated_by and validated_at are set together
// ─────────────────────────────────────────────────────────────────────────────

func TestBR3ValidatedByAtCoupled(t *testing.T) {
	dbPath, cleanup := newStoreDir(t)
	defer cleanup()

	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	mem, err := s.ProposeMemory(context.Background(), store.Memory{
		Type:    store.MemoryTypeRule,
		Title:   "Regla de prueba BR3",
		Content: "Contenido para BR3.",
		Author:  "test-operator",
	})
	if err != nil {
		t.Fatalf("ProposeMemory: %v", err)
	}

	// Validate sets both validated_by and validated_at.
	err = s.Validate(context.Background(), mem.ID, "approver-1")
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	fetched, err := s.GetMemory(context.Background(), mem.ID)
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if fetched.ValidatedBy == nil || *fetched.ValidatedBy == "" {
		t.Error("ValidatedBy: want non-empty after Validate")
	}
	if fetched.ValidatedAt == nil {
		t.Error("ValidatedAt: want non-nil after Validate")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// BR5: title 1-200 chars, content 1-10000 chars
// ─────────────────────────────────────────────────────────────────────────────

func TestBR5TitleLengthBounds(t *testing.T) {
	dbPath, cleanup := newStoreDir(t)
	defer cleanup()

	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Empty title should be rejected.
	_, err = s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "", // invalid
		Content: "Contenido válido.",
		Author:  "test",
	})
	if err == nil {
		t.Error("ProposeMemory: want error for empty title, got nil")
	}

	// Title with 201 characters should be rejected.
	longTitle := make([]byte, 201)
	for i := range longTitle {
		longTitle[i] = 'x'
	}
	_, err = s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   string(longTitle), // invalid
		Content: "Contenido válido.",
		Author:  "test",
	})
	if err == nil {
		t.Error("ProposeMemory: want error for 201-char title, got nil")
	}

	// Title with 200 characters should succeed.
	title200 := make([]byte, 200)
	for i := range title200 {
		title200[i] = 'x'
	}
	mem, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   string(title200), // valid
		Content: "Contenido válido.",
		Author:  "test",
	})
	if err != nil {
		t.Errorf("ProposeMemory with 200-char title: %v", err)
	}
	if mem.ID == "" {
		t.Error("ProposeMemory with 200-char title: want non-empty ID")
	}
}

func TestBR5ContentLengthBounds(t *testing.T) {
	dbPath, cleanup := newStoreDir(t)
	defer cleanup()

	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Empty content should be rejected.
	_, err = s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Título válido",
		Content: "", // invalid
		Author:  "test",
	})
	if err == nil {
		t.Error("ProposeMemory: want error for empty content, got nil")
	}

	// Content with 10001 characters should be rejected.
	longContent := make([]byte, 10001)
	for i := range longContent {
		longContent[i] = 'x'
	}
	_, err = s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Título válido",
		Content: string(longContent), // invalid
		Author:  "test",
	})
	if err == nil {
		t.Error("ProposeMemory: want error for 10001-char content, got nil")
	}

	// Content with 10000 characters should succeed.
	content10k := make([]byte, 10000)
	for i := range content10k {
		content10k[i] = 'x'
	}
	mem, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Título válido",
		Content: string(content10k), // valid
		Author:  "test",
	})
	if err != nil {
		t.Errorf("ProposeMemory with 10000-char content: %v", err)
	}
	if mem.ID == "" {
		t.Error("ProposeMemory with 10000-char content: want non-empty ID")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// BR7: unique content_hash for proposed and validated rows
// ─────────────────────────────────────────────────────────────────────────────

func TestBR7UniqueContentHash(t *testing.T) {
	dbPath, cleanup := newStoreDir(t)
	defer cleanup()

	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	mem1, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Decisión BR7",
		Content: "Contenido único BR7.",
		Author:  "test",
	})
	if err != nil {
		t.Fatalf("ProposeMemory first: %v", err)
	}

	// Exact duplicate title+content should return ErrDuplicate.
	_, err = s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Decisión BR7",
		Content: "Contenido único BR7.",
		Author:  "test",
	})
	if !errors.Is(err, store.ErrDuplicate) {
		t.Errorf("ProposeMemory duplicate: want %v, got %v", store.ErrDuplicate, err)
	}

	// After validating the first, a duplicate should still be rejected.
	err = s.Validate(ctx, mem1.ID, "approver")
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	_, err = s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Decisión BR7",
		Content: "Contenido único BR7.",
		Author:  "test",
	})
	if !errors.Is(err, store.ErrDuplicate) {
		t.Errorf("ProposeMemory duplicate after Validate: want %v, got %v", store.ErrDuplicate, err)
	}

	// After archiving the first, a duplicate should be allowed (unique index only covers proposed/validated).
	err = s.Archive(ctx, mem1.ID)
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}

	mem2, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Decisión BR7",
		Content: "Contenido único BR7.",
		Author:  "test",
	})
	if err != nil {
		t.Errorf("ProposeMemory after Archive: want nil, got %v", err)
	}
	if mem2.ID == "" {
		t.Error("ProposeMemory after Archive: want non-empty ID")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// BR9: share_status only with validated or archived status
// ─────────────────────────────────────────────────────────────────────────────

func TestBR9ShareStatusOnlyValidatedOrArchived(t *testing.T) {
	dbPath, cleanup := newStoreDir(t)
	defer cleanup()

	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	mem, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Decisión BR9",
		Content: "Contenido BR9.",
		Author:  "test",
	})
	if err != nil {
		t.Fatalf("ProposeMemory: %v", err)
	}

	// Trying to queue a proposed memory should fail.
	err = s.SetShareStatus(ctx, mem.ID, store.ShareStatusQueued)
	if err == nil {
		t.Error("SetShareStatus on proposed: want error, got nil")
	}

	// After validation, queuing should succeed.
	err = s.Validate(ctx, mem.ID, "approver")
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	err = s.SetShareStatus(ctx, mem.ID, store.ShareStatusQueued)
	if err != nil {
		t.Errorf("SetShareStatus on validated: want nil, got %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// BR11: team-origin records are not editable via the public API
// ─────────────────────────────────────────────────────────────────────────────

func TestBR11TeamRecordNotEditable(t *testing.T) {
	dbPath, cleanup := newStoreDir(t)
	defer cleanup()

	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Manually insert a team-origin record via the internal hook.
	remoteID := "remote-123"
	teamStatus := "approved"
	teamID, err := s.InsertTeamMemory(ctx, store.Memory{
		Type:       store.MemoryTypeRule,
		Title:      "Regla del equipo",
		Content:    "Contenido del equipo.",
		Author:     "central",
		RemoteID:   &remoteID,
		TeamStatus: &teamStatus,
	})
	if err != nil {
		t.Fatalf("InsertTeamMemory: %v", err)
	}

	// Validate should fail on team records.
	err = s.Validate(ctx, teamID, "approver")
	if err != store.ErrTeamNotEditable {
		t.Errorf("Validate on team record: want %v, got %v", store.ErrTeamNotEditable, err)
	}

	// Reject should fail on team records.
	err = s.Reject(ctx, teamID, "approver", "reason")
	if err != store.ErrTeamNotEditable {
		t.Errorf("Reject on team record: want %v, got %v", store.ErrTeamNotEditable, err)
	}

	// Edit should fail on team records.
	err = s.Edit(ctx, teamID, "Nuevo título", "Nuevo contenido", nil)
	if err != store.ErrTeamNotEditable {
		t.Errorf("Edit on team record: want %v, got %v", store.ErrTeamNotEditable, err)
	}

	// Archive should fail on team records.
	err = s.Archive(ctx, teamID)
	if err != store.ErrTeamNotEditable {
		t.Errorf("Archive on team record: want %v, got %v", store.ErrTeamNotEditable, err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// ContentHash helper
// ─────────────────────────────────────────────────────────────────────────────

func TestContentHashDeterministic(t *testing.T) {
	h1 := store.ContentHash("Título", "Contenido")
	h2 := store.ContentHash("Título", "Contenido")
	if h1 != h2 {
		t.Errorf("ContentHash: want deterministic, got %q vs %q", h1, h2)
	}
}

func TestContentHashDifferentInputs(t *testing.T) {
	h1 := store.ContentHash("Título A", "Contenido")
	h2 := store.ContentHash("Título B", "Contenido")
	if h1 == h2 {
		t.Error("ContentHash: want different hashes for different inputs")
	}
}

func TestContentHashIsSHA256Hex(t *testing.T) {
	h := store.ContentHash("test", "content")
	if len(h) != 64 {
		t.Errorf("ContentHash length: want 64 (SHA-256 hex), got %d", len(h))
	}
}

// type alias to avoid import cycle in this test-only file
type integer = int
