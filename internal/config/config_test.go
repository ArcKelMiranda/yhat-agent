package config

import (
	"os"
	"path/filepath"
	"testing"
)

// RED: round-trip — Save then Load must produce equivalent data.
func TestConfigRoundTrip(t *testing.T) {
	cfg := Config{
		Operator:    "kelvin.miranda",
		CentralRepo: "https://centro.example.invalid",
		LastSync:    "2025-06-01T12:00:00Z",
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Operator != cfg.Operator {
		t.Errorf("Operator: got %q, want %q", loaded.Operator, cfg.Operator)
	}
	if loaded.CentralRepo != cfg.CentralRepo {
		t.Errorf("CentralRepo: got %q, want %q", loaded.CentralRepo, cfg.CentralRepo)
	}
	if loaded.LastSync != cfg.LastSync {
		t.Errorf("LastSync: got %q, want %q", loaded.LastSync, cfg.LastSync)
	}
}

// RED: missing file returns an error.
func TestConfigMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	if err == nil {
		t.Error("expected error for missing file")
	}
}

// RED: malformed YAML returns an error.
func TestConfigMalformedYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("operator: [unclosed"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := Load(path)
	if err == nil {
		t.Error("expected error for malformed YAML")
	}
}

// RED: State round-trip.
func TestStateRoundTrip(t *testing.T) {
	st := State{
		Version:           "v1.2.3",
		RegisteredClients: []string{"opencode", "claude"},
		SchemaVersion:     1,
		LastSync:          "2025-06-01T12:00:00Z",
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := st.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if loaded.Version != st.Version {
		t.Errorf("Version: got %q, want %q", loaded.Version, st.Version)
	}
	if len(loaded.RegisteredClients) != len(st.RegisteredClients) {
		t.Errorf("RegisteredClients len: got %d, want %d", len(loaded.RegisteredClients), len(st.RegisteredClients))
	}
	if loaded.SchemaVersion != st.SchemaVersion {
		t.Errorf("SchemaVersion: got %d, want %d", loaded.SchemaVersion, st.SchemaVersion)
	}
	if loaded.LastSync != st.LastSync {
		t.Errorf("LastSync: got %q, want %q", loaded.LastSync, st.LastSync)
	}
}

// RED: EnsureDir creates the directory and parents.
func TestEnsureDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b", "c")
	if err := EnsureDir(dir); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
}

// RED: YHAT_HOME env overrides default.
func TestDefaultHomeYHAT_HOME(t *testing.T) {
	orig := os.Getenv("YHAT_HOME")
	defer func() {
		if orig != "" {
			os.Setenv("YHAT_HOME", orig)
		} else {
			os.Unsetenv("YHAT_HOME")
		}
	}()
	custom := t.TempDir()
	os.Setenv("YHAT_HOME", custom)
	// Reset the package-level cache by re-importing or by calling the exported function.
	got := DefaultHome()
	if got != custom {
		t.Errorf("DefaultHome with YHAT_HOME: got %q, want %q", got, custom)
	}
}

// RED: on Linux, DefaultHome falls back to $HOME/.yhat/.
func TestDefaultHomeLinuxFallback(t *testing.T) {
	origHome := os.Getenv("HOME")
	origYHAT := os.Getenv("YHAT_HOME")
	defer func() {
		if origHome != "" {
			os.Setenv("HOME", origHome)
		}
		if origYHAT != "" {
			os.Setenv("YHAT_HOME", origYHAT)
		} else {
			os.Unsetenv("YHAT_HOME")
		}
	}()
	os.Unsetenv("YHAT_HOME")
	os.Setenv("HOME", "/home/testuser")
	want := "/home/testuser/.yhat"
	got := DefaultHome()
	if got != want {
		t.Errorf("DefaultHome fallback: got %q, want %q", got, want)
	}
}
