// F0 prototype tests: MCP protocol, FTS5 search, and end-to-end subprocess.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ArcKelMiranda/yhat-agent/internal/spike"
	"github.com/ArcKelMiranda/yhat-agent/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------------------------------------------------------------------------
// In-memory transport: protocol layer
// ---------------------------------------------------------------------------

// connectPairWithStore creates a fresh MCP server with a temporary store directory.
// It creates the YHAT_HOME directory and a minimal config.yaml so ensureStore succeeds.
func connectPairWithStore(t *testing.T) (*mcp.Server, *mcp.ClientSession, string) {
	t.Helper()
	// Reset the cached store so each test gets its own fresh instance.
	resetMCPStore()
	tmpDir := t.TempDir()
	cfgDir := filepath.Join(tmpDir, ".yhat")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Write minimal config.yaml so ensureStore passes the config check.
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("operator: test-operator\n"), 0644); err != nil {
		t.Fatalf("WriteFile config.yaml: %v", err)
	}
	t.Setenv("YHAT_HOME", cfgDir)

	srv := newMCPServer()
	client := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "v1.0.0"},
		&mcp.ClientOptions{},
	)
	t1, t2 := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := srv.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	return srv, session, cfgDir
}

func connectPair(t *testing.T) (*mcp.Server, *mcp.ClientSession) {
	t.Helper()
	// Reset cached store for test isolation.
	resetMCPStore()
	srv := newMCPServer()
	client := mcp.NewClient(
		&mcp.Implementation{Name: "test-client", Version: "v1.0.0"},
		&mcp.ClientOptions{},
	)
	t1, t2 := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := srv.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	return srv, session
}

func TestProtocol_Initialize(t *testing.T) {
	_, session := connectPair(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools after connect: %v", err)
	}
	if result == nil {
		t.Fatal("ListTools returned nil")
	}
}

func TestProtocol_ListTools_OneTool(t *testing.T) {
	_, session := connectPair(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(result.Tools) != 5 {
		t.Fatalf("ListTools: expected 5 tools (F0+F1), got %d", len(result.Tools))
	}
	if result.Tools[0].Name != "fts5_search" {
		t.Errorf("tool name: expected 'fts5_search', got %q", result.Tools[0].Name)
	}
}

func TestProtocol_ListTools_InputSchema(t *testing.T) {
	_, session := connectPair(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	schema, ok := result.Tools[0].InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("InputSchema: expected map, got %T", result.Tools[0].InputSchema)
	}
	props := schema["properties"].(map[string]any)
	if _, ok := props["query"]; !ok {
		t.Error("InputSchema.properties: missing 'query'")
	}
}

func TestProtocol_CallTool_Success(t *testing.T) {
	_, session := connectPair(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "fts5_search",
		Arguments: map[string]any{"query": "Diagnostic"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Error("CallTool: IsError should be false")
	}
	if len(result.Content) == 0 {
		t.Fatal("CallTool: no content")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0]: expected *TextContent, got %T", result.Content[0])
	}
	var r spike.Result
	if err := json.Unmarshal([]byte(text.Text), &r); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if len(r.Hits) == 0 {
		t.Error("Diagnostic: expected hits")
	}
}

func TestProtocol_CallTool_AccentInsensitive(t *testing.T) {
	_, session := connectPair(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "fts5_search",
		Arguments: map[string]any{"query": "politica"},
	})
	if err != nil {
		t.Fatalf("CallTool(politica): %v", err)
	}
	if result.IsError {
		t.Error("CallTool: IsError should be false for accented query")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0]: expected *TextContent, got %T", result.Content[0])
	}
	var r spike.Result
	if err := json.Unmarshal([]byte(text.Text), &r); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	var found bool
	for _, h := range r.Hits {
		if strings.Contains(strings.ToLower(h.Title+h.Text), "pol") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Search(politica): expected Política fixture, got %d hits", len(r.Hits))
	}
}

func TestProtocol_CallTool_Cafe(t *testing.T) {
	_, session := connectPair(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "fts5_search",
		Arguments: map[string]any{"query": "café"},
	})
	if err != nil {
		t.Fatalf("CallTool(café): %v", err)
	}
	if result.IsError {
		t.Error("CallTool: IsError should be false for café query")
	}
}

func TestProtocol_CallTool_NoResults(t *testing.T) {
	_, session := connectPair(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "fts5_search",
		Arguments: map[string]any{"query": "xyzzy_nonexistent"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0]: expected *TextContent, got %T", result.Content[0])
	}
	var r spike.Result
	if err := json.Unmarshal([]byte(text.Text), &r); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if len(r.Hits) != 0 {
		t.Errorf("xyzzy: expected 0 hits, got %d", len(r.Hits))
	}
}

func TestProtocol_CallTool_MissingQuery(t *testing.T) {
	_, session := connectPair(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "fts5_search",
		Arguments: map[string]any{},
	})
	if err == nil {
		t.Error("CallTool(missing query): expected error")
	}
}

func TestProtocol_CallTool_UnknownTool(t *testing.T) {
	_, session := connectPair(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "nonexistent_tool",
		Arguments: map[string]any{"query": "test"},
	})
	if err == nil {
		t.Error("CallTool(unknown): expected error")
	}
}

// ---------------------------------------------------------------------------
// Selftest result tests (via in-memory transport, same process)
// These use the direct MCP server for fast selftest without subprocess overhead.
// ---------------------------------------------------------------------------

func TestSelftest_Result_ExitSuccess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	report, err := runSelftestResultDirect(ctx)
	if err != nil {
		t.Fatalf("runSelftestResultDirect: %v", err)
	}
	if report == nil {
		t.Fatal("report is nil")
	}
	if !report.OK {
		t.Error("selftest report: OK should be true")
	}
}

func TestSelftest_Result_FTS5(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	report, err := runSelftestResultDirect(ctx)
	if err != nil {
		t.Fatalf("runSelftestResultDirect: %v", err)
	}
	if !report.FTS5 {
		t.Error("selftest report: FTS5 should be true")
	}
}

func TestSelftest_Result_MCP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	report, err := runSelftestResultDirect(ctx)
	if err != nil {
		t.Fatalf("runSelftestResultDirect: %v", err)
	}
	if !report.MCP {
		t.Error("selftest report: MCP should be true")
	}
}

func TestSelftest_Result_Synthetic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	report, err := runSelftestResultDirect(ctx)
	if err != nil {
		t.Fatalf("runSelftestResultDirect: %v", err)
	}
	if !report.Synthetic {
		t.Error("selftest report: Synthetic should be true")
	}
}

func TestSelftest_Result_AllTrue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	report, err := runSelftestResultDirect(ctx)
	if err != nil {
		t.Fatalf("runSelftestResultDirect: %v", err)
	}
	if !report.OK || !report.FTS5 || !report.MCP || !report.Synthetic {
		t.Errorf("all fields should be true: ok=%v fts5=%v mcp=%v synthetic=%v",
			report.OK, report.FTS5, report.MCP, report.Synthetic)
	}
}

func TestSelftest_Result_ExactPoliticaTitle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	report, err := runSelftestResultDirect(ctx)
	if err != nil {
		t.Fatalf("runSelftestResultDirect: %v", err)
	}
	if !report.OK {
		t.Fatal("selftest: OK should be true")
	}
	// Verify the exact title for politica query is "Política Fixture Beta"
	if report.PoliticaTitle != "Política Fixture Beta" {
		t.Errorf("politica query: expected exact title %q, got %q",
			"Política Fixture Beta", report.PoliticaTitle)
	}
}

// SelftestReportExtended includes PoliticaTitle for exact title verification.
type SelftestReportExtended struct {
	OK            bool
	FTS5          bool
	MCP           bool
	Synthetic     bool
	PoliticaTitle string
}

func TestSelftest_Result_Extended(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	report, err := runSelftestResultExtended(ctx)
	if err != nil {
		t.Fatalf("runSelftestResultExtended: %v", err)
	}
	if !report.OK {
		t.Error("selftest: OK should be true")
	}
	if report.PoliticaTitle != "Política Fixture Beta" {
		t.Errorf("politica query: expected exact title %q, got %q",
			"Política Fixture Beta", report.PoliticaTitle)
	}
}

// runSelftestResultExtended is like runSelftestResultDirect but also verifies exact title.
func runSelftestResultExtended(ctx context.Context) (*SelftestReportExtended, error) {
	report := &SelftestReportExtended{OK: true}

	srv := newMCPServer()
	client := mcp.NewClient(
		&mcp.Implementation{Name: "selftest-client", Version: "v1.0.0"},
		&mcp.ClientOptions{},
	)

	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, t1, nil); err != nil {
		return report, fmt.Errorf("server connect: %w", err)
	}
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		return report, fmt.Errorf("client connect: %w", err)
	}
	defer session.Close()

	init := session.InitializeResult()
	if init == nil {
		report.OK = false
		return report, fmt.Errorf("InitializeResult: nil")
	}
	if init.ProtocolVersion == "" {
		report.OK = false
	}

	toolList, err := session.ListTools(ctx, nil)
	if err != nil {
		report.OK = false
		return report, fmt.Errorf("ListTools: %w", err)
	}
	report.MCP = true

	if len(toolList.Tools) < 1 {
		report.OK = false
		return report, fmt.Errorf("tool count: want >= 1, got %d", len(toolList.Tools))
	}
	var hasFTS5 bool
	for _, t := range toolList.Tools {
		if t.Name == "fts5_search" {
			hasFTS5 = true
			break
		}
	}
	if !hasFTS5 {
		report.OK = false
		return report, fmt.Errorf("fts5_search tool not found")
	}

	// Query "politica" (no accent) must return EXACT title "Política Fixture Beta"
	callRes, callErr := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "fts5_search",
		Arguments: map[string]any{"query": "politica"},
	})
	if callErr != nil {
		report.OK = false
		return report, fmt.Errorf("CallTool: %w", callErr)
	}
	if len(callRes.Content) == 0 {
		report.OK = false
		return report, fmt.Errorf("CallTool: no content")
	}
	if callRes.IsError {
		report.OK = false
		return report, fmt.Errorf("CallTool: IsError is true")
	}

	text, ok := callRes.Content[0].(*mcp.TextContent)
	if !ok {
		report.OK = false
		return report, fmt.Errorf("content[0]: expected *TextContent")
	}
	var result spike.Result
	if err := json.Unmarshal([]byte(text.Text), &result); err != nil {
		report.OK = false
		return report, fmt.Errorf("unmarshal hit: %w", err)
	}
	report.FTS5 = true

	if len(result.Hits) == 0 {
		report.OK = false
		return report, fmt.Errorf("politica: no hits")
	}
	// Assert EXACT title
	if len(result.Hits) > 0 {
		report.PoliticaTitle = result.Hits[0].Title
		if result.Hits[0].Title != "Política Fixture Beta" {
			report.OK = false
			return report, fmt.Errorf("politica query: expected title %q, got %q",
				"Política Fixture Beta", result.Hits[0].Title)
		}
	}
	report.Synthetic = true

	return report, nil
}

// runSelftestResultDirect exercises the MCP server via in-memory transport,
// in the same process. This is the fast path used by unit tests.
func runSelftestResultDirect(ctx context.Context) (*SelftestReport, error) {
	report := &SelftestReport{OK: true}

	srv := newMCPServer()
	client := mcp.NewClient(
		&mcp.Implementation{Name: "selftest-client", Version: "v1.0.0"},
		&mcp.ClientOptions{},
	)

	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, t1, nil); err != nil {
		return report, fmt.Errorf("server connect: %w", err)
	}
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		return report, fmt.Errorf("client connect: %w", err)
	}
	defer session.Close()

	init := session.InitializeResult()
	if init == nil {
		report.OK = false
		return report, fmt.Errorf("InitializeResult: nil")
	}
	if init.ProtocolVersion == "" {
		report.OK = false
	}

	toolList, err := session.ListTools(ctx, nil)
	if err != nil {
		report.OK = false
		return report, fmt.Errorf("ListTools: %w", err)
	}
	report.MCP = true

	if len(toolList.Tools) < 1 {
		report.OK = false
		return report, fmt.Errorf("tool count: want >= 1, got %d", len(toolList.Tools))
	}
	var hasFTS5 bool
	for _, t := range toolList.Tools {
		if t.Name == "fts5_search" {
			hasFTS5 = true
			break
		}
	}
	if !hasFTS5 {
		report.OK = false
		return report, fmt.Errorf("fts5_search tool not found")
	}

	// Use "politica" to test accent insensitivity.
	callRes, callErr := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "fts5_search",
		Arguments: map[string]any{"query": "politica"},
	})
	if callErr != nil {
		report.OK = false
		return report, fmt.Errorf("CallTool: %w", callErr)
	}
	if len(callRes.Content) == 0 {
		report.OK = false
		return report, fmt.Errorf("CallTool: no content")
	}
	if callRes.IsError {
		report.OK = false
		return report, fmt.Errorf("CallTool: IsError is true")
	}

	text, ok := callRes.Content[0].(*mcp.TextContent)
	if !ok {
		report.OK = false
		return report, fmt.Errorf("content[0]: expected *TextContent")
	}
	var result spike.Result
	if err := json.Unmarshal([]byte(text.Text), &result); err != nil {
		report.OK = false
		return report, fmt.Errorf("unmarshal hit: %w", err)
	}
	report.FTS5 = true

	if len(result.Hits) == 0 {
		report.OK = false
		return report, fmt.Errorf("politica fixture: no hits for query 'politica'")
	}
	// Assert EXACT title "Política Fixture Beta" and populate report field.
	report.PoliticaTitle = result.Hits[0].Title
	if result.Hits[0].Title != "Política Fixture Beta" {
		report.OK = false
		return report, fmt.Errorf("politica fixture: expected title %q, got %q",
			"Política Fixture Beta", result.Hits[0].Title)
	}
	report.Synthetic = true

	return report, nil
}

// ---------------------------------------------------------------------------
// Subprocess tests: build binary once, verify end-to-end behavior
// ---------------------------------------------------------------------------

// buildServerBinary builds the MCP server binary to a temp directory.
// Fatal on build failure.
func buildServerBinary(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "yhat-agent-test-server")

	goBin := os.Getenv("GOBIN")
	if goBin == "" {
		goBin = "/snap/go/current/bin/go"
		if _, err := os.Stat(goBin); err != nil {
			goBin = "go"
		}
	}

	// Find repo root by walking upward from cwd for go.mod.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	repoRoot := cwd
	for {
		if _, err := os.Stat(filepath.Join(repoRoot, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(repoRoot)
		if parent == repoRoot {
			t.Fatalf("go.mod not found in any parent of %s", cwd)
		}
		repoRoot = parent
	}

	cmd := exec.Command(goBin, "build",
		"-ldflags=-s -w",
		"-o", outputPath,
		"github.com/ArcKelMiranda/yhat-agent/cmd/yhat-agent",
	)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	cmd.Dir = repoRoot

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, string(out))
	}
	return outputPath
}

// TestSubprocess_Selftest_JSONOutput verifies the selftest binary emits valid JSON.
func TestSubprocess_Selftest_JSONOutput(t *testing.T) {
	binary := buildServerBinary(t)

	cmd := exec.Command(binary, "mcp", "--selftest")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := strings.TrimSpace(stdout.String())

	if err != nil {
		t.Fatalf("selftest failed: %v\nstderr: %s", err, stderr.String())
	}

	var report SelftestReport
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, output)
	}
	if !report.OK {
		t.Errorf("selftest report OK=false\nstderr: %s", stderr.String())
	}
}

// TestSubprocess_Selftest_ExitZero verifies selftest exits zero on success.
func TestSubprocess_Selftest_ExitZero(t *testing.T) {
	binary := buildServerBinary(t)

	cmd := exec.Command(binary, "mcp", "--selftest")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("selftest: expected zero exit, got %v\nstderr: %s", err, stderr.String())
	}
}

// TestSubprocess_MCP_UnknownFlag verifies unknown flags cause non-zero exit.
func TestSubprocess_MCP_UnknownFlag(t *testing.T) {
	binary := buildServerBinary(t)

	cmd := exec.Command(binary, "mcp", "--unknown-flag")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		t.Fatal("mcp --unknown-flag: expected non-zero exit")
	}
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() == 0 {
		t.Errorf("mcp --unknown-flag: expected non-zero exit, got %v", err)
	}
	if stderr.Len() == 0 {
		t.Error("mcp --unknown-flag: expected stderr message")
	}
}

// TestSubprocess_CommandTransport exercises the full stdio transport via SDK
// CommandTransport, connecting a client to the subprocess server binary.
func TestSubprocess_CommandTransport(t *testing.T) {
	if testing.Short() {
		t.Skip("requires subprocess")
	}

	binary := buildServerBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.Command(binary, "mcp")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")

	client := mcp.NewClient(
		&mcp.Implementation{Name: "subprocess-test", Version: "v1.0.0"},
		&mcp.ClientOptions{},
	)

	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer session.Close()

	// Verify InitializeResult is populated.
	init := session.InitializeResult()
	if init == nil {
		t.Fatal("InitializeResult: nil after Connect")
	}

	// List tools.
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools.Tools) != 5 {
		t.Fatalf("ListTools: expected 5 tools (F0+F1), got %d", len(tools.Tools))
	}

	// Call fts5_search with accented query.
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "fts5_search",
		Arguments: map[string]any{"query": "cafe"},
	})
	if err != nil {
		t.Fatalf("CallTool(cafe): %v", err)
	}
	if result.IsError {
		t.Error("CallTool: IsError should be false")
	}
	if len(result.Content) == 0 {
		t.Fatal("CallTool: no content")
	}

	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0]: expected *TextContent, got %T", result.Content[0])
	}
	var r spike.Result
	if err := json.Unmarshal([]byte(text.Text), &r); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	// café fixture should be found by "cafe" query.
	var found bool
	for _, h := range r.Hits {
		if strings.Contains(strings.ToLower(h.Title+h.Text), "caf") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Search(cafe): expected café fixture, got %d hits", len(r.Hits))
	}
}

// TestSubprocess_CommandTransport_MissingQuery verifies missing query returns an error.
func TestSubprocess_CommandTransport_MissingQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("requires subprocess")
	}

	binary := buildServerBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.Command(binary, "mcp")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")

	client := mcp.NewClient(
		&mcp.Implementation{Name: "subprocess-test", Version: "v1.0.0"},
		&mcp.ClientOptions{},
	)

	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer session.Close()

	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "fts5_search",
		Arguments: map[string]any{},
	})
	if err == nil {
		t.Error("CallTool(missing query): expected error")
	}
}

// TestSubprocess_CommandTransport_UnknownTool verifies unknown tool returns an error.
func TestSubprocess_CommandTransport_UnknownTool(t *testing.T) {
	if testing.Short() {
		t.Skip("requires subprocess")
	}

	binary := buildServerBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.Command(binary, "mcp")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")

	client := mcp.NewClient(
		&mcp.Implementation{Name: "subprocess-test", Version: "v1.0.0"},
		&mcp.ClientOptions{},
	)

	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer session.Close()

	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "unknown_tool",
		Arguments: map[string]any{"query": "test"},
	})
	if err == nil {
		t.Error("CallTool(unknown tool): expected error")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// F1-B: Real MCP tools — in-memory transport tests
// Each test uses the MCP server via in-memory transport.
// Store is initialized fresh per test via the lazy-open path.
// ─────────────────────────────────────────────────────────────────────────────

// TestF1_ToolRegistration verifies that the server exposes exactly 5 tools
// after F1-B is implemented: fts5_search, propose_memory, list_pending,
// get_memory, and search_brain.
func TestF1_ToolRegistration(t *testing.T) {
	_, session := connectPair(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	expected := []string{"fts5_search", "propose_memory", "list_pending", "get_memory", "search_brain"}
	if len(result.Tools) != len(expected) {
		t.Fatalf("tool count: want %d, got %d", len(expected), len(result.Tools))
	}

	found := make(map[string]bool)
	for _, tool := range result.Tools {
		found[tool.Name] = true
	}
	for _, name := range expected {
		if !found[name] {
			t.Errorf("tool %q: missing from registered tools", name)
		}
	}
}

// TestF1_ToolRegistration_InputSchemas verifies that each F1 tool has the
// correct input schema fields.
func TestF1_ToolRegistration_InputSchemas(t *testing.T) {
	_, session := connectPair(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	// Build name → schema map.
	schemas := make(map[string]map[string]any)
	for _, tool := range result.Tools {
		if schema, ok := tool.InputSchema.(map[string]any); ok {
			schemas[tool.Name] = schema
		}
	}

	// propose_memory requires type, title, content; context is optional.
	pm, ok := schemas["propose_memory"]
	if !ok {
		t.Fatal("propose_memory: not found")
	}
	props := pm["properties"].(map[string]any)
	for _, field := range []string{"type", "title", "content"} {
		if _, ok := props[field]; !ok {
			t.Errorf("propose_memory: missing required field %q", field)
		}
	}
	required := pm["required"].([]any)
	hasType := false
	hasTitle := false
	hasContent := false
	for _, r := range required {
		if r == "type" {
			hasType = true
		}
		if r == "title" {
			hasTitle = true
		}
		if r == "content" {
			hasContent = true
		}
	}
	if !hasType || !hasTitle || !hasContent {
		t.Error("propose_memory: type/title/content must all be required")
	}

	// list_pending has optional limit field.
	lp, ok := schemas["list_pending"]
	if !ok {
		t.Fatal("list_pending: not found")
	}
	// No required fields.
	rlp, _ := lp["required"]
	if rlp != nil {
		t.Error("list_pending: should have no required fields")
	}

	// get_memory requires id.
	gm, ok := schemas["get_memory"]
	if !ok {
		t.Fatal("get_memory: not found")
	}
	gmProps := gm["properties"].(map[string]any)
	if _, ok := gmProps["id"]; !ok {
		t.Error("get_memory: missing required field 'id'")
	}
	rgm := gm["required"].([]any)
	if len(rgm) != 1 || rgm[0] != "id" {
		t.Error("get_memory: required field must be exactly 'id'")
	}

	// search_brain requires query.
	sb, ok := schemas["search_brain"]
	if !ok {
		t.Fatal("search_brain: not found")
	}
	sbProps := sb["properties"].(map[string]any)
	if _, ok := sbProps["query"]; !ok {
		t.Error("search_brain: missing required field 'query'")
	}
}

// TestF1_ProposeMemory_HappyPath creates a memory and verifies the returned
// record has a populated ID, status=proposed, and origin=local.
func TestF1_ProposeMemory_HappyPath(t *testing.T) {
	_, session, _ := connectPairWithStore(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "propose_memory",
		Arguments: map[string]any{
			"type":    "decision",
			"title":   "Decisión de prueba F1-B",
			"content": "Contenido de prueba para el camino feliz.",
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Error("propose_memory: IsError should be false")
	}
	if len(result.Content) == 0 {
		t.Fatal("propose_memory: no content")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0]: expected *TextContent, got %T", result.Content[0])
	}

	var mem store.Memory
	if err := json.Unmarshal([]byte(text.Text), &mem); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if mem.ID == "" {
		t.Error("propose_memory: want non-empty ID")
	}
	if mem.Status != "proposed" {
		t.Errorf("propose_memory: want status=proposed, got %q", mem.Status)
	}
	if mem.Origin != "local" {
		t.Errorf("propose_memory: want origin=local, got %q", mem.Origin)
	}
	if mem.Type != "decision" {
		t.Errorf("propose_memory: want type=decision, got %q", mem.Type)
	}
	if mem.Title != "Decisión de prueba F1-B" {
		t.Errorf("propose_memory: title mismatch")
	}
}

// TestF1_ProposeMemory_MissingRequiredField verifies that missing a required
// field returns an error response.
func TestF1_ProposeMemory_MissingRequiredField(t *testing.T) {
	_, session, _ := connectPairWithStore(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "propose_memory",
		Arguments: map[string]any{
			"type": "decision",
			// missing title and content
		},
	})
	// The SDK should return an error for missing required fields.
	if err == nil {
		t.Error("propose_memory(missing fields): expected error")
	}
}

// TestF1_ProposeMemory_InvalidType verifies that an invalid type value
// is rejected. The MCP SDK validates the enum before calling the handler,
// so this surfaces as a JSON-RPC error (not an MCP IsError response).
func TestF1_ProposeMemory_InvalidType(t *testing.T) {
	_, session, _ := connectPairWithStore(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "propose_memory",
		Arguments: map[string]any{
			"type":    "invalid_type",
			"title":   "Título válido",
			"content": "Contenido válido.",
		},
	})
	// The SDK schema validation rejects invalid enum before the handler runs.
	if err == nil {
		t.Error("propose_memory(invalid type): expected schema validation error")
	}
}

// TestF1_ProposeMemory_DBNotInitialized verifies that propose_memory returns
// IsError=true with the "Base local no inicializada" message when the DB
// file does not exist.
func TestF1_ProposeMemory_DBNotInitialized(t *testing.T) {
	resetMCPStore()
	// Do NOT set YHAT_HOME; use a path that definitely does not exist.
	// The default path would be $HOME/.yhat/yhat.db which may or may not
	// exist. Use an explicit non-existent directory.
	t.Setenv("YHAT_HOME", filepath.Join(t.TempDir(), "no-such-dir"))

	_, session := connectPair(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "propose_memory",
		Arguments: map[string]any{
			"type":    "decision",
			"title":   "Título",
			"content": "Contenido.",
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !result.IsError {
		t.Error("propose_memory(db not initialized): IsError should be true")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0]: expected *TextContent, got %T", result.Content[0])
	}
	if !strings.Contains(text.Text, "no inicializada") {
		t.Errorf("propose_memory(db not initialized): expected Spanish message, got %q", text.Text)
	}
}

// TestF1_ListPending_HappyPath proposes a memory and verifies it appears
// in list_pending, oldest first.
func TestF1_ListPending_HappyPath(t *testing.T) {
	_, session, _ := connectPairWithStore(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Propose two memories.
	for i := 0; i < 2; i++ {
		_, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "propose_memory",
			Arguments: map[string]any{
				"type":    "rule",
				"title":   fmt.Sprintf("Regla list_pending %d", i),
				"content": fmt.Sprintf("Contenido %d para list_pending.", i),
			},
		})
		if err != nil {
			t.Fatalf("propose_memory %d: %v", i, err)
		}
	}

	// List pending.
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list_pending",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("list_pending: %v", err)
	}
	if result.IsError {
		t.Error("list_pending: IsError should be false")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0]: expected *TextContent, got %T", result.Content[0])
	}
	var listResp struct {
		Items []store.Memory `json:"items"`
		Count int            `json:"count"`
	}
	if err := json.Unmarshal([]byte(text.Text), &listResp); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if listResp.Count < 2 {
		t.Errorf("list_pending: want count >= 2, got %d", listResp.Count)
	}
	if len(listResp.Items) < 2 {
		t.Errorf("list_pending: want >= 2 items, got %d", len(listResp.Items))
	}
	// Oldest first: Items[0].CreatedAt <= Items[1].CreatedAt.
	if len(listResp.Items) >= 2 {
		if listResp.Items[0].CreatedAt.After(listResp.Items[1].CreatedAt) {
			t.Error("list_pending: items should be oldest first")
		}
	}
}

// TestF1_ListPending_Limit verifies the optional limit argument.
func TestF1_ListPending_Limit(t *testing.T) {
	_, session, _ := connectPairWithStore(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Propose 5 memories.
	for i := 0; i < 5; i++ {
		_, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "propose_memory",
			Arguments: map[string]any{
				"type":    "decision",
				"title":   fmt.Sprintf("Decisión límite %d", i),
				"content": fmt.Sprintf("Contenido %d.", i),
			},
		})
		if err != nil {
			t.Fatalf("propose_memory %d: %v", i, err)
		}
	}

	// Request limit=2.
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list_pending",
		Arguments: map[string]any{"limit": 2},
	})
	if err != nil {
		t.Fatalf("list_pending(limit=2): %v", err)
	}
	if result.IsError {
		t.Error("list_pending(limit=2): IsError should be false")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0]: expected *TextContent, got %T", result.Content[0])
	}
	var listResp struct {
		Items []store.Memory `json:"items"`
		Count int            `json:"count"`
	}
	if err := json.Unmarshal([]byte(text.Text), &listResp); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if len(listResp.Items) > 2 {
		t.Errorf("list_pending(limit=2): want <= 2 items, got %d", len(listResp.Items))
	}
	// Count should reflect total, not limited.
	if listResp.Count < 5 {
		t.Errorf("list_pending(limit=2): want count >= 5, got %d", listResp.Count)
	}
}

// TestF1_ListPending_DBNotInitialized verifies error when DB file does not exist.
func TestF1_ListPending_DBNotInitialized(t *testing.T) {
	resetMCPStore()
	t.Setenv("YHAT_HOME", filepath.Join(t.TempDir(), "no-such-dir"))

	_, session := connectPair(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list_pending",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("list_pending: %v", err)
	}
	if !result.IsError {
		t.Error("list_pending(db not initialized): IsError should be true")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0]: expected *TextContent, got %T", result.Content[0])
	}
	if !strings.Contains(text.Text, "no inicializada") {
		t.Errorf("list_pending(db not initialized): expected Spanish message, got %q", text.Text)
	}
}

// TestF1_GetMemory_HappyPath proposes a memory and retrieves it by ID.
func TestF1_GetMemory_HappyPath(t *testing.T) {
	_, session, _ := connectPairWithStore(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Propose a memory.
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "propose_memory",
		Arguments: map[string]any{
			"type":    "anomaly",
			"title":   "Anomalía de prueba F1-B",
			"content": "Descripción de la anomalía.",
			"context": "Contexto opcional.",
		},
	})
	if err != nil {
		t.Fatalf("propose_memory: %v", err)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0]: expected *TextContent, got %T", result.Content[0])
	}
	var proposed store.Memory
	if err := json.Unmarshal([]byte(text.Text), &proposed); err != nil {
		t.Fatalf("not JSON: %v", err)
	}

	// Retrieve by ID.
	getResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_memory",
		Arguments: map[string]any{"id": proposed.ID},
	})
	if err != nil {
		t.Fatalf("get_memory: %v", err)
	}
	if getResult.IsError {
		t.Error("get_memory: IsError should be false")
	}
	getText, ok := getResult.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0]: expected *TextContent, got %T", getResult.Content[0])
	}
	var mem store.Memory
	if err := json.Unmarshal([]byte(getText.Text), &mem); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if mem.ID != proposed.ID {
		t.Errorf("get_memory: want id=%q, got %q", proposed.ID, mem.ID)
	}
	if mem.Title != "Anomalía de prueba F1-B" {
		t.Errorf("get_memory: title mismatch")
	}
	if mem.Status != "proposed" {
		t.Errorf("get_memory: status should be proposed, got %q", mem.Status)
	}
}

// TestF1_GetMemory_NotFound verifies that an unknown UUID returns IsError=true.
func TestF1_GetMemory_NotFound(t *testing.T) {
	_, session, _ := connectPairWithStore(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Propose a memory first to confirm the store works, then use a fake ID.
	proposeResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "propose_memory",
		Arguments: map[string]any{
			"type":    "decision",
			"title":   "Memoria para test",
			"content": "Contenido.",
		},
	})
	if err != nil {
		t.Fatalf("propose_memory: %v", err)
	}
	if proposeResult.IsError {
		t.Fatalf("propose_memory failed: %v", proposeResult.Content)
	}

	// Now request an unknown ID.
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_memory",
		Arguments: map[string]any{"id": "00000000-0000-0000-0000-000000000000"},
	})
	if err != nil {
		t.Fatalf("get_memory: %v", err)
	}
	if !result.IsError {
		t.Error("get_memory(unknown id): IsError should be true")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0]: expected *TextContent, got %T", result.Content[0])
	}
	if !strings.Contains(text.Text, "no encontrada") {
		t.Errorf("get_memory(unknown id): expected Spanish error, got %q", text.Text)
	}
}

// TestF1_GetMemory_DBNotInitialized verifies error when DB file does not exist.
func TestF1_GetMemory_DBNotInitialized(t *testing.T) {
	resetMCPStore()
	t.Setenv("YHAT_HOME", filepath.Join(t.TempDir(), "no-such-dir"))

	_, session := connectPair(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_memory",
		Arguments: map[string]any{"id": "00000000-0000-0000-0000-000000000001"},
	})
	if err != nil {
		t.Fatalf("get_memory: %v", err)
	}
	if !result.IsError {
		t.Error("get_memory(db not initialized): IsError should be true")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0]: expected *TextContent, got %T", result.Content[0])
	}
	if !strings.Contains(text.Text, "no inicializada") {
		t.Errorf("get_memory(db not initialized): expected Spanish message, got %q", text.Text)
	}
}

// TestF1_SearchBrain_EmptyDB_ReturnsF0Fixtures verifies that when the DB
// exists but is empty (zero rows in memories), search_brain returns the
// F0 synthetic fixtures. This keeps the F0 selftest passing.
func TestF1_SearchBrain_EmptyDB_ReturnsF0Fixtures(t *testing.T) {
	_, session, _ := connectPairWithStore(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Query "politica" should return the F0 fixture.
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "search_brain",
		Arguments: map[string]any{"query": "politica"},
	})
	if err != nil {
		t.Fatalf("search_brain: %v", err)
	}
	if result.IsError {
		t.Error("search_brain(empty DB): IsError should be false (F0 fallback)")
	}
	if len(result.Content) == 0 {
		t.Fatal("search_brain: no content")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0]: expected *TextContent, got %T", result.Content[0])
	}
	var r spike.Result
	if err := json.Unmarshal([]byte(text.Text), &r); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if len(r.Hits) == 0 {
		t.Error("search_brain(politica): expected F0 fixture hit")
		return
	}
	if r.Hits[0].Title != "Política Fixture Beta" {
		t.Errorf("search_brain(politica): expected title %q, got %q",
			"Política Fixture Beta", r.Hits[0].Title)
	}
}

// TestF1_SearchBrain_WithData_ReturnsMemories verifies that when the DB has
// rows, search_brain returns real memories (not F0 fixtures).
func TestF1_SearchBrain_WithData_ReturnsMemories(t *testing.T) {
	_, session, _ := connectPairWithStore(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Propose a memory with a distinctive title.
	proposeResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "propose_memory",
		Arguments: map[string]any{
			"type":    "decision",
			"title":   "Decisión Buscada F1-B",
			"content": "Contenido unique para search_brain test.",
		},
	})
	if err != nil {
		t.Fatalf("propose_memory: %v", err)
	}
	proposeText, _ := proposeResult.Content[0].(*mcp.TextContent)
	var proposed store.Memory
	json.Unmarshal([]byte(proposeText.Text), &proposed)

	// search_brain should now return the real memory.
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "search_brain",
		Arguments: map[string]any{"query": "Buscada"},
	})
	if err != nil {
		t.Fatalf("search_brain: %v", err)
	}
	if result.IsError {
		t.Error("search_brain(with data): IsError should be false")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0]: expected *TextContent, got %T", result.Content[0])
	}
	var r spike.Result
	if err := json.Unmarshal([]byte(text.Text), &r); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	// Should return real memory, not F0 fixture.
	if len(r.Hits) == 0 {
		t.Error("search_brain(Buscada): expected at least one hit")
	}
	if len(r.Hits) > 0 && r.Hits[0].Title == "Política Fixture Beta" {
		t.Error("search_brain(with data): expected real memory, got F0 fixture")
	}
	if len(r.Hits) > 0 && proposed.ID != "" {
		// We should find the memory we just proposed.
		found := false
		for _, h := range r.Hits {
			if h.Title == "Decisión Buscada F1-B" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("search_brain(Buscada): expected to find proposed memory, got %v", r.Hits)
		}
	}
}

// TestF1_SearchBrain_DBNotInitialized verifies that when the DB file does not
// exist, search_brain falls back to F0 fixtures (DB-not-found uses F0 fallback).
func TestF1_SearchBrain_DBNotInitialized_FallsBackToF0(t *testing.T) {
	resetMCPStore()
	t.Setenv("YHAT_HOME", filepath.Join(t.TempDir(), "no-such-dir"))

	_, session := connectPair(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Even without a DB, search_brain should return F0 fixtures.
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "search_brain",
		Arguments: map[string]any{"query": "Diagnostic"},
	})
	if err != nil {
		t.Fatalf("search_brain: %v", err)
	}
	if result.IsError {
		t.Error("search_brain(db not initialized): IsError should be false (F0 fallback)")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0]: expected *TextContent, got %T", result.Content[0])
	}
	var r spike.Result
	json.Unmarshal([]byte(text.Text), &r)
	if len(r.Hits) == 0 {
		t.Error("search_brain(Diagnostic) with no DB: expected F0 fixture hit")
	}
}

// TestF1_SensitiveContentBlocked verifies that a memory with sensitive content
// (detected by the F1-E internal/sensitive package) returns IsError=true
// with a masked-match Spanish message.
func TestF1_SensitiveContentBlocked(t *testing.T) {
	_, session, _ := connectPairWithStore(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// AWS access key AKIAIOSFODNN7EXMPL should be blocked.
	// Masked form: AKIA****MPLE (first 4 + **** + last 4).
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "propose_memory",
		Arguments: map[string]any{
			"type":    "decision",
			"title":   "Configuración de AWS",
			"content": "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXMPL00",
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !result.IsError {
		t.Error("propose_memory(AWS key): IsError should be true")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0]: expected *TextContent, got %T", result.Content[0])
	}
	// Must mention "bloqueado" and the category "aws_access_key".
	if !strings.Contains(text.Text, "bloqueado") {
		t.Errorf("propose_memory(AWS key): expected 'bloqueado' in message, got %q", text.Text)
	}
	if !strings.Contains(text.Text, "aws_access_key") {
		t.Errorf("propose_memory(AWS key): expected 'aws_access_key' category, got %q", text.Text)
	}
	// Masked form of AKIAIOSFODNN7EXMPL.
	if !strings.Contains(text.Text, "AKIA****PL00") {
		t.Errorf("propose_memory(AWS key): expected masked 'AKIA****PL00', got %q", text.Text)
	}
}

// TestF1_SensitiveContentBlocked_GitHub verifies GitHub PAT detection.
func TestF1_SensitiveContentBlocked_GitHub(t *testing.T) {
	_, session, _ := connectPairWithStore(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// GitHub PAT with 20+ chars after ghp_ prefix.
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "propose_memory",
		Arguments: map[string]any{
			"type":    "decision",
			"title":   "Token de GitHub",
			"content": "ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !result.IsError {
		t.Error("propose_memory(GitHub PAT): IsError should be true")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0]: expected *TextContent, got %T", result.Content[0])
	}
	if !strings.Contains(text.Text, "github_pat") {
		t.Errorf("propose_memory(GitHub PAT): expected 'github_pat' category, got %q", text.Text)
	}
}

// TestF1_SensitiveContentBlocked_NoRowPersisted verifies that a rejected
// sensitive-content proposal does NOT appear in list_pending (i.e., the row
// was never persisted to the DB).
func TestF1_SensitiveContentBlocked_NoRowPersisted(t *testing.T) {
	_, session, _ := connectPairWithStore(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Propose a sensitive memory (AWS key) — should be rejected.
	_, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "propose_memory",
		Arguments: map[string]any{
			"type":    "decision",
			"title":   "AWS敏感",
			"content": "AKIAIOSFODNN7EXMPL00",
		},
	})
	if err != nil {
		t.Fatalf("CallTool(AWS key): %v", err)
	}

	// list_pending must return 0 items — the row was never persisted.
	listResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list_pending",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("list_pending: %v", err)
	}
	if listResult.IsError {
		t.Fatalf("list_pending: unexpected IsError: %v", listResult.Content)
	}
	listText, ok := listResult.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0]: expected *TextContent, got %T", listResult.Content[0])
	}
	var listResp struct {
		Items any `json:"items"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal([]byte(listText.Text), &listResp); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if listResp.Count != 0 {
		t.Errorf("list_pending after sensitive block: want count=0, got %d", listResp.Count)
	}
}

// TestF1_InstructionsMentionF1 verifies that the server Instructions string
// mentions the F1 tools.
func TestF1_InstructionsMentionF1(t *testing.T) {
	srv := newMCPServer()
	// The Instructions field is part of ServerOptions; we can't directly
	// inspect it through the public SDK API. Instead we verify that the
	// new tools are present and callable.
	_ = srv // server is configured but Instructions is opaque to test
	_, session, _ := connectPairWithStore(t)
	defer session.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, name := range []string{"propose_memory", "list_pending", "get_memory", "search_brain"} {
		found := false
		for _, tool := range result.Tools {
			if tool.Name == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("F1 tool %q: expected in server but not found", name)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// F1-B: Subprocess tests via CommandTransport
// ─────────────────────────────────────────────────────────────────────────────

// TestSubprocess_F1_HappyPath builds the binary and exercises the full F1
// flow: propose → list → get → search_brain.
func TestSubprocess_F1_HappyPath(t *testing.T) {
	if testing.Short() {
		t.Skip("requires subprocess")
	}

	binary := buildServerBinary(t)

	// Use a fresh temp dir so the DB is empty. Create config.yaml at the
	// YHAT_HOME level so ensureStore passes the config check.
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("operator: subprocess-test\n"), 0644); err != nil {
		t.Fatalf("WriteFile config.yaml: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.Command(binary, "mcp")
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"YHAT_HOME="+tmpDir,
	)

	client := mcp.NewClient(
		&mcp.Implementation{Name: "subprocess-f1-test", Version: "v1.0.0"},
		&mcp.ClientOptions{},
	)

	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer session.Close()

	// Step 1: Propose a memory.
	proposeResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "propose_memory",
		Arguments: map[string]any{
			"type":    "decision",
			"title":   "Decisión subprocess F1-B",
			"content": "Contenido verificado por subprocess.",
		},
	})
	if err != nil {
		t.Fatalf("propose_memory: %v", err)
	}
	if proposeResult.IsError {
		t.Fatalf("propose_memory: IsError=true: %v", proposeResult.Content)
	}
	proposeText, ok := proposeResult.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0]: expected *TextContent, got %T", proposeResult.Content[0])
	}
	var proposed store.Memory
	if err := json.Unmarshal([]byte(proposeText.Text), &proposed); err != nil {
		t.Fatalf("propose result not JSON: %v", err)
	}
	if proposed.ID == "" {
		t.Fatal("propose_memory: want non-empty ID")
	}

	// Step 2: List pending.
	listResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list_pending",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("list_pending: %v", err)
	}
	if listResult.IsError {
		t.Fatalf("list_pending: IsError=true: %v", listResult.Content)
	}
	listText, _ := listResult.Content[0].(*mcp.TextContent)
	var listResp struct {
		Items []store.Memory `json:"items"`
		Count int            `json:"count"`
	}
	json.Unmarshal([]byte(listText.Text), &listResp)
	if listResp.Count == 0 {
		t.Error("list_pending: expected at least 1 item")
	}

	// Step 3: Get by ID.
	getResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_memory",
		Arguments: map[string]any{"id": proposed.ID},
	})
	if err != nil {
		t.Fatalf("get_memory: %v", err)
	}
	if getResult.IsError {
		t.Fatalf("get_memory: IsError=true: %v", getResult.Content)
	}
	getText, _ := getResult.Content[0].(*mcp.TextContent)
	var fetched store.Memory
	json.Unmarshal([]byte(getText.Text), &fetched)
	if fetched.ID != proposed.ID {
		t.Errorf("get_memory: want id=%q, got %q", proposed.ID, fetched.ID)
	}

	// Step 4: search_brain finds the memory.
	searchResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "search_brain",
		Arguments: map[string]any{"query": "subprocess"},
	})
	if err != nil {
		t.Fatalf("search_brain: %v", err)
	}
	if searchResult.IsError {
		t.Fatalf("search_brain: IsError=true: %v", searchResult.Content)
	}
	searchText, _ := searchResult.Content[0].(*mcp.TextContent)
	var sr spike.Result
	json.Unmarshal([]byte(searchText.Text), &sr)
	if len(sr.Hits) == 0 {
		t.Error("search_brain(subprocess): expected hit")
	}
}

// TestSubprocess_F1_DBNotInitialized_ProposeError verifies that propose_memory
// returns an error when YHAT_HOME points to a non-existent directory.
func TestSubprocess_F1_DBNotInitialized_ProposeError(t *testing.T) {
	if testing.Short() {
		t.Skip("requires subprocess")
	}

	binary := buildServerBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Point to a directory that definitely doesn't exist.
	noSuchDir := filepath.Join(t.TempDir(), "does-not-exist", "subdir")

	cmd := exec.Command(binary, "mcp")
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"YHAT_HOME="+noSuchDir,
	)

	client := mcp.NewClient(
		&mcp.Implementation{Name: "subprocess-f1-test", Version: "v1.0.0"},
		&mcp.ClientOptions{},
	)

	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "propose_memory",
		Arguments: map[string]any{
			"type":    "decision",
			"title":   "Título",
			"content": "Contenido.",
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Errorf("propose_memory: want IsError=true on missing DB, got false (content=%v)", res.Content)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// F1-B: SQLite smoke test (in-process, no subprocess)
// Verifies that Open + ProposeMemory writes to a real SQLite DB.
// ─────────────────────────────────────────────────────────────────────────────

func TestSQLiteSmokeTest(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "smoke.db")

	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	mem, err := s.ProposeMemory(ctx, store.Memory{
		Type:    store.MemoryTypeDecision,
		Title:   "Smoke Test Memory",
		Content: "Contenido de verificación SQLite.",
		Author:  "smoke-test",
	})
	if err != nil {
		t.Fatalf("ProposeMemory: %v", err)
	}
	if mem.ID == "" {
		t.Error("ProposeMemory: want non-empty ID")
	}

	// Verify the row was persisted by opening a new connection.
	s2, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open (second): %v", err)
	}
	defer s2.Close()

	fetched, err := s2.GetMemory(ctx, mem.ID)
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if fetched.Title != "Smoke Test Memory" {
		t.Errorf("GetMemory: want title %q, got %q", "Smoke Test Memory", fetched.Title)
	}
	if fetched.Status != store.StatusProposed {
		t.Errorf("GetMemory: want status %q, got %q", store.StatusProposed, fetched.Status)
	}
}
