// F0 prototype: MCP server command using the official go-sdk.
// Protocol: stdio via StdioTransport (normal mode).
// Diagnostic mode: --selftest launches the binary as a subprocess and connects
// an SDK client via CommandTransport to verify the full MCP stack.
// Errors and non-protocol output go to stderr.
// Exit code: 0 on success, non-zero on any failure.
//
// F1-B: Real MCP tools for Cerebro YHat v1.2.
// Adds four persistent tools: propose_memory, list_pending, get_memory, search_brain.
// The store is opened lazily on first F1 tool invocation. If the DB file does not
// exist, the F1 tools return IsError=true with "Base local no inicializada".
// The F0 fts5_search tool is unchanged.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ArcKelMiranda/yhat-agent/internal/sensitive"
	"github.com/ArcKelMiranda/yhat-agent/internal/spike"
	"github.com/ArcKelMiranda/yhat-agent/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------------------------------------------------------------------------
// F1-B: Package-level store (lazy open)
// ---------------------------------------------------------------------------

// mcpStore is the lazily-opened SQLite store for F1 tools.
// nil means not-yet-attempted; a *Store is set after the first successful open.
var mcpStore *store.Store

// mcpStorePath removed: never read by production code, only written
// during the test reset path. Use defaultStorePath() if a path is
// needed.

// resetMCPStore is for testing only: closes and nil-out the cached store so each
// test gets a fresh store for its own temp directory.
func resetMCPStore() {
	if mcpStore != nil {
		mcpStore.Close()
		mcpStore = nil
	}
}

// defaultStorePath returns the default SQLite path.
// Linux/macOS: $YHAT_HOME/yhat.db if YHAT_HOME is set, else $HOME/.yhat/yhat.db.
// Windows: %USERPROFILE%\.yhat\yhat.db.
func defaultStorePath() string {
	if yhatHome := os.Getenv("YHAT_HOME"); yhatHome != "" {
		return filepath.Join(yhatHome, "yhat.db")
	}
	home, err := os.UserHomeDir()
	if err == nil {
		return filepath.Join(home, ".yhat", "yhat.db")
	}
	// Windows fallback.
	if userProfile := os.Getenv("USERPROFILE"); userProfile != "" {
		return filepath.Join(userProfile, ".yhat", "yhat.db")
	}
	// Absolute last resort: /tmp (for test environments without a real home).
	return filepath.Join(os.TempDir(), "yhat.db")
}

// operatorAuthor returns the author name for new memories.
// Precedence:
//  1. YHAT_AUTHOR environment variable (override).
//  2. "operator" field in $YHAT_HOME/config.yaml if that file exists.
//  3. os.Getenv("USERNAME") (Windows).
//  4. os.Getenv("USER") (Linux/macOS).
//  5. "unknown" (last resort; never reject).
func operatorAuthor() string {
	if author := os.Getenv("YHAT_AUTHOR"); author != "" {
		return author
	}
	storePath := defaultStorePath()
	cfgDir := filepath.Dir(storePath)
	cfgFile := filepath.Join(cfgDir, "config.yaml")
	data, err := os.ReadFile(cfgFile)
	if err == nil {
		// Simple YAML parse: look for "operator: <value>" line.
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "operator:") {
				val := strings.TrimSpace(strings.TrimPrefix(line, "operator:"))
				val = strings.Trim(val, "\"")
				if val != "" {
					return val
				}
			}
		}
	}
	if username := os.Getenv("USERNAME"); username != "" {
		return username
	}
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	return "unknown"
}

// ensureStore opens (or creates) the SQLite store at the default path.
// If the DB file does not exist, store.Open creates it and applies migrations.
// If the parent directory OR config.yaml is absent (base not initialized),
// return the "no inicializada" error WITHOUT calling Open.
// Author: operatorAuthor() falls back to OS username; the store is created
// on first valid tool invocation.
// The store is cached in mcpStore for the lifetime of the process.
func ensureStore() error {
	if mcpStore != nil {
		return nil
	}
	path := defaultStorePath()
	dir := filepath.Dir(path)
	if _, statErr := os.Stat(dir); statErr != nil {
		return fmt.Errorf("base local no inicializada. Ejecutá yhat-agent install primero")
	}
	// config.yaml must exist to confirm the operator has run install.
	cfgFile := filepath.Join(dir, "config.yaml")
	if _, cfgErr := os.Stat(cfgFile); cfgErr != nil {
		return fmt.Errorf("base local no inicializada. Ejecutá yhat-agent install primero")
	}
	s, err := store.Open(path)
	if err != nil {
		return err
	}
	// Initialize FTS5 virtual table and sync triggers (idempotent).
	if ftsErr := s.EnsureFTS5(context.Background()); ftsErr != nil {
		s.Close()
		return fmt.Errorf("ensure fts5: %w", ftsErr)
	}
	mcpStore = s
	return nil
}

// isStoreEmpty returns true if the memories table has zero rows.
func isStoreEmpty(ctx context.Context) bool {
	if mcpStore == nil {
		return true
	}
	var count int
	_ = mcpStore.StoreDB().QueryRowContext(ctx, "SELECT COUNT(*) FROM memories").Scan(&count)
	return count == 0
}

// ---------------------------------------------------------------------------
// F1-E sensitive content filter.
// Uses internal/sensitive.Scan to detect credential patterns in
// title+content+context and returns a masked-match Spanish message without
// persisting the row.

// formatSensisitiveMessage returns a Spanish JSON error message for a matched
// secret. The matched substring is masked: at most 8 chars visible (4+4),
// total length capped at 40 bytes.
func formatSensisitiveMessage(category, matched string) string {
	masked := maskSecret(matched)
	return fmt.Sprintf(`{"error": "Contenido bloqueado: parece un secreto de tipo %s (%s)."}`,
		category, masked)
}

// maskSecret masks a secret string, showing at most the first 4 and last 4
// characters with "****" in between. Total length is capped at 40 bytes.
// Examples:
//   - "AKIAIOSFODNN7EXAMPLE" → "AKIA****MPLE" (20 bytes, first 4 + **** + last 4)
//   - "ghp_xxxx" (short)   → "ghp_****"   (8 bytes, first 4 + ****)
//   - "Bearer eyJ..."        → "Bear****..." (first 4 + **** + suffix)
func maskSecret(s string) string {
	const maxLen = 40
	if len(s) > maxLen {
		s = s[:maxLen]
	}
	if len(s) <= 8 {
		// Short secrets: show all available, mask the tail.
		if len(s) <= 4 {
			return s + "****"
		}
		return s[:4] + "****"
	}
	// Normal: first 4 + **** + last 4.
	return s[:4] + "****" + s[len(s)-4:]
}

// ---------------------------------------------------------------------------
// F1-B: MCP tool handlers
// ---------------------------------------------------------------------------

// proposeMemoryInput matches the JSON schema for the propose_memory tool.
type proposeMemoryInput struct {
	Type    string  `json:"type"`
	Title   string  `json:"title"`
	Content string  `json:"content"`
	Context *string `json:"context,omitempty"`
}

// proposeMemoryHandler inserts a new local memory with status=proposed (BR1).
// The author field is always taken from operatorAuthor(), ignoring any author
// the model may have passed. Sensitive content is rejected with Spanish copy.
func proposeMemoryHandler(ctx context.Context, req *mcp.CallToolRequest, input proposeMemoryInput) (*mcp.CallToolResult, any, error) {
	if err := ensureStore(); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(`{"error": %q}`, err.Error())}},
			IsError: true,
		}, nil, nil
	}

	// F1-E sensitive-content scan: build text, scan, return masked match.
	full := input.Title + "\n" + input.Content
	if input.Context != nil {
		full += "\n" + *input.Context
	}
	if matches := sensitive.Scan(full); len(matches) > 0 {
		first := matches[0]
		matched := full[first.Start:first.End]
		msg := formatSensisitiveMessage(first.Category, matched)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: msg}},
			IsError: true,
		}, nil, nil
	}

	mem := store.Memory{
		Type:    input.Type,
		Title:   input.Title,
		Content: input.Content,
		Context: input.Context,
		Author:  operatorAuthor(), // always from config, never from model
	}

	result, err := mcpStore.ProposeMemory(ctx, mem)
	if err != nil {
		return storeErrorResult(err), nil, nil
	}

	data, merr := json.Marshal(result)
	if merr != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: `{"error": "serialization failed"}`}},
			IsError: true,
		}, nil, nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}, nil, nil
}

// listPendingInput matches the JSON schema for the list_pending tool.
type listPendingInput struct {
	Limit *int `json:"limit,omitempty"`
}

// listPendingHandler returns the operator's proposed memories, oldest first,
// paginated at 50 rows by default. Optional limit is 1..50.
func listPendingHandler(ctx context.Context, req *mcp.CallToolRequest, input listPendingInput) (*mcp.CallToolResult, any, error) {
	if err := ensureStore(); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(`{"error": %q}`, err.Error())}},
			IsError: true,
		}, nil, nil
	}

	limit := 50
	if input.Limit != nil {
		if *input.Limit < 1 {
			limit = 1
		} else if *input.Limit > 50 {
			limit = 50
		} else {
			limit = *input.Limit
		}
	}

	items, total, err := mcpStore.ListPendingMemories(ctx, limit)
	if err != nil {
		return storeErrorResult(err), nil, nil
	}

	response := map[string]any{
		"items": items,
		"count": total,
	}
	data, merr := json.Marshal(response)
	if merr != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: `{"error": "serialization failed"}`}},
			IsError: true,
		}, nil, nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}, nil, nil
}

// getMemoryInput matches the JSON schema for the get_memory tool.
type getMemoryInput struct {
	ID string `json:"id"`
}

// getMemoryHandler returns the full record for the given UUID.
// Unknown IDs return IsError=true.
func getMemoryHandler(ctx context.Context, req *mcp.CallToolRequest, input getMemoryInput) (*mcp.CallToolResult, any, error) {
	if err := ensureStore(); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(`{"error": %q}`, err.Error())}},
			IsError: true,
		}, nil, nil
	}

	mem, err := mcpStore.GetMemory(ctx, input.ID)
	if err != nil {
		return storeErrorResult(err), nil, nil
	}

	data, merr := json.Marshal(mem)
	if merr != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: `{"error": "serialization failed"}`}},
			IsError: true,
		}, nil, nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}, nil, nil
}

// searchBrainInput matches the JSON schema for the search_brain tool.
type searchBrainInput struct {
	Query string `json:"query"`
}

// searchBrainHandler searches memories via FTS5 when the store is populated,
// or falls back to the F0 synthetic fixtures when the store is empty or absent.
// LIMIT 5 results. Uses the same outer-quoted literal phrase as fts5_search.
func searchBrainHandler(ctx context.Context, req *mcp.CallToolRequest, input searchBrainInput) (*mcp.CallToolResult, any, error) {
	// If the store is open and has rows, query it.
	if mcpStore != nil && !isStoreEmpty(ctx) {
		return searchBrainFromStore(ctx, input.Query)
	}
	// Fall back to F0 fixtures (fts5_search behavior) when DB is empty or absent.
	return searchBrainF0Fallback(ctx, input.Query)
}

// searchBrainFromStore queries the real memories table via FTS5.
// LIMIT 5. Uses the same escaped literal phrase as fts5_search.
func searchBrainFromStore(ctx context.Context, query string) (*mcp.CallToolResult, any, error) {
	if mcpStore == nil {
		return searchBrainF0Fallback(ctx, query)
	}

	q := strings.TrimSpace(query)
	if q == "" {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: `{"error": "query cannot be empty"}`}},
			IsError: true,
		}, nil, nil
	}

	// Same escaping as fts5_search: outer quotes, double embedded quotes.
	safe := "\"" + strings.ReplaceAll(q, `"`, `""`) + "\""

	memories, err := mcpStore.SearchMemoriesFTS5(ctx, safe, 5)
	if err != nil {
		// If FTS5 query fails (e.g., no FTS5 table yet), fall back to F0 fixtures.
		return searchBrainF0Fallback(ctx, query)
	}

	var hits []spike.Hit
	for _, mem := range memories {
		hits = append(hits, spike.Hit{Title: mem.Title, Text: mem.Content, Rank: 0})
	}

	result := spike.Result{Hits: hits, TotalFound: len(hits), Query: q}
	data, merr := json.Marshal(result)
	if merr != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: `{"error": "serialization failed"}`}},
			IsError: true,
		}, nil, nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}, nil, nil
}

// searchBrainF0Fallback is identical to the current fts5_search body.
// This is the F0 synthetic fixture path: used when the DB is empty or absent.
func searchBrainF0Fallback(ctx context.Context, query string) (*mcp.CallToolResult, any, error) {
	fts, err := spike.NewServer()
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: `{"error": "search engine unavailable"}`}},
			IsError: true,
		}, nil, nil
	}
	defer fts.Close(ctx)

	result, err := fts.Search(ctx, query)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(`{"error": %q}`, err.Error())}},
			IsError: true,
		}, nil, nil
	}

	data, merr := json.Marshal(result)
	if merr != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: `{"error": "serialization failed"}`}},
			IsError: true,
		}, nil, nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}, nil, nil
}

// ---------------------------------------------------------------------------
// Error mapping: store sentinel → Spanish MCP error
// ---------------------------------------------------------------------------

// storeErrorResult maps store sentinel errors to IsError=true MCP responses
// with Spanish copy. The model never sees raw Go errors.
func storeErrorResult(err error) *mcp.CallToolResult {
	msg := err.Error()
	switch {
	case errors.Is(err, store.ErrNotFound):
		msg = "Memoria no encontrada."
	case errors.Is(err, store.ErrDuplicate):
		msg = "Memoria duplicada: ya existe un registro con el mismo título y contenido."
	case errors.Is(err, store.ErrTeamNotEditable):
		msg = "Los registros del equipo no se pueden editar localmente."
	case errors.Is(err, store.ErrInvalidField):
		msg = "Campo inválido: " + cleanFieldError(err)
	default:
		// Wrap unknown errors with Spanish prefix but don't leak internals.
		if strings.Contains(msg, "title length") {
			msg = "Campo inválido: el título debe tener entre 1 y 200 caracteres."
		} else if strings.Contains(msg, "content length") {
			msg = "Campo inválido: el contenido debe tener entre 1 y 10000 caracteres."
		} else if strings.Contains(msg, "tipo") || strings.Contains(msg, "type") {
			msg = "Campo inválido: tipo debe ser decision, rule, anomaly o improvement."
		} else {
			msg = "Error interno. Intenta de nuevo."
		}
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(`{"error": %q}`, msg)}},
		IsError: true,
	}
}

// cleanFieldError removes the internal sentinel prefix from error messages
// for cleaner user-facing copy.
func cleanFieldError(err error) string {
	s := err.Error()
	s = strings.TrimPrefix(s, "invalid field value: ")
	s = strings.TrimPrefix(s, "ErrInvalidField:")
	return s
}

// ---------------------------------------------------------------------------
// F0: runMCPCommand, newMCPServer, searchHandler, SelftestReport, runSelftest
// ---------------------------------------------------------------------------

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

// newMCPServer creates and configures an MCP server with the F0 fts5_search
// tool and the F1-B tools: propose_memory, list_pending, get_memory, search_brain.
func newMCPServer() *mcp.Server {
	srv := mcp.NewServer(
		&mcp.Implementation{Name: "yhat-agent-f0", Version: "0.1.0-f0"},
		&mcp.ServerOptions{
			Instructions: "Servidor MCP de Cerebro YHat v1.2. " +
				"Usa propose_memory para capturar decisiones, reglas, anomalías o mejoras. " +
				"Usa list_pending para revisar las memorias propuestas awaiting aprobación. " +
				"Usa get_memory para consultar una memoria específica por su ID. " +
				"Usa search_brain para buscar en tu base de conocimiento local (o las memorias F0 si la base está vacía). " +
				"Usa fts5_search solo para diagnóstico sintético.",
		},
	)

	// F0 diagnostic tool (unchanged).
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

	// F1-B: propose_memory
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "propose_memory",
		Description: "Captura una nueva memoria local con estado 'proposed'. El campo author se toma de config.yaml; el author que pase el modelo se ignora. Si se detecta contenido sensible (filtro F1-E), se rechaza con mensaje en español.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"type": map[string]any{
					"type":        "string",
					"description": "Tipo de memoria: decision, rule, anomaly o improvement.",
					"enum":        []any{"decision", "rule", "anomaly", "improvement"},
				},
				"title": map[string]any{
					"type":        "string",
					"description": "Título de la memoria (1–200 caracteres).",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Contenido de la memoria (1–10000 caracteres).",
				},
				"context": map[string]any{
					"type":        "string",
					"description": "Contexto opcional.",
				},
			},
			"required":             []any{"type", "title", "content"},
			"additionalProperties": false,
		},
	}, proposeMemoryHandler)

	// F1-B: list_pending
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_pending",
		Description: "Lista las memorias propuestas del operador actual, ordenadas por fecha (más antigua primero), paginadas a 50 por defecto. Argumento opcional limit (1–50).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit": map[string]any{
					"type":        "integer",
					"description": "Límite de resultados (1–50, default 50).",
					"minimum":     1,
					"maximum":     50,
				},
			},
			"additionalProperties": false,
		},
	}, listPendingHandler)

	// F1-B: get_memory
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_memory",
		Description: "Obtiene una memoria completa por su UUID. Devuelve el registro completo con origin, status y metadatos de auditoría.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "UUID de la memoria.",
					"format":      "uuid",
				},
			},
			"required":             []any{"id"},
			"additionalProperties": false,
		},
	}, getMemoryHandler)

	// F1-B: search_brain
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "search_brain",
		Description: "Búsqueda de conocimiento persistente. Si la base local tiene memorias, busca ahí (LÍMITE 5). Si está vacía o no existe, recurre a los fixtures sintéticos F0 (Diagnostic Fixture Alpha, Política Fixture Beta, Café Fixture Gamma). Búsqueda insensible a acentos y mayúsculas.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Texto de búsqueda (máx. 200 caracteres).",
				},
			},
			"required":             []any{"query"},
			"additionalProperties": false,
		},
	}, searchBrainHandler)

	return srv
}

// searchHandler implements the MCP tools/call handler for fts5_search.
// This is the F0 path and is unchanged by F1-B.
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

	if len(tools.Tools) < 1 {
		report.OK = false
		return report, fmt.Errorf("tool count: want >= 1, got %d", len(tools.Tools))
	}
	// Verify fts5_search is present (may be first or among F1 tools).
	var hasFTS5 bool
	for _, t := range tools.Tools {
		if t.Name == "fts5_search" {
			hasFTS5 = true
			break
		}
	}
	if !hasFTS5 {
		report.OK = false
		return report, fmt.Errorf("fts5_search tool not found")
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
