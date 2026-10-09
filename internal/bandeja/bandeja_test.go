// F1-D tests: Bandeja HTTP server. Tests drive the real HTTP endpoints
// with net/http against a listener on 127.0.0.1:0 and assert the store
// mutated exactly the intended fields.

package bandeja_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArcKelMiranda/yhat-agent/internal/bandeja"
	"github.com/ArcKelMiranda/yhat-agent/internal/store"
)

// newTestStore opens a temporary store for testing.
func newTestStore(t *testing.T) (*store.Store, string, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "yhat-bandeja-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	dbPath := filepath.Join(dir, "yhat.db")
	s, err := store.Open(dbPath)
	if err != nil {
		os.RemoveAll(dir)
		t.Fatalf("store.Open: %v", err)
	}
	cleanup := func() {
		s.Close()
		os.RemoveAll(dir)
	}
	return s, dir, cleanup
}

// doReq sends an HTTP POST request to the given API path with the token as a query param.
func doReq(t *testing.T, base, token, apiPath string, body interface{}) *http.Response {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(data)
	}
	url := base + apiPath + "?token=" + token
	req, err := http.NewRequest(http.MethodPost, url, bodyReader)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http.DefaultClient.Do: %v", err)
	}
	return resp
}

// getPage fetches GET / with the given token.
func getPage(t *testing.T, base, token string) *http.Response {
	t.Helper()
	url := base + "/?token=" + token
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("http.Get: %v", err)
	}
	return resp
}

type apiResp struct {
	OK  bool   `json:"ok"`
	ID  string `json:"id,omitempty"`
	Err string `json:"error,omitempty"`
}

func readJSON(t *testing.T, resp *http.Response) apiResp {
	t.Helper()
	var r apiResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		t.Fatalf("decode JSON response: %v", err)
	}
	return r
}

// extractToken parses the token from a bandeja URL.
// srv.URL() returns http://host:port/?token=TOKEN so we extract the token.
func extractToken(t *testing.T, urlStr string) string {
	t.Helper()
	i := strings.Index(urlStr, "token=")
	if i < 0 {
		t.Fatalf("extractToken: no token found in %q", urlStr)
	}
	return urlStr[i+len("token="):]
}

// baseURL returns the http://host:port portion of srv.URL() (strips ?token=...).
func baseURL(t *testing.T, urlStr string) string {
	t.Helper()
	i := strings.Index(urlStr, "?")
	if i < 0 {
		return urlStr
	}
	return urlStr[:i]
}

// ─────────────────────────────────────────────────────────────────────────────
// RED 1: Start creates a listener and returns a Server with a non-empty URL.
// ─────────────────────────────────────────────────────────────────────────────

func TestRED1_StartReturnsServerWithURL(t *testing.T) {
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	if srv.URL() == "" {
		t.Error("URL(): want non-empty")
	}
	if !strings.HasPrefix(srv.URL(), "http://127.0.0.1:") {
		t.Errorf("URL(): want http://127.0.0.1:<port>, got %s", srv.URL())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RED 2: Request without token returns 404.
// ─────────────────────────────────────────────────────────────────────────────

func TestRED2_NoTokenReturns404(t *testing.T) {
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	resp := getPage(t, baseURL(t, srv.URL()), "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET without token: want 404, got %d", resp.StatusCode)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RED 3: Request with invalid token returns 404.
// ─────────────────────────────────────────────────────────────────────────────

func TestRED3_InvalidTokenReturns404(t *testing.T) {
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	resp := getPage(t, baseURL(t, srv.URL()), "invalidtokenxyz")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET with invalid token: want 404, got %d", resp.StatusCode)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RED 4: GET / with valid token returns 200 and HTML containing expected text.
// ─────────────────────────────────────────────────────────────────────────────

func TestRED4_ValidTokenReturnsHTMLPage(t *testing.T) {
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	token := extractToken(t, srv.URL())
	resp := getPage(t, baseURL(t, srv.URL()), token)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET with valid token: want 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "Bandeja") {
		t.Error("page missing 'Bandeja' heading")
	}
	if !strings.Contains(bodyStr, "Pendientes") {
		t.Error("page missing 'Pendientes' section")
	}
	if !strings.Contains(bodyStr, "Enviados") {
		t.Error("page missing 'Enviados' section")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RED 5: POST /api/approve without token returns 404.
// ─────────────────────────────────────────────────────────────────────────────

func TestRED5_ApproveWithoutTokenReturns404(t *testing.T) {
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	resp := doReq(t, baseURL(t, srv.URL()), "", "/api/approve", map[string]interface{}{"id": "any"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("approve without token: want 404, got %d", resp.StatusCode)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RED 6: POST /api/approve with missing id returns 400.
// ─────────────────────────────────────────────────────────────────────────────

func TestRED6_ApproveMissingIDReturns400(t *testing.T) {
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	token := extractToken(t, srv.URL())
	resp := doReq(t, baseURL(t, srv.URL()), token, "/api/approve", map[string]interface{}{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("approve missing id: want 400, got %d", resp.StatusCode)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RED 7: POST /api/approve on valid proposed memory validates it.
// ─────────────────────────────────────────────────────────────────────────────

func TestRED7_ApproveValidMemoryValidates(t *testing.T) {
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	mem, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Decisión de prueba",
		Content: "Contenido de prueba para approve.",
		Author:  "test-author",
	})
	if err != nil {
		t.Fatalf("ProposeMemory: %v", err)
	}

	token := extractToken(t, srv.URL())
	resp := doReq(t, baseURL(t, srv.URL()), token, "/api/approve", map[string]interface{}{"id": mem.ID})
	defer resp.Body.Close()

	r := readJSON(t, resp)
	if !r.OK {
		t.Error("approve: want ok=true")
	}
	if r.ID != mem.ID {
		t.Errorf("approve: want id=%s, got %s", mem.ID, r.ID)
	}

	fetched, err := s.GetMemory(ctx, mem.ID)
	if err != nil {
		t.Fatalf("GetMemory after approve: %v", err)
	}
	if fetched.Status != store.StatusValidated {
		t.Errorf("status after approve: want %s, got %s", store.StatusValidated, fetched.Status)
	}
	if fetched.ValidatedBy == nil || *fetched.ValidatedBy == "" {
		t.Error("ValidatedBy: want non-empty after approve")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RED 8: POST /api/reject without token returns 404.
// ─────────────────────────────────────────────────────────────────────────────

func TestRED8_RejectWithoutTokenReturns404(t *testing.T) {
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	resp := doReq(t, baseURL(t, srv.URL()), "", "/api/reject", map[string]interface{}{"id": "any"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("reject without token: want 404, got %d", resp.StatusCode)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RED 9: POST /api/reject with missing id returns 400.
// ─────────────────────────────────────────────────────────────────────────────

func TestRED9_RejectMissingIDReturns400(t *testing.T) {
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	token := extractToken(t, srv.URL())
	resp := doReq(t, baseURL(t, srv.URL()), token, "/api/reject", map[string]interface{}{})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("reject missing id: want 400, got %d", resp.StatusCode)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RED 10: POST /api/reject with reason rejects and records reason.
// ─────────────────────────────────────────────────────────────────────────────

func TestRED10_RejectWithReasonRecordsReason(t *testing.T) {
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	mem, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeRule,
		Title:   "Regla rechazada",
		Content: "Contenido para rechazo.",
		Author:  "test-author",
	})
	if err != nil {
		t.Fatalf("ProposeMemory: %v", err)
	}

	token := extractToken(t, srv.URL())
	resp := doReq(t, baseURL(t, srv.URL()), token, "/api/reject", map[string]interface{}{
		"id":     mem.ID,
		"reason": "No es relevante para el equipo.",
	})
	defer resp.Body.Close()

	r := readJSON(t, resp)
	if !r.OK {
		t.Error("reject: want ok=true")
	}
	if r.ID != mem.ID {
		t.Errorf("reject: want id=%s, got %s", mem.ID, r.ID)
	}

	fetched, err := s.GetMemory(ctx, mem.ID)
	if err != nil {
		t.Fatalf("GetMemory after reject: %v", err)
	}
	if fetched.Status != store.StatusRejected {
		t.Errorf("status after reject: want %s, got %s", store.StatusRejected, fetched.Status)
	}
	if fetched.RejectReason == nil || *fetched.RejectReason != "No es relevante para el equipo." {
		t.Errorf("reject_reason: got %v", fetched.RejectReason)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RED 11: POST /api/reject truncates reason at 500 chars.
// ─────────────────────────────────────────────────────────────────────────────

func TestRED11_RejectTruncatesLongReason(t *testing.T) {
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	mem, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Decisión larga",
		Content: "Contenido.",
		Author:  "test-author",
	})
	if err != nil {
		t.Fatalf("ProposeMemory: %v", err)
	}

	longReason := strings.Repeat("x", 600)
	token := extractToken(t, srv.URL())
	resp := doReq(t, baseURL(t, srv.URL()), token, "/api/reject", map[string]interface{}{
		"id":     mem.ID,
		"reason": longReason,
	})
	defer resp.Body.Close()

	r := readJSON(t, resp)
	if !r.OK {
		t.Error("reject with long reason: want ok=true")
	}

	fetched, err := s.GetMemory(ctx, mem.ID)
	if err != nil {
		t.Fatalf("GetMemory after reject: %v", err)
	}
	if fetched.RejectReason == nil {
		t.Fatal("RejectReason: want non-nil")
	}
	if len(*fetched.RejectReason) > 500 {
		t.Errorf("RejectReason length: want <=500, got %d", len(*fetched.RejectReason))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RED 12: POST /api/edit without token returns 404.
// ─────────────────────────────────────────────────────────────────────────────

func TestRED12_EditWithoutTokenReturns404(t *testing.T) {
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	resp := doReq(t, baseURL(t, srv.URL()), "", "/api/edit", map[string]interface{}{
		"id":      "any",
		"title":   "t",
		"content": "c",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("edit without token: want 404, got %d", resp.StatusCode)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RED 13: POST /api/edit with missing id returns 400.
// ─────────────────────────────────────────────────────────────────────────────

func TestRED13_EditMissingIDReturns400(t *testing.T) {
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	token := extractToken(t, srv.URL())
	resp := doReq(t, baseURL(t, srv.URL()), token, "/api/edit", map[string]interface{}{
		"title":   "title",
		"content": "content",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("edit missing id: want 400, got %d", resp.StatusCode)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RED 14: POST /api/edit with empty title returns 400.
// ─────────────────────────────────────────────────────────────────────────────

func TestRED14_EditEmptyTitleReturns400(t *testing.T) {
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	mem, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Original",
		Content: "Original content.",
		Author:  "test-author",
	})
	if err != nil {
		t.Fatalf("ProposeMemory: %v", err)
	}

	token := extractToken(t, srv.URL())
	resp := doReq(t, baseURL(t, srv.URL()), token, "/api/edit", map[string]interface{}{
		"id":      mem.ID,
		"title":   "",
		"content": "content",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("edit empty title: want 400, got %d", resp.StatusCode)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RED 15: POST /api/edit with empty content returns 400.
// ─────────────────────────────────────────────────────────────────────────────

func TestRED15_EditEmptyContentReturns400(t *testing.T) {
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	mem, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Original",
		Content: "Original content.",
		Author:  "test-author",
	})
	if err != nil {
		t.Fatalf("ProposeMemory: %v", err)
	}

	token := extractToken(t, srv.URL())
	resp := doReq(t, baseURL(t, srv.URL()), token, "/api/edit", map[string]interface{}{
		"id":      mem.ID,
		"title":   "New title",
		"content": "",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("edit empty content: want 400, got %d", resp.StatusCode)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RED 16: POST /api/edit updates title, content, and context.
// ─────────────────────────────────────────────────────────────────────────────

func TestRED16_EditUpdatesFields(t *testing.T) {
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	mem, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Original title",
		Content: "Original content.",
		Author:  "test-author",
	})
	if err != nil {
		t.Fatalf("ProposeMemory: %v", err)
	}

	token := extractToken(t, srv.URL())
	resp := doReq(t, baseURL(t, srv.URL()), token, "/api/edit", map[string]interface{}{
		"id":      mem.ID,
		"title":   "Nuevo título",
		"content": "Nuevo contenido editada.",
		"context": "contexto adicional",
	})
	defer resp.Body.Close()

	r := readJSON(t, resp)
	if !r.OK {
		t.Error("edit: want ok=true")
	}
	if r.ID != mem.ID {
		t.Errorf("edit: want id=%s, got %s", mem.ID, r.ID)
	}

	fetched, err := s.GetMemory(ctx, mem.ID)
	if err != nil {
		t.Fatalf("GetMemory after edit: %v", err)
	}
	if fetched.Title != "Nuevo título" {
		t.Errorf("title after edit: want %s, got %s", "Nuevo título", fetched.Title)
	}
	if fetched.Content != "Nuevo contenido editada." {
		t.Errorf("content after edit: want %s, got %s", "Nuevo contenido editada.", fetched.Content)
	}
	if fetched.Context == nil || *fetched.Context != "contexto adicional" {
		t.Errorf("context after edit: got %v", fetched.Context)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RED 17: Token is reusable for multiple page loads.
// ─────────────────────────────────────────────────────────────────────────────

func TestRED17_TokenReusableForMultiplePageLoads(t *testing.T) {
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	token := extractToken(t, srv.URL())

	for i := 0; i < 3; i++ {
		resp := getPage(t, baseURL(t, srv.URL()), token)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET attempt %d: want 200, got %d", i+1, resp.StatusCode)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RED 18: Close terminates the server cleanly.
// ─────────────────────────────────────────────────────────────────────────────

func TestRED18_CloseTerminatesServer(t *testing.T) {
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Close should not panic.
	if err := srv.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RED 19: Pendientes section shows proposed memories with action buttons.
// ─────────────────────────────────────────────────────────────────────────────

func TestRED19_PendientesShowsProposedMemories(t *testing.T) {
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	mem, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Decisión pendiente para bandeja",
		Content: "Esta decisión está pendiente de aprobación.",
		Author:  "test-author",
	})
	if err != nil {
		t.Fatalf("ProposeMemory: %v", err)
	}

	token := extractToken(t, srv.URL())
	resp := getPage(t, baseURL(t, srv.URL()), token)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET with valid token: want 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if !strings.Contains(bodyStr, "Decisión pendiente para bandeja") {
		t.Error("page missing proposed memory title")
	}
	if !strings.Contains(bodyStr, "Aprobar") {
		t.Error("page missing Aprobar button")
	}
	if !strings.Contains(bodyStr, "Rechazar") {
		t.Error("page missing Rechazar button")
	}
	if !strings.Contains(bodyStr, "Editar") {
		t.Error("page missing Editar button")
	}
	_ = mem
}

// ─────────────────────────────────────────────────────────────────────────────
// RED 20: Enviados section shows validated memories with share_status != 'none'.
// ─────────────────────────────────────────────────────────────────────────────

func TestRED20_EnviadosShowsValidatedMemories(t *testing.T) {
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	mem, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeRule,
		Title:   "Regla enviada",
		Content: "Contenido de regla enviada.",
		Author:  "test-author",
	})
	if err != nil {
		t.Fatalf("ProposeMemory: %v", err)
	}
	if err := s.Validate(ctx, mem.ID, "approver"); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := s.SetShareStatus(ctx, mem.ID, store.ShareStatusQueued); err != nil {
		t.Fatalf("SetShareStatus: %v", err)
	}

	token := extractToken(t, srv.URL())
	resp := getPage(t, baseURL(t, srv.URL()), token)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if !strings.Contains(bodyStr, "Regla enviada") {
		t.Error("page missing validated+queued memory title in Enviados")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RED 21: Enviados section shows rejected memories with reject_reason.
// ─────────────────────────────────────────────────────────────────────────────

func TestRED21_EnviadosShowsRejectedMemories(t *testing.T) {
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	mem, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Decisión rechazada",
		Content: "Contenido rechazado.",
		Author:  "test-author",
	})
	if err != nil {
		t.Fatalf("ProposeMemory: %v", err)
	}
	if err := s.Reject(ctx, mem.ID, "rejector", "Razón de rechazo."); err != nil {
		t.Fatalf("Reject: %v", err)
	}

	token := extractToken(t, srv.URL())
	resp := getPage(t, baseURL(t, srv.URL()), token)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	if !strings.Contains(bodyStr, "Decisión rechazada") {
		t.Error("page missing rejected memory title in Enviados")
	}
	if !strings.Contains(bodyStr, "Razón de rechazo.") {
		t.Error("page missing reject_reason")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RED 22: Operator identity comes from config.yaml or env fallback.
// ─────────────────────────────────────────────────────────────────────────────

func TestRED22_OperatorFromConfigOrFallback(t *testing.T) {
	// Set YHAT_HOME to a temp dir without config.yaml so resolveOperator()
	// falls back to USER env var. The pre-existing ~/.yhat/config.yaml would
	// otherwise be read first.
	tempDir := t.TempDir()
	origYHAT := os.Getenv("YHAT_HOME")
	os.Setenv("YHAT_HOME", tempDir)
	defer func() {
		if origYHAT != "" {
			os.Setenv("YHAT_HOME", origYHAT)
		} else {
			os.Unsetenv("YHAT_HOME")
		}
	}()

	s, _, storeCleanup := newTestStore(t)
	defer storeCleanup()
	ctx := context.Background()

	// Set a known operator via USER env.
	t.Setenv("USER", "known-operator")

	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	// Propose a memory and approve it.
	mem, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Test operator identity",
		Content: "Content for operator test.",
		Author:  "author",
	})
	if err != nil {
		t.Fatalf("ProposeMemory: %v", err)
	}

	token := extractToken(t, srv.URL())
	resp := doReq(t, baseURL(t, srv.URL()), token, "/api/approve", map[string]interface{}{"id": mem.ID})
	defer resp.Body.Close()

	r := readJSON(t, resp)
	if !r.OK {
		t.Fatalf("approve failed: %s", r.Err)
	}

	fetched, err := s.GetMemory(ctx, mem.ID)
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	// Operator should be "known-operator" from USER env.
	if fetched.ValidatedBy == nil || *fetched.ValidatedBy != "known-operator" {
		t.Errorf("ValidatedBy: got %q, want 'known-operator'", *fetched.ValidatedBy)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RED 23: Approve a memory already validated returns error (store idempotent).
// ─────────────────────────────────────────────────────────────────────────────

func TestRED23_ApproveAlreadyValidated(t *testing.T) {
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	mem, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Already validated",
		Content: "Content.",
		Author:  "author",
	})
	if err != nil {
		t.Fatalf("ProposeMemory: %v", err)
	}
	// Validate first.
	if err := s.Validate(ctx, mem.ID, "first-approver"); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	token := extractToken(t, srv.URL())
	resp := doReq(t, baseURL(t, srv.URL()), token, "/api/approve", map[string]interface{}{"id": mem.ID})
	defer resp.Body.Close()

	// Should succeed (idempotent) but store sets validated_by to current operator.
	if resp.StatusCode != http.StatusOK {
		t.Errorf("approve already validated: want 200, got %d", resp.StatusCode)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RED 24: Edit a non-existent memory returns 500 (store error).
// ─────────────────────────────────────────────────────────────────────────────

func TestRED24_EditNonExistentMemory(t *testing.T) {
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	token := extractToken(t, srv.URL())
	resp := doReq(t, baseURL(t, srv.URL()), token, "/api/edit", map[string]interface{}{
		"id":      "00000000-0000-0000-0000-000000000000",
		"title":   "New title",
		"content": "New content",
	})
	defer resp.Body.Close()
	// Store returns ErrNotFound → 500.
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("edit non-existent: want 500, got %d", resp.StatusCode)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RED 25: Reject a non-existent memory returns 500.
// ─────────────────────────────────────────────────────────────────────────────

func TestRED25_RejectNonExistentMemory(t *testing.T) {
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	token := extractToken(t, srv.URL())
	resp := doReq(t, baseURL(t, srv.URL()), token, "/api/reject", map[string]interface{}{
		"id":     "00000000-0000-0000-0000-000000000000",
		"reason": "test",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("reject non-existent: want 500, got %d", resp.StatusCode)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// RED 26: Reject reason is optional (empty string allowed).
// ─────────────────────────────────────────────────────────────────────────────

func TestRED26_RejectEmptyReasonAllowed(t *testing.T) {
	s, _, cleanup := newTestStore(t)
	defer cleanup()
	ctx := context.Background()

	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Close()

	mem, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Rechazo sin razón",
		Content: "Content.",
		Author:  "author",
	})
	if err != nil {
		t.Fatalf("ProposeMemory: %v", err)
	}

	token := extractToken(t, srv.URL())
	resp := doReq(t, baseURL(t, srv.URL()), token, "/api/reject", map[string]interface{}{
		"id":     mem.ID,
		"reason": "",
	})
	defer resp.Body.Close()

	r := readJSON(t, resp)
	if !r.OK {
		t.Errorf("reject empty reason: want ok=true, got %s", r.Err)
	}
}
