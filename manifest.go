package yhatagent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Manifest records the installed asset state managed by yhat-agent.
type Manifest struct {
	Version     string           `json:"version"`
	InstalledAt time.Time        `json:"installed_at"`
	Assets      map[string]Asset `json:"assets"`
}

// Asset records the state of a single managed file.
type Asset struct {
	TargetPath    string    `json:"target_path"`
	ContentHash   string    `json:"content_hash"`   // SHA-256 of embedded content at install time
	InstalledHash string    `json:"installed_hash"` // SHA-256 of actual file content
	Modified      bool      `json:"modified"`       // true if file was changed after install
	ModifiedAt    time.Time `json:"modified_at,omitempty"`
}

// NewManifest creates a fresh manifest with the current embedded asset hashes.
func NewManifest() (*Manifest, error) {
	m := &Manifest{
		Version:     "1",
		InstalledAt: time.Now().UTC(),
		Assets:      make(map[string]Asset),
	}
	for _, path := range AssetPaths() {
		data, err := Assets.ReadFile(path)
		if err != nil {
			return nil, err
		}
		hash := sha256.Sum256(data)
		asset := Asset{
			ContentHash: hex.EncodeToString(hash[:]),
		}
		switch path {
		case AgentAsset:
			asset.TargetPath = AgentTargetPath()
		case SkillAsset:
			asset.TargetPath = SkillTargetPath()
		}
		m.Assets[path] = asset
	}
	return m, nil
}

// Save writes the manifest atomically to its standard location.
// It writes to a same-directory temp file, flushes, and atomically replaces
// the manifest. Temp files are cleaned up on failure regardless of which step
// failed. This is safe for installer state only (not concurrent access).
func (m *Manifest) Save() (saveErr error) {
	if err := EnsureDir(InstallerStateDir()); err != nil {
		return err
	}
	manifestPath := ManifestPath()

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	// Write to same-directory temp file with safe permissions
	dir := filepath.Dir(manifestPath)
	tmp, err := os.CreateTemp(dir, ".manifest-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	// Track cleanup state. Set to true immediately after temp creation so that
	// any failure after creation — write, chmod, sync, close, or rename — triggers
	// cleanup. Setting it before CreateTemp is also safe (harmless no-op cleanup
	// when temp creation fails and tmpPath is "").
	cleanupNeeded := true
	cleanup := func() {
		if cleanupNeeded {
			os.Remove(tmpPath)
		}
	}
	defer cleanup()

	// Write data
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		saveErr = fmt.Errorf("writing temp manifest: %w", err)
		return
	}

	// Set safe permissions before sync/close
	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		saveErr = fmt.Errorf("setting temp permissions: %w", err)
		return
	}

	// Sync to disk
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		saveErr = fmt.Errorf("syncing temp manifest: %w", err)
		return
	}

	// Close
	if err := tmp.Close(); err != nil {
		saveErr = fmt.Errorf("closing temp manifest: %w", err)
		return
	}

	// Atomic replace: rename is atomic on POSIX filesystems
	if err := os.Rename(tmpPath, manifestPath); err != nil {
		saveErr = fmt.Errorf("atomically replacing manifest: %w", err)
		return
	}

	// Success — mark cleanup as not needed (temp is now manifest)
	cleanupNeeded = false
	return nil
}

// LoadManifest reads the manifest from its standard location.
func LoadManifest() (*Manifest, error) {
	data, err := os.ReadFile(ManifestPath())
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// UpdateInstalledHash records the current file hash for an asset.
func (m *Manifest) UpdateInstalledHash(assetKey, filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(data)
	if asset, ok := m.Assets[assetKey]; ok {
		asset.InstalledHash = hex.EncodeToString(hash[:])
		asset.Modified = asset.ContentHash != asset.InstalledHash
		if asset.Modified {
			asset.ModifiedAt = time.Now().UTC()
		}
		m.Assets[assetKey] = asset
	}
	return nil
}

// IsModified returns true if the file at targetPath has been changed since install.
func (m *Manifest) IsModified(assetKey string) bool {
	asset, ok := m.Assets[assetKey]
	if !ok {
		return false
	}
	return asset.Modified
}

// TargetFor returns the target path for a given asset key.
func (m *Manifest) TargetFor(assetKey string) string {
	if asset, ok := m.Assets[assetKey]; ok {
		return asset.TargetPath
	}
	return ""
}

// HasManifest returns true if a manifest file exists.
func HasManifest() bool {
	_, err := os.Stat(ManifestPath())
	return err == nil
}

// InstallState describes the current state of a managed asset.
type InstallState int

const (
	StateMissing InstallState = iota
	StateInstalled
	StateDrift
	StateCandidate
	StateUnknown
)

// String returns a human-readable state label.
func (s InstallState) String() string {
	switch s {
	case StateMissing:
		return "missing"
	case StateInstalled:
		return "installed"
	case StateDrift:
		return "drift"
	case StateCandidate:
		return "candidate"
	default:
		return "unknown"
	}
}

// FileInfo describes the runtime state of a managed file.
type FileInfo struct {
	AssetKey   string       `json:"asset_key"`
	State      InstallState `json:"state"`
	TargetPath string       `json:"target_path"`
	Hash       string       `json:"hash,omitempty"`
	Message    string       `json:"message,omitempty"`
}
