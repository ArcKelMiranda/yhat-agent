package yhatagent

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ReleaseAsset represents a single release asset.
type ReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// ReleaseMetadata holds parsed GitHub release information.
type ReleaseMetadata struct {
	TagName string         `json:"tag_name"`
	HTMLURL string         `json:"html_url"`
	Assets  []ReleaseAsset `json:"assets"`
}

// ChecksumEntry represents a parsed SHA-256 checksum entry.
type ChecksumEntry struct {
	AssetName string // e.g., "yhat-agent_linux_amd64"
	Hash     string // SHA-256 hex digest
}

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

// FetchLatestRelease queries the GitHub API for the latest release metadata.
func FetchLatestRelease(owner, repo string) (*ReleaseMetadata, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release ReleaseMetadata
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decoding release: %w", err)
	}

	return &release, nil
}

// ParseChecksums parses a checksums file content (SHA256SUMS format).
// It accepts the canonical lowercase hex form from sha256sum/shasum producers.
// Uppercase hex is normalized to lowercase for compatibility with consumers
// that expect lowercase hashes (e.g., Go's hex decoder, VerifyChecksum).
// Empty lines and comment lines (starting with #) are skipped.
func ParseChecksums(content []byte) ([]ChecksumEntry, error) {
	var entries []ChecksumEntry
	scanner := bufio.NewScanner(bytes.NewReader(content))
	// SHA256SUMS format: "<hash>  <asset-name>" (two spaces)
	// Accept both lowercase and uppercase hex; normalize to lowercase.
	re := regexp.MustCompile(`^([a-fA-F0-9]{64})\s{2,}(.+)$`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		matches := re.FindStringSubmatch(line)
		if len(matches) == 3 {
			// Normalize hash to lowercase for consistent output
			entries = append(entries, ChecksumEntry{
				Hash:      strings.ToLower(matches[1]),
				AssetName: strings.TrimSpace(matches[2]),
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning checksums: %w", err)
	}

	return entries, nil
}

// FindPlatformAsset finds the asset matching the current platform.
func FindPlatformAsset(assets []ReleaseAsset, platform string) (string, string, error) {
	// Primary pattern: yhat-agent_<platform>
	primaryPattern := "yhat-agent_" + platform

	// Windows-specific pattern: yhat-agent_<platform>.exe
	primaryPatternWindows := primaryPattern + ".exe"

	// Checksums pattern: yhat-agent_<platform>.sha256sum
	checksumPattern := primaryPattern + ".sha256sum"

	// Windows checksum pattern: yhat-agent_<platform>.exe.sha256sum
	checksumPatternWindows := primaryPatternWindows + ".sha256sum"

	var assetURL, checksumURL string

	for _, asset := range assets {
		switch {
		case asset.Name == primaryPattern || asset.Name == primaryPatternWindows:
			assetURL = asset.BrowserDownloadURL
		case asset.Name == checksumPattern || asset.Name == checksumPatternWindows:
			checksumURL = asset.BrowserDownloadURL
		}
	}

	if assetURL == "" {
		return "", "", fmt.Errorf("no asset found for platform %s", platform)
	}
	if checksumURL == "" {
		return "", "", fmt.Errorf("no checksums file found for platform %s", platform)
	}

	return assetURL, checksumURL, nil
}

// DownloadFile downloads a file to a temporary location.
func DownloadFile(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// VerifyChecksum verifies a downloaded asset against an expected hash.
func VerifyChecksum(data []byte, expectedHash string) bool {
	hash := sha256.Sum256(data)
	actualHex := hex.EncodeToString(hash[:])
	return actualHex == expectedHash
}

// uniqueCandidatePath returns a unique candidate path using exclusive file allocation.
// It uses O_CREATE|O_EXCL semantics to ensure no race condition between checking
// existence and creating the file. Existing symlinks are rejected (exclusive create
// fails on symlink). Returns an error if no unique path can be allocated after
// exhausting all numbered suffixes (up to _999).
func uniqueCandidatePath(basePath string) (string, error) {
	// Try default path with exclusive allocation
	if err := tryExclusivePath(basePath); err == nil {
		return basePath, nil
	}

	// Default exists or is symlink — try numbered suffixes
	dir := filepath.Dir(basePath)
	ext := filepath.Ext(basePath)
	name := filepath.Base(basePath[:len(basePath)-len(ext)])

	for i := 2; i <= 999; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s_%d%s", name, i, ext))
		if err := tryExclusivePath(candidate); err == nil {
			return candidate, nil
		}
		// Path exists or is symlink — try next suffix
	}

	// Exhausted all suffixes — return error instead of overwriting
	return "", fmt.Errorf("exhausted candidate paths up to %s_999%s", name, ext)
}

// tryExclusivePath attempts to exclusively claim a path using O_CREATE|O_EXCL.
// Returns nil if path was successfully claimed (file created as placeholder).
// Returns error if path already exists or is a symlink.
func tryExclusivePath(path string) error {
	// O_EXCL fails if file exists OR if path is a symlink (symlink is not followed,
	// and exclusive create on symlink fails on POSIX). This rejects symlink attacks.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	// Successfully claimed — close and remove the empty placeholder
	f.Close()
	return os.Remove(path)
}

// Update downloads and verifies the latest release, writing a SHA-verified candidate file.
// The candidate is always written side-by-side (never in-place) to avoid corrupting
// the running executable. Users must manually replace the binary.
func Update() (*UpdateResult, error) {
	// Fetch latest release
	release, err := FetchLatestRelease("ArcKelMiranda", "yhat-knowledge")
	if err != nil {
		return nil, fmt.Errorf("fetching release: %w", err)
	}

	// Get current version from manifest if available
	currentVersion := "unknown"
	if manifest, err := LoadManifest(); err == nil && manifest != nil {
		currentVersion = manifest.Version
	}

	// Find platform asset
	assetURL, checksumURL, err := FindPlatformAsset(release.Assets, PlatformName())
	if err != nil {
		return nil, fmt.Errorf("finding platform asset: %w", err)
	}

	// Determine asset name from URL
	assetName := filepath.Base(assetURL)

	// Download checksums
	checksumData, err := DownloadFile(checksumURL)
	if err != nil {
		return nil, fmt.Errorf("downloading checksums: %w", err)
	}

	// Parse checksums and find our asset's hash
	checksums, err := ParseChecksums(checksumData)
	if err != nil {
		return nil, fmt.Errorf("parsing checksums: %w", err)
	}

	var expectedHash string
	for _, entry := range checksums {
		if entry.AssetName == assetName {
			expectedHash = entry.Hash
			break
		}
	}

	if expectedHash == "" {
		return nil, fmt.Errorf("asset %s not found in checksums", assetName)
	}

	// Download asset
	assetData, err := DownloadFile(assetURL)
	if err != nil {
		return nil, fmt.Errorf("downloading asset: %w", err)
	}

	// Verify checksum
	hashVerified := VerifyChecksum(assetData, expectedHash)
	if !hashVerified {
		return nil, fmt.Errorf("checksum verification failed for %s", assetName)
	}

	// Determine candidate path - use unique path to avoid overwriting existing candidates
	execPath, err := os.Executable()
	if err != nil {
		execPath = os.Args[0]
	}

	defaultCandidate := execPath + CandidateSuffix
	candidatePath, err := uniqueCandidatePath(defaultCandidate)
	if err != nil {
		return nil, fmt.Errorf("finding unique candidate path: %w", err)
	}

	result := &UpdateResult{
		CurrentVersion: currentVersion,
		LatestVersion: release.TagName,
		AssetURL:      assetURL,
		AssetName:     assetName,
		HashVerified:  true,
		CandidatePath: candidatePath,
	}

	// Write to candidate using exclusive allocation API.
	// This ensures: no overwrites, no symlink attacks, durable write.
	if err := WriteExclusiveAtomically(candidatePath, assetData, 0755); err != nil {
		return nil, fmt.Errorf("writing candidate %s: %w", candidatePath, err)
	}

	// Provide clear manual replacement instructions
	result.Message = fmt.Sprintf("candidate written to: %s\n\nTo complete the update, stop yhat-agent and run:\n\n  mv %s %s\n\nThen restart yhat-agent.", candidatePath, candidatePath, execPath)

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
