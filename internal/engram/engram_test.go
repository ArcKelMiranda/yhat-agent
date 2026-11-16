package engram_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/ArcKelMiranda/yhat-agent/internal/engram"
)

// fixture builds a minimal Engram database with the columns the reader uses.
func fixture(t *testing.T, rows [][]any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "engram.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE observations (
		id INTEGER PRIMARY KEY AUTOINCREMENT, sync_id TEXT, session_id TEXT NOT NULL DEFAULT 's',
		type TEXT NOT NULL, title TEXT NOT NULL, content TEXT NOT NULL, project TEXT,
		topic_key TEXT, updated_at TEXT NOT NULL, deleted_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if _, err := db.Exec(`INSERT INTO observations (sync_id,type,title,content,project,topic_key,updated_at,deleted_at)
			VALUES (?,?,?,?,?,?,?,?)`, r...); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestReadSelectsByProjectOrKeyPrefix(t *testing.T) {
	path := fixture(t, [][]any{
		{"a", "decision", "in project", "body a", "yhat-knowledge", nil, "2026-01-01 10:00:00", nil},
		{"b", "decision", "by key", "body b", "other", "yhat.rule.fa.sep", "2026-01-01 10:00:01", nil},
		{"c", "bugfix", "noise", "body c", "other", "sdd/x", "2026-01-01 10:00:02", nil},
		{"d", "decision", "wildcard key", "body d", "other", "yhatXrule", "2026-01-01 10:00:03", nil},
		{"e", "decision", "deleted", "body e", "yhat-knowledge", nil, "2026-01-01 10:00:04", "2026-02-01 00:00:00"},
		{"f", "decision", "  ", "blank title", "yhat-knowledge", nil, "2026-01-01 10:00:05", nil},
	})
	got, cur, err := engram.Read(context.Background(), path, engram.DefaultCriteria(), engram.Cursor{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 2 || got[0].Title != "in project" || got[1].Title != "by key" {
		t.Fatalf("got %+v", got)
	}
	if cur.UpdatedAt != "2026-01-01 10:00:01" || cur.ID != 2 {
		t.Fatalf("cursor = %+v", cur)
	}
}

func TestReadResumesFromCursor(t *testing.T) {
	path := fixture(t, [][]any{
		{"a", "rule", "one", "x", "yhat-knowledge", nil, "2026-01-01 10:00:00", nil},
		{"b", "rule", "two", "x", "yhat-knowledge", nil, "2026-01-01 10:00:00", nil}, // same second as one
	})
	ctx := context.Background()
	first, cur, err := engram.Read(ctx, path, engram.DefaultCriteria(), engram.Cursor{})
	if err != nil || len(first) != 2 {
		t.Fatalf("first read = %d, %v", len(first), err)
	}
	again, cur2, err := engram.Read(ctx, path, engram.DefaultCriteria(), cur)
	if err != nil || len(again) != 0 || cur2 != cur {
		t.Fatalf("resume = %+v %+v %v", again, cur2, err)
	}
	// New observation in the same second but with a higher id is still picked up.
	db, _ := sql.Open("sqlite", path)
	db.Exec(`INSERT INTO observations (sync_id,type,title,content,project,updated_at) VALUES ('c','rule','three','x','yhat-knowledge','2026-01-01 10:00:00')`)
	db.Close()
	more, _, err := engram.Read(ctx, path, engram.DefaultCriteria(), cur)
	if err != nil || len(more) != 1 || more[0].Title != "three" {
		t.Fatalf("after insert = %+v %v", more, err)
	}
}

func TestReadNeverWritesAndReportsMissingFile(t *testing.T) {
	path := fixture(t, [][]any{{"a", "rule", "one", "x", "yhat-knowledge", nil, "2026-01-01 10:00:00", nil}})
	before, _ := os.ReadFile(path)
	if _, _, err := engram.Read(context.Background(), path, engram.DefaultCriteria(), engram.Cursor{}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("engram database was modified by Read")
	}
	_, _, err := engram.Read(context.Background(), filepath.Join(t.TempDir(), "nope.db"), engram.DefaultCriteria(), engram.Cursor{})
	if !errors.Is(err, engram.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if _, _, err := engram.Read(context.Background(), path, engram.Criteria{}, engram.Cursor{}); err == nil {
		t.Fatal("empty criteria accepted")
	}
}

func TestCursorRoundTrip(t *testing.T) {
	c := engram.Cursor{UpdatedAt: "2026-01-01 10:00:00", ID: 7}
	back, err := engram.ParseCursor(c.String())
	if err != nil || back != c {
		t.Fatalf("round trip = %+v %v", back, err)
	}
	if z, err := engram.ParseCursor(""); err != nil || z != (engram.Cursor{}) {
		t.Fatalf("zero = %+v %v", z, err)
	}
	if _, err := engram.ParseCursor("garbage"); err == nil {
		t.Fatal("garbage accepted")
	}
}

func TestDefaultPathHonoursEnv(t *testing.T) {
	t.Setenv("ENGRAM_DATA_DIR", "/data/e")
	p, err := engram.DefaultPath()
	if err != nil || p != "/data/e/engram.db" {
		t.Fatalf("path = %q %v", p, err)
	}
}
