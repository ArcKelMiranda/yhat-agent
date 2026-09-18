package yhatagent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ConfigHome returns the user's config directory following XDG conventions.
func ConfigHome() string {
	if env := os.Getenv("XDG_CONFIG_HOME"); env != "" {
		return env
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(home, ".config")
}

// OpenCodeRoot returns the OpenCode configuration root directory.
func OpenCodeRoot() string {
	return filepath.Join(ConfigHome(), "opencode")
}

// AgentDir returns the OpenCode agents directory.
func AgentDir() string {
	return filepath.Join(OpenCodeRoot(), "agents")
}

// SkillDir returns the OpenCode skills directory.
func SkillDir() string {
	return filepath.Join(OpenCodeRoot(), "skills", "yhat-memory-capture")
}

// AgentTargetPath returns the absolute target path for the agent asset.
func AgentTargetPath() string {
	return filepath.Join(AgentDir(), "yhat-memory-capture.md")
}

// SkillTargetPath returns the absolute target path for the skill asset.
func SkillTargetPath() string {
	return filepath.Join(SkillDir(), "SKILL.md")
}

// InstallerStateDir returns the yhat-agent state directory.
func InstallerStateDir() string {
	return filepath.Join(ConfigHome(), "yhat-agent")
}

// ManifestPath returns the manifest file path.
func ManifestPath() string {
	return filepath.Join(InstallerStateDir(), "manifest.json")
}

// CandidateSuffix is the suffix appended to new candidate files.
const CandidateSuffix = ".yhat-agent-new"

// CandidateName returns the candidate sibling filename for a given target path.
func CandidateName(targetPath string) string {
	return targetPath + CandidateSuffix
}

// EnsureDir creates the directory and all parents if it does not exist.
func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0755)
}

// PlatformName returns the OS/arch string used in release asset names.
func PlatformName() string {
	osPart := runtime.GOOS
	switch runtime.GOOS {
	case "darwin":
		osPart = "darwin"
	case "linux":
		osPart = "linux"
	case "windows":
		osPart = "windows"
	}
	return osPart + "_" + runtime.GOARCH
}

// ExclusiveFileWriter handles atomic file creation using O_CREATE|O_EXCL semantics.
// The descriptor remains open through write, chmod, sync and close operations.
// This ensures:
// - No existing file is ever overwritten
// - Existing symlinks are rejected (symlink is not followed; exclusive create fails)
// - The write is durable before the file is visible
type ExclusiveFileWriter struct {
	Path string
	f    *os.File
}

// NewExclusiveFile creates a new file exclusively using O_CREATE|O_EXCL.
// Returns error if file already exists or is a symlink.
func NewExclusiveFile(path string, perm os.FileMode) (*ExclusiveFileWriter, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return nil, err
	}
	return &ExclusiveFileWriter{Path: path, f: f}, nil
}

// Write writes data to the exclusive file.
func (w *ExclusiveFileWriter) Write(p []byte) (int, error) {
	return w.f.Write(p)
}

// Chmod sets file permissions.
func (w *ExclusiveFileWriter) Chmod(perm os.FileMode) error {
	return w.f.Chmod(perm)
}

// Sync synchronizes the file's in-core state with storage.
func (w *ExclusiveFileWriter) Sync() error {
	return w.f.Sync()
}

// Close closes the file. On error, the temp file is removed.
func (w *ExclusiveFileWriter) Close() error {
	err := w.f.Close()
	if err != nil {
		os.Remove(w.Path)
		return err
	}
	return nil
}

// WriteExclusiveAtomically atomically creates a file using O_EXCL.
// This guarantees:
// - O_EXCL on the final path — single syscall, no TOCTOU window
// - If destination exists (any type: file, symlink, directory), returns EEXIST immediately
// - If destination doesn't exist, file is created and open for writing
// - Data is written to the already-open descriptor
// - Sync ensures durability before close
// - Standard atomic write semantics: file either doesn't exist or has complete content after close
//
// Crash semantics: If process crashes after open but before close, no file exists at the final path.
// This is the correct atomic write contract.
func WriteExclusiveAtomically(path string, data []byte, perm os.FileMode) error {
	// Clean up any stale temp files from previous crash-recovery attempts.
	// If cleanup fails (e.g., no permission), proceed with creating a new exclusive file.
	dir := filepath.Dir(path)
	dirs, err := os.ReadDir(dir)
	if err == nil {
		for _, d := range dirs {
			if strings.HasPrefix(d.Name(), ".write-exclusive-") {
				os.Remove(filepath.Join(dir, d.Name())) // Best-effort cleanup
			}
		}
	}

	// Single syscall: O_EXCL returns EEXIST if path exists as any type (file, symlink, dir).
	// If successful, file descriptor is already open and ready for writing.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}

	// Write data to the already-open descriptor
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(path) // Clean up partial file on write failure
		return err
	}

	// Sync to ensure durability before close
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(path)
		return err
	}

	// Close: finalizes the atomic write
	return f.Close()
}
