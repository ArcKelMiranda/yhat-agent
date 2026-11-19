package share_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArcKelMiranda/yhat-agent/internal/share"
	"github.com/ArcKelMiranda/yhat-agent/internal/store"
)

// teamServer returns an HTTP server that answers GET /api/team with the given
// payloads, plus the transport that talks to it.
func teamServer(t *testing.T, payload string) (share.Transport, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/team" {
			t.Errorf("URL path: want /api/team, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(payload)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	lt, err := share.NewTransport(srv.URL, "test-token")
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	return lt, srv.Close
}

// TestPullAndInsert_AcceptsContractFields verifies that a team memory that
// follows the contract (team_status) is stored locally.
func TestPullAndInsert_AcceptsContractFields(t *testing.T) {
	dbPath := t.TempDir() + "/team_contract.db"
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	body := `[{
		"id": "11111111-1111-4111-8111-111111111111",
		"origin": "team",
		"type": "rule",
		"title": "Team rule",
		"content": "The team publishes this rule.",
		"context": "",
		"author": "team",
		"created_at": "2026-01-02T00:00:00Z",
		"team_status": "approved",
		"team_updated_at": "2026-01-03T00:00:00Z"
	}]`
	lt, closeFn := teamServer(t, body)
	defer closeFn()

	inserted, err := share.PullAndInsert(context.Background(), s, lt)
	if err != nil {
		t.Fatalf("PullAndInsert: %v", err)
	}
	if inserted != 1 {
		t.Fatalf("inserted: want 1, got %d", inserted)
	}
}

// TestPullAndInsert_AcceptsStatusFallback verifies that a server that only
// sends the S7 spelling (status) is still understood: the agent maps
// "approved" to the contract field.
func TestPullAndInsert_AcceptsStatusFallback(t *testing.T) {
	dbPath := t.TempDir() + "/team_fallback.db"
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	body := `[{
		"id": "22222222-2222-4222-8222-222222222222",
		"origin": "team",
		"type": "rule",
		"title": "Team rule without team_status",
		"content": "The server only sent the S7 fields.",
		"context": "",
		"author": "team",
		"created_at": "2026-01-02T00:00:00Z",
		"status": "approved",
		"reviewed_at": "2026-01-03T00:00:00Z"
	}]`
	lt, closeFn := teamServer(t, body)
	defer closeFn()

	inserted, err := share.PullAndInsert(context.Background(), s, lt)
	if err != nil {
		t.Fatalf("PullAndInsert: %v", err)
	}
	if inserted != 1 {
		t.Fatalf("inserted: want 1, got %d", inserted)
	}
}

// TestPullAndInsert_SkipsMemoryWithoutTeamStatus verifies that a record with
// no usable team status is skipped instead of failing the whole pull.
func TestPullAndInsert_SkipsMemoryWithoutTeamStatus(t *testing.T) {
	dbPath := t.TempDir() + "/team_skip.db"
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	body := `[
		{
			"id": "33333333-3333-4333-8333-333333333333",
			"origin": "team",
			"type": "rule",
			"title": "Missing team status",
			"content": "No team status at all.",
			"context": "",
			"author": "team",
			"created_at": "2026-01-02T00:00:00Z"
		},
		{
			"id": "44444444-4444-4444-8444-444444444444",
			"origin": "team",
			"type": "rule",
			"title": "Valid neighbour",
			"content": "This one is fine.",
			"context": "",
			"author": "team",
			"created_at": "2026-01-02T00:00:00Z",
			"team_status": "approved"
		}
	]`
	lt, closeFn := teamServer(t, body)
	defer closeFn()

	inserted, err := share.PullAndInsert(context.Background(), s, lt)
	if err != nil {
		t.Fatalf("PullAndInsert: %v", err)
	}
	if inserted != 1 {
		t.Fatalf("inserted: want 1 (the invalid one must be skipped), got %d", inserted)
	}
}

// TestPullAndInsert_DuplicateIsIgnored verifies that re-pulling the same team
// memory does not insert it twice.
func TestPullAndInsert_DuplicateIsIgnored(t *testing.T) {
	dbPath := t.TempDir() + "/team_duplicate.db"
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	memory := map[string]any{
		"id":              "55555555-5555-4555-8555-555555555555",
		"origin":          "team",
		"type":            "rule",
		"title":           "Duplicate team rule",
		"content":         "Pulled twice.",
		"context":         "",
		"author":          "team",
		"created_at":      "2026-01-02T00:00:00Z",
		"team_status":     "approved",
		"team_updated_at": "2026-01-03T00:00:00Z",
	}
	raw, err := json.Marshal([]map[string]any{memory})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	lt, closeFn := teamServer(t, string(raw))
	defer closeFn()

	if _, err := share.PullAndInsert(context.Background(), s, lt); err != nil {
		t.Fatalf("first PullAndInsert: %v", err)
	}
	inserted, err := share.PullAndInsert(context.Background(), s, lt)
	if err != nil {
		t.Fatalf("second PullAndInsert: %v", err)
	}
	if inserted != 0 {
		t.Fatalf("inserted on second pull: want 0, got %d", inserted)
	}
}

// teamPayload builds a one-memory GET /api/team body with the given status.
func teamPayload(t *testing.T, id, status string) string {
	t.Helper()
	raw, err := json.Marshal([]map[string]any{{
		"id":              id,
		"origin":          "team",
		"type":            "rule",
		"title":           "Retire me " + id,
		"content":         "Will be retired by the team.",
		"context":         "",
		"author":          "team",
		"created_at":      "2026-01-02T00:00:00Z",
		"team_status":     status,
		"team_updated_at": "2026-01-03T00:00:00Z",
	}})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return string(raw)
}

func teamStatusOf(t *testing.T, s *store.Store, remoteID string) string {
	t.Helper()
	var status string
	err := s.StoreDB().QueryRow(
		`SELECT team_status FROM memories WHERE origin='team' AND remote_id=?`, remoteID,
	).Scan(&status)
	if err != nil {
		t.Fatalf("read team_status of %s: %v", remoteID, err)
	}
	return status
}

// TestPullAndInsert_AppliesARetirementToAnExistingRecord verifies that when the
// team retires a memory the agent already stored as approved, the local copy
// becomes retired instead of staying approved forever.
func TestPullAndInsert_AppliesARetirementToAnExistingRecord(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/team_retire.db")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	const id = "66666666-6666-4666-8666-666666666666"

	approved, closeA := teamServer(t, teamPayload(t, id, "approved"))
	defer closeA()
	if n, err := share.PullAndInsert(context.Background(), s, approved); err != nil || n != 1 {
		t.Fatalf("first pull: n=%d err=%v", n, err)
	}
	if got := teamStatusOf(t, s, id); got != "approved" {
		t.Fatalf("after first pull: want approved, got %s", got)
	}

	retired, closeR := teamServer(t, teamPayload(t, id, "retired"))
	defer closeR()
	n, err := share.PullAndInsert(context.Background(), s, retired)
	if err != nil {
		t.Fatalf("second pull: %v", err)
	}
	if n != 1 {
		t.Fatalf("changed records on the retirement pull: want 1, got %d", n)
	}
	if got := teamStatusOf(t, s, id); got != "retired" {
		t.Fatalf("after retirement: want retired, got %s", got)
	}

	// Pulling the same retirement again changes nothing.
	n, err = share.PullAndInsert(context.Background(), s, retired)
	if err != nil || n != 0 {
		t.Fatalf("repeat pull: n=%d err=%v", n, err)
	}
}
