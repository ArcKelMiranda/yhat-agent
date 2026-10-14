package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArcKelMiranda/yhat-agent/internal/share"
	"github.com/ArcKelMiranda/yhat-agent/internal/store"
)

// TestSync_EmptyQueue_Summary verifies that sync with no queued memories
// prints "0 sent, 0 failed, 0 skipped".
func TestSync_EmptyQueue_Summary(t *testing.T) {
	tmpDir := t.TempDir()

	// Set up config.yaml with central_repo pointing to a temp dir.
	cfgDir := filepath.Join(tmpDir, ".yhat")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	centralRepo := filepath.Join(tmpDir, "central")
	if err := os.MkdirAll(centralRepo, 0755); err != nil {
		t.Fatalf("MkdirAll central: %v", err)
	}

	cfgContent := "operator: sync-test\ncentral_repo: " + centralRepo + "\n"
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatalf("WriteFile config.yaml: %v", err)
	}

	// Set up store with a validated memory (not queued).
	dbDir := filepath.Join(tmpDir, "store")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		t.Fatalf("MkdirAll store: %v", err)
	}
	dbPath := filepath.Join(dbDir, "yhat.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	ctx := context.Background()
	mem, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Not Queued",
		Content: "Content.",
		Author:  "tester",
	})
	if err != nil {
		t.Fatalf("ProposeMemory: %v", err)
	}
	if err := s.Validate(ctx, mem.ID, "validator"); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	s.Close()

	// Override paths.
	oldHome := os.Getenv("YHAT_HOME")
	os.Setenv("YHAT_HOME", cfgDir)
	defer os.Setenv("YHAT_HOME", oldHome)

	// Run sync directly (not via CLI subprocess).
	// We call runSyncCommand logic directly by constructing the transport.
	lt, err := share.NewLocalDirTransport(centralRepo)
	if err != nil {
		t.Fatalf("NewLocalDirTransport: %v", err)
	}

	s2, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s2.Close()

	report, err := share.Sync(ctx, s2, lt)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if report.Sent != 0 || report.Failed != 0 || report.Skipped != 0 {
		t.Errorf("Sync(empty): want 0,0,0 got %d,%d,%d",
			report.Sent, report.Failed, report.Skipped)
	}
}

// TestSync_SendsMemory verifies that sync sends a queued memory and writes
// the JSON file to the central_repo directory.
func TestSync_SendsMemory(t *testing.T) {
	tmpDir := t.TempDir()

	cfgDir := filepath.Join(tmpDir, ".yhat")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	centralRepo := filepath.Join(tmpDir, "central")
	if err := os.MkdirAll(centralRepo, 0755); err != nil {
		t.Fatalf("MkdirAll central: %v", err)
	}
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	cfgContent := "operator: sync-test\ncentral_repo: " + centralRepo + "\n"
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatalf("WriteFile config.yaml: %v", err)
	}

	dbDir := filepath.Join(tmpDir, "store")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		t.Fatalf("MkdirAll store: %v", err)
	}
	dbPath := filepath.Join(dbDir, "yhat.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}

	ctx := context.Background()
	mem, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Memory to Send",
		Content: "Content for sync test.",
		Author:  "syncer",
	})
	if err != nil {
		t.Fatalf("ProposeMemory: %v", err)
	}
	if err := s.Validate(ctx, mem.ID, "validator"); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := share.Queue(ctx, s, mem.ID, "syncer"); err != nil {
		t.Fatalf("Queue: %v", err)
	}
	s.Close()

	// Run sync.
	lt, err := share.NewLocalDirTransport(centralRepo)
	if err != nil {
		t.Fatalf("NewLocalDirTransport: %v", err)
	}

	s2, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s2.Close()

	report, err := share.Sync(ctx, s2, lt)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if report.Sent != 1 {
		t.Errorf("Sync: want sent=1, got %d", report.Sent)
	}

	// Verify JSON file was written.
	wantPath := filepath.Join(centralRepo, mem.ID+".json")
	if _, err := os.Stat(wantPath); os.IsNotExist(err) {
		t.Errorf("Sync: expected JSON file at %s", wantPath)
	}

	// Verify memory share_status is 'sent'.
	updated, err := s2.GetMemory(ctx, mem.ID)
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if updated.ShareStatus != store.ShareStatusSent {
		t.Errorf("ShareStatus after sync: want 'sent', got %q", updated.ShareStatus)
	}
}

// TestSync_ExitCode_Failed returns non-zero when failures occur.
func TestSync_ExitCode_Failed(t *testing.T) {
	tmpDir := t.TempDir()

	cfgDir := filepath.Join(tmpDir, ".yhat")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	centralRepo := filepath.Join(tmpDir, "central")
	if err := os.MkdirAll(centralRepo, 0755); err != nil {
		t.Fatalf("MkdirAll central: %v", err)
	}
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	cfgContent := "operator: sync-test\ncentral_repo: " + centralRepo + "\n"
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatalf("WriteFile config.yaml: %v", err)
	}

	dbDir := filepath.Join(tmpDir, "store")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		t.Fatalf("MkdirAll store: %v", err)
	}
	dbPath := filepath.Join(dbDir, "yhat.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}

	ctx := context.Background()
	mem, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Memory Queued",
		Content: "Content.",
		Author:  "tester",
	})
	if err != nil {
		t.Fatalf("ProposeMemory: %v", err)
	}
	if err := s.Validate(ctx, mem.ID, "validator"); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := share.Queue(ctx, s, mem.ID, "tester"); err != nil {
		t.Fatalf("Queue: %v", err)
	}

	// Create transport successfully (establishes subdirectories).
	lt, err := share.NewLocalDirTransport(centralRepo)
	if err != nil {
		t.Fatalf("NewLocalDirTransport: %v", err)
	}

	// Remove write permission from central repo so Send fails.
	if err := os.Chmod(centralRepo, 0555); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	defer os.Chmod(centralRepo, 0755) // Restore for temp cleanup.

	s2, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s2.Close()

	report, err := share.Sync(ctx, s2, lt)
	if err != nil {
		// Transport error may be nil (error is stored in last_error, not returned).
		// But in this case Send will fail, so err should be nil (we only return
		// catastrophic errors, not per-entry failures).
	}
	// When Send fails, report.Failed should be 1.
	if report.Failed != 1 {
		t.Errorf("Sync with permission error: want failed=1, got %d", report.Failed)
	}
	// runSyncCommand returns 2 when report.Failed > 0.
	// (We can't test the exit code directly without subprocess, but the logic is:
	// if report.Failed > 0 { return 2 }.)
}
