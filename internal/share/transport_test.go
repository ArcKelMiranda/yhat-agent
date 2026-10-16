package share_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ArcKelMiranda/yhat-agent/internal/share"
)

// ---------------------------------------------------------------------------
// LocalDirTransport tests
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
		_ = tp
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

// ---------------------------------------------------------------------------
// HTTPTransport tests (F2 client)
// ---------------------------------------------------------------------------

// TestHTTPTransport_Send_HappyPath posts a memory to /api/memories and returns 201.
func TestHTTPTransport_Send_HappyPath(t *testing.T) {
	var receivedPayload share.LocalPayload
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodPost {
			t.Errorf("POST /api/memories: want POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/memories" {
			t.Errorf("URL path: want /api/memories, got %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type: want application/json, got %s", r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&receivedPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	httpClient := &http.Client{Timeout: 15 * time.Second}
	ht, err := share.NewHTTPTransport(httpClient, srv.URL, "test-token-abc")
	if err != nil {
		t.Fatalf("NewHTTPTransport: %v", err)
	}

	payload := share.LocalPayload{
		ID:        "http-test-001",
		Origin:    "local",
		Type:      "decision",
		Title:     "HTTP Send Test",
		Content:   "Content for HTTP send test.",
		Author:    "http-tester",
		CreatedAt: "2024-07-01T00:00:00Z",
	}
	ctx := context.Background()
	if err := ht.Send(ctx, payload); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if receivedAuth != "Bearer test-token-abc" {
		t.Errorf("Authorization header: want 'Bearer test-token-abc', got %q", receivedAuth)
	}
	if receivedPayload.ID != "http-test-001" {
		t.Errorf("payload ID: got %q", receivedPayload.ID)
	}
	if receivedPayload.Origin != "local" {
		t.Errorf("payload Origin: got %q", receivedPayload.Origin)
	}
}

// TestHTTPTransport_Send_409Conflict returns an error for duplicate memory.
func TestHTTPTransport_Send_409Conflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"error":"duplicate memory"}`))
	}))
	defer srv.Close()

	httpClient := &http.Client{Timeout: 15 * time.Second}
	ht, err := share.NewHTTPTransport(httpClient, srv.URL, "token")
	if err != nil {
		t.Fatalf("NewHTTPTransport: %v", err)
	}

	payload := share.LocalPayload{ID: "dup-id", Origin: "local", Type: "rule", Title: "Dup", Content: "Content.", Author: "t", CreatedAt: "2024-01-01T00:00:00Z"}
	ctx := context.Background()
	err = ht.Send(ctx, payload)
	if err == nil {
		t.Fatal("Send: expected non-nil error for 409")
	}
}

// TestHTTPTransport_Pull_HappyPath fetches /api/team and returns approved memories.
func TestHTTPTransport_Pull_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/team" {
			t.Errorf("URL path: want /api/team, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Authorization: got %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		team := []share.TeamPayload{
			{
				ID:            "team-001",
				Origin:        "team",
				Type:          "rule",
				Title:         "Team Rule",
				Content:       "Rule content.",
				Author:        "team",
				CreatedAt:     "2024-01-01T00:00:00Z",
				TeamStatus:    "approved",
				TeamUpdatedAt: "2024-01-02T00:00:00Z",
			},
			{
				ID:            "team-002",
				Origin:        "team",
				Type:          "decision",
				Title:         "Team Decision",
				Content:       "Decision content.",
				Author:        "team",
				CreatedAt:     "2024-01-03T00:00:00Z",
				TeamStatus:    "approved",
				TeamUpdatedAt: "2024-01-04T00:00:00Z",
			},
		}
		_ = json.NewEncoder(w).Encode(team)
	}))
	defer srv.Close()

	httpClient := &http.Client{Timeout: 15 * time.Second}
	ht, _ := share.NewHTTPTransport(httpClient, srv.URL, "test-token")
	ctx := context.Background()
	result, err := ht.Pull(ctx, "")
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(result.Memories) != 2 {
		t.Errorf("Pull: want 2 memories, got %d", len(result.Memories))
	}
	if result.Memories[0].TeamStatus != "approved" {
		t.Errorf("TeamStatus[0]: want 'approved', got %q", result.Memories[0].TeamStatus)
	}
}

// TestHTTPTransport_Pull_EmptyList handles empty /api/team response.
func TestHTTPTransport_Pull_EmptyList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]share.TeamPayload{})
	}))
	defer srv.Close()

	httpClient := &http.Client{Timeout: 15 * time.Second}
	ht, _ := share.NewHTTPTransport(httpClient, srv.URL, "token")
	ctx := context.Background()
	result, err := ht.Pull(ctx, "")
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(result.Memories) != 0 {
		t.Errorf("Pull empty: want 0, got %d", len(result.Memories))
	}
}

// TestHTTPTransport_ApplyRejection_Stub logs Spanish line and returns nil.
func TestHTTPTransport_ApplyRejection_Stub(t *testing.T) {
	// ApplyRejection is a F2 stub: it must not call any HTTP endpoint.
	// A nil server confirms no request is ever made.
	httpClient := &http.Client{Timeout: 15 * time.Second}
	ht, _ := share.NewHTTPTransport(httpClient, "http://127.0.0.1:1", "token")
	ctx := context.Background()
	if err := ht.ApplyRejection(ctx, "some-id", "not approved"); err != nil {
		t.Errorf("ApplyRejection: want nil stub, got %v", err)
	}
}

// TestHTTPTransport_Send_Timeout returns a wrapped context deadline exceeded error.
func TestHTTPTransport_Send_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // longer than the 100ms client timeout
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Use a short timeout on the client.
	httpClient := &http.Client{Timeout: 100 * time.Millisecond}
	ht, _ := share.NewHTTPTransport(httpClient, srv.URL, "token")

	payload := share.LocalPayload{ID: "timeout-id", Origin: "local", Type: "rule", Title: "T", Content: "C.", Author: "t", CreatedAt: "2024-01-01T00:00:00Z"}
	ctx := context.Background()
	err := ht.Send(ctx, payload)
	if err == nil {
		t.Fatal("Send with timeout: expected non-nil error")
	}
}

// TestHTTPTransport_Send_BearerHeader verifies the bearer token is sent correctly.
func TestHTTPTransport_Send_BearerHeader(t *testing.T) {
	var authHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	httpClient := &http.Client{Timeout: 15 * time.Second}
	ht, _ := share.NewHTTPTransport(httpClient, srv.URL, "operator-secret-token")

	payload := share.LocalPayload{ID: "auth-test", Origin: "local", Type: "rule", Title: "T", Content: "C.", Author: "t", CreatedAt: "2024-01-01T00:00:00Z"}
	ctx := context.Background()
	if err := ht.Send(ctx, payload); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !strings.HasPrefix(authHeader, "Bearer ") {
		t.Errorf("Authorization: want Bearer prefix, got %q", authHeader)
	}
	if !strings.Contains(authHeader, "operator-secret-token") {
		t.Errorf("Authorization: want 'operator-secret-token', got %q", authHeader)
	}
}

// TestHTTPTransport_JSONShape verifies the POST body matches LocalPayload contract.
func TestHTTPTransport_JSONShape(t *testing.T) {
	var got map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		// Validate required fields.
		for _, field := range []string{"id", "origin", "type", "title", "content", "author", "created_at"} {
			if _, ok := got[field]; !ok {
				t.Errorf("missing field in JSON: %s", field)
			}
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	httpClient := &http.Client{Timeout: 15 * time.Second}
	ht, _ := share.NewHTTPTransport(httpClient, srv.URL, "token")

	payload := share.LocalPayload{
		ID:        "shape-test-id",
		Origin:    "local",
		Type:      "decision",
		Title:     "Shape Test",
		Content:   "Content.",
		Context:   "Optional context.",
		Author:    "shape-tester",
		CreatedAt: "2024-07-01T00:00:00Z",
	}
	ctx := context.Background()
	if err := ht.Send(ctx, payload); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got["origin"] != "local" {
		t.Errorf("origin: want 'local', got %v", got["origin"])
	}
	if got["type"] != "decision" {
		t.Errorf("type: want 'decision', got %v", got["type"])
	}
}
