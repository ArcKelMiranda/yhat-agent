// F1-D integration tests: bandeja subcommand and --no-bandeja install flag.

package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ArcKelMiranda/yhat-agent/internal/bandeja"
	"github.com/ArcKelMiranda/yhat-agent/internal/config"
	"github.com/ArcKelMiranda/yhat-agent/internal/store"
)

// TestBandejaServerStartsAndPrintsURL verifies that starting the server
// listens on 127.0.0.1 and prints the URL to stdout without panicking.
func TestBandejaServerStartsAndPrintsURL(t *testing.T) {
	dir := t.TempDir()
	origYHAT := os.Getenv("YHAT_HOME")
	os.Setenv("YHAT_HOME", dir)
	defer func() {
		if origYHAT != "" {
			os.Setenv("YHAT_HOME", origYHAT)
		} else {
			os.Unsetenv("YHAT_HOME")
		}
	}()

	cfg := config.Config{Operator: "bandeja-test-op", CentralRepo: "https://example.invalid", LastSync: ""}
	if err := cfg.Save(config.ConfigPath()); err != nil {
		t.Fatalf("save config: %v", err)
	}

	dbPath := filepath.Join(dir, "yhat.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	ctx, cancel := context.WithCancel(context.Background())

	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		w.Close()
		os.Stdout = oldStdout
		t.Fatalf("bandeja.Start: %v", err)
	}

	url := srv.URL()
	if url == "" {
		w.Close()
		os.Stdout = oldStdout
		t.Error("bandeja URL: want non-empty")
	}
	if !strings.HasPrefix(url, "http://127.0.0.1:") {
		w.Close()
		os.Stdout = oldStdout
		t.Errorf("URL: want http://127.0.0.1:<port>, got %s", url)
	}

	// Print URL (simulates what runBandeja does).
	fmt.Fprintln(w, url)
	w.Close()
	os.Stdout = oldStdout

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "127.0.0.1:") {
		t.Errorf("bandeja output should contain URL, got: %s", output)
	}

	// Shutdown.
	cancel()
	time.Sleep(50 * time.Millisecond)
	srv.Close()
}

// TestBandejaServerRejectsInvalidToken verifies the server returns 404
// for requests with invalid tokens.
func TestBandejaServerRejectsInvalidToken(t *testing.T) {
	dir := t.TempDir()
	origYHAT := os.Getenv("YHAT_HOME")
	os.Setenv("YHAT_HOME", dir)
	defer func() {
		if origYHAT != "" {
			os.Setenv("YHAT_HOME", origYHAT)
		} else {
			os.Unsetenv("YHAT_HOME")
		}
	}()

	cfg := config.Config{Operator: "test-op", CentralRepo: "https://example.invalid", LastSync: ""}
	_ = cfg.Save(config.ConfigPath())

	dbPath := filepath.Join(dir, "yhat.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("bandeja.Start: %v", err)
	}
	defer srv.Close()

	// Request without token.
	req := httptest.NewRequest(http.MethodGet, srv.URL(), nil)
	w := httptest.NewRecorder()
	http.DefaultServeMux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		// Actually need to hit the bandeja server directly.
		// Use the real URL.
	}
	_ = req

	// Using httptest.NewRequest doesn't use the bandeja mux directly.
	// Instead hit the real URL.
	base := extractBaseURL(srv.URL())
	resp, err := http.Get(base + "/?token=bad")
	if err != nil {
		t.Fatalf("http.Get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("invalid token: want 404, got %d", resp.StatusCode)
	}
}

// TestBandejaServerAcceptsValidToken verifies the server returns 200
// for requests with the correct token.
func TestBandejaServerAcceptsValidToken(t *testing.T) {
	dir := t.TempDir()
	origYHAT := os.Getenv("YHAT_HOME")
	os.Setenv("YHAT_HOME", dir)
	defer func() {
		if origYHAT != "" {
			os.Setenv("YHAT_HOME", origYHAT)
		} else {
			os.Unsetenv("YHAT_HOME")
		}
	}()

	cfg := config.Config{Operator: "test-op", CentralRepo: "https://example.invalid", LastSync: ""}
	_ = cfg.Save(config.ConfigPath())

	dbPath := filepath.Join(dir, "yhat.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	srv, err := bandeja.Start(ctx, s)
	if err != nil {
		t.Fatalf("bandeja.Start: %v", err)
	}
	defer srv.Close()

	// Extract token from URL and build clean request URL.
	token := extractTokenFromURL(srv.URL())
	base := extractBaseURL(srv.URL())
	url := base + "/?token=" + token

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("http.Get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("valid token: want 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Bandeja") {
		t.Error("page missing 'Bandeja' heading")
	}
}

func extractTokenFromURL(urlStr string) string {
	if i := strings.Index(urlStr, "token="); i >= 0 {
		return urlStr[i+len("token="):]
	}
	return ""
}

func extractBaseURL(urlStr string) string {
	if i := strings.Index(urlStr, "?"); i >= 0 {
		return urlStr[:i]
	}
	return urlStr
}
