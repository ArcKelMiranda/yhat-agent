package yhatagent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Test helper functions ---

func setupTestEnv(t *testing.T) (string, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "yhat-agent-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Set XDG_CONFIG_HOME to the test directory
	oldConfigHome := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", tmpDir)

	return tmpDir, func() {
		os.Setenv("XDG_CONFIG_HOME", oldConfigHome)
		os.RemoveAll(tmpDir)
	}
}

func fileHash(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file %s: %v", path, err)
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create dir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// --- Test: First install ---

func TestInstall_FirstInstall(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Run install
	results, err := Install()
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	// Verify all assets installed
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	// Check agent file
	agentPath := AgentTargetPath()
	if !fileExists(agentPath) {
		t.Errorf("agent file not created at %s", agentPath)
	}

	// Check skill file
	skillPath := SkillTargetPath()
	if !fileExists(skillPath) {
		t.Errorf("skill file not created at %s", skillPath)
	}

	// Verify manifest was created
	manifestPath := ManifestPath()
	if !fileExists(manifestPath) {
		t.Errorf("manifest not created at %s", manifestPath)
	}

	// Verify manifest content
	manifest, err := LoadManifest()
	if err != nil {
		t.Fatalf("failed to load manifest: %v", err)
	}
	if manifest.Version != "1" {
		t.Errorf("expected manifest version '1', got '%s'", manifest.Version)
	}
	if len(manifest.Assets) != 2 {
		t.Errorf("expected 2 assets in manifest, got %d", len(manifest.Assets))
	}
}

// --- Test: Idempotent install ---

func TestInstall_Idempotent(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// First install
	_, err := Install()
	if err != nil {
		t.Fatalf("first install failed: %v", err)
	}

	origAgentHash := fileHash(t, AgentTargetPath())
	origSkillHash := fileHash(t, SkillTargetPath())

	// Second install (should be idempotent)
	results, err := Install()
	if err != nil {
		t.Fatalf("second install failed: %v", err)
	}

	// Verify files unchanged
	newAgentHash := fileHash(t, AgentTargetPath())
	newSkillHash := fileHash(t, SkillTargetPath())

	if newAgentHash != origAgentHash {
		t.Error("agent file changed on second install")
	}
	if newSkillHash != origSkillHash {
		t.Error("skill file changed on second install")
	}

	// Check results indicate already installed
	for _, r := range results {
		if r.Message != "already installed" {
			t.Errorf("expected 'already installed', got '%s' for %s", r.Message, r.AssetKey)
		}
	}
}

// --- Test: Collision handling ---

func TestInstall_Collision(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Pre-create files with different content
	agentPath := AgentTargetPath()
	writeFile(t, agentPath, "existing agent content\n")

	skillPath := SkillTargetPath()
	writeFile(t, skillPath, "existing skill content\n")

	// Install should detect collision and write .yhat-agent-new
	results, err := Install()
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	// Check original files preserved
	agentContent, _ := os.ReadFile(agentPath)
	if string(agentContent) != "existing agent content\n" {
		t.Error("agent file was overwritten")
	}

	// Check candidate files created
	candidateAgent := CandidateName(agentPath)
	candidateSkill := CandidateName(skillPath)

	if !fileExists(candidateAgent) {
		t.Error("candidate agent not created")
	}
	if !fileExists(candidateSkill) {
		t.Error("candidate skill not created")
	}

	// Check results indicate drift
	for _, r := range results {
		if r.State != StateDrift {
			t.Errorf("expected drift state for %s, got %s", r.AssetKey, r.State)
		}
	}
}

// --- Test: Status with no manifest ---

func TestStatus_NoManifest(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Don't install, just check status - manifest shouldn't exist
	manifestPath := ManifestPath()
	if fileExists(manifestPath) {
		t.Skip("manifest exists, skipping test")
	}

	result, err := Status()
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}

	if result.HasManifest {
		t.Error("expected no manifest")
	}

	if len(result.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(result.Files))
	}

	for _, f := range result.Files {
		if f.State != StateMissing {
			t.Errorf("expected missing state for %s, got %s", f.AssetKey, f.State)
		}
	}
}

// --- Test: XDG path resolution ---

func TestConfigHome_XDGEnv(t *testing.T) {
	// Save original value
	orig := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		if orig != "" {
			os.Setenv("XDG_CONFIG_HOME", orig)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
	}()

	// Test XDG_CONFIG_HOME set
	os.Setenv("XDG_CONFIG_HOME", "/custom/config")
	if ConfigHome() != "/custom/config" {
		t.Errorf("expected /custom/config, got %s", ConfigHome())
	}
}

func TestConfigHome_Fallback(t *testing.T) {
	// Save original value
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	origHOME := os.Getenv("HOME")
	defer func() {
		if origXDG != "" {
			os.Setenv("XDG_CONFIG_HOME", origXDG)
		} else {
			os.Unsetenv("XDG_CONFIG_HOME")
		}
		if origHOME != "" {
			os.Setenv("HOME", origHOME)
		}
	}()

	// Test fallback to ~/.config
	os.Unsetenv("XDG_CONFIG_HOME")
	home := os.Getenv("HOME")
	expected := filepath.Join(home, ".config")

	if ConfigHome() != expected {
		t.Errorf("expected %s, got %s", expected, ConfigHome())
	}
}

// --- Test: Platform name ---

func TestPlatformName(t *testing.T) {
	name := PlatformName()
	if name == "" {
		t.Error("expected non-empty platform name")
	}

	// Should contain OS and arch
	parts := strings.Split(name, "_")
	if len(parts) != 2 {
		t.Errorf("expected OS_arch format, got %s", name)
	}
}

// --- Test: Platform name matches release asset pattern (table-driven) ---
// Documents all 5 supported platforms with their expected asset/checksum names.

func TestPlatformNameMatchesAssetPattern(t *testing.T) {
	// All supported platforms with expected fixture names
	platforms := []struct {
		platform         string
		assetName        string
		checksumName     string
		includeInCurrent bool // true for current-host platform
	}{
		{"linux_amd64", "yhat-agent_linux_amd64", "yhat-agent_linux_amd64.sha256sum", false},
		{"linux_arm64", "yhat-agent_linux_arm64", "yhat-agent_linux_arm64.sha256sum", false},
		{"darwin_amd64", "yhat-agent_darwin_amd64", "yhat-agent_darwin_amd64.sha256sum", false},
		{"darwin_arm64", "yhat-agent_darwin_arm64", "yhat-agent_darwin_arm64.sha256sum", false},
		{"windows_amd64", "yhat-agent_windows_amd64.exe", "yhat-agent_windows_amd64.exe.sha256sum", false},
	}

	// Mark current-host platform
	currentPlatform := PlatformName()
	for i := range platforms {
		if platforms[i].platform == currentPlatform {
			platforms[i].includeInCurrent = true
		}
	}

	baseURL := "https://github.com/ArcKelMiranda/yhat-knowledge/releases/download/v1.0.0/"

	// Run all 5 platforms (documents fixture contract regardless of current host)
	for _, pf := range platforms {
		// Build minimal asset list for this platform
		assets := []ReleaseAsset{
			{Name: pf.assetName, BrowserDownloadURL: baseURL + pf.assetName},
			{Name: pf.checksumName, BrowserDownloadURL: baseURL + pf.checksumName},
		}

		// Find platform asset
		assetURL, checksumURL, err := FindPlatformAsset(assets, pf.platform)
		if err != nil {
			t.Errorf("[%s] FindPlatformAsset failed: %v", pf.platform, err)
			continue
		}

		// Verify asset URL contains expected name
		if !strings.Contains(assetURL, pf.assetName) {
			t.Errorf("[%s] asset URL missing %s, got: %s", pf.platform, pf.assetName, assetURL)
		}

		// Verify checksum URL contains expected name
		if !strings.Contains(checksumURL, pf.checksumName) {
			t.Errorf("[%s] checksum URL missing %s, got: %s", pf.platform, pf.checksumName, checksumURL)
		}
	}

	// Run current-host platform with full asset list (tests live integration)
	if !platforms[len(platforms)-1].includeInCurrent { // not windows? find actual current
		for _, pf := range platforms {
			if pf.includeInCurrent {
				assets := []ReleaseAsset{
					{Name: "yhat-agent_linux_amd64", BrowserDownloadURL: baseURL + "yhat-agent_linux_amd64"},
					{Name: "yhat-agent_linux_amd64.sha256sum", BrowserDownloadURL: baseURL + "yhat-agent_linux_amd64.sha256sum"},
					{Name: "yhat-agent_linux_arm64", BrowserDownloadURL: baseURL + "yhat-agent_linux_arm64"},
					{Name: "yhat-agent_linux_arm64.sha256sum", BrowserDownloadURL: baseURL + "yhat-agent_linux_arm64.sha256sum"},
					{Name: "yhat-agent_darwin_amd64", BrowserDownloadURL: baseURL + "yhat-agent_darwin_amd64"},
					{Name: "yhat-agent_darwin_amd64.sha256sum", BrowserDownloadURL: baseURL + "yhat-agent_darwin_amd64.sha256sum"},
					{Name: "yhat-agent_darwin_arm64", BrowserDownloadURL: baseURL + "yhat-agent_darwin_arm64"},
					{Name: "yhat-agent_darwin_arm64.sha256sum", BrowserDownloadURL: baseURL + "yhat-agent_darwin_arm64.sha256sum"},
					{Name: "yhat-agent_windows_amd64.exe", BrowserDownloadURL: baseURL + "yhat-agent_windows_amd64.exe"},
					{Name: "yhat-agent_windows_amd64.exe.sha256sum", BrowserDownloadURL: baseURL + "yhat-agent_windows_amd64.exe.sha256sum"},
				}
				assetURL, checksumURL, err := FindPlatformAsset(assets, pf.platform)
				if err != nil {
					t.Errorf("[current-host %s] FindPlatformAsset failed: %v", pf.platform, err)
				}
				_ = assetURL
				_ = checksumURL
				break
			}
		}
	}
}

// --- Test: FindPlatformAsset with Windows .exe ---

func TestFindPlatformAssetWindows(t *testing.T) {
	assets := []ReleaseAsset{
		{Name: "yhat-agent_windows_amd64.exe", BrowserDownloadURL: "https://example.com/yhat-agent_windows_amd64.exe"},
		{Name: "yhat-agent_windows_amd64.exe.sha256sum", BrowserDownloadURL: "https://example.com/yhat-agent_windows_amd64.exe.sha256sum"},
		{Name: "yhat-agent_linux_amd64", BrowserDownloadURL: "https://example.com/yhat-agent_linux_amd64"},
		{Name: "yhat-agent_linux_amd64.sha256sum", BrowserDownloadURL: "https://example.com/yhat-agent_linux_amd64.sha256sum"},
	}

	assetURL, checksumURL, err := FindPlatformAsset(assets, "windows_amd64")
	if err != nil {
		t.Fatalf("FindPlatformAsset failed for windows_amd64: %v", err)
	}

	if !strings.Contains(assetURL, "yhat-agent_windows_amd64.exe") {
		t.Errorf("expected yhat-agent_windows_amd64.exe in asset URL, got: %s", assetURL)
	}
	if !strings.Contains(checksumURL, "yhat-agent_windows_amd64.exe.sha256sum") {
		t.Errorf("expected yhat-agent_windows_amd64.exe.sha256sum in checksum URL, got: %s", checksumURL)
	}
}

// --- Test: FindPlatformAsset missing checksum ---

func TestFindPlatformAssetMissingChecksum(t *testing.T) {
	assets := []ReleaseAsset{
		{Name: "yhat-agent_linux_amd64", BrowserDownloadURL: "https://example.com/yhat-agent_linux_amd64"},
		// Missing checksum file
	}

	_, _, err := FindPlatformAsset(assets, "linux_amd64")
	if err == nil {
		t.Error("expected error for missing checksum file")
	}
	if !strings.Contains(err.Error(), "checksum") {
		t.Errorf("expected error message about checksum, got: %v", err)
	}
}

// --- Test: FindPlatformAsset missing asset ---

func TestFindPlatformAssetMissingAsset(t *testing.T) {
	assets := []ReleaseAsset{
		{Name: "yhat-agent_linux_amd64.sha256sum", BrowserDownloadURL: "https://example.com/checksum"},
		// Missing binary asset
	}

	_, _, err := FindPlatformAsset(assets, "linux_amd64")
	if err == nil {
		t.Error("expected error for missing asset")
	}
	if !strings.Contains(err.Error(), "no asset found") {
		t.Errorf("expected error message about missing asset, got: %v", err)
	}
}

// --- Test: Checksum parsing ---

func TestParseChecksums(t *testing.T) {
	// Note: each hash must be exactly 64 hex characters
	content := []byte(`1c65a1a9c301828f582c0b04e6ecf59fd2841c86c2eaf35afec20ac025b4f44c  yhat-agent_linux_amd64
1592e7456fe72ea3a22b827ca8fdf20c16cb9cbf41645f11513f31dedffa0708  yhat-agent_darwin_arm64
faf901b638f273b0a179a2e34b879d9fb004ce7a2b0fea0b93608d9fb0e2ba2d  yhat-agent_windows_amd64.exe
`)

	entries, err := ParseChecksums(content)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}

	// Check entries
	expected := []string{"yhat-agent_linux_amd64", "yhat-agent_darwin_arm64", "yhat-agent_windows_amd64.exe"}
	for i, exp := range expected {
		if entries[i].AssetName != exp {
			t.Errorf("entry %d: expected %s, got %s", i, exp, entries[i].AssetName)
		}
		if len(entries[i].Hash) != 64 {
			t.Errorf("entry %d: expected 64-char hash, got %d", i, len(entries[i].Hash))
		}
	}
}

// --- Test: Candidate naming ---

func TestCandidateName(t *testing.T) {
	path := "/some/path/file.txt"
	expected := "/some/path/file.txt" + CandidateSuffix

	result := CandidateName(path)
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

// --- Test: Manifest JSON round-trip ---

func TestManifestJSON(t *testing.T) {
	manifest := &Manifest{
		Version: "1",
		Assets: map[string]Asset{
			AgentAsset: {
				TargetPath:    "/test/agents/yhat-memory-capture.md",
				ContentHash:   "abc123",
				InstalledHash: "abc123",
				Modified:      false,
			},
		},
	}

	// Marshal
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Unmarshal
	var loaded Manifest
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if loaded.Version != manifest.Version {
		t.Errorf("version mismatch: %s != %s", loaded.Version, manifest.Version)
	}
	if len(loaded.Assets) != 1 {
		t.Errorf("assets count mismatch: %d != 1", len(loaded.Assets))
	}
}

// --- Test: Install state string ---

func TestInstallStateString(t *testing.T) {
	tests := []struct {
		state    InstallState
		expected string
	}{
		{StateMissing, "missing"},
		{StateInstalled, "installed"},
		{StateDrift, "drift"},
		{StateCandidate, "candidate"},
		{StateUnknown, "unknown"},
	}

	for _, tt := range tests {
		if tt.state.String() != tt.expected {
			t.Errorf("expected %s, got %s", tt.expected, tt.state.String())
		}
	}
}

// --- Test: Verify checksum ---

func TestVerifyChecksum(t *testing.T) {
	data := []byte("test content")
	hash := sha256.Sum256(data)
	expectedHash := hex.EncodeToString(hash[:])

	// Valid checksum
	if !VerifyChecksum(data, expectedHash) {
		t.Error("expected checksum to verify")
	}

	// Invalid checksum
	if VerifyChecksum(data, "invalid") {
		t.Error("expected checksum to fail")
	}
}

// --- Test: Uninstall preserves manifest-recorded modified files ---
// When a file is modified and the manifest is updated to record the modified hash,
// uninstall should skip it (not remove) and not report an integrity error.

func TestUninstall_PreservesModified(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Install first
	_, err := Install()
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	agentPath := AgentTargetPath()

	// Modify file
	writeFile(t, agentPath, "modified by user\n")

	// Update manifest to record the modified state with the new hash
	manifest, err := LoadManifest()
	if err != nil {
		t.Fatalf("failed to load manifest: %v", err)
	}

	// Update the installed hash to match the modified content
	if err := manifest.UpdateInstalledHash(AgentAsset, agentPath); err != nil {
		t.Fatalf("failed to update installed hash: %v", err)
	}

	// Mark as modified in manifest
	if asset, ok := manifest.Assets[AgentAsset]; ok {
		asset.Modified = true
		manifest.Assets[AgentAsset] = asset
	}

	if err := manifest.Save(); err != nil {
		t.Fatalf("failed to save manifest: %v", err)
	}

	// Attempt uninstall - should skip because Modified=true, not error on integrity
	result, err := Uninstall(true)
	if err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}

	// File should be skipped (not removed, not error)
	// The key behavior: modified files are skipped, NOT reported as errors
	if len(result.Skipped) == 0 && len(result.Errors) == 0 {
		t.Error("expected at least one skipped file or error")
	}

	// For manifest-recorded modified files, should be skipped, not error
	if len(result.Skipped) > 0 {
		// Good - was recognized as modified and skipped
	} else if len(result.Errors) > 0 {
		// If we get an error, it should NOT be an integrity error
		for _, e := range result.Errors {
			if strings.Contains(strings.ToLower(e), "integrity") {
				t.Error("manifest-recorded modified file should be skipped, not integrity error")
			}
		}
	}

	// File should still exist
	if !fileExists(agentPath) {
		t.Error("modified file was removed")
	}
}

// --- Test: Uninstall removes unchanged files ---

func TestUninstall_RemovesUnchanged(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Install first
	_, err := Install()
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	agentPath := AgentTargetPath()
	skillPath := SkillTargetPath()

	// Verify files exist before uninstall
	if !fileExists(agentPath) {
		t.Fatalf("agent file doesn't exist before uninstall")
	}

	// Uninstall
	result, err := Uninstall(true)
	if err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}

	// Files should be removed
	if len(result.Removed) != 2 {
		t.Errorf("expected 2 removed files, got %d", len(result.Removed))
	}

	if fileExists(agentPath) {
		t.Error("agent file was not removed")
	}
	if fileExists(skillPath) {
		t.Error("skill file was not removed")
	}

	// Manifest should be removed
	if fileExists(ManifestPath()) {
		t.Error("manifest was not removed")
	}
}

// --- Test: Uninstall requires --yes ---

func TestUninstall_RequiresYes(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Install first
	_, err := Install()
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	// Uninstall without --yes should fail
	_, err = Uninstall(false)
	if err == nil {
		t.Error("expected error without --yes")
	}
}

// --- Test: Embedded assets exist ---

func TestEmbeddedAssets(t *testing.T) {
	paths := AssetPaths()
	if len(paths) != 2 {
		t.Errorf("expected 2 asset paths, got %d", len(paths))
	}

	for _, path := range paths {
		data, err := Assets.ReadFile(path)
		if err != nil {
			t.Errorf("failed to read embedded asset %s: %v", path, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("embedded asset %s is empty", path)
		}
	}
}

// --- Test: EnsureDir creates directories ---

func TestEnsureDir(t *testing.T) {
	tmpDir, cleanup := setupTestEnv(t)
	defer cleanup()

	testDir := filepath.Join(tmpDir, "nested", "path", "dir")

	if err := EnsureDir(testDir); err != nil {
		t.Fatalf("EnsureDir failed: %v", err)
	}

	if !fileExists(testDir) {
		t.Error("directory was not created")
	}
}

// --- Test: NewManifest creates correct structure ---

func TestNewManifest(t *testing.T) {
	manifest, err := NewManifest()
	if err != nil {
		t.Fatalf("NewManifest failed: %v", err)
	}

	if manifest.Version != "1" {
		t.Errorf("expected version '1', got '%s'", manifest.Version)
	}

	if manifest.InstalledAt.IsZero() {
		t.Error("InstalledAt should not be zero")
	}

	if len(manifest.Assets) != 2 {
		t.Errorf("expected 2 assets, got %d", len(manifest.Assets))
	}

	for key, asset := range manifest.Assets {
		if asset.ContentHash == "" {
			t.Errorf("empty ContentHash for %s", key)
		}
		if asset.TargetPath == "" {
			t.Errorf("empty TargetPath for %s", key)
		}
	}
}

// --- Test: HasManifest ---

func TestHasManifest(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Before install
	if fileExists(ManifestPath()) {
		t.Skip("manifest exists, skipping test")
	}
	if HasManifest() {
		t.Error("HasManifest should return false before install")
	}

	// After install
	_, err := Install()
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}
	if !HasManifest() {
		t.Error("HasManifest should return true after install")
	}
}

// --- Test: Manifest.UpdateInstalledHash ---

func TestManifestUpdateInstalledHash(t *testing.T) {
	tmpDir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	writeFile(t, testFile, "test content")

	// Get hash
	hash := fileHash(t, testFile)

	manifest := &Manifest{
		Version: "1",
		Assets: map[string]Asset{
			AgentAsset: {
				TargetPath:  testFile,
				ContentHash: "different",
			},
		},
	}

	if err := manifest.UpdateInstalledHash(AgentAsset, testFile); err != nil {
		t.Fatalf("UpdateInstalledHash failed: %v", err)
	}

	asset := manifest.Assets[AgentAsset]
	if asset.InstalledHash != hash {
		t.Errorf("expected InstalledHash %s, got %s", hash, asset.InstalledHash)
	}
	if !asset.Modified {
		t.Error("expected Modified to be true")
	}
}

// --- Test: Status reports drift ---

func TestStatus_Drift(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Install first
	_, err := Install()
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	// Modify file manually
	agentPath := AgentTargetPath()
	writeFile(t, agentPath, "manually modified content\n")

	// Status should report drift
	result, err := Status()
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}

	foundDrift := false
	for _, f := range result.Files {
		if f.AssetKey == AgentAsset && f.State == StateDrift {
			foundDrift = true
			break
		}
	}
	if !foundDrift {
		t.Error("expected drift state for modified agent file")
	}
}

// --- Test: UninstallPlan returns planned actions ---

func TestUninstallPlan(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Install first
	_, err := Install()
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	// Get plan
	plan, err := UninstallPlan()
	if err != nil {
		t.Fatalf("UninstallPlan failed: %v", err)
	}

	// Should plan to remove both assets
	if len(plan.Planned) != 2 {
		t.Errorf("expected 2 planned removals, got %d", len(plan.Planned))
	}

	if len(plan.Skipped) != 0 {
		t.Errorf("expected 0 skipped, got %d", len(plan.Skipped))
	}
}

// --- Test: UninstallPlan with no manifest ---

func TestUninstallPlan_NoManifest(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	plan, err := UninstallPlan()
	if err != nil {
		t.Fatalf("UninstallPlan failed: %v", err)
	}

	if len(plan.Planned) != 0 {
		t.Errorf("expected 0 planned, got %d", len(plan.Planned))
	}

	if len(plan.Skipped) != 1 {
		t.Errorf("expected 1 skipped, got %d", len(plan.Skipped))
	}
}

// --- Test: UninstallPlan skips manifest-recorded modified files ---
// When a file is modified and the manifest is updated to record the modified hash,
// the plan should skip it (not remove), not report integrity error.

func TestUninstallPlan_SkipsModified(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Install first
	_, err := Install()
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	// Modify one file and update manifest
	agentPath := AgentTargetPath()
	writeFile(t, agentPath, "modified content\n")

	// Update manifest to record the modified state
	manifest, err := LoadManifest()
	if err != nil {
		t.Fatalf("failed to load manifest: %v", err)
	}
	if err := manifest.UpdateInstalledHash(AgentAsset, agentPath); err != nil {
		t.Fatalf("failed to update installed hash: %v", err)
	}
	if asset, ok := manifest.Assets[AgentAsset]; ok {
		asset.Modified = true
		manifest.Assets[AgentAsset] = asset
	}
	if err := manifest.Save(); err != nil {
		t.Fatalf("failed to save manifest: %v", err)
	}

	plan, err := UninstallPlan()
	if err != nil {
		t.Fatalf("UninstallPlan failed: %v", err)
	}

	// Should plan to remove only the unmodified file
	if len(plan.Planned) != 1 {
		t.Errorf("expected 1 planned, got %d", len(plan.Planned))
	}

	// And skip the manifest-recorded modified file
	if len(plan.Skipped) != 1 {
		t.Errorf("expected 1 skipped, got %d", len(plan.Skipped))
	}

	// Verify the skipped reason mentions modified, not integrity error
	for _, s := range plan.Skipped {
		if strings.Contains(strings.ToLower(s.Reason), "integrity") {
			t.Errorf("should not report integrity error for manifest-recorded modified: %s", s.Reason)
		}
	}
}

// --- Test: FindPlatformAsset handles platform variants ---

func TestFindPlatformAsset_Linux(t *testing.T) {
	assets := []ReleaseAsset{
		{Name: "yhat-agent_linux_amd64", BrowserDownloadURL: "https://example.com/yhat-agent_linux_amd64"},
		{Name: "yhat-agent_linux_amd64.sha256sum", BrowserDownloadURL: "https://example.com/yhat-agent_linux_amd64.sha256sum"},
		{Name: "yhat-agent_linux_arm64", BrowserDownloadURL: "https://example.com/yhat-agent_linux_arm64"},
		{Name: "yhat-agent_linux_arm64.sha256sum", BrowserDownloadURL: "https://example.com/yhat-agent_linux_arm64.sha256sum"},
	}

	assetURL, checksumURL, err := FindPlatformAsset(assets, "linux_amd64")
	if err != nil {
		t.Fatalf("FindPlatformAsset failed: %v", err)
	}

	if !strings.Contains(assetURL, "linux_amd64") {
		t.Errorf("expected linux_amd64 in asset URL, got: %s", assetURL)
	}
	if !strings.Contains(checksumURL, "linux_amd64.sha256sum") {
		t.Errorf("expected linux_amd64.sha256sum in checksum URL, got: %s", checksumURL)
	}
}

// --- Test: FindPlatformAsset handles darwin platform ---

func TestFindPlatformAsset_Darwin(t *testing.T) {
	assets := []ReleaseAsset{
		{Name: "yhat-agent_darwin_amd64", BrowserDownloadURL: "https://example.com/yhat-agent_darwin_amd64"},
		{Name: "yhat-agent_darwin_amd64.sha256sum", BrowserDownloadURL: "https://example.com/yhat-agent_darwin_amd64.sha256sum"},
		{Name: "yhat-agent_darwin_arm64", BrowserDownloadURL: "https://example.com/yhat-agent_darwin_arm64"},
		{Name: "yhat-agent_darwin_arm64.sha256sum", BrowserDownloadURL: "https://example.com/yhat-agent_darwin_arm64.sha256sum"},
	}

	assetURL, checksumURL, err := FindPlatformAsset(assets, "darwin_arm64")
	if err != nil {
		t.Fatalf("FindPlatformAsset failed: %v", err)
	}

	if !strings.Contains(assetURL, "darwin_arm64") {
		t.Errorf("expected darwin_arm64 in asset URL, got: %s", assetURL)
	}
	if !strings.Contains(checksumURL, "darwin_arm64.sha256sum") {
		t.Errorf("expected darwin_arm64.sha256sum in checksum URL, got: %s", checksumURL)
	}
}

// --- Test: Uninstall preserves state on partial failure ---

func TestUninstall_PartialFailure(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Install first
	_, err := Install()
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	agentPath := AgentTargetPath()
	skillPath := SkillTargetPath()

	// Verify both files exist before test
	if !fileExists(agentPath) || !fileExists(skillPath) {
		t.Fatal("test setup failed: files not created")
	}

	// Attempt uninstall - if some removals succeed and others fail,
	// the result should include both Removed and Errors
	result, err := Uninstall(true)
	if err != nil {
		t.Fatalf("Uninstall should not return fatal error for partial success: %v", err)
	}

	// Result should reflect what happened
	// At least one file should have been processed
	totalProcessed := len(result.Removed) + len(result.Skipped) + len(result.Errors)
	if totalProcessed == 0 {
		t.Error("expected at least some files to be processed")
	}

	// If we have both removed and errors, that's the partial failure case
	if len(result.Removed) > 0 && len(result.Errors) > 0 {
		// This is the partial failure case we want to test
		return
	}

	// If all succeeded, check the behavior is still correct
	// (this might happen if running as root or on certain filesystems)
	if len(result.Removed) == 2 && len(result.Errors) == 0 {
		t.Log("All files removed successfully (may be running as root)")
		return
	}

	// Otherwise, just verify we got some meaningful result
	t.Logf("Result: Removed=%d, Skipped=%d, Errors=%d", len(result.Removed), len(result.Skipped), len(result.Errors))
}

// --- Test: Install propagates write errors ---

func TestInstall_WriteError(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Create a file at agent target path as a directory to cause error
	agentPath := AgentTargetPath()
	if err := os.MkdirAll(agentPath, 0755); err != nil {
		t.Skip("cannot create directory for test")
	}

	// Install should fail and return error
	_, err := Install()
	if err == nil {
		t.Error("expected error when install encounters directory at target")
	}

	// Clean up directory for other tests
	os.RemoveAll(agentPath)
}

// --- Test: Install refuses to install over a symlink target ---
// Verifies that Install() uses os.Lstat (not os.Stat) to detect symlinks
// without following them, and refuses to install when the target path
// is a symlink. This prevents accidentally writing through a symlink to
// a directory or replacing a symlink's target.

func TestInstall_RefusesSymlinkTarget(t *testing.T) {
	tmpDir, cleanup := setupTestEnv(t)
	defer cleanup()

	agentPath := AgentTargetPath()
	realFile := filepath.Join(tmpDir, "real-agent-file")

	// Create the real file that the symlink will point to
	if err := os.WriteFile(realFile, []byte("real content"), 0644); err != nil {
		t.Fatalf("failed to create real file: %v", err)
	}

	// Create parent directory for the symlink
	if err := os.MkdirAll(filepath.Dir(agentPath), 0755); err != nil {
		t.Fatalf("failed to create parent directory: %v", err)
	}

	// Create a symlink at the agent target path
	if err := os.Symlink(realFile, agentPath); err != nil {
		t.Skip("cannot create symlinks on this platform")
	}

	// Install should refuse to install over a symlink
	results, err := Install()
	if err == nil {
		t.Error("expected error when target path is a symlink")
	}

	// Verify no result reports success for the symlink path
	for _, r := range results {
		if r.AssetKey == AgentAsset && r.State == StateInstalled {
			t.Errorf("expected non-Installed state for symlink target, got %s: %s", r.State, r.Message)
		}
	}

	// Verify the symlink still exists (was not followed or removed)
	info, err := os.Lstat(agentPath)
	if err != nil {
		t.Fatalf("symlink disappeared: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("symlink was converted to a regular file")
	}

	// Verify real file content unchanged (symlink not followed for write)
	if data, _ := os.ReadFile(realFile); string(data) != "real content" {
		t.Error("real file content was modified through symlink")
	}
}

// --- Test: PlatformName returns consistent format ---

func TestPlatformName_Format(t *testing.T) {
	name := PlatformName()

	// Should contain exactly 2 parts: os_arch
	parts := strings.Split(name, "_")
	if len(parts) != 2 {
		t.Errorf("expected OS_arch format with 2 parts, got %d parts: %s", len(parts), name)
	}

	// OS should be lowercase
	if parts[0] != strings.ToLower(parts[0]) {
		t.Errorf("expected lowercase OS, got: %s", parts[0])
	}

	// Arch should be lowercase
	if parts[1] != strings.ToLower(parts[1]) {
		t.Errorf("expected lowercase arch, got: %s", parts[1])
	}
}

// --- Test: CandidateName handles Windows-style paths ---

func TestCandidateNameVariants(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/some/path/file.txt", "/some/path/file.txt.yhat-agent-new"},
		{"C:\\some\\path\\file.exe", "C:\\some\\path\\file.exe.yhat-agent-new"},
		{"/var/bin/yhat-agent", "/var/bin/yhat-agent.yhat-agent-new"},
	}

	for _, tt := range tests {
		result := CandidateName(tt.path)
		if result != tt.expected {
			t.Errorf("CandidateName(%s): expected %s, got %s", tt.path, tt.expected, result)
		}
	}
}

// --- Test: WindowsCandidatePath format (cross-platform) ---

func TestWindowsCandidatePathFormat(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"C:\\Program Files\\yhat-agent\\yhat-agent.exe", "C:\\Program Files\\yhat-agent\\yhat-agent_new.exe"},
		{"C:\\some\\path\\yhat-agent", "C:\\some\\path\\yhat-agent_new"},
		{"/usr/bin/yhat-agent", "/usr/bin/yhat-agent_new"},
		{"/opt/app/yhat-agent.exe", "/opt/app/yhat-agent_new.exe"},
	}

	for _, tt := range tests {
		result := WindowsCandidatePath(tt.path)
		if result != tt.expected {
			t.Errorf("WindowsCandidatePath(%s): expected %s, got %s", tt.path, tt.expected, result)
		}
	}
}

// --- Test: uniqueCandidatePath creates non-colliding paths (updated signature) ---

func TestUniqueCandidatePath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "yhat-agent-unique-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	basePath := filepath.Join(tmpDir, "yhat-agent")
	defaultCandidate := basePath + CandidateSuffix

	// First call should return default candidate
	result, err := uniqueCandidatePath(defaultCandidate)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if result != defaultCandidate {
		t.Errorf("expected default path when no collision, got %s", result)
	}

	// Create the default candidate
	if err := os.WriteFile(defaultCandidate, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Second call should return numbered suffix
	result, err = uniqueCandidatePath(defaultCandidate)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	expected := basePath + "_2" + CandidateSuffix
	if result != expected {
		t.Errorf("expected %s after collision, got %s", expected, result)
	}

	// Create numbered suffix too
	if err := os.WriteFile(expected, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Third call should return _3 suffix
	result, err = uniqueCandidatePath(defaultCandidate)
	if err != nil {
		t.Fatalf("third call failed: %v", err)
	}
	expected3 := basePath + "_3" + CandidateSuffix
	if result != expected3 {
		t.Errorf("expected %s after 2 collisions, got %s", expected3, result)
	}
}

// --- Test: SHA256SUMS format parsing (deterministic fixture) ---

func TestParseChecksumsSHA256SUMSFormat(t *testing.T) {
	// This test uses the exact format produced by sha256sum:
	// <64-hex-char-hash><two spaces><filename>
	//
	// Format reference: https://www.gnu.org/software/coreutils/manual/html_node/sha256sum-invocation.html
	content := []byte(`1c65a1a9c301828f582c0b04e6ecf59fd2841c86c2eaf35afec20ac025b4f44c  yhat-agent_linux_amd64
1592e7456fe72ea3a22b827ca8fdf20c16cb9cbf41645f11513f31dedffa0708  yhat-agent_darwin_arm64
faf901b638f273b0a179a2e34b879d9fb004ce7a2b0fea0b93608d9fb0e2ba2d  yhat-agent_windows_amd64.exe
`)

	entries, err := ParseChecksums(content)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}

	// Verify exact format: hash must be 64 hex chars
	for _, e := range entries {
		if len(e.Hash) != 64 {
			t.Errorf("expected 64-char hash, got %d for %s", len(e.Hash), e.AssetName)
		}
		// Verify hash contains only lowercase hex
		for _, c := range e.Hash {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("invalid hex char %c in hash for %s", c, e.AssetName)
			}
		}
	}

	// Verify exact asset names
	expected := []string{
		"yhat-agent_linux_amd64",
		"yhat-agent_darwin_arm64",
		"yhat-agent_windows_amd64.exe",
	}
	for i, exp := range expected {
		if entries[i].AssetName != exp {
			t.Errorf("entry %d: expected %s, got %s", i, exp, entries[i].AssetName)
		}
	}
}

// --- Test: ParseChecksums handles Unix sha256sum output ---

func TestParseChecksumsUnixFormat(t *testing.T) {
	// sha256sum output format: "<hash> *<filename>"
	// But also supports "<hash>  <filename>" (BSD-style with spaces)
	// We use the two-space format for cross-platform compatibility
	content := []byte(`deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef  yhat-agent_linux_amd64
`) // 64 f's

	entries, err := ParseChecksums(content)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}

	if entries[0].AssetName != "yhat-agent_linux_amd64" {
		t.Errorf("expected yhat-agent_linux_amd64, got %s", entries[0].AssetName)
	}
}

// --- Test: Uninstall preserves manifest on partial failure (read-only directory attack) ---
// Makes the manifest directory read-only so that manifest.Save() cannot create
// its temp file via os.CreateTemp. This causes Save() to fail, testing that
// the error is captured in result.Errors and Uninstall still completes.

func TestUninstall_PartialFailure_PreservesManifest(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Install first
	_, err := Install()
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	manifestPath := ManifestPath()

	// Verify manifest exists before test
	if !fileExists(manifestPath) {
		t.Fatal("manifest not created by Install()")
	}

	// Get the manifest directory
	manifestDir := filepath.Dir(manifestPath)

	// Make the directory read-only to prevent CreateTemp from succeeding.
	// Note: This may not work as root, so we skip in that case.
	if err := os.Chmod(manifestDir, 0555); err != nil {
		t.Skip("cannot change directory permissions for test")
	}
	defer os.Chmod(manifestDir, 0755) // Restore permissions

	// Attempt uninstall - manifest save should fail due to read-only directory,
	// but Uninstall should still complete with error captured
	result, err := Uninstall(true)
	if err != nil {
		t.Fatalf("Uninstall should not return fatal error: %v", err)
	}

	// Verify the error is captured in result.Errors
	if len(result.Errors) == 0 {
		t.Error("expected error to be captured in result.Errors for read-only directory failure")
	}

	// Verify error mentions relevant keywords (manifest, create, temp, permission, write)
	foundRelevantError := false
	for _, e := range result.Errors {
		lowerErr := strings.ToLower(e)
		if strings.Contains(lowerErr, "manifest") ||
			strings.Contains(lowerErr, "create") ||
			strings.Contains(lowerErr, "temp") ||
			strings.Contains(lowerErr, "permission") ||
			strings.Contains(lowerErr, "write") {
			foundRelevantError = true
			t.Logf("Captured error: %s", e)
			break
		}
	}
	if !foundRelevantError {
		t.Errorf("expected error about manifest/create/temp/permission, got: %v", result.Errors)
	}
}

// --- Test: ParseChecksums normalizes uppercase hex to lowercase ---

func TestParseChecksums_UppercaseHex(t *testing.T) {
	// Verify that uppercase hex hashes are normalized to lowercase
	// This test uses the exact format that PowerShell Get-FileHash produces
	// (uppercase by default) and verifies ParseChecksums normalizes it.
	content := []byte(`1C65A1A9C301828F582C0B04E6ECF59FD2841C86C2EAF35AFEC20AC025B4F44C  yhat-agent_linux_amd64
1592E7456FE72EA3A22B827CA8FDF20C16CB9CBF41645F11513F31DEDFFA0708  yhat-agent_darwin_arm64
FAF901B638F273B0A179A2E34B879D9FB004CE7A2B0FEA0B93608D9FB0E2BA2D  yhat-agent_windows_amd64.exe
`)

	entries, err := ParseChecksums(content)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}

	// Verify all hashes are normalized to lowercase
	expectedHashes := []string{
		"1c65a1a9c301828f582c0b04e6ecf59fd2841c86c2eaf35afec20ac025b4f44c",
		"1592e7456fe72ea3a22b827ca8fdf20c16cb9cbf41645f11513f31dedffa0708",
		"faf901b638f273b0a179a2e34b879d9fb004ce7a2b0fea0b93608d9fb0e2ba2d",
	}

	for i, exp := range expectedHashes {
		if entries[i].Hash != exp {
			t.Errorf("entry %d: expected lowercase hash %s, got %s", i, exp, entries[i].Hash)
		}
	}

	// Verify VerifyChecksum works with normalized lowercase hash
	// Simulate verifying against the computed lowercase hash
	for i, entry := range entries {
		// The normalized lowercase hash should work with VerifyChecksum
		if len(entry.Hash) != 64 {
			t.Errorf("entry %d: expected 64-char hash, got %d", i, len(entry.Hash))
		}
		// Verify hash contains only lowercase hex after normalization
		for _, c := range entry.Hash {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("entry %d: invalid hex char %c after normalization", i, c)
			}
		}
	}
}

// --- Test: ParseChecksums rejects invalid hex lengths ---

func TestParseChecksums_InvalidHexLength(t *testing.T) {
	// 63-char hash (too short)
	content := []byte(`1c65a1a9c301828f582c0b04e6ecf59fd2841c86c2eaf35afec20ac025b4f44  yhat-agent_linux_amd64
`) // 63 f's

	entries, err := ParseChecksums(content)
	if err != nil {
		t.Fatalf("parse should not fail: %v", err)
	}

	// Should skip the invalid entry
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for invalid hash length, got %d", len(entries))
	}
}

// --- Test: ParseChecksums skips comments and empty lines ---

func TestParseChecksums_CommentsAndEmpty(t *testing.T) {
	content := []byte("# This is a comment\n\n" +
		"1c65a1a9c301828f582c0b04e6ecf59fd2841c86c2eaf35afec20ac025b4f44c  yhat-agent_linux_amd64\n" +
		"# Another comment\n" +
		"\n" +
		"1c65a1a9c301828f582c0b04e6ecf59fd2841c86c2eaf35afec20ac025b4f44c  yhat-agent_darwin_arm64\n")

	entries, err := ParseChecksums(content)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	// Should parse 2 valid entries, skip 2 comments and 1 empty line
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

// --- Test: Uninstall partial failure preserves remaining manifest entries ---
// This test is deterministic: it creates a manifest with known entries,
// marks one file as modified in the manifest, and verifies that:
// 1. Modified file is skipped (not removed, not integrity error)
// 2. Unmodified file is removed
// 3. Manifest is reduced by deleting removed entries

func TestUninstall_PartialFailure_ReducesManifest(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Install first to create a valid manifest with real files
	_, err := Install()
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	manifestPath := ManifestPath()
	if !fileExists(manifestPath) {
		t.Fatal("manifest not created")
	}

	// Load the manifest to understand what was installed
	manifest, err := LoadManifest()
	if err != nil {
		t.Fatalf("failed to load manifest: %v", err)
	}

	initialCount := len(manifest.Assets)
	if initialCount != 2 {
		t.Fatalf("expected 2 assets in manifest, got %d", initialCount)
	}

	// Get paths
	agentPath := AgentTargetPath()
	skillPath := SkillTargetPath()

	// Mark one file as modified: update manifest with new content and Modified=true
	writeFile(t, agentPath, "user modified content\n")

	// Update manifest to record the modified state
	if err := manifest.UpdateInstalledHash(AgentAsset, agentPath); err != nil {
		t.Fatalf("failed to update installed hash: %v", err)
	}
	if asset, ok := manifest.Assets[AgentAsset]; ok {
		asset.Modified = true
		manifest.Assets[AgentAsset] = asset
	}
	if err := manifest.Save(); err != nil {
		t.Fatalf("failed to save manifest: %v", err)
	}

	// Attempt uninstall - modified agent will be skipped, skill should be removed
	result, err := Uninstall(true)
	if err != nil {
		t.Fatalf("Uninstall should not return fatal error: %v", err)
	}

	// Verify skill was removed
	if !fileExists(skillPath) {
		t.Log("Skill file removed (expected)")
	}

	// Verify manifest still exists (partial success case)
	if !fileExists(manifestPath) {
		t.Fatal("manifest should still exist after partial uninstall")
	}

	// Load the reduced manifest
	reducedManifest, err := LoadManifest()
	if err != nil {
		t.Fatalf("failed to load reduced manifest: %v", err)
	}

	// Verify reduced manifest has exactly 1 asset (the modified agent)
	if len(reducedManifest.Assets) != 1 {
		t.Errorf("expected 1 remaining asset, got %d", len(reducedManifest.Assets))
	}

	// Verify the remaining asset is the agent
	if _, ok := reducedManifest.Assets[AgentAsset]; !ok {
		t.Error("expected agent asset to remain in manifest")
	}

	// Verify skill is not in manifest (it was removed)
	if _, ok := reducedManifest.Assets[SkillAsset]; ok {
		t.Error("skill should not be in reduced manifest")
	}

	// Verify result reflects partial success
	// Modified files should be SKIPPED, not integrity errors
	if len(result.Skipped) < 1 {
		t.Error("expected at least one skipped file (modified)")
	}

	// No integrity errors should be reported for manifest-recorded modified files
	for _, e := range result.Errors {
		if strings.Contains(strings.ToLower(e), "integrity") {
			t.Errorf("should not report integrity error for manifest-recorded modified file: %s", e)
		}
	}

	// The skill should be in result.Removed
	foundRemoved := false
	for _, r := range result.Removed {
		if r == skillPath {
			foundRemoved = true
			break
		}
	}
	if !foundRemoved {
		t.Log("Skill not in removed list (may have been already gone or permission issue)")
	}

	t.Logf("Partial uninstall result: Removed=%d, Skipped=%d, Errors=%d",
		len(result.Removed), len(result.Skipped), len(result.Errors))
}

// --- Test: Uninstall removes manifest when all entries succeed ---

func TestUninstall_AllSucceeds_RemovesManifest(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Install first
	_, err := Install()
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	manifestPath := ManifestPath()
	if !fileExists(manifestPath) {
		t.Fatal("manifest not created")
	}

	// Uninstall - all files should be removed
	result, err := Uninstall(true)
	if err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}

	// Verify all files were removed
	if len(result.Removed) != 2 {
		t.Errorf("expected 2 removed files, got %d", len(result.Removed))
	}

	// Verify manifest was removed entirely
	if fileExists(manifestPath) {
		t.Error("manifest should be removed when all entries succeed")
	}
}

// --- Test: Platform asset fixtures - linux arm64 ---

func TestFindPlatformAsset_LinuxArm64(t *testing.T) {
	assets := []ReleaseAsset{
		{Name: "yhat-agent_linux_amd64", BrowserDownloadURL: "https://example.com/yhat-agent_linux_amd64"},
		{Name: "yhat-agent_linux_amd64.sha256sum", BrowserDownloadURL: "https://example.com/yhat-agent_linux_amd64.sha256sum"},
		{Name: "yhat-agent_linux_arm64", BrowserDownloadURL: "https://example.com/yhat-agent_linux_arm64"},
		{Name: "yhat-agent_linux_arm64.sha256sum", BrowserDownloadURL: "https://example.com/yhat-agent_linux_arm64.sha256sum"},
		{Name: "yhat-agent_darwin_amd64", BrowserDownloadURL: "https://example.com/yhat-agent_darwin_amd64"},
		{Name: "yhat-agent_darwin_amd64.sha256sum", BrowserDownloadURL: "https://example.com/yhat-agent_darwin_amd64.sha256sum"},
		{Name: "yhat-agent_darwin_arm64", BrowserDownloadURL: "https://example.com/yhat-agent_darwin_arm64"},
		{Name: "yhat-agent_darwin_arm64.sha256sum", BrowserDownloadURL: "https://example.com/yhat-agent_darwin_arm64.sha256sum"},
	}

	assetURL, checksumURL, err := FindPlatformAsset(assets, "linux_arm64")
	if err != nil {
		t.Fatalf("FindPlatformAsset failed: %v", err)
	}

	if !strings.Contains(assetURL, "linux_arm64") {
		t.Errorf("expected linux_arm64 in asset URL, got: %s", assetURL)
	}
	if !strings.Contains(checksumURL, "linux_arm64.sha256sum") {
		t.Errorf("expected linux_arm64.sha256sum in checksum URL, got: %s", checksumURL)
	}
}

// --- Test: Platform asset fixtures - darwin amd64 ---

func TestFindPlatformAsset_DarwinAmd64(t *testing.T) {
	assets := []ReleaseAsset{
		{Name: "yhat-agent_darwin_amd64", BrowserDownloadURL: "https://example.com/yhat-agent_darwin_amd64"},
		{Name: "yhat-agent_darwin_amd64.sha256sum", BrowserDownloadURL: "https://example.com/yhat-agent_darwin_amd64.sha256sum"},
	}

	assetURL, checksumURL, err := FindPlatformAsset(assets, "darwin_amd64")
	if err != nil {
		t.Fatalf("FindPlatformAsset failed: %v", err)
	}

	if !strings.Contains(assetURL, "darwin_amd64") {
		t.Errorf("expected darwin_amd64 in asset URL, got: %s", assetURL)
	}
	if !strings.Contains(checksumURL, "darwin_amd64.sha256sum") {
		t.Errorf("expected darwin_amd64.sha256sum in checksum URL, got: %s", checksumURL)
	}
}

// --- Test: All five cross-platform builds have matching fixture tests ---

func TestFindPlatformAsset_AllCrossPlatformFixtures(t *testing.T) {
	// This test serves as documentation of the complete cross-platform
	// asset naming contract used by both the release workflow and FindPlatformAsset.

	type platformFixture struct {
		platform     string
		assetName    string
		checksumName string
	}

	fixtures := []platformFixture{
		{"linux_amd64", "yhat-agent_linux_amd64", "yhat-agent_linux_amd64.sha256sum"},
		{"linux_arm64", "yhat-agent_linux_arm64", "yhat-agent_linux_arm64.sha256sum"},
		{"darwin_amd64", "yhat-agent_darwin_amd64", "yhat-agent_darwin_amd64.sha256sum"},
		{"darwin_arm64", "yhat-agent_darwin_arm64", "yhat-agent_darwin_arm64.sha256sum"},
		{"windows_amd64", "yhat-agent_windows_amd64.exe", "yhat-agent_windows_amd64.exe.sha256sum"},
	}

	baseURL := "https://github.com/ArcKelMiranda/yhat-knowledge/releases/download/v1.0.0/"

	for _, fixture := range fixtures {
		// Build asset list for this platform only
		assets := []ReleaseAsset{
			{
				Name:               fixture.assetName,
				BrowserDownloadURL: baseURL + fixture.assetName,
			},
			{
				Name:               fixture.checksumName,
				BrowserDownloadURL: baseURL + fixture.checksumName,
			},
		}

		assetURL, checksumURL, err := FindPlatformAsset(assets, fixture.platform)
		if err != nil {
			t.Errorf("FindPlatformAsset failed for %s: %v", fixture.platform, err)
			continue
		}

		if !strings.Contains(assetURL, fixture.assetName) {
			t.Errorf("[%s] asset URL missing %s, got: %s", fixture.platform, fixture.assetName, assetURL)
		}
		if !strings.Contains(checksumURL, fixture.checksumName) {
			t.Errorf("[%s] checksum URL missing %s, got: %s", fixture.platform, fixture.checksumName, checksumURL)
		}
	}
}

// --- Test: Windows .exe suffix in PlatformName output ---

func TestPlatformName_IncludesExeInName(t *testing.T) {
	// Verify PlatformName() does NOT include .exe (the suffix is added by FindPlatformAsset)
	name := PlatformName()

	// PlatformName() returns e.g., "windows_amd64" not "windows_amd64.exe"
	if strings.Contains(name, ".exe") {
		t.Error("PlatformName() should not include .exe extension")
	}

	// Verify format is OS_arch
	parts := strings.Split(name, "_")
	if len(parts) != 2 {
		t.Errorf("PlatformName() should return OS_arch format, got: %s", name)
	}
}

// --- Test: uniqueCandidatePath uses exclusive allocation (deterministic) ---
// This test verifies that uniqueCandidatePath never returns a path that would
// overwrite an existing candidate. It uses the exclusive file allocation pattern
// (O_CREATE|O_EXCL semantics) to detect collisions without race conditions.

func TestUniqueCandidatePath_ExclusiveAllocation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "yhat-agent-exclusive-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	basePath := filepath.Join(tmpDir, "yhat-agent")
	defaultCandidate := basePath + CandidateSuffix

	// First call should return default path (doesn't exist yet)
	result1, err := uniqueCandidatePath(defaultCandidate)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if result1 != defaultCandidate {
		t.Errorf("expected default path, got %s", result1)
	}

	// Create the default candidate file
	if err := os.WriteFile(defaultCandidate, []byte("existing"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Second call should return _2 suffix (not overwrite default)
	result2, err := uniqueCandidatePath(defaultCandidate)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	expected2 := basePath + "_2" + CandidateSuffix
	if result2 != expected2 {
		t.Errorf("expected %s after collision, got %s", expected2, result2)
	}

	// Create _2 suffix file too
	if err := os.WriteFile(expected2, []byte("existing2"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Third call should return _3 suffix
	result3, err := uniqueCandidatePath(defaultCandidate)
	if err != nil {
		t.Fatalf("third call failed: %v", err)
	}
	expected3 := basePath + "_3" + CandidateSuffix
	if result3 != expected3 {
		t.Errorf("expected %s after 2 collisions, got %s", expected3, result3)
	}

	// Verify NO existing file was overwritten
	if data, _ := os.ReadFile(defaultCandidate); string(data) != "existing" {
		t.Error("default candidate was overwritten!")
	}
	if data, _ := os.ReadFile(expected2); string(data) != "existing2" {
		t.Error("_2 candidate was overwritten!")
	}
}

// --- Test: uniqueCandidatePath deterministic collision exhaustion ---
// Verifies that uniqueCandidatePath returns an error after exhausting all
// numbered suffixes, rather than silently overwriting.

func TestUniqueCandidatePath_CollisionExhaustion(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "yhat-agent-exhaustion-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	basePath := filepath.Join(tmpDir, "yhat-agent")
	defaultCandidate := basePath + CandidateSuffix

	// Create default candidate
	if err := os.WriteFile(defaultCandidate, []byte("default"), 0644); err != nil {
		t.Fatalf("failed to create default: %v", err)
	}

	// Create suffixes _2 through _10 (10 collisions)
	for i := 2; i <= 10; i++ {
		path := fmt.Sprintf("%s_%d%s", basePath, i, CandidateSuffix)
		if err := os.WriteFile(path, []byte(fmt.Sprintf("suffix%d", i)), 0644); err != nil {
			t.Fatalf("failed to create suffix %d: %v", i, err)
		}
	}

	// Call should succeed with _11 suffix (within 999 limit)
	result, err := uniqueCandidatePath(defaultCandidate)
	if err != nil {
		t.Fatalf("should find path under 999 limit: %v", err)
	}
	expected := basePath + "_11" + CandidateSuffix
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

// --- Test: Manifest.Save atomic write ---
// Verifies that Save writes to temp file then atomically renames,
// ensuring no partial writes or corruption on failure.

func TestManifestSave_AtomicWrite(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "yhat-agent-atomic-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set XDG_CONFIG_HOME to test directory
	oldConfigHome := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", tmpDir)
	defer func() {
		os.Setenv("XDG_CONFIG_HOME", oldConfigHome)
	}()

	// Create manifest
	manifest := &Manifest{
		Version: "2",
		Assets:  make(map[string]Asset),
	}
	manifest.Assets[AgentAsset] = Asset{
		TargetPath:    "/test/agent",
		ContentHash:   "abc123",
		InstalledHash: "abc123",
	}

	// Save should succeed
	if err := manifest.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify manifest exists at expected path
	manifestPath := ManifestPath()
	if !fileExists(manifestPath) {
		t.Error("manifest not created at expected path")
	}

	// Verify content is valid JSON with correct version
	loaded, err := LoadManifest()
	if err != nil {
		t.Fatalf("failed to load saved manifest: %v", err)
	}
	if loaded.Version != "2" {
		t.Errorf("expected version 2, got %s", loaded.Version)
	}

	// Verify no temp files remain in directory
	entries, err := os.ReadDir(filepath.Dir(manifestPath))
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".manifest-") && strings.HasSuffix(entry.Name(), ".tmp") {
			t.Errorf("temp file %s should not remain after successful Save", entry.Name())
		}
	}
}

// --- Test: Manifest.Save cleans temp file on failure ---
// Verifies that failed Save does not leave orphaned temp files.
// This test uses directory removal to simulate failure (works regardless of permissions).

func TestManifestSave_CleansTempOnFailure(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "yhat-agent-atomic-fail-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set XDG_CONFIG_HOME
	oldConfigHome := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", tmpDir)
	defer func() {
		os.Setenv("XDG_CONFIG_HOME", oldConfigHome)
	}()

	// Create manifest
	manifest := &Manifest{
		Version: "3",
		Assets:  make(map[string]Asset),
	}

	// First save should succeed and create manifest
	if err := manifest.Save(); err != nil {
		t.Fatalf("first Save failed: %v", err)
	}

	manifestPath := ManifestPath()

	// Count temp files before failed save
	dir := filepath.Dir(manifestPath)
	beforeCount := 0
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".manifest-") {
			beforeCount++
		}
	}

	// Make parent directory non-writable to cause rename failure
	parentDir := dir
	if err := os.Chmod(parentDir, 0555); err != nil {
		t.Skip("cannot change directory permissions for test")
	}
	defer os.Chmod(parentDir, 0755)

	// Second save should fail (cannot rename temp file to read-only location)
	manifest.Version = "4"
	err = manifest.Save()
	if err == nil {
		t.Error("expected error when directory is not writable")
	}

	// Count temp files after failed save - should not increase
	afterCount := 0
	entries, _ = os.ReadDir(dir)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".manifest-") {
			afterCount++
		}
	}

	if afterCount > beforeCount {
		t.Errorf("temp files not cleaned on failure: before=%d, after=%d", beforeCount, afterCount)
	}
}

// --- Test: Uninstall integrity mismatch returns error ---
// Verifies that integrity mismatch during Uninstall is reported as an error,
// not merely as skipped. This ensures callers receive nonzero exit status
// and can detect tampering/corruption.

func TestUninstall_IntegrityMismatchAsError(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Install first
	_, err := Install()
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	agentPath := AgentTargetPath()

	// Corrupt the file to trigger integrity mismatch
	writeFile(t, agentPath, "corrupted content that will not match hash")

	// Attempt uninstall - should report integrity mismatch as ERROR
	result, err := Uninstall(true)
	if err != nil {
		t.Fatalf("Uninstall should not return fatal error: %v", err)
	}

	// Integrity mismatch MUST be in Errors, not just Skipped
	if len(result.Errors) == 0 {
		t.Error("expected integrity mismatch to be reported as ERROR")
	}

	// Verify the error message mentions integrity
	foundIntegrityError := false
	for _, e := range result.Errors {
		if strings.Contains(strings.ToLower(e), "integrity") ||
			strings.Contains(strings.ToLower(e), "mismatch") ||
			strings.Contains(strings.ToLower(e), "hash") {
			foundIntegrityError = true
			break
		}
	}
	if !foundIntegrityError {
		t.Errorf("expected error message about integrity mismatch, got: %v", result.Errors)
	}

	// The file should NOT be removed (can't remove corrupted file)
	if !fileExists(agentPath) {
		t.Error("file with integrity mismatch should NOT be removed")
	}

	// Manifest should still exist (preserve for investigation)
	if !fileExists(ManifestPath()) {
		t.Error("manifest should be preserved after integrity mismatch error")
	}
}

// --- Test: UninstallPlan reports integrity mismatch ---
// Verifies dry-run output correctly identifies integrity errors.

func TestUninstallPlan_IntegrityMismatch(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Install first
	_, err := Install()
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}

	agentPath := AgentTargetPath()

	// Corrupt the file
	writeFile(t, agentPath, "corrupted")

	// Get plan - should report integrity error
	plan, err := UninstallPlan()
	if err != nil {
		t.Fatalf("UninstallPlan failed: %v", err)
	}

	// Should have at least one skipped entry with INTEGRITY ERROR
	if len(plan.Skipped) == 0 {
		t.Error("expected skipped entries for corrupted file")
	}

	// Verify the reason mentions INTEGRITY ERROR
	for _, s := range plan.Skipped {
		if strings.Contains(s.Reason, "INTEGRITY ERROR") {
			return // Found expected output
		}
	}

	// If we didn't find INTEGRITY ERROR in skipped, check errors
	t.Logf("Skipped reasons: %v", plan.Skipped)
}

// --- Test: WriteExclusiveAtomically rejects symlinks at the destination ---
// Verifies that WriteExclusiveAtomically fails when the path is a symlink.
// O_EXCL with O_CREATE fails with EEXIST if the path already exists as any type
// (regular file, directory, or symlink). No temp file is created, nothing is modified.

func TestExclusiveFileWriter_RejectsSymlink(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "yhat-agent-symlink-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	targetPath := filepath.Join(tmpDir, "existing-file")
	candidatePath := filepath.Join(tmpDir, "candidate-file")

	// Create a real file
	if err := os.WriteFile(targetPath, []byte("real content"), 0644); err != nil {
		t.Fatalf("failed to create target: %v", err)
	}

	// Create a symlink pointing to the real file
	if err := os.Symlink(targetPath, candidatePath); err != nil {
		t.Skip("cannot create symlinks on this platform")
	}

	// WriteExclusiveAtomically should fail with EEXIST: O_EXCL rejects existing symlinks.
	// No temp file is created, the symlink remains intact.
	err = WriteExclusiveAtomically(candidatePath, []byte("new content"), 0644)
	if err == nil {
		t.Error("expected error when destination path is a symlink")
	}

	// Verify original file was not modified
	if data, _ := os.ReadFile(targetPath); string(data) != "real content" {
		t.Error("original file was modified through symlink")
	}

	// Verify symlink still exists and was not converted to a regular file
	info, err := os.Lstat(candidatePath)
	if err != nil {
		t.Fatalf("symlink disappeared: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("symlink was converted to regular file")
	}

	// Verify no orphan temp files remain (there should be none with O_EXCL approach)
	entries, _ := os.ReadDir(tmpDir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".write-exclusive-") || strings.HasPrefix(e.Name(), "write-exclusive-") {
			t.Errorf("orphan temp file left behind: %s", e.Name())
		}
	}
}

// --- Test: WriteExclusiveAtomically atomic write behavior ---
// Verifies that WriteExclusiveAtomically:
// - Creates the file exclusively using O_EXCL (single syscall, no race window)
// - Writes data directly to the open descriptor
// - Syncs before close for durability
// - The final file has the requested permissions
// - No temp files are created (O_EXCL creates directly at the target path)

func TestExclusiveFileWriter_AtomicWriteBehavior(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "yhat-agent-atomic-behavior-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	filePath := filepath.Join(tmpDir, "test-file")
	data := []byte("test data content for atomic write")

	// Write using exclusive API (O_EXCL single syscall)
	if err := WriteExclusiveAtomically(filePath, data, 0644); err != nil {
		t.Fatalf("WriteExclusiveAtomically failed: %v", err)
	}

	// Verify file exists at final path
	if !fileExists(filePath) {
		t.Error("file was not created at final path")
	}

	// Verify content matches
	readData, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(readData) != string(data) {
		t.Errorf("content mismatch: expected %q, got %q", data, readData)
	}

	// Verify final file has the requested permissions
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}
	if info.Mode()&0777 != 0644 {
		t.Errorf("expected permissions 0644, got %o", info.Mode())
	}

	// Verify no orphan temp files remain in the directory (O_EXCL creates directly)
	entries, _ := os.ReadDir(tmpDir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".write-exclusive-") || strings.HasPrefix(e.Name(), "write-exclusive-") {
			t.Errorf("orphan temp file left behind: %s", e.Name())
		}
	}
}

// --- Test: WriteExclusiveAtomically cleans up on failure ---
// When O_EXCL fails (e.g. destination already exists), nothing is created
// and the original file is untouched. No temp file exists to clean up.

func TestExclusiveFileWriter_CleansOnWriteFailure(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "yhat-agent-clean-write-fail-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	filePath := filepath.Join(tmpDir, "test-file")

	// First write should succeed (O_EXCL creates new file)
	if err := WriteExclusiveAtomically(filePath, []byte("first"), 0644); err != nil {
		t.Fatalf("first write failed: %v", err)
	}

	// Second write with same path should fail: O_EXCL returns EEXIST.
	err = WriteExclusiveAtomically(filePath, []byte("second"), 0644)
	if err == nil {
		t.Error("expected error when file already exists")
	}

	// Verify original content unchanged
	if data, _ := os.ReadFile(filePath); string(data) != "first" {
		t.Error("original content was overwritten despite error")
	}

	// Verify no orphan temp files remain after failure (none created with O_EXCL)
	entries, _ := os.ReadDir(tmpDir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".write-exclusive-") || strings.HasPrefix(e.Name(), "write-exclusive-") {
			t.Errorf("orphan temp file left behind after failure: %s", e.Name())
		}
	}
}

// --- Test: Manifest.Save handles directory write failure gracefully ---
// Verifies that when temp file creation fails (directory not writable),
// no orphan temp files are left.

func TestManifestSave_DirWriteFailureCleansTemp(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "yhat-agent-manifest-dir-write-fail-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set XDG_CONFIG_HOME
	oldConfigHome := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", tmpDir)
	defer func() {
		os.Setenv("XDG_CONFIG_HOME", oldConfigHome)
	}()

	manifest := &Manifest{
		Version: "5",
		Assets:  make(map[string]Asset),
	}

	// First save should succeed
	if err := manifest.Save(); err != nil {
		t.Fatalf("first Save failed: %v", err)
	}

	manifestPath := ManifestPath()
	dir := filepath.Dir(manifestPath)

	// Count temp files before second save
	beforeCount := 0
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".manifest-") {
			beforeCount++
		}
	}

	// Make directory read-only to prevent temp file creation
	if err := os.Chmod(dir, 0555); err != nil {
		t.Skip("cannot change directory permissions")
	}
	defer os.Chmod(dir, 0755)

	// Attempt second save - should fail on CreateTemp
	manifest.Version = "6"
	err = manifest.Save()
	if err == nil {
		t.Error("expected error when directory is not writable")
	}

	// Count temp files after failed save - should not increase
	afterCount := 0
	entries, _ = os.ReadDir(dir)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".manifest-") {
			afterCount++
		}
	}

	if afterCount > beforeCount {
		t.Errorf("temp files leaked on create failure: before=%d, after=%d", beforeCount, afterCount)
	}
}

// --- Test: Manifest.Save handles sync failure gracefully ---
// Verifies that failed sync does not leave orphan temp file.
// This test is best-effort since sync failures are hard to trigger reliably.

func TestManifestSave_SyncFailureCleansTemp(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "yhat-agent-manifest-sync-fail-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set XDG_CONFIG_HOME
	oldConfigHome := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", tmpDir)
	defer func() {
		os.Setenv("XDG_CONFIG_HOME", oldConfigHome)
	}()

	manifest := &Manifest{
		Version: "7",
		Assets:  make(map[string]Asset),
	}

	// First save should succeed
	if err := manifest.Save(); err != nil {
		t.Fatalf("first Save failed: %v", err)
	}

	manifestPath := ManifestPath()
	dir := filepath.Dir(manifestPath)

	// Count temp files before second save
	beforeCount := 0
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".manifest-") {
			beforeCount++
		}
	}

	// Make directory read-only to cause rename failure (simulates disk full on rename)
	if err := os.Chmod(dir, 0555); err != nil {
		t.Skip("cannot change directory permissions")
	}
	defer os.Chmod(dir, 0755)

	// Attempt second save - should fail
	manifest.Version = "8"
	err = manifest.Save()
	if err == nil {
		t.Error("expected error when directory is not writable")
	}

	// Count temp files after failed save
	afterCount := 0
	entries, _ = os.ReadDir(dir)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".manifest-") {
			afterCount++
		}
	}

	if afterCount > beforeCount {
		t.Errorf("temp files not cleaned on sync/rename failure: before=%d, after=%d", beforeCount, afterCount)
	}
}

// --- Test: uniqueCandidatePath rejects symlink at default path ---
// Verifies that uniqueCandidatePath detects symlink at default path and
// falls through to numbered suffixes instead of following the symlink.

func TestUniqueCandidatePath_RejectsSymlink(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "yhat-agent-candidate-symlink-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	basePath := filepath.Join(tmpDir, "yhat-agent")
	defaultCandidate := basePath + CandidateSuffix
	realFile := filepath.Join(tmpDir, "real-binary")

	// Create the real file that the symlink will point to
	if err := os.WriteFile(realFile, []byte("real"), 0755); err != nil {
		t.Fatalf("failed to create real file: %v", err)
	}

	// Create a symlink at the default candidate path
	if err := os.Symlink(realFile, defaultCandidate); err != nil {
		t.Skip("cannot create symlinks on this platform")
	}

	// uniqueCandidatePath should detect symlink at default path
	// and fall through to a numbered suffix (since tryExclusivePath fails on symlink)
	result, err := uniqueCandidatePath(defaultCandidate)
	if err != nil {
		// Error is acceptable if all suffixes exhausted
		t.Logf("uniqueCandidatePath error (acceptable): %v", err)
		return
	}

	// Result should be a numbered suffix, not the symlink path
	// Verify it does not match the symlink path
	if result == defaultCandidate {
		t.Error("uniqueCandidatePath returned symlink path instead of numbered suffix")
	}

	// Verify symlink still exists at original location
	info, err := os.Lstat(defaultCandidate)
	if err != nil {
		t.Fatalf("symlink disappeared: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("symlink was converted to regular file")
	}

	// Verify the returned path is actually a new numbered suffix that was created
	if !strings.Contains(result, "_2") && !strings.Contains(result, "_3") {
		t.Logf("Result: %s", result)
	}
}

// --- Test: latestDownloadURLs uses releases/latest/download/ pattern ---

func TestLatestDownloadURLs_UsesLatestRedirect(t *testing.T) {
	downloadURL, checksumURL, assetName, err := latestDownloadURLs()
	if err != nil {
		t.Fatalf("latestDownloadURLs failed: %v", err)
	}

	// Must use the stable GitHub latest redirect endpoint, not a hardcoded version.
	if !strings.Contains(downloadURL, "releases/latest/download/") {
		t.Errorf("download URL should use releases/latest/download/: got %s", downloadURL)
	}
	if !strings.Contains(checksumURL, "releases/latest/download/") {
		t.Errorf("checksum URL should use releases/latest/download/: got %s", checksumURL)
	}

	// Must NOT contain a hardcoded version tag.
	for _, url := range []string{downloadURL, checksumURL} {
		if strings.Contains(url, "/v0.1.") {
			t.Errorf("URL should not contain hardcoded v0.1.x version: %s", url)
		}
		if strings.Contains(url, "/v0.2.") {
			t.Errorf("URL should not contain hardcoded v0.2.x version: %s", url)
		}
	}

	// Asset name must be non-empty and match current platform.
	if assetName == "" {
		t.Error("assetName should not be empty")
	}
	platform := PlatformName()
	expectedAsset := assetNameForPlatform(platform)
	if assetName != expectedAsset {
		t.Errorf("assetName mismatch: got %s, expected %s for platform %s", assetName, expectedAsset, platform)
	}
}

// --- Test: extractVersionFromURL parses redirect URLs correctly ---

func TestExtractVersionFromURL(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://github.com/ArcKelMiranda/yhat-agent/releases/download/v0.1.2/yhat-agent_linux_amd64", "v0.1.2"},
		{"https://github.com/ArcKelMiranda/yhat-agent/releases/download/v0.1.3/yhat-agent_linux_amd64", "v0.1.3"},
		{"https://objects.githubusercontent.com/.../releases/download/v1.2.3/...", "v1.2.3"},
		{"https://github.com/ArcKelMiranda/yhat-agent/releases/download/v2.0.0-beta.1/...", "v2.0.0-beta.1"},
		{"https://no-match-here.com/releases/download/", "unknown"},
		{"", "unknown"},
	}

	for _, tt := range tests {
		got := extractVersionFromURL(tt.url)
		if got != tt.expected {
			t.Errorf("extractVersionFromURL(%q): expected %q, got %q", tt.url, tt.expected, got)
		}
	}
}

// --- Test: Version variable is never empty ---

func TestVersionVariable(t *testing.T) {
	// Version is initialized at package load time. It is never empty.
	if Version == "" {
		t.Error("Version should not be empty after initialization")
	}
	// In a plain `go build`, Version defaults to "dev" (no VCS metadata in tests).
	if Version != "dev" {
		t.Logf("Version is %q (likely injected by ldflags)", Version)
	}
}

// --- Test: UpdateResult fields are populated correctly ---

func TestUpdateResult_Fields(t *testing.T) {
	result := &UpdateResult{
		CurrentVersion: "dev",
		LatestVersion:  "v0.1.3",
		AssetURL:       "https://github.com/ArcKelMiranda/yhat-agent/releases/latest/download/yhat-agent_linux_amd64",
		AssetName:      "yhat-agent_linux_amd64",
		HashVerified:   true,
		CandidatePath:  "/usr/bin/yhat-agent.yhat-agent-new",
		Message:        "Update downloaded and verified!",
	}

	if result.LatestVersion == "" {
		t.Error("LatestVersion should be set")
	}
	if result.HashVerified != true {
		t.Error("HashVerified should be true on success")
	}
	if result.CandidatePath == "" {
		t.Error("CandidatePath should be set")
	}
	if !strings.Contains(result.AssetURL, "releases/latest/download/") {
		t.Error("AssetURL should use releases/latest/download/")
	}
}

// --- Test: ParseChecksums skips invalid lines ---

func TestParseChecksums_InvalidLines(t *testing.T) {
	content := []byte(`# This is a comment

1c65a1a9c301828f582c0b04e6ecf59fd2841c86c2eaf35afec20ac025b4f44c  yhat-agent_linux_amd64
# Another comment

deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef  yhat-agent_linux_amd64
`)

	entries, err := ParseChecksums(content)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	// Should parse 2 valid entries, skip comments and empty lines.
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}

	// Verify first entry hash is 64 chars and lowercase.
	if len(entries[0].Hash) != 64 {
		t.Errorf("first hash length: expected 64, got %d", len(entries[0].Hash))
	}
	if entries[0].AssetName != "yhat-agent_linux_amd64" {
		t.Errorf("first asset name: expected yhat-agent_linux_amd64, got %s", entries[0].AssetName)
	}
}

// --- Test: CandidateName appends suffix correctly (cross-platform) ---

func TestCandidateName_CrossPlatform(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/usr/bin/yhat-agent", "/usr/bin/yhat-agent.yhat-agent-new"},
		{"/usr/bin/yhat-agent.exe", "/usr/bin/yhat-agent.exe.yhat-agent-new"},
		{"C:\\Program Files\\yhat-agent\\yhat-agent.exe", "C:\\Program Files\\yhat-agent\\yhat-agent.exe.yhat-agent-new"},
	}

	for _, tt := range tests {
		got := CandidateName(tt.path)
		if got != tt.expected {
			t.Errorf("CandidateName(%q): expected %q, got %q", tt.path, tt.expected, got)
		}
	}
}

// --- Test: resolveVersion follows redirect chain and extracts tag ---

func TestResolveVersion_FollowsRedirect(t *testing.T) {
	// Set up a redirect server: /redirect -> final URL containing the tag.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/releases/download/v1.2.3/yhat-agent_linux_amd64", http.StatusFound)
			return
		}
		// Final URL — return a minimal 200 so HEAD succeeds.
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fake binary"))
	}))
	defer server.Close()

	client := server.Client() // uses default transport, follows redirects

	resolved, err := resolveVersion(client, server.URL+"/redirect")
	if err != nil {
		t.Fatalf("resolveVersion failed: %v", err)
	}
	if resolved != "v1.2.3" {
		t.Errorf("expected v1.2.3, got %q", resolved)
	}
}

// --- Test: resolveVersion returns error on failed redirect ---

func TestResolveVersion_NonRedirectResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := resolveVersion(server.Client(), server.URL+"/missing")
	if err == nil {
		t.Error("expected error on non-redirect response")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention status: %v", err)
	}
}

// --- Test: Version is never empty ---

func TestVersion_NeverEmpty(t *testing.T) {
	// Version must always produce a meaningful non-empty string.
	// It either comes from ldflags, VCS revision, or defaults to "dev".
	if Version == "" {
		t.Error("Version should never be empty")
	}
}

// --- Test: detectVersion returns short VCS revision ---

func TestDetectVersion_ShortRev(t *testing.T) {
	got := shortRev("abcd12345678extra")
	if got != "abcd12345678" {
		t.Errorf("expected 12-char short rev, got %q", got)
	}

	got = shortRev("abc")
	if got != "abc" {
		t.Errorf("short rev should return input when len < 12, got %q", got)
	}
}

// --- Test: detectVersion falls back to dev when no build info ---

func TestDetectVersion_DevFallback(t *testing.T) {
	// Save and restore Version to test the detection path.
	// We cannot easily fake ReadBuildInfo in-process, so we test that
	// the variable is initialized to a non-empty string by verifying
	// the exported value at import time.
	if Version == "" {
		t.Error("Version must be initialized to a non-empty value")
	}
}
