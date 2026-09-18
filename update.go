package yhatagent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"
)

// UpdateResult describes the outcome of an update operation.
type UpdateResult struct {
	CurrentVersion string
	LatestVersion  string
	AssetURL       string
	AssetName      string
	HashVerified   bool
	CandidatePath  string
	Message        string
}

// injectedVersion holds the value set by -ldflags '-X .../injectedVersion=X'.
// It is empty when ldflags did not set it, allowing detectVersion to fall
// through to VCS metadata.
var injectedVersion string

// Version is the installed or building version of yhat-agent.
// It is set at compile time via -ldflags '-X .../injectedVersion=v1.2.3',
// or defaults to a value derived from runtime/debug.ReadBuildInfo
// (vcs.revision, vcs.modified) when built from a module with VCS metadata.
// It is never "unknown" — callers can always display it meaningfully.
var Version = detectVersion()

// BuildInfo describes how and when this binary was built (e.g. VCS time/status).
var BuildInfo = detectBuildInfo()

// detectVersion returns a meaningful version string.
// Priority: (1) ldflags-injected injectedVersion, (2) VCS revision from
// ReadBuildInfo, (3) "dev". The module version is intentionally not
// used because `go install @latest` produces a binary with no module metadata.
func detectVersion() string {
	// If ldflags injected a version, use it.
	if injectedVersion != "" {
		return injectedVersion
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	for _, s := range bi.Settings {
		if s.Key == "vcs.revision" {
			return shortRev(s.Value)
		}
	}
	return "dev"
}

// detectBuildInfo returns a short VCS status string for display.
func detectBuildInfo() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, s := range bi.Settings {
		if s.Key == "vcs.modified" && s.Value == "true" {
			return "(uncommitted changes)"
		}
	}
	for _, s := range bi.Settings {
		if s.Key == "vcs.time" {
			return s.Value
		}
	}
	return ""
}

// shortRev returns the first 12 characters of a VCS revision.
func shortRev(rev string) string {
	if len(rev) > 12 {
		return rev[:12]
	}
	return rev
}

// versionRE extracts the tag from a GitHub redirect URL like
// ".../releases/download/v1.2.3/yhat-agent_linux_amd64".
var versionRE = regexp.MustCompile(`/releases/download/([^/]+)/`)

// latestDownloadBase returns the base URL for the latest release downloads.
func latestDownloadBase() string {
	return "https://github.com/ArcKelMiranda/yhat-agent/releases/latest/download"
}

// getPlatformAssetName returns the asset name for the current platform.
func getPlatformAssetName() (string, error) {
	platform := PlatformName()
	name := assetNameForPlatform(platform)
	if name == "" {
		return "", fmt.Errorf("unsupported platform: %s", platform)
	}
	return name, nil
}

// latestDownloadURLs returns the download and checksum URLs for the latest
// release using GitHub's stable releases/latest/download/ redirect endpoint.
// No GitHub API token or rate-limited endpoint is used.
func latestDownloadURLs() (downloadURL, checksumURL, assetName string, err error) {
	assetName, err = getPlatformAssetName()
	if err != nil {
		return "", "", "", err
	}

	base := latestDownloadBase()
	downloadURL = base + "/" + assetName
	checksumURL = base + "/" + assetName + ".sha256sum"
	return downloadURL, checksumURL, assetName, nil
}

// extractVersionFromURL derives the tag version from the redirect URL returned
// by GitHub's releases/latest/download/ endpoint.
func extractVersionFromURL(redirectURL string) string {
	m := versionRE.FindStringSubmatch(redirectURL)
	if m != nil {
		return m[1]
	}
	return "unknown"
}

// resolveVersion issues a HEAD request through the supplied client,
// lets the transport follow redirects automatically, validates that the
// final response succeeded, and extracts the tag from the resolved URL.
// It returns an error on network failure or non-2xx final response.
func resolveVersion(client *http.Client, downloadURL string) (string, error) {
	req, err := http.NewRequest("HEAD", downloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "yhat-agent")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("HEAD %s: %w", downloadURL, err)
	}
	defer resp.Body.Close()

	// The transport has followed all redirects. Validate the final status.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HEAD %s: status %d", downloadURL, resp.StatusCode)
	}

	// resp.Request.URL is the URL after all redirects were followed.
	return extractVersionFromURL(resp.Request.URL.String()), nil
}

// DownloadFile downloads a file from url into memory.
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

// VerifyChecksum verifies data against an expected SHA-256 hash.
func VerifyChecksum(data []byte, expectedHash string) bool {
	hash := sha256.Sum256(data)
	actualHex := hex.EncodeToString(hash[:])
	return strings.EqualFold(actualHex, expectedHash)
}

// Update downloads and verifies the latest release using direct download URLs.
// It avoids the GitHub API entirely, using the stable releases/latest/download/
// redirect endpoint. A checksum file is downloaded first to verify integrity
// before writing the candidate binary.
func Update() (*UpdateResult, error) {
	// Determine download URLs for the current platform.
	downloadURL, checksumURL, assetName, err := latestDownloadURLs()
	if err != nil {
		return nil, fmt.Errorf("platform error: %w", err)
	}

	// Resolve the actual version tag by following the latest redirect.
	// This avoids hardcoding a version while still reporting what was fetched.
	version := "unknown"
	if resolved, err := resolveVersion(&http.Client{}, downloadURL); err == nil {
		version = resolved
	} else {
		// Non-fatal: report unknown version but still attempt the download.
	}

	// Current version is the build-in Version variable; the manifest tracks
	// embedded asset content, not release tags, so we don't compare versions here.

	// Download checksum file first.
	checksumData, err := DownloadFile(checksumURL)
	if err != nil {
		return nil, fmt.Errorf("downloading checksums: %w", err)
	}

	// Parse checksum — find the line for our asset.
	entries, err := ParseChecksums(checksumData)
	if err != nil {
		return nil, fmt.Errorf("parsing checksums: %w", err)
	}

	var expectedHash string
	for _, e := range entries {
		if e.AssetName == assetName {
			expectedHash = e.Hash
			break
		}
	}

	if expectedHash == "" {
		return nil, fmt.Errorf("no checksum entry for %q in %s", assetName, checksumURL)
	}

	// Download the binary.
	assetData, err := DownloadFile(downloadURL)
	if err != nil {
		return nil, fmt.Errorf("downloading asset: %w", err)
	}

	// Verify checksum before writing anything.
	if !VerifyChecksum(assetData, expectedHash) {
		return nil, fmt.Errorf("checksum verification failed for %s", assetName)
	}

	// Determine candidate path using safe side-by-side allocation.
	execPath, err := os.Executable()
	if err != nil {
		execPath = os.Args[0]
	}
	candidatePath := CandidateName(execPath)

	// Use exclusive allocation to avoid overwriting existing files.
	// uniqueCandidatePath finds the first unused numbered suffix.
	if candidatePath, err = uniqueCandidatePath(candidatePath); err != nil {
		return nil, fmt.Errorf("finding candidate path: %w", err)
	}

	// Write using exclusive allocation — never overwrites the running executable.
	if err := WriteExclusiveAtomically(candidatePath, assetData, 0755); err != nil {
		return nil, fmt.Errorf("writing candidate: %w", err)
	}

	// Build result.
	execName := filepath.Base(execPath)
	result := &UpdateResult{
		CurrentVersion: Version,
		LatestVersion:  version,
		AssetURL:       downloadURL,
		AssetName:      assetName,
		HashVerified:   true,
		CandidatePath:  candidatePath,
		Message: fmt.Sprintf(
			"Update downloaded and verified!\n\nTo complete the update:\n\n  # Stop yhat-agent if running\n  # Backup current (optional): mv %s %s.backup\n  # Apply update: mv %s %s\n  # Restart yhat-agent\n\nNew binary: %s",
			execPath, execName, candidatePath, execPath, candidatePath,
		),
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
