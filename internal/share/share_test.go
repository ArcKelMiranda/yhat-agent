package share_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArcKelMiranda/yhat-agent/internal/share"
	"github.com/ArcKelMiranda/yhat-agent/internal/store"
)

// ---------------------------------------------------------------------------
// RED phase: write tests that capture the expected failure, then implement.
// ---------------------------------------------------------------------------

// TestQueue_HappyPath validates that a validated memory with share_status=none
// is enqueued and its share_status is flipped to 'queued'.
func TestQueue_HappyPath(t *testing.T) {
	dbPath := t.TempDir() + "/queue_happy.db"
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Propose and validate a memory.
	mem, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Shareable Memory",
		Content: "Content for Queue happy path test.",
		Author:  "tester",
	})
	if err != nil {
		t.Fatalf("ProposeMemory: %v", err)
	}
	if err := s.Validate(ctx, mem.ID, "validator"); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	// Queue it.
	if err := share.Queue(ctx, s, mem.ID, "operator-x"); err != nil {
		t.Fatalf("Queue: %v", err)
	}

	// Verify share_status is 'queued'.
	updated, err := s.GetMemory(ctx, mem.ID)
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if updated.ShareStatus != store.ShareStatusQueued {
		t.Errorf("ShareStatus after Queue: want 'queued', got %q", updated.ShareStatus)
	}

	// Verify queue entry exists.
	queue, err := s.ListUploadQueue(ctx)
	if err != nil {
		t.Fatalf("ListUploadQueue: %v", err)
	}
	var found bool
	for _, e := range queue {
		if e.MemoryID == mem.ID && e.OrderedBy == "operator-x" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Queue: expected entry in upload_queue")
	}
}

// TestQueue_WrongStatus_Rejected validates that a rejected memory is not shareable.
func TestQueue_WrongStatus_Rejected(t *testing.T) {
	dbPath := t.TempDir() + "/queue_wrong.db"
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	mem, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeRule,
		Title:   "Rule to Reject",
		Content: "This rule will be rejected.",
		Author:  "tester",
	})
	if err != nil {
		t.Fatalf("ProposeMemory: %v", err)
	}
	// Reject it.
	if err := s.Reject(ctx, mem.ID, "validator", "not shareable"); err != nil {
		t.Fatalf("Reject: %v", err)
	}

	err = share.Queue(ctx, s, mem.ID, "operator-x")
	if err == nil {
		t.Fatal("Queue(rejected memory): expected error")
	}
	if !errors.Is(err, share.ErrNotShareable) {
		t.Errorf("Queue(rejected memory): want ErrNotShareable, got %v", err)
	}
}

// TestQueue_WrongStatus_Proposed validates that a proposed memory is not shareable.
func TestQueue_WrongStatus_Proposed(t *testing.T) {
	dbPath := t.TempDir() + "/queue_proposed.db"
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	mem, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeImprovement,
		Title:   "Proposed Improvement",
		Content: "Not yet validated.",
		Author:  "tester",
	})
	if err != nil {
		t.Fatalf("ProposeMemory: %v", err)
	}

	err = share.Queue(ctx, s, mem.ID, "operator-x")
	if err == nil {
		t.Fatal("Queue(proposed memory): expected error")
	}
	if !errors.Is(err, share.ErrNotShareable) {
		t.Errorf("Queue(proposed memory): want ErrNotShareable, got %v", err)
	}
}

// TestQueue_AlreadyQueued validates that a memory already queued is rejected.
func TestQueue_AlreadyQueued(t *testing.T) {
	dbPath := t.TempDir() + "/queue_already.db"
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	mem, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Already Queued Memory",
		Content: "Content.",
		Author:  "tester",
	})
	if err != nil {
		t.Fatalf("ProposeMemory: %v", err)
	}
	if err := s.Validate(ctx, mem.ID, "validator"); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	// Queue once.
	if err := share.Queue(ctx, s, mem.ID, "operator-x"); err != nil {
		t.Fatalf("Queue first: %v", err)
	}

	// Try to queue again.
	err = share.Queue(ctx, s, mem.ID, "operator-y")
	if err == nil {
		t.Fatal("Queue(already queued): expected error")
	}
	if !errors.Is(err, share.ErrNotShareable) {
		t.Errorf("Queue(already queued): want ErrNotShareable, got %v", err)
	}
}

// TestQueue_MissingMemory validates that an unknown ID returns store.ErrNotFound.
func TestQueue_MissingMemory(t *testing.T) {
	dbPath := t.TempDir() + "/queue_missing.db"
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	err = share.Queue(ctx, s, "00000000-0000-0000-0000-000000000000", "operator-x")
	if err == nil {
		t.Fatal("Queue(unknown ID): expected error")
	}
	// Should be store.ErrNotFound, not ErrNotShareable.
	if errors.Is(err, share.ErrNotShareable) {
		t.Errorf("Queue(unknown ID): want store.ErrNotFound, got ErrNotShareable")
	}
}

// TestSync_EmptyQueue returns zero counters.
func TestSync_EmptyQueue(t *testing.T) {
	dbPath := t.TempDir() + "/sync_empty.db"
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	transportDir := t.TempDir()
	lt, err := share.NewLocalDirTransport(transportDir)
	if err != nil {
		t.Fatalf("NewLocalDirTransport: %v", err)
	}

	ctx := context.Background()
	report, err := share.Sync(ctx, s, lt)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if report.Sent != 0 || report.Failed != 0 || report.Skipped != 0 {
		t.Errorf("Sync(empty queue): want 0,0,0 got %d,%d,%d",
			report.Sent, report.Failed, report.Skipped)
	}
}

// TestSync_SendsQueuedMemory validates that a queued memory is sent and
// share_status is flipped to 'sent'.
func TestSync_SendsQueuedMemory(t *testing.T) {
	dbPath := t.TempDir() + "/sync_happy.db"
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	mem, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Memory to Sync",
		Content: "Content for sync happy path.",
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

	transportDir := t.TempDir()
	lt, err := share.NewLocalDirTransport(transportDir)
	if err != nil {
		t.Fatalf("NewLocalDirTransport: %v", err)
	}

	report, err := share.Sync(ctx, s, lt)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if report.Sent != 1 {
		t.Errorf("Sync: want sent=1, got %d", report.Sent)
	}
	if report.Failed != 0 {
		t.Errorf("Sync: want failed=0, got %d", report.Failed)
	}

	// Verify the memory is now 'sent'.
	updated, err := s.GetMemory(ctx, mem.ID)
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if updated.ShareStatus != store.ShareStatusSent {
		t.Errorf("ShareStatus after sync: want 'sent', got %q", updated.ShareStatus)
	}

	// Verify the JSON file was written.
	wantPath := filepath.Join(transportDir, mem.ID+".json")
	if _, err := os.Stat(wantPath); os.IsNotExist(err) {
		t.Errorf("Sync: expected JSON file at %s", wantPath)
	}

	// Verify the queue is empty.
	queue, err := s.ListUploadQueue(ctx)
	if err != nil {
		t.Fatalf("ListUploadQueue: %v", err)
	}
	if len(queue) != 0 {
		t.Errorf("Queue after sync: want empty, got %d entries", len(queue))
	}
}

// TestSync_SkipsAlreadySent validates that memories with share_status='sent'
// are skipped (not re-sent).
func TestSync_SkipsAlreadySent(t *testing.T) {
	dbPath := t.TempDir() + "/sync_sent.db"
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	mem, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Already Sent Memory",
		Content: "Content.",
		Author:  "tester",
	})
	if err != nil {
		t.Fatalf("ProposeMemory: %v", err)
	}
	if err := s.Validate(ctx, mem.ID, "validator"); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	// Mark as already sent directly (bypass Queue for this test setup).
	if err := s.SetShareStatus(ctx, mem.ID, store.ShareStatusSent); err != nil {
		t.Fatalf("SetShareStatus: %v", err)
	}
	if _, err := s.EnqueueUpload(ctx, mem.ID, "tester"); err != nil {
		t.Fatalf("EnqueueUpload: %v", err)
	}

	transportDir := t.TempDir()
	lt, err := share.NewLocalDirTransport(transportDir)
	if err != nil {
		t.Fatalf("NewLocalDirTransport: %v", err)
	}

	report, err := share.Sync(ctx, s, lt)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if report.Skipped != 1 {
		t.Errorf("Sync: want skipped=1, got %d", report.Skipped)
	}
	if report.Sent != 0 {
		t.Errorf("Sync: want sent=0, got %d", report.Sent)
	}

	// Verify no JSON file was written.
	wantPath := filepath.Join(transportDir, mem.ID+".json")
	if _, err := os.Stat(wantPath); !os.IsNotExist(err) {
		t.Errorf("Sync(skipped): expected no file at %s", wantPath)
	}
}

// TestSync_NilTransport returns error.
func TestSync_NilTransport(t *testing.T) {
	dbPath := t.TempDir() + "/sync_nil_transport.db"
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	_, err = share.Sync(ctx, s, nil)
	if err == nil {
		t.Error("Sync(nil transport): expected error")
	}
}
