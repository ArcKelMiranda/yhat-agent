//go:build windows

package yhatagent

import (
	"os"
	"testing"
)

// --- Test: Windows candidate path ---

func TestWindowsCandidatePath(t *testing.T) {
	path := WindowsCandidatePath("C:\\Program Files\\yhat-agent\\yhat-agent.exe")
	if !containsString(path, "_new.exe") {
		t.Errorf("expected _new.exe suffix, got: %s", path)
	}
	if !containsString(path, "yhat-agent_new.exe") {
		t.Errorf("expected yhat-agent_new.exe, got: %s", path)
	}
}

// --- Test: Windows candidate path with no extension ---

func TestWindowsCandidatePathNoExt(t *testing.T) {
	path := WindowsCandidatePath("C:\\some\\path\\yhat-agent")
	if !containsString(path, "_new") {
		t.Errorf("expected _new suffix, got: %s", path)
	}
}

// --- Test: FindPlatformAsset handles Windows .exe suffix ---

func TestFindPlatformAsset_WindowsExeSuffix(t *testing.T) {
	assets := []ReleaseAsset{
		{Name: "yhat-agent_windows_amd64.exe", BrowserDownloadURL: "https://example.com/yhat-agent_windows_amd64.exe"},
		{Name: "yhat-agent_windows_amd64.exe.sha256sum", BrowserDownloadURL: "https://example.com/yhat-agent_windows_amd64.exe.sha256sum"},
	}

	assetURL, checksumURL, err := FindPlatformAsset(assets, "windows_amd64")
	if err != nil {
		t.Fatalf("FindPlatformAsset failed: %v", err)
	}

	if !containsString(assetURL, "windows_amd64.exe") {
		t.Errorf("expected windows_amd64.exe in asset URL, got: %s", assetURL)
	}
	if !containsString(checksumURL, "windows_amd64.exe.sha256sum") {
		t.Errorf("expected windows_amd64.exe.sha256sum in checksum URL, got: %s", checksumURL)
	}
}

// --- Test: FindPlatformAsset Windows without .exe in platform name ---

func TestFindPlatformAsset_WindowsPlatformVariants(t *testing.T) {
	// Platform name should be windows_amd64 (without .exe)
	assets := []ReleaseAsset{
		{Name: "yhat-agent_windows_amd64.exe", BrowserDownloadURL: "https://example.com/yhat-agent_windows_amd64.exe"},
		{Name: "yhat-agent_windows_amd64.exe.sha256sum", BrowserDownloadURL: "https://example.com/yhat-agent_windows_amd64.exe.sha256sum"},
	}

	assetURL, _, err := FindPlatformAsset(assets, "windows_amd64")
	if err != nil {
		t.Fatalf("FindPlatformAsset failed: %v", err)
	}

	if !containsString(assetURL, "windows_amd64.exe") {
		t.Errorf("expected windows_amd64.exe in asset URL, got: %s", assetURL)
	}
}

// --- Test: latestDownloadURLs constructs correct Windows URLs ---

func TestLatestDownloadURLs_Windows(t *testing.T) {
	downloadURL, checksumURL, assetName, err := latestDownloadURLs()
	if err != nil {
		t.Fatalf("latestDownloadURLs failed: %v", err)
	}

	if assetName != "yhat-agent_windows_amd64.exe" {
		t.Errorf("expected yhat-agent_windows_amd64.exe, got: %s", assetName)
	}

	expectedDownloadBase := "https://github.com/ArcKelMiranda/yhat-agent/releases/latest/download/"
	if !containsString(downloadURL, expectedDownloadBase) {
		t.Errorf("download URL should use releases/latest/download/: got %s", downloadURL)
	}
	if !containsString(downloadURL, "yhat-agent_windows_amd64.exe") {
		t.Errorf("download URL should contain Windows asset name: got %s", downloadURL)
	}
	if !containsString(checksumURL, "yhat-agent_windows_amd64.exe.sha256sum") {
		t.Errorf("checksum URL should contain Windows checksum name: got %s", checksumURL)
	}
}

// --- Test: uniqueCandidatePath on Windows-style paths ---

func TestUniqueCandidatePath_WindowsPaths(t *testing.T) {
	tmpDir := t.TempDir()

	defaultCandidate := tmpDir + "\\yhat-agent.exe.yhat-agent-new"

	// First call — no collision — returns default
	result, err := uniqueCandidatePath(defaultCandidate)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if result != defaultCandidate {
		t.Errorf("expected default when no collision, got %s", result)
	}

	// Create the default — next call should return _2 suffix
	if err := createFile(defaultCandidate); err != nil {
		t.Fatalf("failed to create default candidate: %v", err)
	}

	result, err = uniqueCandidatePath(defaultCandidate)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	expected := tmpDir + "\\yhat-agent_2.yhat-agent-new"
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}

	// Verify original file was not overwritten
	content, err := os.ReadFile(defaultCandidate)
	if err != nil {
		t.Fatalf("failed to read original: %v", err)
	}
	if string(content) != "test content" {
		t.Error("original file was overwritten")
	}
}

// createFile is a test helper that creates a file with test content.
func createFile(path string) error {
	return os.WriteFile(path, []byte("test content"), 0644)
}

// containsString is a simple string contains helper.
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
