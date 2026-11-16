package main

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArcKelMiranda/yhat-agent/internal/store"
)

type obsRow struct {
	typ, title, content, project, key, updated string
}

// engramFixture writes a minimal Engram database and returns its path.
func engramFixture(t *testing.T, rows []obsRow) string {
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
	for i, r := range rows {
		if _, err := db.Exec(`INSERT INTO observations (sync_id,type,title,content,project,topic_key,updated_at)
			VALUES (?,?,?,?,?,?,?)`, "obs-"+string(rune('a'+i)), r.typ, r.title, r.content, r.project, r.key, r.updated); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func importHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("YHAT_HOME", home)
	t.Setenv("YHAT_AUTHOR", "ana")
	s, err := store.Open(filepath.Join(home, "yhat.db"))
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	return home
}

func listAll(t *testing.T, home string) []store.Memory {
	t.Helper()
	s, err := store.Open(filepath.Join(home, "yhat.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ms, _, err := s.ListPendingMemories(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	return ms
}

func TestImportEngram_CreatesProposedProspects(t *testing.T) {
	home := importHome(t)
	eng := engramFixture(t, []obsRow{
		{"decision", "Fuente de verdad", "yhat-knowledge es la fuente", "yhat-knowledge", "", "2026-01-01 10:00:00"},
		{"discovery", "Mapeo A a B", "EntidadA se une con Branch", "other", "yhat.map.entidada-branch", "2026-01-01 10:00:01"},
		{"bugfix", "Ruido de desarrollo", "no es conocimiento", "yhat-mcp-server", "sdd/x", "2026-01-01 10:00:02"},
	})
	var out bytes.Buffer
	if code := runImportEngram([]string{"--db", eng}, &out); code != 0 {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	got := listAll(t, home)
	if len(got) != 2 {
		t.Fatalf("imported %d, want 2: %s", len(got), out.String())
	}
	byTitle := map[string]store.Memory{}
	for _, m := range got {
		byTitle[m.Title] = m
	}
	d := byTitle["Fuente de verdad"]
	if d.Type != "decision" || d.Author != "ana" || d.Status != store.StatusProposed || d.ShareStatus != "none" || d.Origin != "local" {
		t.Errorf("decision prospect wrong: %+v", d)
	}
	m := byTitle["Mapeo A a B"]
	if m.Type != "mapping" {
		t.Errorf("yhat.map key must map to mapping, got %q", m.Type)
	}
	if m.Context == nil || !strings.Contains(*m.Context, "yhat.map.entidada-branch") || !strings.Contains(*m.Context, "obs-b") {
		t.Errorf("provenance missing from context: %v", m.Context)
	}
	if !strings.Contains(out.String(), "2 imported") {
		t.Errorf("summary missing counts: %s", out.String())
	}
}

func TestImportEngram_SecondRunBringsOnlyNew(t *testing.T) {
	home := importHome(t)
	eng := engramFixture(t, []obsRow{{"decision", "Uno", "primero", "yhat-knowledge", "", "2026-01-01 10:00:00"}})
	var out bytes.Buffer
	if code := runImportEngram([]string{"--db", eng}, &out); code != 0 {
		t.Fatalf("first run exit %d", code)
	}
	out.Reset()
	if code := runImportEngram([]string{"--db", eng}, &out); code != 0 || !strings.Contains(out.String(), "0 imported") {
		t.Fatalf("second run: exit %d, %s", code, out.String())
	}
	db, _ := sql.Open("sqlite", eng)
	db.Exec(`INSERT INTO observations (sync_id,type,title,content,project,updated_at) VALUES ('n','rule','Dos','segundo','yhat-knowledge','2026-01-02 10:00:00')`)
	db.Close()
	out.Reset()
	if code := runImportEngram([]string{"--db", eng}, &out); code != 0 || !strings.Contains(out.String(), "1 imported") {
		t.Fatalf("third run: exit %d, %s", code, out.String())
	}
	if n := len(listAll(t, home)); n != 2 {
		t.Fatalf("store has %d memories, want 2", n)
	}
}

func TestImportEngram_DryRunWritesNothing(t *testing.T) {
	home := importHome(t)
	eng := engramFixture(t, []obsRow{{"decision", "Uno", "primero", "yhat-knowledge", "", "2026-01-01 10:00:00"}})
	var out bytes.Buffer
	if code := runImportEngram([]string{"--db", eng, "--dry-run"}, &out); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out.String(), "Uno") || !strings.Contains(out.String(), "dry run") {
		t.Errorf("dry run output should list the prospect: %s", out.String())
	}
	if n := len(listAll(t, home)); n != 0 {
		t.Fatalf("dry run stored %d memories", n)
	}
	// The cursor did not move: a real run still brings it.
	out.Reset()
	if code := runImportEngram([]string{"--db", eng}, &out); code != 0 || !strings.Contains(out.String(), "1 imported") {
		t.Fatalf("real run after dry run: %d %s", code, out.String())
	}
}

func TestImportEngram_SkipsDuplicatesSecretsAndOversized(t *testing.T) {
	home := importHome(t)
	secret := "AWS_SECRET=AKIAIOSFODNN7EXMPL00"
	rows := []obsRow{
		{"decision", "Original", "mismo texto", "yhat-knowledge", "", "2026-01-01 10:00:00"},
		{"decision", "Original", "mismo texto", "yhat-knowledge", "", "2026-01-01 10:00:01"},
		{"decision", "Con secreto", "clave " + secret, "yhat-knowledge", "", "2026-01-01 10:00:02"},
		{"decision", strings.Repeat("t", 201), "titulo largo", "yhat-knowledge", "", "2026-01-01 10:00:03"},
		{"decision", "Contenido largo", strings.Repeat("c", 10001), "yhat-knowledge", "", "2026-01-01 10:00:04"},
	}
	eng := engramFixture(t, rows)
	var out bytes.Buffer
	if code := runImportEngram([]string{"--db", eng}, &out); code != 0 {
		t.Fatalf("exit %d: %s", code, out.String())
	}
	if n := len(listAll(t, home)); n != 1 {
		t.Fatalf("stored %d, want 1", n)
	}
	for _, want := range []string{"1 imported", "1 duplicate", "3 skipped"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("summary lacks %q: %s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "AKIAIOSFODNN7EXMPL00") {
		t.Error("the secret was printed")
	}
}

func TestImportEngram_Errors(t *testing.T) {
	// No local store: do not create one.
	home := t.TempDir()
	t.Setenv("YHAT_HOME", home)
	eng := engramFixture(t, []obsRow{{"decision", "Uno", "x", "yhat-knowledge", "", "2026-01-01 10:00:00"}})
	var out bytes.Buffer
	if code := runImportEngram([]string{"--db", eng}, &out); code != 1 {
		t.Fatalf("no store: exit %d", code)
	}
	if _, err := os.Stat(filepath.Join(home, "yhat.db")); err == nil {
		t.Error("import-engram created a database")
	}
	// Missing Engram database.
	importHome(t)
	out.Reset()
	if code := runImportEngram([]string{"--db", filepath.Join(t.TempDir(), "nope.db")}, &out); code != 1 {
		t.Fatalf("missing engram: exit %d", code)
	}
	// Unknown flag.
	if code := runImportEngram([]string{"--nope"}, &out); code != 2 {
		t.Fatalf("bad flag: exit %d, want 2", code)
	}
}

func TestEngramMemoryType(t *testing.T) {
	cases := []struct{ key, typ, want string }{
		{"yhat.rule.fa.display-separator", "discovery", "rule"},
		{"yhat.obs.x.y", "x", "observation"},
		{"yhat.assum.x.y", "x", "assumption"},
		{"yhat.process.x.y", "x", "process"},
		{"yhat.map.a.b", "x", "mapping"},
		{"yhat.excep.a.b", "x", "exception"},
		{"yhat.def.a.b", "x", "entity-definition"},
		{"yhat.intg.a", "x", "integration"},
		{"yhat.anom.a.b", "x", "anomaly"},
		{"yhat.tech.a.b", "x", "technical-rule"},
		{"yhat.decision.a.b", "x", "decision"},
		{"", "decision", "decision"},
		{"", "bugfix", "anomaly"},
		{"", "config", "technical-rule"},
		{"", "pattern", "process"},
		{"", "architecture", "observation"},
		{"yhat.unknown.a", "bugfix", "anomaly"},
	}
	for _, c := range cases {
		if got := engramMemoryType(c.key, c.typ); got != c.want {
			t.Errorf("engramMemoryType(%q,%q) = %q, want %q", c.key, c.typ, got, c.want)
		}
	}
}
