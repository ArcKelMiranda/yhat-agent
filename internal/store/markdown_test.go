package store_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArcKelMiranda/yhat-agent/internal/store"
)

func openMD(t *testing.T) (*store.Store, string) {
	t.Helper()
	dbPath, cleanup := newStoreDir(t)
	t.Cleanup(cleanup)
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, filepath.Join(filepath.Dir(dbPath), "memories")
}

func propose(t *testing.T, s *store.Store, title string) store.Memory {
	t.Helper()
	ctx := "contexto de prueba"
	m, err := s.ProposeMemory(context.Background(), store.Memory{
		Type: store.MemoryTypeDecision, Title: title, Content: "Contenido de la memoria.", Context: &ctx, Author: "ana",
	})
	if err != nil {
		t.Fatalf("ProposeMemory: %v", err)
	}
	return m
}

func readMD(t *testing.T, dir, id string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, id+".md"))
	if err != nil {
		t.Fatalf("read md: %v", err)
	}
	return string(b)
}

func TestMarkdown_ProposeWritesFile(t *testing.T) {
	s, dir := openMD(t)
	m := propose(t, s, "Regla de redondeo")
	got := readMD(t, dir, m.ID)
	for _, want := range []string{
		"---\n", `id: "` + m.ID + `"`, "type: decision", "status: proposed", "origin: local",
		`author: "ana"`, "share_status: none", "created_at: ", `title: "Regla de redondeo"`,
		"# Regla de redondeo", "Contenido de la memoria.", "## Contexto", "contexto de prueba",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("md missing %q:\n%s", want, got)
		}
	}
	info, _ := os.Stat(filepath.Join(dir, m.ID+".md"))
	if info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestMarkdown_StateChangesAreMirrored(t *testing.T) {
	s, dir := openMD(t)
	ctx := context.Background()

	a := propose(t, s, "Aprobada")
	if err := s.Validate(ctx, a.ID, "luis"); err != nil {
		t.Fatal(err)
	}
	if got := readMD(t, dir, a.ID); !strings.Contains(got, "status: validated") || !strings.Contains(got, `validated_by: "luis"`) {
		t.Errorf("validate not mirrored:\n%s", got)
	}

	r := propose(t, s, "Rechazada")
	if err := s.Reject(ctx, r.ID, "luis", "no aplica"); err != nil {
		t.Fatal(err)
	}
	if got := readMD(t, dir, r.ID); !strings.Contains(got, "status: rejected") || !strings.Contains(got, `reject_reason: "no aplica"`) {
		t.Errorf("reject not mirrored:\n%s", got)
	}

	e := propose(t, s, "Editable")
	nc := "nuevo contexto"
	if err := s.Edit(ctx, e.ID, "Titulo nuevo", "Contenido nuevo", &nc); err != nil {
		t.Fatal(err)
	}
	if got := readMD(t, dir, e.ID); !strings.Contains(got, "# Titulo nuevo") || !strings.Contains(got, "Contenido nuevo") || strings.Contains(got, "Editable") {
		t.Errorf("edit not mirrored:\n%s", got)
	}

	if err := s.SetShareStatus(ctx, a.ID, "queued"); err != nil {
		t.Fatal(err)
	}
	if got := readMD(t, dir, a.ID); !strings.Contains(got, "share_status: queued") {
		t.Errorf("share status not mirrored:\n%s", got)
	}

	if err := s.Archive(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	if got := readMD(t, dir, r.ID); !strings.Contains(got, "status: archived") {
		t.Errorf("archive not mirrored:\n%s", got)
	}
}

func TestMarkdown_FrontmatterSurvivesHostileText(t *testing.T) {
	s, dir := openMD(t)
	m := propose(t, s, "Titulo \"con comillas\"\nsegunda linea: ---")
	got := readMD(t, dir, m.ID)
	front := strings.SplitN(got, "\n---\n", 2)[0]
	if strings.Count(front, "\ntitle:") != 1 || strings.Contains(front, "\nsegunda") {
		t.Errorf("title broke the frontmatter:\n%s", front)
	}
}

func TestMarkdown_ExportRebuildsMissingFiles(t *testing.T) {
	s, dir := openMD(t)
	a, b := propose(t, s, "Una"), propose(t, s, "Dos")
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	n, err := s.ExportMarkdown(context.Background())
	if err != nil || n != 2 {
		t.Fatalf("ExportMarkdown = %d, %v; want 2, nil", n, err)
	}
	readMD(t, dir, a.ID)
	readMD(t, dir, b.ID)
}

func TestMarkdown_WriteFailureDoesNotFailTheDatabaseOperation(t *testing.T) {
	dbPath, cleanup := newStoreDir(t)
	defer cleanup()
	dir := filepath.Dir(dbPath)
	if err := os.WriteFile(filepath.Join(dir, "memories"), []byte("soy un archivo"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	m := propose(t, s, "Sigue guardando en SQLite")
	if got, err := s.GetMemory(context.Background(), m.ID); err != nil || got.Title != m.Title {
		t.Fatalf("memory not in SQLite: %v, %v", got.Title, err)
	}
	if _, err := s.ExportMarkdown(context.Background()); err == nil {
		t.Error("ExportMarkdown should report the write failure")
	}
}

func TestMarkdown_WriteRejectsUnsafeIDs(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []string{"", "../x", "a/b", `a\b`, ".hidden", "a b"} {
		if err := store.WriteMarkdown(dir, store.Memory{ID: id, Title: "t", Content: "c"}); err == nil {
			t.Errorf("id %q accepted", id)
		}
	}
}

func TestMarkdown_RelativeAndMemoryPathsDoNotMirror(t *testing.T) {
	cwd := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	for _, p := range []string{"rel.db", ":memory:"} {
		s, err := store.Open(p)
		if err != nil {
			t.Fatal(err)
		}
		if s.MarkdownDir() != "" {
			t.Errorf("%q: MarkdownDir = %q, want empty", p, s.MarkdownDir())
		}
		propose(t, s, "Sin espejo "+p)
		s.Close()
	}
	if _, err := os.Stat(filepath.Join(cwd, "memories")); err == nil {
		t.Error("created ./memories for a relative DB path")
	}
}
