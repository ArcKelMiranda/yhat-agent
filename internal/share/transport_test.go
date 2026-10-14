package share_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArcKelMiranda/yhat-agent/internal/share"
)

// ---------------------------------------------------------------------------
// RED: write tests that capture the expected failures before implementation.
// These are the transport-level tests that validate Send, Pull, and
// ApplyRejection behavior.
// ---------------------------------------------------------------------------

// TestTransport_Send_WritesJSON verifies that Send writes an atomic JSON file
// to the correct path without leaving a partial file on error.
func TestTransport_Send_WritesJSON(t *testing.T) {
	dir := t.TempDir()
	lt, err := share.NewLocalDirTransport(dir)
	if err != nil {
		t.Fatalf("NewLocalDirTransport: %v", err)
	}

	payload := share.LocalPayload{
		ID:        "test-memory-001",
		Origin:    "local",
		Type:      "decision",
		Title:     "Send Test Memory",
		Content:   "Content for send test.",
		Author:    "tester",
		CreatedAt: "2024-01-01T00:00:00Z",
	}

	ctx := context.Background()
	if err := lt.Send(ctx, payload); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Verify the file exists at the correct path.
	wantPath := filepath.Join(dir, "test-memory-001.json")
	data, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", wantPath, err)
	}

	var got share.LocalPayload
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	if got.ID != "test-memory-001" {
		t.Errorf("ID: want 'test-memory-001', got %q", got.ID)
	}
	if got.Origin != "local" {
		t.Errorf("Origin: want 'local', got %q", got.Origin)
	}
	if got.Type != "decision" {
		t.Errorf("Type: want 'decision', got %q", got.Type)
	}
	if got.Title != "Send Test Memory" {
		t.Errorf("Title: want 'Send Test Memory', got %q", got.Title)
	}
}

// TestTransport_Send_Atomic verifies that Send writes atomically (tmp+rename).
func TestTransport_Send_Atomic(t *testing.T) {
	dir := t.TempDir()
	lt, err := share.NewLocalDirTransport(dir)
	if err != nil {
		t.Fatalf("NewLocalDirTransport: %v", err)
	}

	payload := share.LocalPayload{
		ID:        "atomic-test-001",
		Origin:    "local",
		Type:      "rule",
		Title:     "Atomic Test",
		Content:   "Content.",
		Author:    "tester",
		CreatedAt: "2024-01-01T00:00:00Z",
	}

	ctx := context.Background()
	if err := lt.Send(ctx, payload); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Verify no tmp files remain.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if e.Name()[0] == '.' && e.Name()[1:] == "send-tmp-" {
			t.Errorf("tmp file left behind: %s", e.Name())
		}
	}
}

// TestTransport_Pull_EmptyDir returns empty result when team/ is absent or empty.
func TestTransport_Pull_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	lt, err := share.NewLocalDirTransport(dir)
	if err != nil {
		t.Fatalf("NewLocalDirTransport: %v", err)
	}

	ctx := context.Background()
	result, err := lt.Pull(ctx, "")
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(result.Memories) != 0 {
		t.Errorf("Pull(empty): want 0 memories, got %d", len(result.Memories))
	}
	if result.NextCursor != "" {
		t.Errorf("Pull(empty): want empty NextCursor, got %q", result.NextCursor)
	}
}

// TestTransport_Pull_ReturnsTeamMemories verifies that Pull reads team/*.json files.
func TestTransport_Pull_ReturnsTeamMemories(t *testing.T) {
	dir := t.TempDir()
	lt, err := share.NewLocalDirTransport(dir)
	if err != nil {
		t.Fatalf("NewLocalDirTransport: %v", err)
	}

	// Pre-populate team/ with two files.
	team1 := share.TeamPayload{
		ID:            "team-mem-001",
		Origin:        "team",
		Type:          "rule",
		Title:         "Team Rule Alpha",
		Content:       "Rule content from team.",
		Author:        "operator-a",
		CreatedAt:     "2024-01-01T00:00:00Z",
		TeamStatus:    "approved",
		TeamUpdatedAt: "2024-01-02T00:00:00Z",
	}
	team2 := share.TeamPayload{
		ID:            "team-mem-002",
		Origin:        "team",
		Type:          "decision",
		Title:         "Team Decision Beta",
		Content:       "Decision from team.",
		Author:        "operator-b",
		CreatedAt:     "2024-01-03T00:00:00Z",
		TeamStatus:    "approved",
		TeamUpdatedAt: "2024-01-04T00:00:00Z",
	}

	for i, tp := range []share.TeamPayload{team1, team2} {
		data, _ := json.Marshal(tp)
		id := "team-mem-001"
		if i == 1 {
			id = "team-mem-002"
		}
		path := filepath.Join(dir, "team", id+".json")
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	ctx := context.Background()
	result, err := lt.Pull(ctx, "")
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(result.Memories) != 2 {
		t.Errorf("Pull: want 2 memories, got %d", len(result.Memories))
	}
	for _, mem := range result.Memories {
		if mem.Origin != "team" {
			t.Errorf("Origin: want 'team', got %q", mem.Origin)
		}
		if mem.TeamStatus != "approved" {
			t.Errorf("TeamStatus: want 'approved', got %q", mem.TeamStatus)
		}
	}
}

// TestTransport_Pull_CursorPagination verifies cursor-based pagination.
func TestTransport_Pull_CursorPagination(t *testing.T) {
	dir := t.TempDir()
	lt, err := share.NewLocalDirTransport(dir)
	if err != nil {
		t.Fatalf("NewLocalDirTransport: %v", err)
	}

	// Pre-populate team/ with files a, b, c (lexicographic).
	for _, suffix := range []string{"aaa", "bbb", "ccc"} {
		tp := share.TeamPayload{
			ID:            "team-mem-" + suffix,
			Origin:        "team",
			Type:          "rule",
			Title:         "Team Memory",
			Content:       "Content.",
			Author:        "tester",
			CreatedAt:     "2024-01-01T00:00:00Z",
			TeamStatus:    "approved",
			TeamUpdatedAt: "2024-01-01T00:00:00Z",
		}
		data, _ := json.Marshal(tp)
		path := filepath.Join(dir, "team", "team-mem-"+suffix+".json")
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	ctx := context.Background()

	// First pull: all three.
	r1, err := lt.Pull(ctx, "")
	if err != nil {
		t.Fatalf("Pull 1: %v", err)
	}
	if len(r1.Memories) != 3 {
		t.Errorf("Pull 1: want 3, got %d", len(r1.Memories))
	}

	// Second pull with cursor: should be empty (no more entries after cursor).
	r2, err := lt.Pull(ctx, r1.NextCursor)
	if err != nil {
		t.Fatalf("Pull 2: %v", err)
	}
	if len(r2.Memories) != 0 {
		t.Errorf("Pull 2 with cursor: want 0, got %d", len(r2.Memories))
	}
}

// TestTransport_ApplyRejection_NoOp verifies that ApplyRejection is a no-op for F2.
func TestTransport_ApplyRejection_NoOp(t *testing.T) {
	dir := t.TempDir()
	lt, err := share.NewLocalDirTransport(dir)
	if err != nil {
		t.Fatalf("NewLocalDirTransport: %v", err)
	}

	ctx := context.Background()
	if err := lt.ApplyRejection(ctx, "some-id", "not approved"); err != nil {
		t.Errorf("ApplyRejection: want nil (no-op), got %v", err)
	}
}
