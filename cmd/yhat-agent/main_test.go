package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArcKelMiranda/yhat-agent/internal/config"
	"github.com/ArcKelMiranda/yhat-agent/internal/store"
)

// RED: runInstallF1 writes config.yaml and state.json to the YHAT_HOME temp dir,
// creates yhat.db, and returns no error.
func TestRunInstallF1(t *testing.T) {
	dir := t.TempDir()
	origYHAT := os.Getenv("YHAT_HOME")
	os.Setenv("YHAT_HOME", dir)
	defer func() {
		if origYHAT != "" {
			os.Setenv("YHAT_HOME", origYHAT)
		} else {
			os.Unsetenv("YHAT_HOME")
		}
	}()

	// Also set USER so the operator name is deterministic.
	origUSER := os.Getenv("USER")
	os.Setenv("USER", "testoperator")
	defer func() {
		if origUSER != "" {
			os.Setenv("USER", origUSER)
		} else {
			os.Unsetenv("USER")
		}
	}()

	if err := runInstallF1(); err != nil {
		t.Fatalf("runInstallF1: %v", err)
	}

	// config.yaml must exist and parse.
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.Operator != "testoperator" {
		t.Errorf("operator: got %q, want %q", cfg.Operator, "testoperator")
	}
	if cfg.CentralRepo != "https://centro.example.invalid" {
		t.Errorf("central_repo: got %q", cfg.CentralRepo)
	}

	// state.json must exist and parse.
	stPath := filepath.Join(dir, "state.json")
	st, err := config.LoadState(stPath)
	if err != nil {
		t.Fatalf("config.LoadState: %v", err)
	}
	if st.Version == "" {
		t.Error("version should not be empty")
	}
	if len(st.RegisteredClients) != 2 {
		t.Errorf("registered_clients len: got %d, want 2", len(st.RegisteredClients))
	}
	if st.SchemaVersion != store.CurrentSchemaVersion {
		t.Errorf("schema_version: got %d, want %d", st.SchemaVersion, store.CurrentSchemaVersion)
	}
	if st.LastSync != "" {
		t.Errorf("last_sync: got %q, want empty", st.LastSync)
	}

	// yhat.db must exist.
	dbPath := filepath.Join(dir, "yhat.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("yhat.db was not created")
	}

	// Store must open with correct schema.
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()
	sv, err := s.GetSyncState(context.Background(), "schema_version")
	if err != nil {
		t.Fatalf("GetSyncState schema_version: %v", err)
	}
	if sv != "1" {
		t.Errorf("schema_version in DB: got %q, want \"1\"", sv)
	}
}

// RED: runStatusCerebro shows correct counts on an empty store.
func TestRunStatusCerebroEmpty(t *testing.T) {
	dir := t.TempDir()
	origYHAT := os.Getenv("YHAT_HOME")
	os.Setenv("YHAT_HOME", dir)
	defer func() {
		if origYHAT != "" {
			os.Setenv("YHAT_HOME", origYHAT)
		} else {
			os.Unsetenv("YHAT_HOME")
		}
	}()

	// Create minimal config.yaml.
	cfg := config.Config{Operator: "testuser", CentralRepo: "https://example.com", LastSync: ""}
	if err := cfg.Save(config.ConfigPath()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	st := config.State{Version: "v1.0.0", RegisteredClients: []string{"opencode"}, SchemaVersion: 1, LastSync: ""}
	if err := st.Save(config.StatePath()); err != nil {
		t.Fatalf("save state: %v", err)
	}

	cs := runStatusCerebro(false)

	if cs.HomePath != dir {
		t.Errorf("HomePath: got %q, want %q", cs.HomePath, dir)
	}
	if cs.DBPath != filepath.Join(dir, "yhat.db") {
		t.Errorf("DBPath: got %q", cs.DBPath)
	}
	// No DB file yet — DBExists should be false.
	if cs.DBExists {
		t.Error("DBExists: expected false when no DB file")
	}
	if cs.Operator != "testuser" {
		t.Errorf("Operator: got %q, want %q", cs.Operator, "testuser")
	}
	// Empty store → all counts zero.
	if cs.Counts["proposed"] != 0 {
		t.Errorf("proposed count: got %d, want 0", cs.Counts["proposed"])
	}
	if cs.Counts["validated"] != 0 {
		t.Errorf("validated count: got %d, want 0", cs.Counts["validated"])
	}
}

// RED: runStatusCerebro shows correct counts after inserting memories.
func TestRunStatusCerebroWithMemories(t *testing.T) {
	dir := t.TempDir()
	origYHAT := os.Getenv("YHAT_HOME")
	os.Setenv("YHAT_HOME", dir)
	defer func() {
		if origYHAT != "" {
			os.Setenv("YHAT_HOME", origYHAT)
		} else {
			os.Unsetenv("YHAT_HOME")
		}
	}()

	cfg := config.Config{Operator: "testuser", CentralRepo: "https://example.com", LastSync: ""}
	_ = cfg.Save(config.ConfigPath())
	st := config.State{Version: "v1.0.0", RegisteredClients: []string{"opencode"}, SchemaVersion: 1, LastSync: ""}
	_ = st.Save(config.StatePath())

	dbPath := filepath.Join(dir, "yhat.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Insert 2 proposed, 1 validated, 1 rejected.
	mem1, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Test decision 1",
		Content: "Content for decision 1",
		Author:  "testuser",
	})
	if err != nil {
		t.Fatalf("ProposeMemory 1: %v", err)
	}
	mem2, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeRule,
		Title:   "Test rule 1",
		Content: "Content for rule 1",
		Author:  "testuser",
	})
	if err != nil {
		t.Fatalf("ProposeMemory 2: %v", err)
	}
	mem3, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeAnomaly,
		Title:   "Test anomaly 1",
		Content: "Content for anomaly 1",
		Author:  "testuser",
	})
	if err != nil {
		t.Fatalf("ProposeMemory 3: %v", err)
	}
	mem4, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeImprovement,
		Title:   "Test improvement 1",
		Content: "Content for improvement 1",
		Author:  "testuser",
	})
	if err != nil {
		t.Fatalf("ProposeMemory 4: %v", err)
	}

	// Validate mem3.
	if err := s.Validate(ctx, mem3.ID, "validator"); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	// Reject mem4.
	if err := s.Reject(ctx, mem4.ID, "rejector", "not relevant"); err != nil {
		t.Fatalf("Reject: %v", err)
	}

	// Validate mem3, reject mem4, archive mem1.
	if err := s.Validate(ctx, mem3.ID, "validator"); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := s.Reject(ctx, mem4.ID, "rejector", "not relevant"); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if err := s.Archive(ctx, mem1.ID); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	// Final state: mem1 archived, mem2 proposed, mem3 validated, mem4 rejected.
	cs := runStatusCerebro(false)
	if cs.Counts["proposed"] != 1 {
		t.Errorf("proposed: got %d, want 1", cs.Counts["proposed"])
	}
	if cs.Counts["validated"] != 1 {
		t.Errorf("validated: got %d, want 1", cs.Counts["validated"])
	}
	if cs.Counts["rejected"] != 1 {
		t.Errorf("rejected: got %d, want 1", cs.Counts["rejected"])
	}
	if cs.Counts["archived"] != 1 {
		t.Errorf("archived: got %d, want 1", cs.Counts["archived"])
	}
	_ = mem2 // used above
}

// RED: runStatusCerebro tolerates missing config.yaml and state.json.
func TestRunStatusCerebroMissingFiles(t *testing.T) {
	dir := t.TempDir()
	origYHAT := os.Getenv("YHAT_HOME")
	os.Setenv("YHAT_HOME", dir)
	defer func() {
		if origYHAT != "" {
			os.Setenv("YHAT_HOME", origYHAT)
		} else {
			os.Unsetenv("YHAT_HOME")
		}
	}()

	cs := runStatusCerebro(false)
	// Should not panic; operator defaults to empty (printed as "?").
	if cs.Operator != "" {
		t.Errorf("Operator: got %q, want empty string when config missing", cs.Operator)
	}
	if cs.ConfigError == "" {
		t.Error("ConfigError should be set when config missing")
	}
	if cs.DBExists {
		t.Error("DBExists: expected false")
	}
	if cs.Counts["proposed"] != 0 {
		t.Errorf("proposed: got %d, want 0", cs.Counts["proposed"])
	}
}

// RED: runStatusCerebroText output contains expected lines.
func TestRunStatusCerebroTextOutput(t *testing.T) {
	dir := t.TempDir()
	origYHAT := os.Getenv("YHAT_HOME")
	os.Setenv("YHAT_HOME", dir)
	defer func() {
		if origYHAT != "" {
			os.Setenv("YHAT_HOME", origYHAT)
		} else {
			os.Unsetenv("YHAT_HOME")
		}
	}()

	cfg := config.Config{Operator: "jane.doe", CentralRepo: "https://central.invalid", LastSync: ""}
	_ = cfg.Save(config.ConfigPath())
	st := config.State{Version: "v1.2.0", RegisteredClients: []string{"opencode", "claude"}, SchemaVersion: 1, LastSync: ""}
	_ = st.Save(config.StatePath())

	// Create DB so "schema v1" appears instead of "not initialised".
	dbPath := filepath.Join(dir, "yhat.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	_ = s.Close()

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cs := runStatusCerebro(false)
	runStatusCerebroText(cs)

	w.Close()
	os.Stdout = oldStdout

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "YHat home:") {
		t.Error("output missing 'YHat home:'")
	}
	if !strings.Contains(output, dir) {
		t.Error("output missing home dir path")
	}
	if !strings.Contains(output, "DB:") {
		t.Error("output missing 'DB:'")
	}
	if !strings.Contains(output, "schema v1") {
		t.Error("output missing 'schema v1'")
	}
	if !strings.Contains(output, "Memories:") {
		t.Error("output missing 'Memories:'")
	}
	if !strings.Contains(output, "jane.doe") {
		t.Error("output missing operator name")
	}
	if !strings.Contains(output, "Last sync:") {
		t.Error("output missing 'Last sync:'")
	}
}
