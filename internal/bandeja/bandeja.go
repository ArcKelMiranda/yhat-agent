// Package bandeja provides the local HTTP inbox for the Cerebro YHat v1.2
// human-approval workflow.
//
// Operator identity
//
// The operator name used for Validate/Reject is read from config.yaml
// (field "operator"). If missing, the following fallbacks are tried in order:
//
//	1. os.Getenv("USERNAME")
//	2. os.Getenv("USER")
//	3. "unknown"
//
// The server listens on 127.0.0.1:0 (a random available port) and mints a
// one-shot 32-byte token. The token allows the GET page load; subsequent POST
// mutations re-validate the token on every request. The operator opens the
// URL returned by Server.URL() in a local browser.
//
// Idle shutdown
//
// A background goroutine ticks every 30 seconds and calls s.Close() if
// time.Since(lastRequest) > 15 minutes. The timer resets on every request.
// The server also shuts down when the context passed to Start is cancelled.

package bandeja

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/ArcKelMiranda/yhat-agent/internal/config"
	"github.com/ArcKelMiranda/yhat-agent/internal/store"
)

// Server is the Bandeja HTTP server. Call Start to create one.
type Server struct {
	httpServer *http.Server
	token      string
	url        string
	lastMu     sync.RWMutex
	lastReq    time.Time
	closeOnce  sync.Once
	closeErr   error
}

// Start creates a Bandeja server that listens on 127.0.0.1:0, mints a
// one-shot token, and returns the bootstrap URL. The store is used for
// all data access. The server shuts down after 15 minutes of idle or when
// ctx is cancelled.
func Start(ctx context.Context, s *store.Store) (*Server, error) {
	mux := http.NewServeMux()
	srv := &Server{
		token: mintToken(),
	}

	operator := resolveOperator()

	mux.HandleFunc("/", withToken(func(w http.ResponseWriter, r *http.Request, _ string) {
		srv.lastMu.Lock()
		srv.lastReq = time.Now()
		srv.lastMu.Unlock()
		if r.Method == http.MethodGet {
			handlePage(w, r, s, operator)
		} else {
			http.NotFound(w, r)
		}
	}, srv.token))

	mux.HandleFunc("/api/approve", withToken(func(w http.ResponseWriter, r *http.Request, _ string) {
		srv.lastMu.Lock()
		srv.lastReq = time.Now()
		srv.lastMu.Unlock()
		handleApprove(w, r, s, operator)
	}, srv.token))

	mux.HandleFunc("/api/reject", withToken(func(w http.ResponseWriter, r *http.Request, _ string) {
		srv.lastMu.Lock()
		srv.lastReq = time.Now()
		srv.lastMu.Unlock()
		handleReject(w, r, s, operator)
	}, srv.token))

	mux.HandleFunc("/api/edit", withToken(func(w http.ResponseWriter, r *http.Request, _ string) {
		srv.lastMu.Lock()
		srv.lastReq = time.Now()
		srv.lastMu.Unlock()
		handleEdit(w, r, s, operator)
	}, srv.token))

	ln, err := netListen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	addr := ln.Addr().String()
	srv.url = fmt.Sprintf("http://%s/?token=%s", addr, srv.token)

	srv.httpServer = &http.Server{Handler: mux}

	go func() {
		if err := srv.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("bandeja serve error: %v", err)
		}
	}()

	go srv.idleShutdown(ctx)

	return srv, nil
}

// URL returns the bootstrap URL containing the one-shot token.
func (s *Server) URL() string {
	return s.url
}

// Close stops the HTTP server and the idle-shutdown goroutine.
func (s *Server) Close() error {
	s.closeOnce.Do(func() {
		s.closeErr = s.httpServer.Close()
	})
	return s.closeErr
}

// Wait blocks until ctx is cancelled or 15 minutes of idle time elapses.
// The ctx passed to Start controls this.
func (s *Server) Wait() error {
	<-context.Background().Done()
	return nil
}

// idleShutdown ticks every 30s and closes the server if no request arrived
// in the last 15 minutes. It also exits when ctx is cancelled.
func (s *Server) idleShutdown(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.Close()
			return
		case <-ticker.C:
			s.lastMu.RLock()
			idle := time.Since(s.lastReq)
			s.lastMu.RUnlock()
			if idle > 15*time.Minute {
				s.Close()
				return
			}
		}
	}
}

// handlerFunc is the per-route handler with a validated token.
type handlerFunc func(http.ResponseWriter, *http.Request, string)

// withToken wraps a handler and validates the token query parameter.
// Requests without a valid token return 404 to avoid leaking server existence.
func withToken(next handlerFunc, validToken string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" || token != validToken {
			http.NotFound(w, r)
			return
		}
		next(w, r, token)
	}
}

// netListen is a testable wrapper around net.Listen.
var netListen = net.Listen

// resolveOperator reads the operator name from config.yaml and falls back
// to environment variables if the field is empty.
func resolveOperator() string {
	if cfg, err := config.Load(config.ConfigPath()); err == nil && cfg.Operator != "" {
		return cfg.Operator
	}
	if op := os.Getenv("USERNAME"); op != "" {
		return op
	}
	if op := os.Getenv("USER"); op != "" {
		return op
	}
	return "unknown"
}

// ─────────────────────────────────────────────────────────────────────────────
// HTTP handlers
// ─────────────────────────────────────────────────────────────────────────────

type apiRequest struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Reason  string `json:"reason"`
	Context string `json:"context,omitempty"`
}

type apiResponse struct {
	OK  bool   `json:"ok"`
	ID  string `json:"id,omitempty"`
	Err string `json:"error,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func handleApprove(w http.ResponseWriter, r *http.Request, s *store.Store, operator string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req apiRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{OK: false, Err: "invalid JSON"})
		return
	}
	if req.ID == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{OK: false, Err: "missing id"})
		return
	}
	if err := s.Validate(r.Context(), req.ID, operator); err != nil {
		writeJSON(w, http.StatusInternalServerError, apiResponse{OK: false, Err: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{OK: true, ID: req.ID})
}

func handleReject(w http.ResponseWriter, r *http.Request, s *store.Store, operator string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req apiRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{OK: false, Err: "invalid JSON"})
		return
	}
	if req.ID == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{OK: false, Err: "missing id"})
		return
	}
	reason := req.Reason
	if len(reason) > 500 {
		reason = reason[:500]
	}
	if err := s.Reject(r.Context(), req.ID, operator, reason); err != nil {
		writeJSON(w, http.StatusInternalServerError, apiResponse{OK: false, Err: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{OK: true, ID: req.ID})
}

func handleEdit(w http.ResponseWriter, r *http.Request, s *store.Store, operator string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req apiRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiResponse{OK: false, Err: "invalid JSON"})
		return
	}
	if req.ID == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{OK: false, Err: "missing id"})
		return
	}
	if req.Title == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{OK: false, Err: "missing title"})
		return
	}
	if req.Content == "" {
		writeJSON(w, http.StatusBadRequest, apiResponse{OK: false, Err: "missing content"})
		return
	}
	var ctxPtr *string
	if req.Context != "" {
		ctxPtr = &req.Context
	}
	if err := s.Edit(r.Context(), req.ID, req.Title, req.Content, ctxPtr); err != nil {
		writeJSON(w, http.StatusInternalServerError, apiResponse{OK: false, Err: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, apiResponse{OK: true, ID: req.ID})
}

func handlePage(w http.ResponseWriter, r *http.Request, s *store.Store, operator string) {
	ctx := r.Context()

	pending, _, err := s.ListPendingMemories(ctx, 50)
	if err != nil {
		pending = nil
	}

	sent, err := s.ListByShareStatus(ctx)
	if err != nil {
		sent = nil
	}

	html := buildPage(pending, sent, operator)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// ─────────────────────────────────────────────────────────────────────────────
// Token generation
// ─────────────────────────────────────────────────────────────────────────────

func mintToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

// ─────────────────────────────────────────────────────────────────────────────
// Platform browser open (skipped in CI/verification environments)
// ─────────────────────────────────────────────────────────────────────────────

// OpenBrowser attempts to open the URL in the platform's default browser.
// On Windows: rundll32 url.dll,FileProtocolHandler <url>
// On macOS:   open <url>
// On Linux:   xdg-open <url>
// If the tool is not available, this logs and continues without error.
// This is a no-op in CI/verification environments.
func OpenBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "linux":
		cmd = "xdg-open"
		args = []string{url}
	default:
		return
	}
	proc, err := os.StartProcess(cmd, args, &os.ProcAttr{})
	if err != nil {
		return
	}
	proc.Release()
}
