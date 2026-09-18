package yhatagent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// UpdateResult describes the outcome of an update operation.
type UpdateResult struct {
	CurrentVersion string
	LatestVersion string
	AssetURL      string
	AssetName     string
	HashVerified  bool
	CandidatePath string
	Message       string
}

// getPlatformDetails returns the asset name and download URL for the current platform.
func getPlatformDetails() (assetName, downloadURL, checksumURL string, err error) {
	platform := PlatformName()
	owner := "ArcKelMiranda"
	repo := "yhat-agent"
	version := "v0.1.2"

	baseURL := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s", owner, repo, version)

	switch platform {
	case "linux_amd64":
		assetName = "yhat-agent_linux_amd64"
		downloadURL = baseURL + "/" + assetName
		checksumURL = baseURL + "/" + assetName + ".sha256sum"
	case "windows_amd64":
		assetName = "yhat-agent_windows_amd64.exe"
		downloadURL = baseURL + "/" + assetName
		checksumURL = baseURL + "/" + assetName + ".sha256sum"
	case "darwin_amd64":
		assetName = "yhat-agent_darwin_amd64"
		downloadURL = baseURL + "/" + assetName
		checksumURL = baseURL + "/" + assetName + ".sha256sum"
	case "darwin_arm64":
		assetName = "yhat-agent_darwin_arm64"
		downloadURL = baseURL + "/" + assetName
		checksumURL = baseURL + "/" + assetName + ".sha256sum"
	default:
		return "", "", "", fmt.Errorf("unsupported platform: %s", platform)
	}

	return assetName, downloadURL, checksumURL, nil
}

// DownloadFile downloads a file from URL to a temporary location.
func DownloadFile(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "yhat-agent")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned status %d for %s", resp.StatusCode, url)
	}

	return io.ReadAll(resp.Body)
}

// VerifyChecksum verifies a downloaded asset against an expected hash.
func VerifyChecksum(data []byte, expectedHash string) bool {
	hash := sha256.Sum256(data)
	actualHex := hex.EncodeToString(hash[:])
	return strings.EqualFold(actualHex, expectedHash)
}

// Update downloads and verifies the latest release from GitHub.
func Update() (*UpdateResult, error) {
	// Get platform details
	assetName, downloadURL, checksumURL, err := getPlatformDetails()
	if err != nil {
		return nil, fmt.Errorf("platform error: %w", err)
	}

	// Get current version
	currentVersion := "unknown"
	if manifest, err := LoadManifest(); err == nil && manifest != nil {
		currentVersion = manifest.Version
	}

	// Download checksum file
	checksumData, err := DownloadFile(checksumURL)
	if err != nil {
		return nil, fmt.Errorf("downloading checksums: %w", err)
	}

	// Parse checksum - format is "hash  filename"
	checksumLine := strings.TrimSpace(string(checksumData))
	parts := strings.Split(checksumLine, " ")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid checksum format: %s", checksumLine)
	}
	expectedHash := strings.TrimSpace(parts[0])

	// Download asset
	assetData, err := DownloadFile(downloadURL)
	if err != nil {
		return nil, fmt.Errorf("downloading asset: %w", err)
	}

	// Verify checksum
	hashVerified := VerifyChecksum(assetData, expectedHash)
	if !hashVerified {
		return nil, fmt.Errorf("checksum verification failed for %s", assetName)
	}

	// Determine candidate path
	execPath, err := os.Executable()
	if err != nil {
		execPath = os.Args[0]
	}

	candidatePath := execPath + CandidateSuffix

	// Write to candidate file
	if err := os.WriteFile(candidatePath, assetData, 0755); err != nil {
		return nil, fmt.Errorf("writing candidate: %w", err)
	}

	// Build result
	result := &UpdateResult{
		CurrentVersion: currentVersion,
		LatestVersion: "v0.1.2",
		AssetURL:      downloadURL,
		AssetName:     assetName,
		HashVerified:  true,
		CandidatePath: candidatePath,
		Message: fmt.Sprintf("Update downloaded and verified!\n\nTo complete the update:\n\n  # Stop yhat-agent if running\n  # Backup current (optional): mv %s %s.backup\n  # Apply update: mv %s %s\n  # Restart yhat-agent\n\nNew binary: %s", execPath, execPath, candidatePath, execPath, candidatePath),
	}

	return result, nil
}

// GetCurrentBinaryPath returns the path to the currently running executable.
func GetCurrentBinaryPath() (string, error) {
	return os.Executable()
}

// GetPlatformName returns the normalized platform identifier.
func GetPlatformName() string {
	return PlatformName()
}
