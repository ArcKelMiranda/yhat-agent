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
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------------------------------------------------------------------------
// In-memory transport: protocol layer
// ---------------------------------------------------------------------------

func connectPair(t *testing.T) (*mcp.Server, *mcp.ClientSession) {
	t.Helper()
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
	if len(result.Tools) != 1 {
		t.Fatalf("ListTools: expected 1 tool, got %d", len(result.Tools))
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

	if len(toolList.Tools) != 1 {
		report.OK = false
		return report, fmt.Errorf("tool count: want 1, got %d", len(toolList.Tools))
	}
	if toolList.Tools[0].Name != "fts5_search" {
		report.OK = false
		return report, fmt.Errorf("tool name: want fts5_search, got %q", toolList.Tools[0].Name)
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

	if len(toolList.Tools) != 1 {
		report.OK = false
		return report, fmt.Errorf("tool count: want 1, got %d", len(toolList.Tools))
	}
	if toolList.Tools[0].Name != "fts5_search" {
		report.OK = false
		return report, fmt.Errorf("tool name: want fts5_search, got %q", toolList.Tools[0].Name)
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
	if len(tools.Tools) != 1 {
		t.Fatalf("ListTools: expected 1 tool, got %d", len(tools.Tools))
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
