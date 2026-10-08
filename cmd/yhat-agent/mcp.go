// F0 prototype: MCP server command using the official go-sdk.
// Protocol: stdio via StdioTransport (normal mode).
// Diagnostic mode: --selftest launches the binary as a subprocess and connects
// an SDK client via CommandTransport to verify the full MCP stack.
// Errors and non-protocol output go to stderr.
// Exit code: 0 on success, non-zero on any failure.

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/ArcKelMiranda/yhat-agent/internal/spike"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// runMCPCommand dispatches the MCP subcommand.
func runMCPCommand(args []string) int {
	fs := flag.NewFlagSet("yhat-agent mcp", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: yhat-agent mcp [--selftest]")
	}

	var selftest bool
	fs.BoolVar(&selftest, "selftest", false, "run self-test and emit JSON diagnostic")

	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "yhat-agent mcp: unexpected argument: %s\n", fs.Arg(0))
		return 1
	}

	if selftest {
		if err := runSelftest(); err != nil {
			fmt.Fprintf(os.Stderr, "selftest failed: %v\n", err)
			return 1
		}
		return 0
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Hour)
	defer cancel()
	srv := newMCPServer()
	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil {
		fmt.Fprintf(os.Stderr, "mcp: server error: %v\n", err)
		return 1
	}
	return 0
}

// newMCPServer creates and configures an MCP server with the fts5_search tool.
func newMCPServer() *mcp.Server {
	srv := mcp.NewServer(
		&mcp.Implementation{Name: "yhat-agent-f0", Version: "0.1.0-f0"},
		&mcp.ServerOptions{
			Instructions: "Ephemeral diagnostic prototype with synthetic data only. No real knowledge or production capability.",
		},
	)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fts5_search",
		Description: "F0 diagnostic: full-text search of synthetic example data using SQLite FTS5. Accent- and case-insensitive. This tool has no access to real knowledge or production systems; it only searches ephemeral synthetic fixtures for diagnostic purposes.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query text (max 200 characters).",
				},
			},
			"required":             []any{"query"},
			"additionalProperties": false,
		},
	}, searchHandler)

	return srv
}

// searchHandler implements the MCP tools/call handler for fts5_search.
func searchHandler(ctx context.Context, req *mcp.CallToolRequest, input struct {
	Query string `json:"query"`
}) (*mcp.CallToolResult, any, error) {
	fts, err := spike.NewServer()
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: `{"error": "search engine unavailable"}`}},
			IsError: true,
		}, nil, nil
	}
	defer fts.Close(ctx)

	result, err := fts.Search(ctx, input.Query)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(`{"error": %q}`, err.Error())}},
			IsError: true,
		}, nil, nil
	}

	data, err := json.Marshal(result)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: `{"error": "serialization failed"}`}},
			IsError: true,
		}, nil, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}, nil, nil
}

// SelftestReport is the minimal typed JSON report emitted by --selftest.
type SelftestReport struct {
	OK            bool   `json:"ok"`
	FTS5          bool   `json:"fts5"`
	MCP           bool   `json:"mcp"`
	Synthetic     bool   `json:"synthetic"`
	PoliticaTitle string `json:"politica_title,omitempty"`
}

// runSelftest launches the binary as a subprocess MCP server and connects
// an SDK client via CommandTransport. Every failed assertion causes OK=false.
func runSelftest() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	report, err := runSelftestResult(ctx)
	if err != nil {
		return err
	}

	data, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}
	data = append(data, '\n')
	if _, err := os.Stdout.Write(data); err != nil {
		return fmt.Errorf("stdout write: %w", err)
	}
	return nil
}

// runSelftestResult performs all checks and returns the report.
func runSelftestResult(ctx context.Context) (*SelftestReport, error) {
	report := &SelftestReport{OK: true}

	// Find the binary to test: the currently running executable.
	exe, err := os.Executable()
	if err != nil {
		report.OK = false
		return report, fmt.Errorf("Executable: %w", err)
	}

	// Launch the binary as an MCP server subprocess with bounded lifetime.
	cmd := exec.CommandContext(ctx, exe, "mcp")

	client := mcp.NewClient(
		&mcp.Implementation{Name: "selftest-client", Version: "v1.0.0"},
		&mcp.ClientOptions{},
	)

	// Connect via CommandTransport: this wraps the subprocess's stdin/stdout
	// as an MCP transport, using the official SDK.
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		report.OK = false
		return report, fmt.Errorf("Connect: %w", err)
	}
	defer session.Close()

	// InitializeResult is set by Client.Connect automatically.
	init := session.InitializeResult()
	if init == nil {
		report.OK = false
		return report, fmt.Errorf("InitializeResult: nil")
	}
	if init.ProtocolVersion == "" {
		report.OK = false
		return report, fmt.Errorf("protocol version is empty")
	}
	// Assert server name if ServerInfo is available.
	if init.ServerInfo != nil && init.ServerInfo.Name != "" && init.ServerInfo.Name != "yhat-agent-f0" {
		report.OK = false
		return report, fmt.Errorf("server name: want yhat-agent-f0, got %q", init.ServerInfo.Name)
	}

	// Assert exactly one tool named "fts5_search".
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		report.OK = false
		return report, fmt.Errorf("ListTools: %w", err)
	}
	report.MCP = true

	if len(tools.Tools) != 1 {
		report.OK = false
		return report, fmt.Errorf("tool count: want 1, got %d", len(tools.Tools))
	}
	if tools.Tools[0].Name != "fts5_search" {
		report.OK = false
		return report, fmt.Errorf("tool name: want fts5_search, got %q", tools.Tools[0].Name)
	}

	// Call fts5_search with "politica"; assert IsError is false and exact title.
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "fts5_search",
		Arguments: map[string]any{"query": "politica"},
	})
	if err != nil {
		report.OK = false
		return report, fmt.Errorf("CallTool: %w", err)
	}
	if result.IsError {
		report.OK = false
		return report, fmt.Errorf("CallTool: IsError is true")
	}
	if len(result.Content) == 0 {
		report.OK = false
		return report, fmt.Errorf("CallTool: no content")
	}

	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		report.OK = false
		return report, fmt.Errorf("content[0]: expected *TextContent")
	}

	// Decode the JSON response.
	var searchResult struct {
		Hits []struct {
			Title string `json:"title"`
		} `json:"hits"`
		TotalFound int    `json:"total_found"`
		Query      string `json:"query"`
	}
	if err := json.Unmarshal([]byte(text.Text), &searchResult); err != nil {
		report.OK = false
		return report, fmt.Errorf("unmarshal search result: %w", err)
	}
	report.FTS5 = true

	if len(searchResult.Hits) == 0 {
		report.OK = false
		return report, fmt.Errorf("fts5_search(politica) returned no hits")
	}

	// Assert EXACT title for politica query.
	if len(searchResult.Hits) > 0 {
		report.PoliticaTitle = searchResult.Hits[0].Title
		if searchResult.Hits[0].Title != "Política Fixture Beta" {
			report.OK = false
			return report, fmt.Errorf("fts5_search(politica): expected title %q, got %q",
				"Política Fixture Beta", searchResult.Hits[0].Title)
		}
	}
	report.Synthetic = true

	return report, nil
}
