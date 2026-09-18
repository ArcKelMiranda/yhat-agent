package yhatagent

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// ReleaseAsset describes a single GitHub release asset.
type ReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// ChecksumEntry describes a parsed SHA-256 checksum line.
type ChecksumEntry struct {
	AssetName string `json:"asset_name"`
	Hash      string `json:"hash"`
}

// assetNameForPlatform returns the expected asset name for a given platform,
// without the checksum suffix.
func assetNameForPlatform(platform string) string {
	switch platform {
	case "linux_amd64":
		return "yhat-agent_linux_amd64"
	case "linux_arm64":
		return "yhat-agent_linux_arm64"
	case "darwin_amd64":
		return "yhat-agent_darwin_amd64"
	case "darwin_arm64":
		return "yhat-agent_darwin_arm64"
	case "windows_amd64":
		return "yhat-agent_windows_amd64.exe"
	default:
		return "yhat-agent_" + platform
	}
}

// checksumNameForPlatform returns the expected checksum asset name.
func checksumNameForPlatform(platform string) string {
	return assetNameForPlatform(platform) + ".sha256sum"
}

// FindPlatformAsset locates the binary and checksum asset URLs for a given platform
// from a list of release assets. It returns the download URLs for both the binary
// and its SHA-256 checksum file.
func FindPlatformAsset(assets []ReleaseAsset, platform string) (assetURL, checksumURL string, err error) {
	expectedBinary := assetNameForPlatform(platform)
	expectedChecksum := checksumNameForPlatform(platform)

	var binaryURL, checksumFileURL string

	for _, a := range assets {
		if a.Name == expectedBinary {
			binaryURL = a.BrowserDownloadURL
		} else if a.Name == expectedChecksum {
			checksumFileURL = a.BrowserDownloadURL
		}
	}

	if binaryURL == "" {
		return "", "", fmt.Errorf("no asset found for platform %q (expected %q)", platform, expectedBinary)
	}
	if checksumFileURL == "" {
		return "", "", fmt.Errorf("no checksum asset found for platform %q (expected %q)", platform, expectedChecksum)
	}

	return binaryURL, checksumFileURL, nil
}

// sha256LineRE matches a SHA-256 hash line in the format produced by sha256sum:
// "<64-hex-chars>  filename"  or  "<64-hex-chars> *filename"
// The hash may be upper or lower case; the separator is two spaces or " *".
var sha256LineRE = regexp.MustCompile(`^(?i)([0-9a-f]{64})\s*[\s*]\s*(.+)$`)

// ParseChecksums parses SHA-256 checksum file content and returns a list of entries.
// Empty lines and comment lines (starting with #) are skipped.
// Invalid hex lengths are skipped.
// Hashes are normalized to lowercase.
func ParseChecksums(content []byte) ([]ChecksumEntry, error) {
	var entries []ChecksumEntry

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		m := sha256LineRE.FindStringSubmatch(line)
		if m == nil {
			// Skip lines that don't match the expected format
			continue
		}

		entries = append(entries, ChecksumEntry{
			Hash:      strings.ToLower(m[1]),
			AssetName: strings.TrimSpace(m[2]),
		})
	}

	return entries, nil
}

// uniqueCandidatePath returns a path that does not exist.
// It checks the defaultCandidate first; if it is already present, it falls
// through to numbered suffixes _2 … _999. Each candidate is probed with
// O_EXCL to atomically detect pre-existing files (including symlinks)
// without following or overwriting them.
func uniqueCandidatePath(defaultCandidate string) (string, error) {
	// The base path strips the CandidateSuffix from defaultCandidate.
	// For "/some/path/yhat-agent.yhat-agent-new", base = "/some/path/yhat-agent".
	base := strings.TrimSuffix(defaultCandidate, CandidateSuffix)
	if base == defaultCandidate {
		base = defaultCandidate
	}

	// Check candidates in order: default, _2, _3, …, _999.
	for i := 1; i <= 999; i++ {
		var candidate string
		if i == 1 {
			candidate = defaultCandidate
		} else {
			candidate = base + "_" + strconv.Itoa(i) + CandidateSuffix
		}
		// O_EXCL fails immediately with EEXIST if the path already exists
		// as a file, directory, or symlink. This is the single-syscall probe.
		f, err := openFileFunc(candidate, os.O_CREATE|os.O_EXCL|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			// Path occupied — try the next suffix.
			continue
		}
		// Successfully claimed this path. Close and remove the placeholder;
		// the caller will write the real binary to this exact path.
		f.Close()
		os.Remove(candidate)
		return candidate, nil
	}

	return "", fmt.Errorf("no available candidate path up to _999 for %s", base)
}

// openFileFunc is the injectable os.OpenFile used by tryExclusivePath.
// Tests reassign this via setOpenFileFunc; production code uses os.OpenFile.
var openFileFunc func(path string, flag int, perm os.FileMode) (*os.File, error) = os.OpenFile

func init() {
	openFileFunc = os.OpenFile
}

// setOpenFileFunc replaces the open-file implementation for testing.
func setOpenFileFunc(fn func(path string, flag int, perm os.FileMode) (*os.File, error)) {
	openFileFunc = fn
}

// resetOpenFileFunc restores the real os.OpenFile after a test.
func resetOpenFileFunc() {
	openFileFunc = os.OpenFile
}
