//go:build linux || darwin

package yhatagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Test: latestDownloadURLs constructs correct Unix URLs ---

func TestLatestDownloadURLs_Linux(t *testing.T) {
	downloadURL, checksumURL, assetName, err := latestDownloadURLs()
	if err != nil {
		t.Fatalf("latestDownloadURLs failed: %v", err)
	}

	expectedDownloadBase := "https://github.com/ArcKelMiranda/yhat-agent/releases/latest/download"
	if !strings.HasPrefix(downloadURL, expectedDownloadBase) {
		t.Errorf("download URL should start with %s, got %s", expectedDownloadBase, downloadURL)
	}
	if !strings.HasSuffix(downloadURL, assetName) {
		t.Errorf("download URL should end with %s, got %s", assetName, downloadURL)
	}
	if !strings.HasSuffix(checksumURL, assetName+".sha256sum") {
		t.Errorf("checksum URL mismatch: got %s", checksumURL)
	}
}

// --- Test: uniqueCandidatePath returns default when available ---

func TestUniqueCandidatePath_ReturnsDefaultWhenFree(t *testing.T) {
	tmpDir := t.TempDir()

	basePath := filepath.Join(tmpDir, "yhat-agent")
	defaultCandidate := basePath + CandidateSuffix

	// No files exist — should return the default path.
	result, err := uniqueCandidatePath(defaultCandidate)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if result != defaultCandidate {
		t.Errorf("expected default %s, got %s", defaultCandidate, result)
	}

	// Verify no orphan files were created.
	entries, _ := os.ReadDir(tmpDir)
	for _, e := range entries {
		if !e.IsDir() {
			t.Errorf("unexpected file created: %s", e.Name())
		}
	}
}

// --- Test: uniqueCandidatePath skips existing default and returns _2 ---

func TestUniqueCandidatePath_SkipsExistingDefault(t *testing.T) {
	tmpDir := t.TempDir()

	basePath := filepath.Join(tmpDir, "yhat-agent")
	defaultCandidate := basePath + CandidateSuffix

	// Pre-create the default — function should skip to _2.
	if err := os.WriteFile(defaultCandidate, []byte("existing"), 0644); err != nil {
		t.Fatalf("failed to create default: %v", err)
	}

	result, err := uniqueCandidatePath(defaultCandidate)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	expected := basePath + "_2" + CandidateSuffix
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}

	// Original file must not be overwritten.
	if content, _ := os.ReadFile(defaultCandidate); string(content) != "existing" {
		t.Error("default file was overwritten")
	}
}

// --- Test: VerifyChecksum integration with ParseChecksums output ---

func TestUpdate_ChecksumVerifyRoundTrip(t *testing.T) {
	content := []byte(`1c65a1a9c301828f582c0b04e6ecf59fd2841c86c2eaf35afec20ac025b4f44c  yhat-agent_linux_amd64
1592e7456fe72ea3a22b827ca8fdf20c16cb9cbf41645f11513f31dedffa0708  yhat-agent_linux_arm64
faf901b638f273b0a179a2e34b879d9fb004ce7a2b0fea0b93608d9fb0e2ba2d  yhat-agent_darwin_arm64
`)

	entries, err := ParseChecksums(content)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}

	// Verify each entry can be found and hash is valid 64-char lowercase hex.
	platforms := []string{"yhat-agent_linux_amd64", "yhat-agent_linux_arm64", "yhat-agent_darwin_arm64"}
	for _, name := range platforms {
		found := false
		for _, e := range entries {
			if e.AssetName == name {
				found = true
				if len(e.Hash) != 64 {
					t.Errorf("hash for %s is not 64 chars: %d", name, len(e.Hash))
				}
				// Verify lowercase normalization.
				for _, c := range e.Hash {
					if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
						t.Errorf("hash for %s contains non-lowercase-hex: %c", name, c)
					}
				}
				break
			}
		}
		if !found {
			t.Errorf("asset %s not found in parsed entries", name)
		}
	}
}

// --- Test: assetNameForPlatform covers all supported platforms ---

func TestAssetNameForPlatform_AllPlatforms(t *testing.T) {
	tests := []struct {
		platform        string
		expectedAsset   string
		expectedSumFile string
	}{
		{"linux_amd64", "yhat-agent_linux_amd64", "yhat-agent_linux_amd64.sha256sum"},
		{"linux_arm64", "yhat-agent_linux_arm64", "yhat-agent_linux_arm64.sha256sum"},
		{"darwin_amd64", "yhat-agent_darwin_amd64", "yhat-agent_darwin_amd64.sha256sum"},
		{"darwin_arm64", "yhat-agent_darwin_arm64", "yhat-agent_darwin_arm64.sha256sum"},
		{"windows_amd64", "yhat-agent_windows_amd64.exe", "yhat-agent_windows_amd64.exe.sha256sum"},
	}

	for _, tt := range tests {
		gotAsset := assetNameForPlatform(tt.platform)
		if gotAsset != tt.expectedAsset {
			t.Errorf("[%s] asset: expected %s, got %s", tt.platform, tt.expectedAsset, gotAsset)
		}
		gotSum := checksumNameForPlatform(tt.platform)
		if gotSum != tt.expectedSumFile {
			t.Errorf("[%s] checksum: expected %s, got %s", tt.platform, tt.expectedSumFile, gotSum)
		}
	}
}

// formatInt converts a non-negative integer to a decimal string.
func formatInt(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
