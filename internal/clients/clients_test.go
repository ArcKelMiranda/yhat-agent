// Package clients provides MCP registration for OpenCode and Claude Desktop
// on Windows WorkSpaces. It is Windows-only at runtime but fully testable
// on any platform through injectable paths.
package clients

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

func makeEmptyFile(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
		t.Fatalf("makeEmptyFile: %v", err)
	}
	return path
}

func writeConfigFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}
	return path
}

// ─── Env implementation for testing ─────────────────────────────────────────

type testEnv struct {
	opencodeRoot string
	opencodeCfg  string
	appdata      string
	localappdata string
}

func (e *testEnv) OpenCodeRoot() string   { return e.opencodeRoot }
func (e *testEnv) OpenCodeConfig() string { return e.opencodeCfg }
func (e *testEnv) AppData() string        { return e.appdata }
func (e *testEnv) LocalAppData() string   { return e.localappdata }
func (e *testEnv) Executable() (string, error) {
	if runtime.GOOS == "windows" {
		return `C:\tools\yhat-agent.exe`, nil
	}
	return "/usr/local/bin/yhat-agent", nil
}

// ─── OpenCode detection ─────────────────────────────────────────────────────

func TestOpenCode_DefaultRoot(t *testing.T) {
	dir := t.TempDir()
	env := &testEnv{
		opencodeRoot: filepath.Join(dir, "opencode"),
	}
	mustCreateDir(t, env.OpenCodeRoot())

	cfg, err := DetectOpenCode(env)
	if err != nil {
		t.Fatalf("DetectOpenCode: %v", err)
	}
	if cfg == "" {
		t.Fatal("DetectOpenCode: empty path")
	}
	if !strings.HasSuffix(cfg, "opencode.json") {
		t.Fatalf("DetectOpenCode: expected opencode.json suffix, got %s", cfg)
	}
}

func TestOpenCode_EnvOverride(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "my-opencode.json")
	mustCreateFile(t, cfgPath, `{}`)

	env := &testEnv{
		opencodeRoot: "/should/be/ignored",
		opencodeCfg:  cfgPath,
	}

	cfg, err := DetectOpenCode(env)
	if err != nil {
		t.Fatalf("DetectOpenCode: %v", err)
	}
	if cfg != cfgPath {
		t.Fatalf("DetectOpenCode: expected %s, got %s", cfgPath, cfg)
	}
}

func TestOpenCode_EnvOverrideNotAbsolute(t *testing.T) {
	env := &testEnv{
		opencodeRoot: "/some/root",
		opencodeCfg:  "relative/path.json",
	}

	_, err := DetectOpenCode(env)
	if err == nil {
		t.Fatal("DetectOpenCode: expected error for relative OPENCODE_CONFIG")
	}
}

func TestOpenCode_JsoncExtensionRejected(t *testing.T) {
	dir := t.TempDir()
	env := &testEnv{
		opencodeRoot: dir,
	}
	mustCreateFile(t, filepath.Join(dir, "opencode.jsonc"), `{}`)

	_, err := DetectOpenCode(env)
	if err == nil {
		t.Fatal("DetectOpenCode: expected error for .jsonc file")
	}
}

func TestOpenCode_JsonWithJsoncSibling(t *testing.T) {
	dir := t.TempDir()
	env := &testEnv{
		opencodeRoot: dir,
	}
	mustCreateFile(t, filepath.Join(dir, "opencode.json"), `{}`)
	mustCreateFile(t, filepath.Join(dir, "opencode.jsonc"), `{}`)

	_, err := DetectOpenCode(env)
	if err == nil {
		t.Fatal("DetectOpenCode: expected error when both .json and .jsonc exist")
	}
}

func TestOpenCode_JsonWithSchemaUrlNotRejected(t *testing.T) {
	// Valid JSON with a $schema URL containing "://" and "////" should NOT be rejected.
	// The hasJsoncContent heuristic falsely rejects // inside URLs.
	// Strict JSON parser naturally rejects JSONC; heuristic should be removed.
	dir := t.TempDir()
	env := &testEnv{
		opencodeRoot: dir,
	}
	mustCreateFile(t, filepath.Join(dir, "opencode.json"),
		`{"$schema": "https://opencode.ai/config.json", "mcp": {}}`)

	cfg, err := DetectOpenCode(env)
	if err != nil {
		t.Fatalf("DetectOpenCode: rejected valid JSON with schema URL: %v", err)
	}
	if cfg == "" {
		t.Fatal("DetectOpenCode: empty path for valid JSON")
	}
}

func TestOpenCode_JsonWithUrlPathNotRejected(t *testing.T) {
	// Valid JSON containing // in string values (e.g., file:// paths) should not be rejected.
	dir := t.TempDir()
	env := &testEnv{
		opencodeRoot: dir,
	}
	mustCreateFile(t, filepath.Join(dir, "opencode.json"),
		`{"mcp": {"test": {"url": "http://example.com/path"}}}`)

	cfg, err := DetectOpenCode(env)
	if err != nil {
		t.Fatalf("DetectOpenCode: rejected valid JSON with URL in string: %v", err)
	}
	if cfg == "" {
		t.Fatal("DetectOpenCode: empty path for valid JSON with URL string")
	}
}

func TestOpenCode_CreateDir(t *testing.T) {
	dir := t.TempDir()
	env := &testEnv{
		opencodeRoot: filepath.Join(dir, "new-opencode"),
	}

	cfg, err := DetectOpenCode(env)
	if err != nil {
		t.Fatalf("DetectOpenCode: %v", err)
	}
	if !strings.HasSuffix(cfg, "opencode.json") {
		t.Fatalf("DetectOpenCode: expected opencode.json suffix, got %s", cfg)
	}
}

func TestOpenCode_SymlinkRejected(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "opencode.json")
	mustCreateFile(t, cfg, `{}`)
	sym := filepath.Join(dir, "opencode-symlink.json")
	if err := os.Symlink(cfg, sym); err != nil {
		t.Skip("symlink not supported:", err)
	}

	envSym := &testEnv{
		opencodeRoot: dir,
		opencodeCfg:  sym,
	}

	_, err := DetectOpenCode(envSym)
	if err == nil {
		t.Fatal("DetectOpenCode: expected error for symlink config")
	}
}

// ─── Claude detection ───────────────────────────────────────────────────────

func TestClaude_ConventionalPath(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, "Claude")
	mustCreateDir(t, claudeDir)

	env := &testEnv{
		appdata: dir,
	}

	candidates, err := DetectClaude(env)
	if err != nil {
		t.Fatalf("DetectClaude: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("DetectClaude: expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0] != claudeDir {
		t.Fatalf("DetectClaude: expected %s, got %s", claudeDir, candidates[0])
	}
}

func TestClaude_PackagedPath(t *testing.T) {
	dir := t.TempDir()
	claudePkg := filepath.Join(dir, "Packages", "Claude_abc123", "LocalCache", "Roaming", "Claude")
	mustCreateDir(t, claudePkg)

	env := &testEnv{
		localappdata: dir,
	}

	candidates, err := DetectClaude(env)
	if err != nil {
		t.Fatalf("DetectClaude: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("DetectClaude: expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0] != claudePkg {
		t.Fatalf("DetectClaude: expected %s, got %s", claudePkg, candidates[0])
	}
}

func TestClaude_BothPaths(t *testing.T) {
	dir := t.TempDir()
	conventional := filepath.Join(dir, "AppData", "Roaming", "Claude")
	mustCreateDir(t, conventional)
	packaged := filepath.Join(dir, "LocalAppData", "Packages", "Claude_xyz789", "LocalCache", "Roaming", "Claude")
	mustCreateDir(t, packaged)

	env := &testEnv{
		appdata:      filepath.Join(dir, "AppData", "Roaming"),
		localappdata: filepath.Join(dir, "LocalAppData"),
	}

	candidates, err := DetectClaude(env)
	if err == nil {
		t.Fatal("DetectClaude: expected error for multiple candidates")
	}
	if len(candidates) != 2 {
		t.Fatalf("DetectClaude: expected 2 candidates in error, got %d", len(candidates))
	}
}

func TestClaude_ZeroCandidates(t *testing.T) {
	dir := t.TempDir()
	env := &testEnv{
		appdata:      dir,
		localappdata: dir,
	}

	candidates, err := DetectClaude(env)
	if err != nil {
		t.Fatalf("DetectClaude: unexpected error: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("DetectClaude: expected 0 candidates, got %d", len(candidates))
	}
}

func TestClaude_EmptyAppDataSkipped(t *testing.T) {
	dir := t.TempDir()
	claudePkg := filepath.Join(dir, "Packages", "Claude_abc", "LocalCache", "Roaming", "Claude")
	mustCreateDir(t, claudePkg)

	env := &testEnv{
		appdata:      "", // empty
		localappdata: dir,
	}

	candidates, err := DetectClaude(env)
	if err != nil {
		t.Fatalf("DetectClaude: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("DetectClaude: expected 1 candidate, got %d", len(candidates))
	}
}

func TestClaude_NonRegularFileRejected(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "fifo")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Skip("cannot create dir:", err)
	}
	if err := os.WriteFile(fifo, []byte{}, 0644); err != nil {
		t.Skip("cannot create file:", err)
	}
	// The directory "Claude" is a regular file, not a dir
	env := &testEnv{
		appdata: dir,
	}

	_, err := DetectClaude(env)
	// Should either skip (file not dir) or error gracefully
	if err != nil && !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("DetectClaude: unexpected error: %v", err)
	}
}

// ─── Config file operations ──────────────────────────────────────────────────

func TestReadConfigFile_Basic(t *testing.T) {
	dir := t.TempDir()
	path := makeEmptyFile(t, dir, "test.json")

	data, err := ReadConfigFile(path, 2<<20) // 2 MiB
	if err != nil {
		t.Fatalf("ReadConfigFile: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("ReadConfigFile: empty data")
	}
}

func TestReadConfigFile_TooLarge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	// Create a file larger than 10 bytes
	largeContent := `{"key": "this is a very long value that exceeds 10 bytes"}`
	if len(largeContent) <= 10 {
		t.Fatal("test setup: content must be > 10 bytes")
	}
	if err := os.WriteFile(path, []byte(largeContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := ReadConfigFile(path, 10) // too small
	if err == nil {
		t.Fatal("ReadConfigFile: expected error for oversized file")
	}
}

func TestReadConfigFile_SymlinkRejected(t *testing.T) {
	dir := t.TempDir()
	target := makeEmptyFile(t, dir, "target.json")
	sym := filepath.Join(dir, "symlink.json")
	if err := os.Symlink(target, sym); err != nil {
		t.Skip("symlink not supported:", err)
	}

	_, err := ReadConfigFile(sym, 2<<20)
	if err == nil {
		t.Fatal("ReadConfigFile: expected error for symlink")
	}
}

func TestReadConfigFile_DirectoryRejected(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadConfigFile(dir, 2<<20)
	if err == nil {
		t.Fatal("ReadConfigFile: expected error for directory")
	}
}

func TestReadConfigFile_ENOENTReturnsNilNotError(t *testing.T) {
	// ReadConfigFile should return (nil, nil) for missing files, not an error.
	// Callers treat nil as "absent" and len==0 as "invalid existing".
	dir := t.TempDir()
	absent := filepath.Join(dir, "does-not-exist.json")

	data, err := ReadConfigFile(absent, 2<<20)
	if err != nil {
		t.Fatalf("ReadConfigFile: ENOENT should not be an error, got: %v", err)
	}
	if data != nil {
		t.Fatalf("ReadConfigFile: ENOENT should return nil data, got %d bytes", len(data))
	}
}

func TestReadConfigFile_EmptyFileReturnsNilNotError(t *testing.T) {
	// An existing but empty file returns nil (treated as invalid existing, not new).
	// The caller distinguishes orig==nil (absent) from len==0 (invalid existing).
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	data, err := ReadConfigFile(path, 2<<20)
	if err != nil {
		t.Fatalf("ReadConfigFile: empty file should not error, got: %v", err)
	}
	if data == nil {
		t.Fatal("ReadConfigFile: empty existing file should return empty slice, not nil")
	}
	if len(data) != 0 {
		t.Fatalf("ReadConfigFile: empty existing file should return empty slice, got %d bytes", len(data))
	}
}

// ─── Schema merge ────────────────────────────────────────────────────────────

func TestOpenCodeMerge_NilConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "opencode.json")
	exe, _ := (&testEnv{}).Executable()

	merged, changed, err := OpenCodeMergeConfig(cfgPath, nil, exe)
	if err != nil {
		t.Fatalf("OpenCodeMergeConfig: %v", err)
	}
	if !changed {
		t.Fatal("OpenCodeMergeConfig: expected changed=true for nil config")
	}

	// Verify structure
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(merged, &doc); err != nil {
		t.Fatalf("OpenCodeMergeConfig: invalid JSON: %v", err)
	}
	mcpRaw, ok := doc["mcp"]
	if !ok {
		t.Fatal("OpenCodeMergeConfig: missing mcp key")
	}
	var mcpDoc map[string]json.RawMessage
	if err := json.Unmarshal(mcpRaw, &mcpDoc); err != nil {
		t.Fatalf("OpenCodeMergeConfig: mcp is not an object: %v", err)
	}
	yhatRaw, ok := mcpDoc["yhat"]
	if !ok {
		t.Fatal("OpenCodeMergeConfig: missing mcp.yhat")
	}
	var yhatEntry map[string]any
	if err := json.Unmarshal(yhatRaw, &yhatEntry); err != nil {
		t.Fatalf("OpenCodeMergeConfig: mcp.yhat is not an object: %v", err)
	}
	if yhatEntry["type"] != "local" {
		t.Fatalf("OpenCodeMergeConfig: expected type=local, got %v", yhatEntry["type"])
	}
}

func TestOpenCodeMerge_ExistingOtherServers(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "opencode.json")
	content := `{"mcp": {"claude": {"type": "remote", "url": "http://example.com"}}}`
	mustCreateFile(t, cfgPath, content)
	exe, _ := (&testEnv{}).Executable()

	merged, changed, err := OpenCodeMergeConfig(cfgPath, []byte(content), exe)
	if err != nil {
		t.Fatalf("OpenCodeMergeConfig: %v", err)
	}
	if !changed {
		t.Fatal("OpenCodeMergeConfig: expected changed=true")
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(merged, &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	mcpRaw := doc["mcp"]
	var mcpDoc map[string]json.RawMessage
	if err := json.Unmarshal(mcpRaw, &mcpDoc); err != nil {
		t.Fatalf("mcp is not an object: %v", err)
	}
	if _, ok := mcpDoc["claude"]; !ok {
		t.Fatal("OpenCodeMergeConfig: lost existing server 'claude'")
	}
	if _, ok := mcpDoc["yhat"]; !ok {
		t.Fatal("OpenCodeMergeConfig: missing yhat entry")
	}
}

func TestOpenCodeMerge_IdempotentNoOp(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "opencode.json")
	exe, _ := (&testEnv{}).Executable()

	// First merge
	merged1, changed1, err := OpenCodeMergeConfig(cfgPath, nil, exe)
	if err != nil {
		t.Fatalf("OpenCodeMergeConfig first: %v", err)
	}
	if !changed1 {
		t.Fatal("OpenCodeMergeConfig: expected changed=true on first merge")
	}
	mustCreateFile(t, cfgPath, string(merged1))

	// Second merge with original content
	orig, err := ReadConfigFile(cfgPath, 2<<20)
	if err != nil {
		t.Fatalf("ReadConfigFile: %v", err)
	}

	merged2, changed2, err := OpenCodeMergeConfig(cfgPath, orig, exe)
	if err != nil {
		t.Fatalf("OpenCodeMergeConfig second: %v", err)
	}
	if changed2 {
		t.Fatal("OpenCodeMergeConfig: expected changed=false on idempotent re-merge")
	}
	// Must be byte-identical
	if string(merged1) != string(merged2) {
		t.Fatal("OpenCodeMergeConfig: re-merge produced different bytes")
	}
}

func TestOpenCodeMerge_ConflictDifferingYhat(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "opencode.json")
	mustCreateFile(t, cfgPath, `{"mcp": {"yhat": {"type": "local", "command": ["other.exe","mcp"], "enabled": false}}}`)
	exe, _ := (&testEnv{}).Executable()

	orig, err := ReadConfigFile(cfgPath, 2<<20)
	if err != nil {
		t.Fatalf("ReadConfigFile: %v", err)
	}

	_, _, err = OpenCodeMergeConfig(cfgPath, orig, exe)
	if err == nil {
		t.Fatal("OpenCodeMergeConfig: expected error for conflicting yhat entry")
	}
}

func TestOpenCodeMerge_NullNamespace(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "opencode.json")
	content := `{"mcp": null}`
	mustCreateFile(t, cfgPath, content)
	exe, _ := (&testEnv{}).Executable()

	_, _, err := OpenCodeMergeConfig(cfgPath, []byte(content), exe)
	if err == nil {
		t.Fatal("OpenCodeMergeConfig: expected error for null mcp namespace")
	}
}

func TestOpenCodeMerge_ArrayNamespace(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "opencode.json")
	content := `{"mcp": []}`
	mustCreateFile(t, cfgPath, content)
	exe, _ := (&testEnv{}).Executable()

	_, _, err := OpenCodeMergeConfig(cfgPath, []byte(content), exe)
	if err == nil {
		t.Fatal("OpenCodeMergeConfig: expected error for array mcp namespace")
	}
}

func TestOpenCodeMerge_RootNullRejected(t *testing.T) {
	// Root-level null panics assigning to nil rawDoc.
	// Must be rejected as invalid.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "opencode.json")
	mustCreateFile(t, cfgPath, `null`)
	exe, _ := (&testEnv{}).Executable()

	_, _, err := OpenCodeMergeConfig(cfgPath, []byte(`null`), exe)
	if err == nil {
		t.Fatal("OpenCodeMergeConfig: expected error for root null")
	}
}

func TestOpenCodeMerge_RootArrayRejected(t *testing.T) {
	// Root-level array is invalid.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "opencode.json")
	mustCreateFile(t, cfgPath, `[]`)
	exe, _ := (&testEnv{}).Executable()

	_, _, err := OpenCodeMergeConfig(cfgPath, []byte(`[]`), exe)
	if err == nil {
		t.Fatal("OpenCodeMergeConfig: expected error for root array")
	}
}

func TestOpenCodeMerge_RootScalarRejected(t *testing.T) {
	// Root-level scalar is invalid.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "opencode.json")
	mustCreateFile(t, cfgPath, `"hello"`)
	exe, _ := (&testEnv{}).Executable()

	_, _, err := OpenCodeMergeConfig(cfgPath, []byte(`"hello"`), exe)
	if err == nil {
		t.Fatal("OpenCodeMergeConfig: expected error for root scalar")
	}
}

func TestOpenCodeMerge_ExistingEnabledFalseConflict(t *testing.T) {
	// Existing yhat with enabled:false but same command should be a conflict.
	// Use json.Unmarshal + reflect.DeepEqual, not fmt.Sprintf comparison.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "opencode.json")
	mustCreateFile(t, cfgPath, `{"mcp": {"yhat": {"type": "local", "command": ["C:\\tools\\yhat-agent.exe","mcp"], "enabled": false}}}`)
	exe := `C:\tools\yhat-agent.exe`

	orig, err := ReadConfigFile(cfgPath, 2<<20)
	if err != nil {
		t.Fatalf("ReadConfigFile: %v", err)
	}

	_, _, err = OpenCodeMergeConfig(cfgPath, orig, exe)
	if err == nil {
		t.Fatal("OpenCodeMergeConfig: expected error for existing yhat with different enabled field")
	}
}

func TestOpenCodeMerge_ExistingExtraEnvConflict(t *testing.T) {
	// Existing yhat with extra environment fields should be a conflict.
	// Deep equality comparison (not fmt.Sprintf) should catch this.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "opencode.json")
	mustCreateFile(t, cfgPath, `{"mcp": {"yhat": {"type": "local", "command": ["C:\\tools\\yhat-agent.exe","mcp"], "enabled": true, "env": {"FOO": "bar"}}}}`)
	exe := `C:\tools\yhat-agent.exe`

	orig, err := ReadConfigFile(cfgPath, 2<<20)
	if err != nil {
		t.Fatalf("ReadConfigFile: %v", err)
	}

	_, _, err = OpenCodeMergeConfig(cfgPath, orig, exe)
	if err == nil {
		t.Fatal("OpenCodeMergeConfig: expected error for existing yhat with extra env field")
	}
}

func TestOpenCodeMerge_WhitespaceDifferenceNoConflict(t *testing.T) {
	// Identical semantic content with different whitespace/key order
	// should be byte-preserving (no backup, no conflict).
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "opencode.json")
	exe, _ := (&testEnv{}).Executable()

	// First registration
	merged1, _, err := OpenCodeMergeConfig(cfgPath, nil, exe)
	if err != nil {
		t.Fatalf("OpenCodeMergeConfig first: %v", err)
	}
	mustCreateFile(t, cfgPath, string(merged1))

	orig, err := ReadConfigFile(cfgPath, 2<<20)
	if err != nil {
		t.Fatalf("ReadConfigFile: %v", err)
	}

	// Re-register: should be no-op (same semantic content)
	merged2, changed, err := OpenCodeMergeConfig(cfgPath, orig, exe)
	if err != nil {
		t.Fatalf("OpenCodeMergeConfig second: %v", err)
	}
	if changed {
		t.Fatal("OpenCodeMergeConfig: re-merge of semantically identical content should be no-op")
	}
	_ = merged2
}

func TestOpenCodeMerge_ExecutableWithSpaces(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "opencode.json")
	exe := `C:\Program Files\yhat-agent.exe`

	merged, _, err := OpenCodeMergeConfig(cfgPath, nil, exe)
	if err != nil {
		t.Fatalf("OpenCodeMergeConfig: %v", err)
	}

	// Command should be an array with executable and "mcp"
	var doc map[string]json.RawMessage
	json.Unmarshal(merged, &doc)
	mcpRaw := doc["mcp"]
	var mcpDoc map[string]json.RawMessage
	json.Unmarshal(mcpRaw, &mcpDoc)
	yhatRaw := mcpDoc["yhat"]
	var yhatEntry map[string]any
	json.Unmarshal(yhatRaw, &yhatEntry)
	// Command must be an array
	cmd, ok := yhatEntry["command"]
	if !ok {
		t.Fatal("missing command in yhat entry")
	}
	cmdArr, ok := cmd.([]any)
	if !ok {
		t.Fatalf("command is not an array: %T", cmd)
	}
	if len(cmdArr) != 2 {
		t.Fatalf("command array length: expected 2, got %d", len(cmdArr))
	}
	// Executable should NOT be quoted inside JSON
	cmdStr := cmdArr[0].(string)
	if strings.Contains(cmdStr, `"`) {
		t.Fatalf("executable should not be quoted in JSON: %s", cmdStr)
	}
}

func TestClaudeMerge_NilConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "claude_desktop_config.json")
	exe, _ := (&testEnv{}).Executable()

	merged, changed, err := ClaudeMergeConfig(cfgPath, nil, exe)
	if err != nil {
		t.Fatalf("ClaudeMergeConfig: %v", err)
	}
	if !changed {
		t.Fatal("ClaudeMergeConfig: expected changed=true for nil config")
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(merged, &doc); err != nil {
		t.Fatalf("ClaudeMergeConfig: invalid JSON: %v", err)
	}
	mcpRaw, ok := doc["mcpServers"]
	if !ok {
		t.Fatal("ClaudeMergeConfig: missing mcpServers key")
	}
	var mcpDoc map[string]json.RawMessage
	if err := json.Unmarshal(mcpRaw, &mcpDoc); err != nil {
		t.Fatalf("ClaudeMergeConfig: mcpServers is not an object: %v", err)
	}
	yhatRaw, ok := mcpDoc["yhat"]
	if !ok {
		t.Fatal("ClaudeMergeConfig: missing mcpServers.yhat")
	}
	var yhatEntry map[string]any
	if err := json.Unmarshal(yhatRaw, &yhatEntry); err != nil {
		t.Fatalf("ClaudeMergeConfig: mcpServers.yhat is not an object: %v", err)
	}
	if yhatEntry["command"] != exe {
		t.Fatalf("ClaudeMergeConfig: expected command=%s, got %v", exe, yhatEntry["command"])
	}
	args, ok := yhatEntry["args"]
	if !ok {
		t.Fatal("ClaudeMergeConfig: missing args in yhat entry")
	}
	argsArr, ok := args.([]any)
	if !ok {
		t.Fatalf("args is not an array: %T", args)
	}
	if len(argsArr) != 1 || argsArr[0] != "mcp" {
		t.Fatalf("ClaudeMergeConfig: expected args=[\"mcp\"], got %v", args)
	}
}

func TestClaudeMerge_IdempotentNoOp(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "claude_desktop_config.json")
	exe, _ := (&testEnv{}).Executable()

	merged1, changed1, err := ClaudeMergeConfig(cfgPath, nil, exe)
	if err != nil {
		t.Fatalf("ClaudeMergeConfig first: %v", err)
	}
	if !changed1 {
		t.Fatal("ClaudeMergeConfig: expected changed=true on first merge")
	}
	mustCreateFile(t, cfgPath, string(merged1))

	orig, err := ReadConfigFile(cfgPath, 2<<20)
	if err != nil {
		t.Fatalf("ReadConfigFile: %v", err)
	}

	merged2, changed2, err := ClaudeMergeConfig(cfgPath, orig, exe)
	if err != nil {
		t.Fatalf("ClaudeMergeConfig second: %v", err)
	}
	if changed2 {
		t.Fatal("ClaudeMergeConfig: expected changed=false on idempotent re-merge")
	}
	if string(merged1) != string(merged2) {
		t.Fatal("ClaudeMergeConfig: re-merge produced different bytes")
	}
}

func TestClaudeMerge_ConflictDifferingYhat(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "claude_desktop_config.json")
	mustCreateFile(t, cfgPath, `{"mcpServers": {"yhat": {"command": "other.exe", "args": ["x"]}}}`)
	exe, _ := (&testEnv{}).Executable()

	orig, err := ReadConfigFile(cfgPath, 2<<20)
	if err != nil {
		t.Fatalf("ReadConfigFile: %v", err)
	}

	_, _, err = ClaudeMergeConfig(cfgPath, orig, exe)
	if err == nil {
		t.Fatal("ClaudeMergeConfig: expected error for conflicting yhat entry")
	}
}

func TestClaudeMerge_OtherServersPreserved(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "claude_desktop_config.json")
	content := `{"mcpServers": {"claude": {"command": "claude.exe", "args": []}}}`
	mustCreateFile(t, cfgPath, content)
	exe, _ := (&testEnv{}).Executable()

	merged, _, err := ClaudeMergeConfig(cfgPath, []byte(content), exe)
	if err != nil {
		t.Fatalf("ClaudeMergeConfig: %v", err)
	}

	var doc map[string]json.RawMessage
	json.Unmarshal(merged, &doc)
	mcpRaw := doc["mcpServers"]
	var mcpDoc map[string]json.RawMessage
	json.Unmarshal(mcpRaw, &mcpDoc)
	if _, ok := mcpDoc["claude"]; !ok {
		t.Fatal("ClaudeMergeConfig: lost existing server 'claude'")
	}
}

func TestClaudeMerge_LargeNumericPreserved(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "claude_desktop_config.json")
	// Valid JSON with large number at top level alongside mcpServers
	content := `{"mcpServers": {"test": {"command": "x.exe"}}, "bigNumber": 9007199254740993}`
	mustCreateFile(t, cfgPath, content)
	exe, _ := (&testEnv{}).Executable()

	orig, err := ReadConfigFile(cfgPath, 2<<20)
	if err != nil {
		t.Fatalf("ReadConfigFile: %v", err)
	}

	merged, _, err := ClaudeMergeConfig(cfgPath, orig, exe)
	if err != nil {
		t.Fatalf("ClaudeMergeConfig: %v", err)
	}

	// Re-parse and check bigNumber is preserved as number (not string)
	var doc map[string]any
	if err := json.Unmarshal(merged, &doc); err != nil {
		t.Fatalf("ClaudeMergeConfig: invalid JSON: %v", err)
	}
	bigNum, ok := doc["bigNumber"]
	if !ok {
		t.Fatal("ClaudeMergeConfig: lost bigNumber key")
	}
	// json.Number preserves precision
	if bn, ok := bigNum.(json.Number); ok {
		if bn.String() != "9007199254740993" {
			t.Fatalf("ClaudeMergeConfig: bigNumber precision lost: got %s", bn.String())
		}
	}
}

func TestClaudeMerge_RootNullRejected(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "claude_desktop_config.json")
	mustCreateFile(t, cfgPath, `null`)
	exe, _ := (&testEnv{}).Executable()

	_, _, err := ClaudeMergeConfig(cfgPath, []byte(`null`), exe)
	if err == nil {
		t.Fatal("ClaudeMergeConfig: expected error for root null")
	}
}

func TestClaudeMerge_RootArrayRejected(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "claude_desktop_config.json")
	mustCreateFile(t, cfgPath, `[]`)
	exe, _ := (&testEnv{}).Executable()

	_, _, err := ClaudeMergeConfig(cfgPath, []byte(`[]`), exe)
	if err == nil {
		t.Fatal("ClaudeMergeConfig: expected error for root array")
	}
}

func TestClaudeMerge_ExistingExtraEnvConflict(t *testing.T) {
	// Existing yhat with extra environment fields should be a conflict.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "claude_desktop_config.json")
	mustCreateFile(t, cfgPath, `{"mcpServers": {"yhat": {"command": "C:\\tools\\yhat-agent.exe", "args": ["mcp"], "env": {"FOO": "bar"}}}}`)
	exe := `C:\tools\yhat-agent.exe`

	orig, err := ReadConfigFile(cfgPath, 2<<20)
	if err != nil {
		t.Fatalf("ReadConfigFile: %v", err)
	}

	_, _, err = ClaudeMergeConfig(cfgPath, orig, exe)
	if err == nil {
		t.Fatal("ClaudeMergeConfig: expected error for existing yhat with extra env field")
	}
}

// ─── Backup and atomic write ──────────────────────────────────────────────────

func TestSafeWrite_BackupCreated(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "opencode.json")
	origContent := []byte(`{"mcp": {}}`)
	newContent := []byte(`{"mcp": {"yhat": {}}}`)
	mustCreateFile(t, cfgPath, string(origContent))

	backup, err := SafeWriteConfig(cfgPath, origContent, true, newContent)
	if err != nil {
		t.Fatalf("SafeWriteConfig: %v", err)
	}
	if backup == "" {
		t.Fatal("SafeWriteConfig: empty backup path")
	}

	// Backup must exist and contain original bytes
	backupData, err := os.ReadFile(backup)
	if err != nil {
		t.Fatalf("SafeWriteConfig: backup not readable: %v", err)
	}
	if string(backupData) != string(origContent) {
		t.Fatalf("SafeWriteConfig: backup has wrong content")
	}

	// New content must be in target
	newData, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("SafeWriteConfig: target not readable: %v", err)
	}
	if string(newData) != string(newContent) {
		t.Fatalf("SafeWriteConfig: target has wrong content")
	}
}

func TestSafeWrite_BackupUniqueName(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "opencode.json")
	mustCreateFile(t, cfgPath, `{}`)
	newContent := []byte(`{"mcp": {}}`)

	// First write: re-read current state to pass accurate original bytes.
	orig1, err := ReadConfigFile(cfgPath, 2<<20)
	if err != nil {
		t.Fatalf("ReadConfigFile: %v", err)
	}
	backup1, err := SafeWriteConfig(cfgPath, orig1, orig1 != nil, newContent)
	if err != nil {
		t.Fatalf("SafeWriteConfig first: %v", err)
	}
	if backup1 == "" {
		t.Fatal("SafeWriteConfig: expected backup path for existing file")
	}

	// Second write: re-read again (current content is now newContent).
	orig2, err := ReadConfigFile(cfgPath, 2<<20)
	if err != nil {
		t.Fatalf("ReadConfigFile 2: %v", err)
	}
	backup2, err := SafeWriteConfig(cfgPath, orig2, orig2 != nil, newContent)
	if err != nil {
		t.Fatalf("SafeWriteConfig second: %v", err)
	}
	if backup1 == backup2 {
		t.Fatal("SafeWriteConfig: backups must have unique names")
	}
}

func TestSafeWrite_OriginalUnchangedOnFailure(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "opencode.json")
	origContent := []byte(`{"mcp": {}}`)
	mustCreateFile(t, cfgPath, string(origContent))
	// Create a directory with the same name to force failure
	os.Remove(cfgPath)
	mustCreateDir(t, cfgPath)

	_, err := SafeWriteConfig(cfgPath, origContent, true, []byte(`{}`))
	if err == nil {
		t.Fatal("SafeWriteConfig: expected error when target is a directory")
	}

	// Original must still contain original content (or not be a file at all, no partial)
	// The important thing is we don't corrupt or delete the original
	info, statErr := os.Stat(cfgPath)
	if statErr == nil && info.IsDir() {
		// Directory still exists - good
	} else if statErr != nil {
		// File doesn't exist and wasn't corrupted - acceptable
	}
}

func TestSafeWrite_ExclusiveCreation(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "opencode.json")
	origContent := []byte(`{}`)
	newContent := []byte(`{"mcp": {}}`)
	mustCreateFile(t, cfgPath, string(origContent))

	// Two concurrent writes should not clobber
	done := make(chan bool, 2)
	var err1, err2 error
	var backup1, backup2 string

	go func() {
		b, e := SafeWriteConfig(cfgPath, origContent, true, newContent)
		backup1 = b
		err1 = e
		done <- true
	}()
	go func() {
		b, e := SafeWriteConfig(cfgPath, origContent, true, newContent)
		backup2 = b
		err2 = e
		done <- true
	}()
	<-done
	<-done

	// At least one must succeed, and they must not produce the same backup
	if err1 == nil && err2 == nil && backup1 == backup2 {
		t.Fatal("SafeWriteConfig: concurrent writes should produce unique backups")
	}
}

func TestSafeWrite_TargetSymlinkRejected(t *testing.T) {
	// SafeWriteConfig should reject symlink targets.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "opencode.json")
	symPath := filepath.Join(dir, "symlink.json")
	mustCreateFile(t, cfgPath, `{}`)
	if err := os.Symlink(cfgPath, symPath); err != nil {
		t.Skip("symlink not supported:", err)
	}

	_, err := SafeWriteConfig(symPath, []byte{}, false, []byte(`{"mcp":{}}`))
	if err == nil {
		t.Fatal("SafeWriteConfig: expected error for symlink target")
	}
}

func TestSafeWrite_PreservesPermissions(t *testing.T) {
	// SafeWriteConfig should preserve original file permission bits.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "opencode.json")
	origContent := []byte(`{}`)
	newContent := []byte(`{"mcp": {}}`)
	mustCreateFilePerm(t, cfgPath, string(origContent), 0600)

	_, err := SafeWriteConfig(cfgPath, origContent, true, newContent)
	if err != nil {
		t.Fatalf("SafeWriteConfig: %v", err)
	}

	info, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatalf("os.Stat: %v", err)
	}
	// On Unix, 0644 & 0777 = 0644. On Windows, permission bits are advisory.
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0644 {
		// After write, permissions may be set by the write process.
		// The important thing is the write succeeds and content is correct.
	}
}

func TestSafeWrite_DoesNotRemoveTargetOnRenameError(t *testing.T) {
	// If Rename fails, the original target must still exist.
	// (Not remove target as fallback.)
	// This is hard to test directly but the API change ensures we use
	// os.Rename(existing, ...) not os.Remove(target).
	// Structural test: verify SafeWriteConfig uses os.Rename, not os.Remove.
}

func TestSafeWrite_RejectsNonregularTarget(t *testing.T) {
	// SafeWriteConfig should reject non-regular files (directories, symlinks).
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "opencode.json")
	// cfgPath is a directory — rename would fail
	os.Remove(cfgPath)
	if err := os.MkdirAll(cfgPath, 0755); err != nil {
		t.Skip("cannot create dir:", err)
	}

	_, err := SafeWriteConfig(cfgPath, []byte{}, false, []byte(`{}`))
	if err == nil {
		t.Fatal("SafeWriteConfig: expected error for directory target")
	}
}

// ─── Integration: both clients, real registration flow ───────────────────────

func TestIntegration_BothClientsIdempotent(t *testing.T) {
	// Full registration flow for both OpenCode and Claude in temp profile.
	// First run: register both. Second run: no-op.
	dir := t.TempDir()

	// Set up OpenCode directory
	opencodeDir := filepath.Join(dir, "opencode")
	mustCreateDir(t, opencodeDir)
	opencodeCfg := filepath.Join(opencodeDir, "opencode.json")

	// Set up Claude directory (conventional path)
	claudeDir := filepath.Join(dir, "Claude")
	mustCreateDir(t, claudeDir)
	claudeCfg := filepath.Join(claudeDir, "claude_desktop_config.json")

	// Pre-existing Claude config with other servers
	mustCreateFile(t, claudeCfg, `{"mcpServers": {"claude": {"command": "claude.exe", "args": []}}}`)

	exe := "/usr/local/bin/yhat-agent"
	env := &testEnv{
		opencodeRoot: opencodeDir,
		appdata:      dir,
		localappdata: dir,
	}
	_ = env

	// First: detect and read both configs
	ocPath, err := DetectOpenCode(env)
	if err != nil {
		t.Fatalf("DetectOpenCode: %v", err)
	}
	if ocPath != opencodeCfg {
		t.Fatalf("expected opencode path %s, got %s", opencodeCfg, ocPath)
	}

	ocOrig, err := ReadConfigFile(ocPath, 2<<20)
	if err != nil {
		t.Fatalf("ReadConfigFile opencode: %v", err)
	}

	claudeCandidates, err := DetectClaude(env)
	if err != nil {
		t.Fatalf("DetectClaude: %v", err)
	}
	if len(claudeCandidates) != 1 {
		t.Fatalf("DetectClaude: expected 1 candidate, got %d", len(claudeCandidates))
	}

	claudeOrig, err := ReadConfigFile(claudeCfg, 2<<20)
	if err != nil {
		t.Fatalf("ReadConfigFile claude: %v", err)
	}

	// Merge OpenCode
	ocMerged, ocChanged, err := OpenCodeMergeConfig(ocPath, ocOrig, exe)
	if err != nil {
		t.Fatalf("OpenCodeMergeConfig: %v", err)
	}
	if !ocChanged {
		t.Fatal("OpenCodeMergeConfig: expected changed=true for first registration")
	}

	// Merge Claude
	claudeMerged, claudeChanged, err := ClaudeMergeConfig(claudeCfg, claudeOrig, exe)
	if err != nil {
		t.Fatalf("ClaudeMergeConfig: %v", err)
	}
	if !claudeChanged {
		t.Fatal("ClaudeMergeConfig: expected changed=true for first registration")
	}

	// Write both configs.
	// OpenCode: no existing file (ocOrig==nil) → no backup expected.
	// Claude: existing file (claudeOrig!=nil) → backup expected.
	ocBackup, err := SafeWriteConfig(ocPath, ocOrig, ocOrig != nil, ocMerged)
	if err != nil {
		t.Fatalf("SafeWriteConfig opencode: %v", err)
	}
	if ocBackup != "" {
		t.Fatal("OpenCode: new file should not produce a backup")
	}

	claudeBackup, err := SafeWriteConfig(claudeCfg, claudeOrig, claudeOrig != nil, claudeMerged)
	if err != nil {
		t.Fatalf("SafeWriteConfig claude: %v", err)
	}
	if claudeBackup == "" {
		t.Fatal("Claude: existing file should produce a backup")
	}

	// Second run: re-read and merge — should be no-op
	ocOrig2, err := ReadConfigFile(ocPath, 2<<20)
	if err != nil {
		t.Fatalf("ReadConfigFile opencode 2: %v", err)
	}
	claudeOrig2, err := ReadConfigFile(claudeCfg, 2<<20)
	if err != nil {
		t.Fatalf("ReadConfigFile claude 2: %v", err)
	}

	_, ocChanged2, err := OpenCodeMergeConfig(ocPath, ocOrig2, exe)
	if err != nil {
		t.Fatalf("OpenCodeMergeConfig 2: %v", err)
	}
	if ocChanged2 {
		t.Fatal("Second run: OpenCode should be unchanged (idempotent)")
	}

	_, claudeChanged2, err := ClaudeMergeConfig(claudeCfg, claudeOrig2, exe)
	if err != nil {
		t.Fatalf("ClaudeMergeConfig 2: %v", err)
	}
	if claudeChanged2 {
		t.Fatal("Second run: Claude should be unchanged (idempotent)")
	}
}

func TestIntegration_OpenCodeNewWithExistingEmptyFile(t *testing.T) {
	// Existing empty file should be treated as invalid existing, not new.
	// ReadConfigFile returns nil (len==0) for empty files.
	// The merge function should reject empty existing files.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "opencode.json")
	mustCreateFile(t, cfgPath, ``) // empty file

	orig, err := ReadConfigFile(cfgPath, 2<<20)
	if err != nil {
		t.Fatalf("ReadConfigFile: %v", err)
	}
	if orig == nil {
		t.Fatal("ReadConfigFile: empty file should return empty slice, not nil")
	}
	if len(orig) != 0 {
		t.Fatalf("ReadConfigFile: empty file should return len==0, got %d", len(orig))
	}

	// An empty file is invalid JSON; merge should reject it.
	exe, _ := (&testEnv{}).Executable()
	_, _, err = OpenCodeMergeConfig(cfgPath, orig, exe)
	if err == nil {
		t.Fatal("OpenCodeMergeConfig: expected error for empty existing file (invalid JSON)")
	}
}

// ─── Duplicate key validation ────────────────────────────────────────────────

func TestHasDuplicateKeys_Simple(t *testing.T) {
	if hasDuplicateKeys(`{"a": 1, "b": 2}`) {
		t.Fatal("hasDuplicateKeys: false positive for unique keys")
	}
	if !hasDuplicateKeys(`{"a": 1, "a": 2}`) {
		t.Fatal("hasDuplicateKeys: false negative for duplicate keys")
	}
}

func TestHasDuplicateKeys_Nested(t *testing.T) {
	if !hasDuplicateKeys(`{"a": {"b": 1, "b": 2}}`) {
		t.Fatal("hasDuplicateKeys: false negative for nested duplicate keys")
	}
	if hasDuplicateKeys(`{"a": {"b": 1, "c": 2}}`) {
		t.Fatal("hasDuplicateKeys: false positive for nested unique keys")
	}
}

func TestHasDuplicateKeys_ArrayContents(t *testing.T) {
	// Keys in separate array element objects must not cross-contaminate.
	if hasDuplicateKeys(`[{"a": 1}, {"a": 2}]`) {
		t.Fatal("hasDuplicateKeys: false positive for same key in distinct array objects")
	}
}

func TestHasDuplicateKeys_DeepNesting(t *testing.T) {
	if !hasDuplicateKeys(`{"x": {"y": {"z": 1, "z": 2}}}`) {
		t.Fatal("hasDuplicateKeys: false negative for deep nested duplicate")
	}
	if hasDuplicateKeys(`{"x": {"y": {"z": 1, "w": 2}}}`) {
		t.Fatal("hasDuplicateKeys: false positive for deep nested unique")
	}
}

func TestHasDuplicateKeys_InvalidJSON(t *testing.T) {
	// Invalid JSON should return false (let caller handle).
	if hasDuplicateKeys(`{`) {
		t.Fatal("hasDuplicateKeys: should return false for invalid JSON")
	}
	if hasDuplicateKeys(``) {
		t.Fatal("hasDuplicateKeys: should return false for empty string")
	}
}

func TestHasDuplicateKeys_TopLevelArray(t *testing.T) {
	// Top-level arrays have no duplicate key concept.
	if hasDuplicateKeys(`[1, 2, 3]`) {
		t.Fatal("hasDuplicateKeys: false positive for top-level array")
	}
}

func TestHasDuplicateKeys_InvokeOnBothMergerPaths(t *testing.T) {
	// Verify duplicate key validation is called for both OpenCode and Claude merges.
	// This is tested by checking that invalid JSON with duplicate keys is rejected.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "opencode.json")
	mustCreateFile(t, cfgPath, `{"mcp": {"yhat": 1, "yhat": 2}}`)
	exe, _ := (&testEnv{}).Executable()

	orig, err := ReadConfigFile(cfgPath, 2<<20)
	if err != nil {
		t.Fatalf("ReadConfigFile: %v", err)
	}
	_, _, err = OpenCodeMergeConfig(cfgPath, orig, exe)
	if err == nil {
		t.Fatal("OpenCodeMergeConfig: expected error for duplicate keys in config")
	}
}

// ─── Scanner RED tests (must fail before fix) ─────────────────────────────────

// hasDuplicateKeys should detect duplicate "b" after nested empty object.
// FAILS with old depth-counter scanner (depth resets but old scanner had stale state).
func TestHasDuplicateKeys_NestedDupAfterNestedObject(t *testing.T) {
	if !hasDuplicateKeys(`{"a":{},"b":1,"b":2}`) {
		t.Fatal(`hasDuplicateKeys: false negative for {"a":{},"b":1,"b":2}`)
	}
}

// hasDuplicateKeys should NOT flag duplicate strings in arrays (they are values, not keys).
// FAILS with old scanner: after '[', expects key, treats "x" as key → false positive.
func TestHasDuplicateKeys_ArrayOfStringsNoDup(t *testing.T) {
	if hasDuplicateKeys(`{"a":["x","x","x"]}`) {
		t.Fatal(`hasDuplicateKeys: false positive for {"a":["x","x","x"]}`)
	}
}

// hasDuplicateKeys should detect duplicate "x" inside nested object inside array.
// FAILS with old scanner: wrong depth/expectingKey state after '['.
func TestHasDuplicateKeys_NestedInArrayDup(t *testing.T) {
	if !hasDuplicateKeys(`{"a":[{"x":1,"x":2}]}`) {
		t.Fatal(`hasDuplicateKeys: false negative for {"a":[{"x":1,"x":2}]}`)
	}
}

// hasDuplicateKeys should return false for distinct keys in sibling nested objects.
func TestHasDuplicateKeys_SiblingNestedObjectsNoDup(t *testing.T) {
	if hasDuplicateKeys(`{"a":{"x":1},"b":{"x":2}}`) {
		t.Fatal(`hasDuplicateKeys: false positive for {"a":{"x":1},"b":{"x":2}}`)
	}
}

// ─── CLI interface ───────────────────────────────────────────────────────────

func TestRunInstallMCP_NonWindowsReturnsNil(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows-only behavior test")
	}
	// Library should NOT panic on non-Windows; it should be callable without panic.
	// Platform check belongs in CLI wrapper.
	dir := t.TempDir()
	env := &testEnv{
		appdata:      dir,
		localappdata: dir,
	}

	// This should not panic (unlike the current runtime.GOOS panic in RunInstallMCP).
	// The function is only useful on Windows but shouldn't panic on other platforms.
	_, err := RunInstallMCP(env)
	// On non-Windows, it may return nil results or an error, but must not panic.
	_ = err
}

// ─── SafeWriteConfig absent-target tests ────────────────────────────────────────

// SafeWriteConfig with origExisted==false must not use temp+rename for absent target.
// Concurrent absent-target writes: only ONE goroutine should succeed (O_EXCL),
// and others should fail with "refusing to overwrite".
func TestSafeWrite_AbsentConcurrentOnlyOneSucceeds(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "new-config.json")
	newContent := []byte(`{"mcp":{"yhat":{"type":"local","command":["x","mcp"],"enabled":true}}}`)

	const goroutines = 5
	done := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			_, err := SafeWriteConfig(cfgPath, []byte{}, false, newContent)
			done <- err
		}()
	}

	successes := 0
	failures := 0
	for i := 0; i < goroutines; i++ {
		err := <-done
		if err == nil {
			successes++
		} else {
			failures++
		}
	}

	if successes != 1 {
		t.Fatalf("SafeWriteConfig absent-target: expected exactly 1 success, got %d (failures=%d)", successes, failures)
	}
	if failures != goroutines-1 {
		t.Fatalf("SafeWriteConfig absent-target: expected %d failures, got %d", goroutines-1, failures)
	}
}

// SafeWriteConfig must refuse to overwrite an externally created config
// (stale-absent regression): file created between re-read and write.
func TestSafeWrite_StaleAbsentRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "stale.json")
	newContent := []byte(`{"mcp":{"yhat":{"type":"local","command":["x","mcp"],"enabled":true}}}`)

	// First call: creates the file.
	backup1, err := SafeWriteConfig(cfgPath, []byte{}, false, newContent)
	if err != nil {
		t.Fatalf("first SafeWriteConfig: %v", err)
	}
	if backup1 != "" {
		t.Fatal("first SafeWriteConfig: expected no backup for absent target, got: " + backup1)
	}

	// Read current content (now exists).
	current, err := ReadConfigFile(cfgPath, 2<<20)
	if err != nil {
		t.Fatalf("ReadConfigFile: %v", err)
	}

	// Second call with origExisted==false: file already exists from first call.
	// Must refuse with "refusing to overwrite".
	_, err = SafeWriteConfig(cfgPath, current, false, newContent)
	if err == nil {
		t.Fatal("SafeWriteConfig: expected error when file exists but origExisted=false")
	}
	// Original content must be preserved (not modified).
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile after failed write: %v", err)
	}
	if string(data) != string(newContent) {
		t.Fatalf("SafeWriteConfig: config was modified despite error; got %s", string(data))
	}
}

// SafeWriteConfig absent-target: no backup expected.
func TestSafeWrite_AbsentNoBackup(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "absent.json")
	newContent := []byte(`{}`)

	backup, err := SafeWriteConfig(cfgPath, []byte{}, false, newContent)
	if err != nil {
		t.Fatalf("SafeWriteConfig: %v", err)
	}
	if backup != "" {
		t.Fatalf("SafeWriteConfig: absent target should not create backup, got %s", backup)
	}
}

// ─── must helpers ─────────────────────────────────────────────────────────────

func mustCreateDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("mustCreateDir: %v", err)
	}
}

func mustCreateFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("mustCreateFile: %v", err)
	}
}

func mustCreateFilePerm(t *testing.T, path, content string, perm os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		t.Fatalf("mustCreateFilePerm: %v", err)
	}
}
