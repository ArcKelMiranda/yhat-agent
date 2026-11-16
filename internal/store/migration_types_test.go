// Migration 002: the memory type set grows from 4 to 12 values.

package store_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArcKelMiranda/yhat-agent/internal/store"
)

var allMemoryTypes = []string{
	"decision", "rule", "anomaly", "improvement",
	"observation", "assumption", "process", "mapping",
	"exception", "entity-definition", "integration", "technical-rule",
}

func TestProposeMemoryAcceptsEveryType(t *testing.T) {
	dbPath, cleanup := newStoreDir(t)
	defer cleanup()
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	for _, typ := range allMemoryTypes {
		m, err := s.ProposeMemory(context.Background(), store.Memory{
			Type: typ, Title: "title " + typ, Content: "content for " + typ, Author: "tester",
		})
		if err != nil {
			t.Fatalf("type %q rejected: %v", typ, err)
		}
		if m.Type != typ || m.Status != store.StatusProposed {
			t.Fatalf("type %q: got type=%q status=%q", typ, m.Type, m.Status)
		}
	}
}

func TestProposeMemoryRejectsUnknownType(t *testing.T) {
	dbPath, cleanup := newStoreDir(t)
	defer cleanup()
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	_, err = s.ProposeMemory(context.Background(), store.Memory{
		Type: "gossip", Title: "t", Content: "c", Author: "tester",
	})
	if !errors.Is(err, store.ErrInvalidField) {
		t.Fatalf("want ErrInvalidField, got %v", err)
	}
}

// A database created by the previous release (schema 1, four types) must open,
// keep every row and queue entry, and accept the new types afterwards.
func TestMigrationV1ToV2KeepsRowsAndAcceptsNewTypes(t *testing.T) {
	dbPath, cleanup := newStoreDir(t)
	defer cleanup()

	oldSQL, err := os.ReadFile(filepath.Join("migrations", "001_init.sql"))
	if err != nil {
		t.Fatalf("read 001: %v", err)
	}
	raw, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := raw.Exec(string(oldSQL)); err != nil {
		t.Fatalf("apply 001: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO sync_state (key,value) VALUES ('schema_version','1')`); err != nil {
		t.Fatalf("set version: %v", err)
	}
	for _, ddl := range []string{
		`CREATE VIRTUAL TABLE memories_fts USING fts5(title, content, tokenize='unicode61 remove_diacritics 2')`,
		`CREATE TRIGGER trg_memories_ai AFTER INSERT ON memories BEGIN
			INSERT INTO memories_fts(rowid, title, content) VALUES (NEW.rowid, NEW.title, NEW.content);
		END`,
	} {
		if _, err := raw.Exec(ddl); err != nil {
			t.Fatalf("seed fts: %v", err)
		}
	}
	if _, err := raw.Exec(`INSERT INTO memories (id,origin,type,title,content,content_hash,author,status,share_status,validated_by,validated_at)
		VALUES ('m1','local','rule','old title','old content',?,'ana','validated','queued','ana',CURRENT_TIMESTAMP)`, store.ContentHash("old title", "old content")); err != nil {
		t.Fatalf("seed memory: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO upload_queue (memory_id, ordered_by) VALUES ('m1','ana')`); err != nil {
		t.Fatalf("seed queue: %v", err)
	}
	raw.Close()

	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open v1 database: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	v, err := s.GetSyncState(ctx, "schema_version")
	if err != nil || v != "2" {
		t.Fatalf("schema_version = %q, %v; want 2", v, err)
	}
	m, err := s.GetMemory(ctx, "m1")
	if err != nil {
		t.Fatalf("old row lost: %v", err)
	}
	if m.Title != "old title" || m.Type != "rule" || m.Status != "validated" || m.ShareStatus != "queued" {
		t.Fatalf("old row changed: %+v", m)
	}
	q, err := s.ListUploadQueue(ctx)
	if err != nil || len(q) != 1 || q[0].MemoryID != "m1" {
		t.Fatalf("queue after migration = %+v, %v", q, err)
	}
	if _, err := s.ProposeMemory(ctx, store.Memory{Type: "mapping", Title: "n", Content: "new", Author: "ana"}); err != nil {
		t.Fatalf("new type after migration: %v", err)
	}
	// BR7 still enforced after the table rebuild.
	if _, err := s.ProposeMemory(ctx, store.Memory{Type: "rule", Title: "old title", Content: "old content", Author: "ana"}); err == nil {
		t.Fatal("duplicate content_hash accepted after migration")
	}

	// The FTS index (keyed by rowid) and its trigger survive the table rebuild.
	if err := s.EnsureFTS5(ctx); err != nil {
		t.Fatalf("EnsureFTS5: %v", err)
	}
	hits, err := s.SearchMemoriesFTS5(ctx, `"old"`, 10)
	if err != nil || len(hits) != 1 || hits[0].ID != "m1" {
		t.Fatalf("FTS search after migration = %+v, %v", hits, err)
	}
	if _, err := s.ProposeMemory(ctx, store.Memory{Type: "process", Title: "fresh words", Content: "zebra", Author: "ana"}); err != nil {
		t.Fatalf("propose after migration: %v", err)
	}
	hits, err = s.SearchMemoriesFTS5(ctx, `"zebra"`, 10)
	if err != nil || len(hits) != 1 {
		t.Fatalf("trigger lost after migration: %+v, %v", hits, err)
	}

	// Reopening is a no-op.
	s.Close()
	s2, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	s2.Close()
}

func fkEnabled(t *testing.T, s *store.Store) bool {
	t.Helper()
	var on int
	if err := s.StoreDB().QueryRow("PRAGMA foreign_keys").Scan(&on); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	return on == 1
}

func TestForeignKeysOnAfterFreshAndMigratedOpen(t *testing.T) {
	dbPath, cleanup := newStoreDir(t)
	defer cleanup()
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("fresh Open: %v", err)
	}
	if !fkEnabled(t, s) {
		t.Fatal("foreign_keys is OFF after a fresh Open")
	}
	s.Close()

	dbPath2, cleanup2 := newStoreDir(t)
	defer cleanup2()
	seedV1(t, dbPath2, false)
	s2, err := store.Open(dbPath2)
	if err != nil {
		t.Fatalf("migrated Open: %v", err)
	}
	defer s2.Close()
	if !fkEnabled(t, s2) {
		t.Fatal("foreign_keys is OFF after migrating a v1 database")
	}
}

// A v1 database that already violates a foreign key must not be half-migrated:
// Open fails, nothing changes and the schema stays at 1.
func TestMigrationFailureRollsBackAndLeavesSchemaAtV1(t *testing.T) {
	dbPath, cleanup := newStoreDir(t)
	defer cleanup()
	seedV1(t, dbPath, true)

	if s, err := store.Open(dbPath); err == nil {
		s.Close()
		t.Fatal("Open succeeded on a database with a foreign key violation")
	}

	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var version string
	if err := raw.QueryRow(`SELECT value FROM sync_state WHERE key='schema_version'`).Scan(&version); err != nil || version != "1" {
		t.Fatalf("schema_version = %q, %v; want 1", version, err)
	}
	var n int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM memories`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("memories rows = %d, %v; want 1", n, err)
	}
	if err := raw.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='memories_new'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("memories_new leaked after rollback (%d, %v)", n, err)
	}
}

// seedV1 writes a schema-1 database with one memory; orphan adds a queue row
// pointing at a memory that does not exist (a pre-existing FK violation).
func seedV1(t *testing.T, dbPath string, orphan bool) {
	t.Helper()
	oldSQL, err := os.ReadFile(filepath.Join("migrations", "001_init.sql"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	for _, q := range []string{
		string(oldSQL),
		`INSERT INTO sync_state (key,value) VALUES ('schema_version','1')`,
		`INSERT INTO memories (id,origin,type,title,content,content_hash,author) VALUES ('m1','local','rule','t','c','h','ana')`,
	} {
		if _, err := raw.Exec(q); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	if orphan {
		if _, err := raw.Exec(`INSERT INTO upload_queue (memory_id, ordered_by) VALUES ('ghost','ana')`); err != nil {
			t.Fatalf("seed orphan: %v", err)
		}
	}
}
