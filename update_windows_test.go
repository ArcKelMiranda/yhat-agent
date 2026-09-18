//go:build windows

package yhatagent

import (
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
	// Test that platform name without .exe still finds .exe asset
	assets := []ReleaseAsset{
		{Name: "yhat-agent_windows_amd64.exe", BrowserDownloadURL: "https://example.com/yhat-agent_windows_amd64.exe"},
		{Name: "yhat-agent_windows_amd64.exe.sha256sum", BrowserDownloadURL: "https://example.com/yhat-agent_windows_amd64.exe.sha256sum"},
	}

	// Platform name should be windows_amd64 (without .exe)
	assetURL, _, err := FindPlatformAsset(assets, "windows_amd64")
	if err != nil {
		t.Fatalf("FindPlatformAsset failed: %v", err)
	}

	if !containsString(assetURL, "windows_amd64.exe") {
		t.Errorf("expected windows_amd64.exe in asset URL, got: %s", assetURL)
	}
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
