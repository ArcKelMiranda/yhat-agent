// Package config provides typed access to the Cerebro YHat v1.2 operator
// configuration (config.yaml) and runtime state (state.json).
//
// # Path hierarchy
//
// The F1 home root is determined by, in order of precedence:
//
//  1. $YHAT_HOME environment variable (set by the operator).
//     When set, all paths are relative to this root.
//  2. $HOME/.yhat/ on Linux / macOS.
//  3. %USERPROFILE%\.yhat\ on Windows.
//
// Inside the root:
//
//	config.yaml  — operator name and central repo URL (human-editable YAML)
//	state.json   — installed version, registered clients, schema version,
//	               last sync timestamp (machine-written JSON)
//	yhat.db      — SQLite store (opened by the install/status commands)
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

// Config holds the operator-supplied Cerebro configuration.
// It is serialised to YAML at $YHAT_HOME/config.yaml.
type Config struct {
	Operator    string `yaml:"operator"`
	CentralRepo string `yaml:"central_repo"`
	LastSync    string `yaml:"last_sync"`
}

// State holds the runtime state written by the install command.
// It is serialised to JSON at $YHAT_HOME/state.json.
type State struct {
	Version           string   `json:"version"`
	RegisteredClients []string `json:"registered_clients"`
	SchemaVersion     int      `json:"schema_version"`
	LastSync          string   `json:"last_sync"`
}

// Load reads a Config from a YAML file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yamlUnmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Save writes a Config to a YAML file atomically.
func (c *Config) Save(path string) error {
	data, err := yamlMarshal(c)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data, 0644)
}

// LoadState reads a State from a JSON file.
func LoadState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// Save writes a State to a JSON file atomically.
func (s *State) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data, 0644)
}

// EnsureDir creates the directory and all parents if it does not exist.
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

// DefaultHome returns the platform-appropriate F1 home root.
//
//	YHAT_HOME env > $HOME/.yhat (Linux/macOS) > %USERPROFILE%\.yhat\ (Windows)
func DefaultHome() string {
	if home := os.Getenv("YHAT_HOME"); home != "" {
		return home
	}
	if runtime.GOOS == "windows" {
		if userProfile := os.Getenv("USERPROFILE"); userProfile != "" {
			return filepath.Join(userProfile, ".yhat")
		}
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".yhat")
	}
	// Fallback — should not reach here on a real system.
	return ".yhat"
}

// DBPath returns the path to the SQLite store inside the F1 home root.
func DBPath() string {
	return filepath.Join(DefaultHome(), "yhat.db")
}

// ConfigPath returns the path to the config.yaml inside the F1 home root.
func ConfigPath() string {
	return filepath.Join(DefaultHome(), "config.yaml")
}

// StatePath returns the path to the state.json inside the F1 home root.
func StatePath() string {
	return filepath.Join(DefaultHome(), "state.json")
}

// writeFileAtomic writes data to a temporary file in the same directory and
// then atomically renames it to the target path. On failure the temp file is
// removed. This avoids partial-write reads by consumers.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".config-tmp-")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	removeTmp := func() { os.Remove(tmpPath) }
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		removeTmp()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		removeTmp()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		removeTmp()
		return err
	}
	if err := tmp.Close(); err != nil {
		removeTmp()
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		removeTmp()
		return err
	}
	return nil
}
