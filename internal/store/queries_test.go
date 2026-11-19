package store_test

import (
	"context"
	"testing"

	"github.com/ArcKelMiranda/yhat-agent/internal/store"
)

// TestMarkUploadSent_HappyPath validates that MarkUploadSent flips share_status
// to 'sent' and removes the queue entry.
func TestMarkUploadSent_HappyPath(t *testing.T) {
	dbPath := t.TempDir() + "/mark_sent.db"
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Propose and validate a memory.
	mem, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Memory to send",
		Content: "Content for MarkUploadSent test.",
		Author:  "test",
	})
	if err != nil {
		t.Fatalf("ProposeMemory: %v", err)
	}
	if err := s.Validate(ctx, mem.ID, "validator"); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	// Enqueue it.
	_, err = s.EnqueueUpload(ctx, mem.ID, "tester")
	if err != nil {
		t.Fatalf("EnqueueUpload: %v", err)
	}

	// Mark as sent.
	if err := s.MarkUploadSent(ctx, mem.ID); err != nil {
		t.Fatalf("MarkUploadSent: %v", err)
	}

	// Verify share_status is 'sent'.
	updated, err := s.GetMemory(ctx, mem.ID)
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if updated.ShareStatus != store.ShareStatusSent {
		t.Errorf("ShareStatus: want 'sent', got %q", updated.ShareStatus)
	}

	// Verify queue entry is gone.
	queue, err := s.ListUploadQueue(ctx)
	if err != nil {
		t.Fatalf("ListUploadQueue: %v", err)
	}
	for _, e := range queue {
		if e.MemoryID == mem.ID {
			t.Errorf("queue entry for %s: should have been removed", mem.ID)
		}
	}
}

// TestMarkUploadSent_NotFound validates that MarkUploadSent returns ErrNotFound
// for an unknown memory ID.
func TestMarkUploadSent_NotFound(t *testing.T) {
	dbPath := t.TempDir() + "/mark_sent_notfound.db"
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	if err := s.MarkUploadSent(ctx, "00000000-0000-0000-0000-000000000000"); err != store.ErrNotFound {
		t.Errorf("MarkUploadSent: want ErrNotFound, got %v", err)
	}
}

// TestRecordUploadFailure_IncrementsAttempts validates that RecordUploadFailure
// increments attempts and records last_error.
func TestRecordUploadFailure_IncrementsAttempts(t *testing.T) {
	dbPath := t.TempDir() + "/record_fail.db"
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	mem, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeRule,
		Title:   "Memory to fail",
		Content: "Content for RecordUploadFailure test.",
		Author:  "test",
	})
	if err != nil {
		t.Fatalf("ProposeMemory: %v", err)
	}
	if err := s.Validate(ctx, mem.ID, "validator"); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	_, err = s.EnqueueUpload(ctx, mem.ID, "tester")
	if err != nil {
		t.Fatalf("EnqueueUpload: %v", err)
	}

	// Record a failure.
	if err := s.RecordUploadFailure(ctx, mem.ID, "connection refused"); err != nil {
		t.Fatalf("RecordUploadFailure: %v", err)
	}

	queue, err := s.ListUploadQueue(ctx)
	if err != nil {
		t.Fatalf("ListUploadQueue: %v", err)
	}
	if len(queue) != 1 {
		t.Fatalf("ListUploadQueue: want 1 entry, got %d", len(queue))
	}
	entry := queue[0]
	if entry.Attempts != 1 {
		t.Errorf("Attempts: want 1, got %d", entry.Attempts)
	}
	if entry.LastError == nil || *entry.LastError != "connection refused" {
		t.Errorf("LastError: want 'connection refused', got %v", entry.LastError)
	}
	if entry.NextRetryAt == nil {
		t.Error("NextRetryAt: want non-nil")
	}
}

// TestRecordUploadFailure_UnknownMemory is idempotent: returns nil.
func TestRecordUploadFailure_UnknownMemory(t *testing.T) {
	dbPath := t.TempDir() + "/record_fail_unknown.db"
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	if err := s.RecordUploadFailure(ctx, "00000000-0000-0000-0000-000000000000", "should not panic"); err != nil {
		t.Errorf("RecordUploadFailure(unknown): want nil, got %v", err)
	}
}

// TestSearchMemoriesFTS5_HidesRetiredTeamMemories verifies that a memory the
// team retired is no longer returned by search, while approved team records
// and local memories still are.
func TestSearchMemoriesFTS5_HidesRetiredTeamMemories(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(t.TempDir() + "/search_retired.db")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	if err := s.EnsureFTS5(ctx); err != nil {
		t.Fatalf("EnsureFTS5: %v", err)
	}

	insert := func(remoteID, title, status string) {
		rid, st := remoteID, status
		if _, err := s.InsertTeamMemory(ctx, store.Memory{
			RemoteID: &rid, Type: store.MemoryTypeRule, Title: title,
			Content: "Searchable body.", Author: "team", TeamStatus: &st,
		}); err != nil {
			t.Fatalf("InsertTeamMemory %s: %v", title, err)
		}
	}
	insert("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "Zebra approved", "approved")
	insert("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "Zebra retired", "retired")

	hits, err := s.SearchMemoriesFTS5(ctx, `"zebra"`, 10)
	if err != nil {
		t.Fatalf("SearchMemoriesFTS5: %v", err)
	}
	if len(hits) != 1 || hits[0].Title != "Zebra approved" {
		var titles []string
		for _, h := range hits {
			titles = append(titles, h.Title)
		}
		t.Fatalf("want only the approved record, got %v", titles)
	}
}
