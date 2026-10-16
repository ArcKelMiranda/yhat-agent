// Package share implements the F2 share-and-sync surfaces for Cerebro YHat v1.2.
// It enqueues validated memories, drains the upload queue, and syncs with the
// central repository via a Transport abstraction.
//
// Wire contract: see docs/central-api.md.
package share

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Payload types
// ---------------------------------------------------------------------------

// LocalPayload is the JSON shape written to <central_repo>/<memory_id>.json
// when sending a local memory to the central repository (F2).
type LocalPayload struct {
	ID        string `json:"id"`
	Origin    string `json:"origin"` // always "local"
	Type      string `json:"type"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Context   string `json:"context,omitempty"`
	Author    string `json:"author"`
	CreatedAt string `json:"created_at"`
}

// TeamPayload is the JSON shape read from <central_repo>/team/<id>.json
// when the sync pulls team-approved memories (F2 → F3 future).
type TeamPayload struct {
	ID            string `json:"id"`
	Origin        string `json:"origin"` // always "team"
	Type          string `json:"type"`
	Title         string `json:"title"`
	Content       string `json:"content"`
	Context       string `json:"context,omitempty"`
	Author        string `json:"author"`
	CreatedAt     string `json:"created_at"`
	TeamStatus    string `json:"team_status"` // "approved"
	TeamUpdatedAt string `json:"team_updated_at"`
}

// RejectedPayload is the JSON shape written to <central_repo>/rejected/<id>.json
// when a memory is rejected by the team (F2 → F3 future).
type RejectedPayload struct {
	MemoryID   string `json:"memory_id"`
	Reason     string `json:"reason"`
	RejectedAt string `json:"rejected_at"`
}

// PullResult is the return value of Transport.Pull.
type PullResult struct {
	// Memories contains team-approved memories pulled in this batch.
	Memories []TeamPayload
	// NextCursor is the cursor to pass to the next Pull call.
	// Empty string means no more entries.
	NextCursor string
}

// ---------------------------------------------------------------------------
// Transport interface
// ---------------------------------------------------------------------------

// Transport abstracts the central repository operations for the sync loop.
// Two implementations exist: LocalDirTransport (F2, file-based) and
// HTTPTransport (F2 HTTP client → F3 server).
type Transport interface {
	// Send uploads a single memory to the central repository.
	// The memory is serialised as LocalPayload and written atomically.
	// Returns nil on success; non-nil on failure (bubbles up as last_error).
	Send(ctx context.Context, payload LocalPayload) error

	// Pull retrieves the next batch of team-approved memories since cursor.
	// Cursor is stored in sync_state under key "pull_cursor".
	// Returns PullResult with the batch and next cursor; next_cursor="" means done.
	Pull(ctx context.Context, cursor string) (PullResult, error)

	// ApplyRejection records a team rejection for the given memory.
	// The payload is written as RejectedPayload to <central_repo>/rejected/<id>.json.
	// F2: this is a no-op (implemented for F3 protocol completeness).
	ApplyRejection(ctx context.Context, id, reason string) error
}

// ---------------------------------------------------------------------------
// LocalDirTransport
// ---------------------------------------------------------------------------

// LocalDirTransport is a file-based transport that simulates the F3 server
// locally using JSON files. This enables end-to-end testing of the sync flow
// without a real HTTP server.
//
// Directory layout:
//
//	<root>/
//	  <memory_id>.json          — outgoing local memories
//	  team/
//	    <memory_id>.json        — team-approved memories (future F3)
//	  rejected/
//	    <memory_id>.json        — rejections (future F3)
type LocalDirTransport struct {
	root string
}

// NewLocalDirTransport creates a LocalDirTransport rooted at dir.
// The root directory and required sub-directories are created if absent.
func NewLocalDirTransport(dir string) (*LocalDirTransport, error) {
	lt := &LocalDirTransport{root: dir}
	// Ensure sub-dirs exist.
	for _, sub := range []string{"team", "rejected"} {
		path := filepath.Join(dir, sub)
		if err := os.MkdirAll(path, 0755); err != nil {
			return nil, fmt.Errorf("NewLocalDirTransport mkdir %s: %w", sub, err)
		}
	}
	return lt, nil
}

// Root returns the transport's root directory.
func (lt *LocalDirTransport) Root() string {
	return lt.root
}

// Send writes payload atomically to <root>/<id>.json.
func (lt *LocalDirTransport) Send(ctx context.Context, payload LocalPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("json.Marshal: %w", err)
	}

	path := filepath.Join(lt.root, payload.ID+".json")
	// Atomic write: write to temp file in the same directory, then rename.
	tmp, err := os.CreateTemp(lt.root, ".send-tmp-")
	if err != nil {
		return fmt.Errorf("CreateTemp: %w", err)
	}
	tmpPath := tmp.Name()
	removeTmp := func() { os.Remove(tmpPath) }

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		removeTmp()
		return fmt.Errorf("tmp.Write: %w", err)
	}
	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		removeTmp()
		return fmt.Errorf("tmp.Chmod: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		removeTmp()
		return fmt.Errorf("tmp.Sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		removeTmp()
		return fmt.Errorf("tmp.Close: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		removeTmp()
		return fmt.Errorf("Rename: %w", err)
	}
	return nil
}

// Pull reads team/ entries from cursor, returning team-approved memories.
// It persists the next cursor in sync_state under "pull_cursor" via the
// store.SetSyncState method passed as a callback.
//
// The cursor is the filename (without extension) of the last processed entry.
//
// For F2: Pull reads <root>/team/*.json files. Each file's name (without .json)
// is the remote_id. The next cursor is the filename of the highest entry
// lexicographically after the current cursor.
func (lt *LocalDirTransport) Pull(ctx context.Context, cursor string) (PullResult, error) {
	teamDir := filepath.Join(lt.root, "team")
	entries, err := os.ReadDir(teamDir)
	if err != nil {
		if os.IsNotExist(err) {
			return PullResult{NextCursor: ""}, nil
		}
		return PullResult{}, fmt.Errorf("ReadDir team: %w", err)
	}

	// Collect .json files sorted lexicographically.
	var files []string
	for _, e := range entries {
		if !e.IsDir() && len(e.Name()) > 5 && e.Name()[len(e.Name())-5:] == ".json" {
			files = append(files, e.Name()[:len(e.Name())-5]) // strip .json
		}
	}
	sort.Strings(files) // deterministic order for cursor pagination

	// Filter entries strictly after cursor.
	var result []TeamPayload
	var nextCursor string
	for _, fname := range files {
		if cursor != "" && fname <= cursor {
			continue
		}
		data, err := os.ReadFile(filepath.Join(teamDir, fname+".json"))
		if err != nil {
			return PullResult{}, fmt.Errorf("ReadFile %s: %w", fname, err)
		}
		var tp TeamPayload
		if err := json.Unmarshal(data, &tp); err != nil {
			return PullResult{}, fmt.Errorf("Unmarshal %s: %w", fname, err)
		}
		result = append(result, tp)
		nextCursor = fname // last seen is the next cursor
	}

	return PullResult{
		Memories:   result,
		NextCursor: nextCursor,
	}, nil
}

// ApplyRejection is a no-op for F2. The protocol is documented for F3.
func (lt *LocalDirTransport) ApplyRejection(ctx context.Context, id, reason string) error {
	return nil
}

// ---------------------------------------------------------------------------
// HTTPTransport
// ---------------------------------------------------------------------------

// HTTPTransport is an F2/F3 client transport that speaks HTTP.
// It sends memories to POST /api/memories and pulls approved memories from
// GET /api/team using bearer-token authentication.
//
// It satisfies the Transport interface.
type HTTPTransport struct {
	client  *http.Client
	baseURL string // stripped of trailing slash
	token   string
}

// NewHTTPTransport creates an HTTPTransport.
// client must be non-nil; 15s timeout is recommended.
// baseURL must not have a trailing slash (trailing slash is stripped automatically).
// token is the bearer token sent as Authorization: Bearer <token>.
func NewHTTPTransport(client *http.Client, baseURL, token string) (*HTTPTransport, error) {
	ht := &HTTPTransport{
		client:  client,
		baseURL: strings.TrimSuffix(baseURL, "/"),
		token:   token,
	}
	return ht, nil
}

// newRequest builds an http.Request with the bearer token header.
func (ht *HTTPTransport) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, ht.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+ht.token)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

// Send posts the LocalPayload to POST /api/memories.
// Returns nil on 2xx; propagates all other status codes as errors.
func (ht *HTTPTransport) Send(ctx context.Context, payload LocalPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("json.Marshal: %w", err)
	}

	req, err := ht.newRequest(ctx, http.MethodPost, "/api/memories", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("NewRequest: %w", err)
	}

	resp, err := ht.client.Do(req)
	if err != nil {
		return fmt.Errorf("client.Do: %w", err)
	}
	defer resp.Body.Close()

	// Any non-2xx status is an error that bubbles up as last_error.
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("HTTPTransport.Send: server returned %s", resp.Status)
	}
	return nil
}

// Pull fetches team-approved memories from GET /api/team.
// The cursor parameter is sent as the cursor query param.
// Returns PullResult with the team memories; NextCursor is empty for F2.
func (ht *HTTPTransport) Pull(ctx context.Context, cursor string) (PullResult, error) {
	path := "/api/team"
	if cursor != "" {
		path = "/api/team?cursor=" + cursor
	}

	req, err := ht.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return PullResult{}, fmt.Errorf("NewRequest: %w", err)
	}

	resp, err := ht.client.Do(req)
	if err != nil {
		return PullResult{}, fmt.Errorf("client.Do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return PullResult{}, fmt.Errorf("HTTPTransport.Pull: server returned %s", resp.Status)
	}

	var memories []TeamPayload
	if err := json.NewDecoder(resp.Body).Decode(&memories); err != nil {
		return PullResult{}, fmt.Errorf("decode /api/team response: %w", err)
	}

	return PullResult{Memories: memories, NextCursor: ""}, nil
}

// ApplyRejection is a F2 stub: it logs a Spanish no-op line and returns nil.
// The F3 protocol for posting rejections is not yet implemented.
//
//	F2 stub: ApplyRejection no-post — protocolo F3 aún no implementado.
func (ht *HTTPTransport) ApplyRejection(ctx context.Context, id, reason string) error {
	// F2 stub: ApplyRejection no-post — protocolo F3 aún no implementado.
	// When F3 implements POST /api/rejections, this stub will be replaced
	// with the real HTTP call.
	return nil
}

// BaseURL returns the transport's base URL (for debugging/testing).
func (ht *HTTPTransport) BaseURL() string { return ht.baseURL }

// Token returns the configured bearer token (for debugging/testing).
func (ht *HTTPTransport) Token() string { return ht.token }

// ---------------------------------------------------------------------------
// Transport factory
// ---------------------------------------------------------------------------

// NewTransport returns the appropriate Transport for the given URL.
// If url starts with "http://" or "https://", it returns HTTPTransport
// using the provided http.Client (default 15s timeout) and token.
// Otherwise it returns LocalDirTransport rooted at url.
func NewTransport(url string, token string) (Transport, error) {
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		client := &http.Client{Timeout: 15 * time.Second}
		return NewHTTPTransport(client, url, token)
	}
	return NewLocalDirTransport(url)
}

// ---------------------------------------------------------------------------
// Sentinel errors
// ---------------------------------------------------------------------------

var (
	// ErrNotShareable is returned by Queue when the memory is not in a
	// shareable state (not validated, or already shared/rejected).
	ErrNotShareable = errors.New("memory is not in a shareable state")
)
