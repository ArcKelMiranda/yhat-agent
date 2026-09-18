package yhatagent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

// UninstallResult describes the outcome of an uninstall operation.
type UninstallResult struct {
	Removed []string
	Skipped []SkippedFile
	Errors  []string
}

// SkippedFile describes a file that was not removed.
type SkippedFile struct {
	Path   string
	Reason string
}

// UninstallPlan returns the planned actions for uninstall without making changes.
func UninstallPlan() (*UninstallPlanResult, error) {
	if !HasManifest() {
		return &UninstallPlanResult{
			Skipped: []SkippedFile{{
				Path:   ManifestPath(),
				Reason: "no manifest found",
			}},
		}, nil
	}

	manifest, err := LoadManifest()
	if err != nil {
		return nil, fmt.Errorf("loading manifest: %w", err)
	}

	plan := &UninstallPlanResult{}

	for assetKey, asset := range manifest.Assets {
		targetPath := asset.TargetPath
		if targetPath == "" {
			targetPath = manifest.TargetFor(assetKey)
		}

		info, err := os.Stat(targetPath)
		if os.IsNotExist(err) {
			continue
		} else if err != nil {
			plan.Skipped = append(plan.Skipped, SkippedFile{
				Path:   targetPath,
				Reason: fmt.Sprintf("stat error: %v", err),
			})
			continue
		}

		if info.IsDir() {
			plan.Skipped = append(plan.Skipped, SkippedFile{
				Path:   targetPath,
				Reason: "is a directory",
			})
			continue
		}

		// Check if file has been modified
		if manifest.IsModified(assetKey) {
			plan.Skipped = append(plan.Skipped, SkippedFile{
				Path:   targetPath,
				Reason: "manually modified",
			})
			continue
		}

		// Verify file matches what we installed
		if err := verifyFileIntegrity(targetPath, asset.InstalledHash); err != nil {
			// Integrity mismatch in dry-run: report as error-level, not just skipped
			plan.Skipped = append(plan.Skipped, SkippedFile{
				Path:   targetPath,
				Reason: fmt.Sprintf("INTEGRITY ERROR: %v", err),
			})
			continue
		}

		// Would be removed
		plan.Planned = append(plan.Planned, targetPath)
	}

	return plan, nil
}

// UninstallPlanResult describes what uninstall would do without making changes.
type UninstallPlanResult struct {
	Planned []string
	Skipped []SkippedFile
}

// Uninstall removes only unchanged files recorded in the manifest.
// It requires explicit --yes confirmation.
//
// Manifest persistence policy:
// - If ALL managed entries are successfully removed → remove manifest entirely
// - If ANY managed entry remains (removed + skipped + error) → persist reduced manifest
// - Removed entries are deleted from the manifest atomically after successful removals
func Uninstall(confirmed bool) (*UninstallResult, error) {
	if !confirmed {
		return nil, fmt.Errorf("uninstall requires explicit --yes confirmation")
	}

	if !HasManifest() {
		return &UninstallResult{
			Skipped: []SkippedFile{{
				Path:   ManifestPath(),
				Reason: "no manifest found",
			}},
		}, nil
	}

	manifest, err := LoadManifest()
	if err != nil {
		return nil, fmt.Errorf("loading manifest: %w", err)
	}

	result := &UninstallResult{}
	// Track removed asset keys to build reduced manifest
	var removedAssetKeys []string

	for assetKey, asset := range manifest.Assets {
		targetPath := asset.TargetPath
		if targetPath == "" {
			targetPath = manifest.TargetFor(assetKey)
		}

		info, err := os.Stat(targetPath)
		if os.IsNotExist(err) {
			// Already gone — treat as removed for manifest purposes
			result.Removed = append(result.Removed, targetPath)
			removedAssetKeys = append(removedAssetKeys, assetKey)
			continue
		} else if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("stat %s: %v", targetPath, err))
			continue
		}

		if info.IsDir() {
			result.Skipped = append(result.Skipped, SkippedFile{
				Path:   targetPath,
				Reason: "is a directory",
			})
			continue
		}

		// Check if file has been modified
		if manifest.IsModified(assetKey) {
			result.Skipped = append(result.Skipped, SkippedFile{
				Path:   targetPath,
				Reason: "manually modified",
			})
			continue
		}

		// Verify file matches what we installed (not just content hash, but installed hash)
		if err := verifyFileIntegrity(targetPath, asset.InstalledHash); err != nil {
			// Integrity mismatch is an explicit error — not merely skipped.
			// This signals that the file may have been tampered with or corrupted,
		// and we cannot safely remove it without risking damage.
			result.Errors = append(result.Errors, fmt.Sprintf("%s: integrity mismatch: %v", targetPath, err))
			// Do NOT continue here — manifest entry is preserved for investigation
			continue
		}

		// Safe to remove
		if err := os.Remove(targetPath); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("removing %s: %v", targetPath, err))
			// Preserve state for other files even if this removal fails
			continue
		}

		result.Removed = append(result.Removed, targetPath)
		removedAssetKeys = append(removedAssetKeys, assetKey)

		// Also remove empty parent directories if they were created by install
		// but only if they contain no other files
		removeEmptyParents(targetPath, AgentDir(), SkillDir())
	}

	// Determine what remains in manifest after this operation
	remainingCount := len(manifest.Assets) - len(removedAssetKeys)

	if remainingCount > 0 {
		// Some managed entries remain — reduce manifest atomically by removing
		// entries for successfully removed files.
		for _, key := range removedAssetKeys {
			delete(manifest.Assets, key)
		}
		// Persist reduced manifest; if this fails, the error is reported
		if err := manifest.Save(); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("saving reduced manifest: %v", err))
		}
	} else {
		// All managed entries removed — remove manifest entirely
		if err := os.Remove(ManifestPath()); err != nil && !os.IsNotExist(err) {
			result.Errors = append(result.Errors, fmt.Sprintf("removing manifest: %v", err))
		}
	}

	return result, nil
}

// verifyFileIntegrity checks if a file's current hash matches the expected hash.
func verifyFileIntegrity(filePath, expectedHash string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	hash := sha256.Sum256(data)
	actualHex := hex.EncodeToString(hash[:])

	if actualHex != expectedHash {
		return fmt.Errorf("hash mismatch: expected %s, got %s", expectedHash, actualHex)
	}

	return nil
}

// removeEmptyParents removes parent directories up to (but not including) the stop dirs.
func removeEmptyParents(filePath string, stopDirs ...string) {
	dir := filePath

	for {
		parent := ""
		for i := len(dir) - 1; i >= 0; i-- {
			if dir[i] == '/' {
				parent = dir[:i]
				break
			}
		}
		if parent == "" {
			break
		}

		// Check if this is a stop directory
		isStop := false
		for _, stop := range stopDirs {
			if parent == stop || parent == stop+"/" {
				isStop = true
				break
			}
		}
		if isStop {
			break
		}

		// Check if directory is empty
		entries, err := os.ReadDir(parent)
		if err != nil || len(entries) > 0 {
			break
		}

		os.Remove(parent)
		dir = parent
	}
}
