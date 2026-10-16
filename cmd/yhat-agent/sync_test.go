package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

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

// TestHTTPTransport_E2E_Subprocess starts the mock F3 server as a subprocess,
// then drives HTTPTransport.Send and HTTPTransport.Pull against it.
// It casts 3 votes via the F3 HTTP API, then asserts the memory is approved.
func TestHTTPTransport_E2E_Subprocess(t *testing.T) {
	// Check that the mock F3 binary exists.
	f3bin := "/tmp/yhat-central-bin/yhat-central"
	if _, err := os.Stat(f3bin); os.IsNotExist(err) {
		t.Skipf("mock F3 server not found at %s; skipping e2e test", f3bin)
	}

	port := "28083"
	baseURL := "http://127.0.0.1:" + port

	// Start the mock F3 server as a subprocess.
	proc, err := os.StartProcess(f3bin, []string{f3bin, port}, &os.ProcAttr{
		Files: []*os.File{nil, os.Stdout, os.Stderr},
	})
	if err != nil {
		t.Fatalf("StartProcess mock F3: %v", err)
	}

	// Clean up the subprocess on test exit.
	waitDone := make(chan struct{})
	go func() {
		proc.Wait()
		close(waitDone)
	}()

	// Wait for the server to be ready (up to 5 seconds).
	client := &http.Client{Timeout: 2 * time.Second}
	var ready bool
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		req, _ := http.NewRequest("GET", baseURL+"/api/operator/me", nil)
		req.Header.Set("Authorization", "Bearer test-token-abc")
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ready = true
				break
			}
		}
	}
	if !ready {
		proc.Wait()
		t.Fatal("mock F3 server did not become ready in 5s")
	}

	// Create HTTPTransport.
	httpClient := &http.Client{Timeout: 15 * time.Second}
	ht, err := share.NewHTTPTransport(httpClient, baseURL, "test-token-abc")
	if err != nil {
		t.Fatalf("NewHTTPTransport: %v", err)
	}

	ctx := context.Background()

	// Step 1: Send a memory via HTTPTransport.Send.
	memID := "e2e-test-memory-001"
	payload := share.LocalPayload{
		ID:        memID,
		Origin:    "local",
		Type:      "decision",
		Title:     "E2E Test Decision",
		Content:   "This is an end-to-end test decision.",
		Author:    "e2e-tester",
		CreatedAt: "2024-07-01T00:00:00Z",
	}
	if err := ht.Send(ctx, payload); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Step 2: Cast 3 approvals via the F3 HTTP API (3-of-5 quorum).
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequestWithContext(ctx, "POST",
			baseURL+"/api/team/"+memID+"/approve", nil)
		req.Header.Set("Authorization", "Bearer test-token-abc")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("vote %d: %v", i+1, err)
		}
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			t.Fatalf("vote %d: server returned %s", i+1, resp.Status)
		}
	}

	// Step 3: Pull via HTTPTransport.Pull and verify the memory is approved.
	result, err := ht.Pull(ctx, "")
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}

	var found bool
	for _, mem := range result.Memories {
		if mem.ID == memID {
			found = true
			if mem.TeamStatus != "approved" {
				t.Errorf("TeamStatus: want 'approved', got %q", mem.TeamStatus)
			}
			if mem.Origin != "team" {
				t.Errorf("Origin: want 'team', got %q", mem.Origin)
			}
			if mem.Title != "E2E Test Decision" {
				t.Errorf("Title: got %q", mem.Title)
			}
			break
		}
	}
	if !found {
		t.Errorf("Pull: memory %s not found in team memories; got %v", memID, result.Memories)
	}

	// Step 4: Verify /api/operator/me returns the operator name.
	req, _ := http.NewRequestWithContext(ctx, "GET", baseURL+"/api/operator/me", nil)
	req.Header.Set("Authorization", "Bearer test-token-abc")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /api/operator/me: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/operator/me: got %s", resp.Status)
	}
	var operator map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&operator); err != nil {
		t.Fatalf("decode operator response: %v", err)
	}
	if operator["name"] != "test-operator" {
		t.Errorf("operator name: want 'test-operator', got %q", operator["name"])
	}

	// Clean up: signal the server to stop.
	proc.Signal(syscall.SIGTERM)
	// Wait for the process to exit with a timeout.
	select {
	case <-waitDone:
		// Process exited cleanly.
	case <-time.After(5 * time.Second):
		// Force kill if it doesn't exit cleanly.
		proc.Kill()
	}
}
