// Package clients provides MCP registration for OpenCode and Claude Desktop
// on Windows WorkSpaces. Library is platform-neutral; callers apply platform
// gates as needed. All paths are injectable for cross-platform testing.
package clients

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

// ─── Env interface ────────────────────────────────────────────────────────────

// Env abstracts platform-specific environment queries. Inject for testing.
type Env interface {
	// OpenCodeRoot returns the OpenCode configuration root directory.
	OpenCodeRoot() string
	// OpenCodeConfig returns the absolute path from OPENCODE_CONFIG env var,
	// or empty string if not set.
	OpenCodeConfig() string
	// AppData returns %APPDATA% on Windows, or equivalent config root.
	AppData() string
	// LocalAppData returns %LOCALAPPDATA% on Windows, or equivalent.
	LocalAppData() string
	// Executable returns the absolute path of the current binary.
	Executable() (string, error)
}

// ─── Result types ─────────────────────────────────────────────────────────────

// Result describes the outcome of registering with one client.
type Result struct {
	Client  string // "opencode" or "claude"
	State   string // "configured", "unchanged", "skipped", "error"
	Config  string // absolute config path, if discovered
	Backup  string // absolute backup path, if created
	Message string
}

// RunInstallMCP runs MCP registration for all applicable clients.
// On non-Windows platforms it returns nil results with no error (no-op);
// platform gating is the caller's responsibility when needed.
func RunInstallMCP(env Env) ([]Result, error) {
	exe, err := env.Executable()
	if err != nil {
		return nil, fmt.Errorf("getting executable path: %w", err)
	}

	var results []Result

	// OpenCode
	ocResult := installOpenCode(env, exe)
	results = append(results, ocResult)

	// Claude
	claudeResult := installClaude(env, exe)
	results = append(results, claudeResult)

	return results, nil
}

// ─── OpenCode ────────────────────────────────────────────────────────────────

func installOpenCode(env Env, exe string) Result {
	result := Result{Client: "opencode"}

	cfgPath, err := DetectOpenCode(env)
	if err != nil {
		result.State = "error"
		result.Message = err.Error()
		return result
	}
	result.Config = cfgPath

	// ENOENT → nil (absent); empty file → []byte{} (invalid existing).
	// All other errors propagate.
	orig, readErr := ReadConfigFile(cfgPath, 2<<20)
	if readErr != nil {
		result.State = "error"
		result.Message = readErr.Error()
		return result
	}

	// orig == nil means file does not exist → new config.
	// orig != nil && len(orig) == 0 means empty file → invalid existing.
	merged, changed, mergeErr := OpenCodeMergeConfig(cfgPath, orig, exe)
	if mergeErr != nil {
		result.State = "error"
		result.Message = mergeErr.Error()
		return result
	}

	if !changed {
		result.State = "unchanged"
		result.Message = "yhat already registered"
		return result
	}

	// Pass original state to SafeWriteConfig so it can compare
	// before committing and refuse concurrent writes.
	origExisted := orig != nil
	origBytes := orig
	if origBytes == nil {
		origBytes = []byte{} // non-nil for comparison API
	}

	backup, writeErr := SafeWriteConfig(cfgPath, origBytes, origExisted, merged)
	if writeErr != nil {
		result.State = "error"
		result.Message = writeErr.Error()
		return result
	}
	result.Backup = backup
	result.State = "configured"
	result.Message = "registered"
	return result
}

// DetectOpenCode finds the OpenCode config file path.
// It respects OPENCODE_CONFIG env var if set to an absolute path.
// If OPENCODE_CONFIG is not set, it returns OpenCodeRoot()/opencode.json,
// creating the directory if needed.
// Returns only regular .json files; .jsonc siblings cause an error.
// Strict JSON parser rejects JSONC naturally; no heuristic stripping.
func DetectOpenCode(env Env) (string, error) {
	if env.OpenCodeConfig() != "" {
		cfg := env.OpenCodeConfig()
		if !filepath.IsAbs(cfg) {
			return "", errors.New("OPENCODE_CONFIG must be an absolute path")
		}
		if !strings.HasSuffix(strings.ToLower(cfg), ".json") || strings.HasSuffix(strings.ToLower(cfg), ".jsonc") {
			return "", errors.New("OPENCODE_CONFIG must be a .json file, not .jsonc")
		}
		info, err := os.Lstat(cfg)
		if err == nil && (info.Mode()&os.ModeSymlink) != 0 {
			return "", errors.New("OPENCODE_CONFIG is a symlink")
		}
		return cfg, nil
	}

	root := env.OpenCodeRoot()

	entries, err := os.ReadDir(root)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("reading OpenCode root: %w", err)
	}

	var jsonFile, jsoncFile string
	for _, entry := range entries {
		name := strings.ToLower(entry.Name())
		if name == "opencode.json" {
			jsonFile = filepath.Join(root, entry.Name())
		} else if name == "opencode.jsonc" {
			jsoncFile = filepath.Join(root, entry.Name())
		}
	}

	if jsoncFile != "" {
		return "", errors.New("opencode.jsonc found alongside opencode.json: refusing ambiguous config")
	}

	if jsonFile != "" {
		info, err := os.Lstat(jsonFile)
		if err == nil && (info.Mode()&os.ModeSymlink) != 0 {
			return "", errors.New("opencode.json is a symlink")
		}
		return jsonFile, nil
	}

	if err := os.MkdirAll(root, 0755); err != nil {
		return "", fmt.Errorf("creating OpenCode root: %w", err)
	}
	return filepath.Join(root, "opencode.json"), nil
}

// OpenCodeMergeConfig merges the yhat MCP entry into the OpenCode config.
// exe is the absolute path of the current binary.
// If orig is nil, creates a new config with mcp.yhat entry.
// orig len==0 means file exists but is empty (invalid existing → error).
// Returns merged bytes, whether the content changed, and any error.
// If the existing yhat entry is semantically identical, returns orig with changed=false.
func OpenCodeMergeConfig(cfgPath string, orig []byte, exe string) ([]byte, bool, error) {
	want := map[string]any{
		"type":    "local",
		"command": []any{exe, "mcp"},
		"enabled": true,
	}

	// orig == nil → file does not exist → create new.
	if orig == nil {
		doc := map[string]any{
			"mcp": map[string]any{
				"yhat": want,
			},
		}
		merged, err := json.Marshal(doc)
		return merged, true, err
	}

	// orig != nil && len(orig) == 0 → empty file → invalid.
	if len(orig) == 0 {
		return nil, false, errors.New("config file is empty (invalid JSON)")
	}

	// Validate duplicate keys before parsing.
	if hasDuplicateKeys(string(orig)) {
		return nil, false, errors.New("config contains duplicate keys")
	}

	// Parse existing config preserving all raw messages.
	var rawDoc map[string]json.RawMessage
	if err := json.Unmarshal(orig, &rawDoc); err != nil {
		return nil, false, fmt.Errorf("parsing config: %w", err)
	}

	// json.Unmarshal succeeds for root null/array/scalar but creates nil/empty map.
	if rawDoc == nil {
		return nil, false, errors.New("config root must be an object, not null")
	}

	// Validate mcp namespace.
	mcpRaw, hasMcp := rawDoc["mcp"]
	if hasMcp {
		var mcpVal any
		if err := json.Unmarshal(mcpRaw, &mcpVal); err != nil {
			return nil, false, fmt.Errorf("parsing mcp: %w", err)
		}
		switch v := mcpVal.(type) {
		case nil:
			return nil, false, errors.New("mcp namespace is null")
		case []any:
			return nil, false, errors.New("mcp namespace must be an object, not array")
		case map[string]any:
			_ = v
		default:
			return nil, false, fmt.Errorf("mcp namespace has unexpected type %T", mcpVal)
		}
	}

	var mcpDoc map[string]json.RawMessage
	if hasMcp {
		if err := json.Unmarshal(mcpRaw, &mcpDoc); err != nil {
			return nil, false, fmt.Errorf("parsing mcp: %w", err)
		}
	} else {
		mcpDoc = make(map[string]json.RawMessage)
	}

	// Check for existing yhat entry.
	if yhatRaw, exists := mcpDoc["yhat"]; exists {
		// Compare using json.Unmarshal + reflect.DeepEqual (small desired shape).
		// This catches any difference: enabled field, extra env, whitespace, key order.
		var existingYhat map[string]any
		if err := json.Unmarshal(yhatRaw, &existingYhat); err != nil {
			return nil, false, fmt.Errorf("parsing yhat entry: %w", err)
		}

		var wantLocal map[string]any
		wantBytes, _ := json.Marshal(want)
		if err := json.Unmarshal(wantBytes, &wantLocal); err != nil {
			return nil, false, err
		}

		if !reflect.DeepEqual(wantLocal, existingYhat) {
			return nil, false, errors.New("conflicting yhat entry: existing differs from desired")
		}

		// Semantically identical: no-op, return original bytes.
		return orig, false, nil
	}

	// Add yhat entry.
	wantJSON, err := json.Marshal(want)
	if err != nil {
		return nil, false, err
	}
	mcpDoc["yhat"] = wantJSON

	rawDoc["mcp"] = mustMarshalJSON(mcpDoc)
	merged, err := json.Marshal(rawDoc)
	if err != nil {
		return nil, false, err
	}

	return merged, true, nil
}

// ─── Claude Desktop ──────────────────────────────────────────────────────────

func installClaude(env Env, exe string) Result {
	result := Result{Client: "claude"}

	candidates, err := DetectClaude(env)
	if err != nil {
		result.State = "error"
		result.Message = err.Error()
		return result
	}

	if len(candidates) == 0 {
		result.State = "skipped"
		result.Message = "no Claude installation found"
		return result
	}

	if len(candidates) > 1 {
		result.State = "error"
		result.Message = fmt.Sprintf("multiple Claude installations found: %v", candidates)
		return result
	}

	claudeDir := candidates[0]
	configPath := filepath.Join(claudeDir, "claude_desktop_config.json")
	result.Config = configPath

	// ENOENT → nil (absent); empty file → []byte{} (invalid existing).
	orig, readErr := ReadConfigFile(configPath, 2<<20)
	if readErr != nil {
		result.State = "error"
		result.Message = readErr.Error()
		return result
	}

	merged, changed, mergeErr := ClaudeMergeConfig(configPath, orig, exe)
	if mergeErr != nil {
		result.State = "error"
		result.Message = mergeErr.Error()
		return result
	}

	if !changed {
		result.State = "unchanged"
		result.Message = "yhat already registered"
		return result
	}

	origExisted := orig != nil
	origBytes := orig
	if origBytes == nil {
		origBytes = []byte{}
	}

	backup, writeErr := SafeWriteConfig(configPath, origBytes, origExisted, merged)
	if writeErr != nil {
		result.State = "error"
		result.Message = writeErr.Error()
		return result
	}
	result.Backup = backup
	result.State = "configured"
	result.Message = "registered"
	return result
}

// DetectClaude returns Claude config directories found on this system.
// It searches the conventional APPDATA/Claude path and the packaged
// LOCALAPPDATA/Packages/Claude_*/LocalCache/Roaming/Claude path.
// Returns an error if multiple candidates are found.
func DetectClaude(env Env) ([]string, error) {
	var candidates []string

	// Conventional path: APPDATA/Claude
	appdata := env.AppData()
	if appdata != "" {
		conv := filepath.Join(appdata, "Claude")
		info, err := os.Lstat(conv)
		if err == nil {
			if info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
				candidates = append(candidates, conv)
			}
			// Symlink dirs or non-dirs are skipped (not errors, not candidates).
		}
		// os.IsNotExist: skip silently. Other errors: skip silently.
	}

	// Packaged path: LOCALAPPDATA/Packages/Claude_*/LocalCache/Roaming/Claude
	localappdata := env.LocalAppData()
	if localappdata != "" {
		packagesDir := filepath.Join(localappdata, "Packages")
		entries, err := os.ReadDir(packagesDir)
		if err != nil && !os.IsNotExist(err) {
			// Skip on read error.
		} else if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "Claude_") {
					continue
				}
				claudeDir := filepath.Join(packagesDir, entry.Name(), "LocalCache", "Roaming", "Claude")
				info, err := os.Lstat(claudeDir)
				if err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
					candidates = append(candidates, claudeDir)
				}
			}
		}
	}

	if len(candidates) > 1 {
		return candidates, fmt.Errorf("multiple Claude installations found: %v", candidates)
	}

	return candidates, nil
}

// ClaudeMergeConfig merges the yhat MCP entry into the Claude config.
// exe is the absolute path of the current binary.
// If orig is nil, creates a new config with mcpServers.yhat entry.
// orig len==0 means file exists but is empty (invalid existing → error).
// Returns merged bytes, whether the content changed, and any error.
func ClaudeMergeConfig(cfgPath string, orig []byte, exe string) ([]byte, bool, error) {
	want := map[string]any{
		"command": exe,
		"args":    []any{"mcp"},
	}

	if orig == nil {
		doc := map[string]any{
			"mcpServers": map[string]any{
				"yhat": want,
			},
		}
		merged, err := json.Marshal(doc)
		return merged, true, err
	}

	if len(orig) == 0 {
		return nil, false, errors.New("config file is empty (invalid JSON)")
	}

	if hasDuplicateKeys(string(orig)) {
		return nil, false, errors.New("config contains duplicate keys")
	}

	var rawDoc map[string]json.RawMessage
	if err := json.Unmarshal(orig, &rawDoc); err != nil {
		return nil, false, fmt.Errorf("parsing config: %w", err)
	}

	// json.Unmarshal succeeds for root null/array/scalar but creates nil/empty map.
	if rawDoc == nil {
		return nil, false, errors.New("config root must be an object, not null")
	}

	mcpServersRaw, hasServers := rawDoc["mcpServers"]
	if hasServers {
		var serversVal any
		if err := json.Unmarshal(mcpServersRaw, &serversVal); err != nil {
			return nil, false, fmt.Errorf("parsing mcpServers: %w", err)
		}
		switch v := serversVal.(type) {
		case nil:
			return nil, false, errors.New("mcpServers namespace is null")
		case []any:
			return nil, false, errors.New("mcpServers namespace must be an object, not array")
		case map[string]any:
			_ = v
		default:
			return nil, false, fmt.Errorf("mcpServers namespace has unexpected type %T", serversVal)
		}
	}

	var serversDoc map[string]json.RawMessage
	if hasServers {
		if err := json.Unmarshal(mcpServersRaw, &serversDoc); err != nil {
			return nil, false, fmt.Errorf("parsing mcpServers: %w", err)
		}
	} else {
		serversDoc = make(map[string]json.RawMessage)
	}

	if yhatRaw, exists := serversDoc["yhat"]; exists {
		var existingYhat map[string]any
		if err := json.Unmarshal(yhatRaw, &existingYhat); err != nil {
			return nil, false, fmt.Errorf("parsing yhat entry: %w", err)
		}

		var wantLocal map[string]any
		wantBytes, _ := json.Marshal(want)
		if err := json.Unmarshal(wantBytes, &wantLocal); err != nil {
			return nil, false, err
		}

		if !reflect.DeepEqual(wantLocal, existingYhat) {
			return nil, false, errors.New("conflicting yhat entry: existing differs from desired")
		}

		return orig, false, nil
	}

	wantJSON, err := json.Marshal(want)
	if err != nil {
		return nil, false, err
	}
	serversDoc["yhat"] = wantJSON

	rawDoc["mcpServers"] = mustMarshalJSON(serversDoc)
	merged, err := json.Marshal(rawDoc)
	if err != nil {
		return nil, false, err
	}

	return merged, true, nil
}

// ─── File operations ──────────────────────────────────────────────────────────

// ReadConfigFile reads a config file with a size limit.
// ENOENT → returns (nil, nil) so callers can distinguish absent from invalid.
// Empty file → returns ([]byte{}, nil) so callers can distinguish empty from absent.
// Non-ENOENT errors propagate. Rejects symlinks, directories, and non-regular files.
func ReadConfigFile(path string, maxBytes int) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // absent → nil, nil
		}
		return nil, fmt.Errorf("stat: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("refusing to read symlink")
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("refusing to read non-regular file")
	}
	if info.Size() > int64(maxBytes) {
		return nil, fmt.Errorf("file too large (>%d bytes)", maxBytes)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	lr := io.LimitReader(f, int64(maxBytes)+1)
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	if len(data) > maxBytes {
		return nil, fmt.Errorf("file too large (>%d bytes)", maxBytes)
	}

	// Empty file → return empty non-nil slice (invalid existing).
	// nil → file does not exist (absent).
	return data, nil
}

// SafeWriteConfig writes newContent to cfgPath atomically.
// origBytes and origExisted describe the state read at registration time.
// For existing targets: re-reads, compares, backs up original, and renames a temp file.
// For absent targets: uses O_EXCL to create the file exclusively; on failure removes
// only the file this branch created, never a pre-existing one.
// Rejects symlink targets. Preserves original file permissions.
func SafeWriteConfig(cfgPath string, origBytes []byte, origExisted bool, newContent []byte) (backup string, err error) {
	// Re-read current state via bounded ReadConfigFile.
	// ENOENT is expected when origExisted is false.
	current, readErr := ReadConfigFile(cfgPath, 2<<20)
	if readErr != nil {
		// Non-ENOENT errors fail the write (symlink, non-regular, etc.).
		return "", fmt.Errorf("re-reading config for comparison: %w", readErr)
	}

	// Compare: absent vs present must match what we expected.
	if origExisted {
		if current == nil {
			return "", errors.New("config disappeared during write preparation")
		}
		if string(current) != string(origBytes) {
			return "", errors.New("config was modified externally during write; original preserved")
		}
	} else {
		// origExisted == false: file should still be absent.
		if current != nil {
			return "", errors.New("config created externally during write; refusing to overwrite")
		}
	}

	// Re-check target is a regular file (not symlink, not directory) before writing.
	info, statErr := os.Lstat(cfgPath)
	if statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("refusing to write over symlink target")
		}
		if !info.Mode().IsRegular() {
			return "", errors.New("refusing to write to non-regular file")
		}
	} else if !os.IsNotExist(statErr) {
		return "", fmt.Errorf("stat target: %w", statErr)
	}

	// ── Branch: file already existed → temp + backup + rename ──────────────────
	if origExisted {
		// Create backup of original if it existed.
		if len(origBytes) > 0 {
			backup, err = createBackup(cfgPath, origBytes)
			if err != nil {
				return "", fmt.Errorf("creating backup: %w", err)
			}
		}

		tmpDir := filepath.Dir(cfgPath)
		tmpFile, err := os.CreateTemp(tmpDir, ".mcp-write-*")
		if err != nil {
			return backup, fmt.Errorf("creating temp file: %w", err)
		}
		tmpPath := tmpFile.Name()
		removeTmp := func() { os.Remove(tmpPath) } // best-effort cleanup on failure

		// Write, sync, close.
		if _, wErr := tmpFile.Write(newContent); wErr != nil {
			tmpFile.Close()
			removeTmp()
			return backup, fmt.Errorf("writing temp: %w", wErr)
		}
		if sErr := tmpFile.Sync(); sErr != nil {
			tmpFile.Close()
			removeTmp()
			return backup, fmt.Errorf("syncing temp: %w", sErr)
		}
		if cErr := tmpFile.Close(); cErr != nil {
			removeTmp()
			return backup, fmt.Errorf("closing temp: %w", cErr)
		}

		// Preserve original permissions.
		if len(origBytes) > 0 && info != nil {
			if pErr := os.Chmod(tmpPath, info.Mode().Perm()); pErr != nil {
				removeTmp()
				return backup, fmt.Errorf("setting temp permissions: %w", pErr)
			}
		}

		// Re-read immediately before rename to catch final external edits.
		fresh, freshErr := ReadConfigFile(cfgPath, 2<<20)
		if freshErr != nil {
			removeTmp()
			return backup, fmt.Errorf("pre-commit re-read: %w", freshErr)
		}
		if fresh == nil || string(fresh) != string(origBytes) {
			removeTmp()
			return backup, errors.New("config modified externally during write; original preserved")
		}

		// Commit: rename temp to target. POSIX rename atomically replaces dst.
		if rErr := os.Rename(tmpPath, cfgPath); rErr != nil {
			removeTmp()
			return backup, fmt.Errorf("renaming temp to target: %w", rErr)
		}
		return backup, nil
	}

	// ── Branch: file absent → exclusive creation (no temp, no backup) ────────────
	// Re-read one final time: file must still be absent.
	fresh, freshErr := ReadConfigFile(cfgPath, 2<<20)
	if freshErr != nil {
		return "", fmt.Errorf("pre-create re-read: %w", freshErr)
	}
	if fresh != nil {
		return "", errors.New("config created externally during write; refusing to overwrite")
	}

	// O_EXCL fails if the file already exists (atomic create-or-fail).
	f, openErr := os.OpenFile(cfgPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if openErr != nil {
		return "", fmt.Errorf("exclusive create: %w", openErr)
	}

	// Write, sync, close; on failure remove only the file we just created.
	if _, wErr := f.Write(newContent); wErr != nil {
		f.Close()
		os.Remove(cfgPath) // safe: we just created this file
		return "", fmt.Errorf("writing config: %w", wErr)
	}
	if sErr := f.Sync(); sErr != nil {
		f.Close()
		os.Remove(cfgPath)
		return "", fmt.Errorf("syncing config: %w", sErr)
	}
	if cErr := f.Close(); cErr != nil {
		os.Remove(cfgPath)
		return "", fmt.Errorf("closing config: %w", cErr)
	}

	return "", nil
}

// createBackup writes the original bytes to a unique backup file beside cfgPath.
// Uses os.CreateTemp for unique naming, writes with sync, closes with checked error.
func createBackup(cfgPath string, content []byte) (string, error) {
	dir := filepath.Dir(cfgPath)
	base := filepath.Base(cfgPath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	// Try up to 10 unique names using timestamp, then crypto/rand fallback.
	var backupPath string
	for attempt := 0; attempt < 10; attempt++ {
		suffix := fmt.Sprintf(".%s.backup.%d", name, time.Now().UnixNano())
		candidate := filepath.Join(dir, suffix+ext)
		f, err := os.OpenFile(candidate, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			if os.IsExist(err) {
				continue // try again
			}
			return "", err
		}
		backupPath = candidate

		if _, wErr := f.Write(content); wErr != nil {
			f.Close()
			os.Remove(candidate)
			return "", wErr
		}
		if sErr := f.Sync(); sErr != nil {
			f.Close()
			os.Remove(candidate)
			return "", sErr
		}
		if cErr := f.Close(); cErr != nil {
			os.Remove(candidate)
			return "", cErr
		}
		return backupPath, nil
	}

	// Fallback: use crypto/rand for truly unique suffix.
	randBytes := make([]byte, 8)
	if _, rErr := rand.Read(randBytes); rErr != nil {
		return "", rErr
	}
	suffix := fmt.Sprintf(".%s.backup.%x", name, randBytes)
	backupPath = filepath.Join(dir, suffix+ext)
	f, err := os.OpenFile(backupPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	if _, wErr := f.Write(content); wErr != nil {
		f.Close()
		os.Remove(backupPath)
		return "", wErr
	}
	if sErr := f.Sync(); sErr != nil {
		f.Close()
		os.Remove(backupPath)
		return "", sErr
	}
	if cErr := f.Close(); cErr != nil {
		os.Remove(backupPath)
		return "", cErr
	}
	return backupPath, nil
}

// ─── Duplicate key validation ────────────────────────────────────────────────

// hasDuplicateKeys returns true if s contains duplicate keys within any
// JSON object at any nesting level. It uses recursive token parsing with
// object-local key sets and array recursion, UseNumber for numeric precision,
// and proper EOF checking. Invalid JSON returns false (caller handles).
func hasDuplicateKeys(s string) bool {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	return scanForDuplicates(dec)
}

// scanForDuplicates consumes one complete JSON value from dec and returns
// true if any object within it has duplicate keys. It uses true recursive descent:
// each object gets its own key set; arrays recursively consume all elements.
func scanForDuplicates(dec *json.Decoder) bool {
	// Consume one token (could be primitive or delimiter).
	tok, err := dec.Token()
	if err != nil {
		return false
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		// Primitive: no object keys possible here.
		return false
	}

	switch delim {
	case '{':
		// Fresh key set for this object scope.
		keys := map[string]bool{}
		for dec.More() {
			// Read key string.
			keyTok, keyErr := dec.Token()
			if keyErr != nil {
				return false
			}
			key, ok := keyTok.(string)
			if !ok {
				return false
			}
			if keys[key] {
				return true // duplicate at this level
			}
			keys[key] = true

			// Recursively consume the complete VALUE.
			if scanForDuplicates(dec) {
				return true
			}
		}
		// Consume closing '}'.
		dec.Token()
		return false

	case '[':
		// Recursively consume all array elements.
		for dec.More() {
			if scanForDuplicates(dec) {
				return true
			}
		}
		// Consume closing ']'.
		dec.Token()
		return false

	default:
		return false
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func mustMarshalJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
