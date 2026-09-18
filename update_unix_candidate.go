//go:build linux || darwin

package yhatagent

import (
	"path/filepath"
)

// WindowsCandidatePath returns the side-by-side candidate path for Windows.
// On Unix systems this is equivalent to the standard CandidateName,
// but is provided for cross-platform test compatibility.
func WindowsCandidatePath(execPath string) string {
	dir := filepath.Dir(execPath)
	ext := filepath.Ext(execPath)
	name := filepath.Base(execPath[:len(execPath)-len(ext)])
	return filepath.Join(dir, name+"_new"+ext)
}
