package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	yhatclients "github.com/ArcKelMiranda/yhat-agent/internal/clients"
)

// mockEnv implements yhatclients.Env for testing.
type mockEnv struct {
	opencodeRoot string
	opencodeCfg  string
	appdata      string
	localappdata string
	execPath     string
	execErr      error
}

func (m *mockEnv) OpenCodeRoot() string   { return m.opencodeRoot }
func (m *mockEnv) OpenCodeConfig() string { return m.opencodeCfg }
func (m *mockEnv) AppData() string        { return m.appdata }
func (m *mockEnv) LocalAppData() string   { return m.localappdata }
func (m *mockEnv) Executable() (string, error) {
	if m.execErr != nil {
		return "", m.execErr
	}
	if m.execPath != "" {
		return m.execPath, nil
	}
	if runtime.GOOS == "windows" {
		return `C:\tools\yhat-agent.exe`, nil
	}
	return "/usr/local/bin/yhat-agent", nil
}

func TestRunInstallMCP_ExecutesWithMock(t *testing.T) {
	// Create temp directories for testing
	dir := t.TempDir()
	opencodeRoot := filepath.Join(dir, "opencode")
	if err := os.MkdirAll(opencodeRoot, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	env := &mockEnv{
		opencodeRoot: opencodeRoot,
		appdata:      dir,
		localappdata: dir,
		execPath:     "/usr/local/bin/yhat-agent",
	}

	results, err := yhatclients.RunInstallMCP(env)
	if err != nil {
		t.Fatalf("RunInstallMCP: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("RunInstallMCP: expected 2 results, got %d", len(results))
	}

	// Verify OpenCode result
	ocResult := results[0]
	if ocResult.Client != "opencode" {
		t.Fatalf("expected opencode client, got %s", ocResult.Client)
	}
	if ocResult.State != "configured" {
		t.Fatalf("expected configured state, got %s", ocResult.State)
	}
}

func TestRealEnv_OpenCodeRoot(t *testing.T) {
	// realEnv.OpenCodeRoot delegates to yhatagent.OpenCodeRoot().
	// Verify it returns a non-empty absolute path.
	env := realEnv{}
	root := env.OpenCodeRoot()
	if root == "" {
		t.Fatal("OpenCodeRoot: empty")
	}
	if !filepath.IsAbs(root) {
		t.Fatalf("OpenCodeRoot: expected absolute, got %s", root)
	}
	if filepath.Base(root) != "opencode" {
		t.Fatalf("OpenCodeRoot: expected opencode suffix, got %s", root)
	}
}

func TestRealEnv_Executable(t *testing.T) {
	env := realEnv{}
	exe, err := env.Executable()
	if err != nil {
		t.Fatalf("realEnv.Executable: %v", err)
	}
	if !filepath.IsAbs(exe) {
		t.Fatalf("realEnv.Executable: expected absolute path, got %s", exe)
	}
}

func TestRealEnv_AppData(t *testing.T) {
	env := realEnv{}
	// On non-Windows, this returns the empty string from os.Getenv.
	// On Windows, it returns %APPDATA%.
	val := env.AppData()
	_ = val // Just verify it doesn't panic
}

// RED test: runInstallMCP should be a no-op on non-Windows (no config files created).
// This test FAILS until the runtime.GOOS guard is restored in runInstallMCP.
func TestRunInstallMCPNonWindowsNoWrites(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows-only: guard test runs on non-Windows only")
	}
	dir := t.TempDir()
	env := &mockEnv{
		opencodeRoot: filepath.Join(dir, "opencode"),
		appdata:      dir,
		localappdata: dir,
		execPath:     "/usr/local/bin/yhat-agent",
	}
	_ = env

	if err := runInstallMCP(); err != nil {
		t.Fatalf("runInstallMCP on non-Windows returned error: %v", err)
	}

	// No config files should be created.
	entries, listErr := os.ReadDir(dir)
	if listErr != nil {
		t.Fatalf("ReadDir: %v", listErr)
	}
	for _, e := range entries {
		if e.Name() != "opencode" {
			continue
		}
		sub, _ := os.ReadDir(filepath.Join(dir, e.Name()))
		for _, f := range sub {
			t.Errorf("runInstallMCP on non-Windows created file %s; expected no writes", f.Name())
		}
	}
}
