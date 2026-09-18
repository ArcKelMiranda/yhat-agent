package yhatagent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// InstallResult describes the outcome of installing one asset.
type InstallResult struct {
	AssetKey   string
	TargetPath string
	State     InstallState
	Message   string
}

// Install copies the embedded assets to their target locations.
// It handles collisions by writing a .yhat-agent-new sibling file.
// Returns an error if any asset fails to install (non-idempotent case).
func Install() ([]InstallResult, error) {
	if err := EnsureDir(AgentDir()); err != nil {
		return nil, fmt.Errorf("ensuring agent dir: %w", err)
	}
	if err := EnsureDir(SkillDir()); err != nil {
		return nil, fmt.Errorf("ensuring skill dir: %w", err)
	}

	manifest, err := NewManifest()
	if err != nil {
		return nil, fmt.Errorf("creating manifest: %w", err)
	}

	var results []InstallResult
	var installErrors []string

	for _, assetKey := range AssetPaths() {
		result := InstallResult{AssetKey: assetKey}
		data, err := Assets.ReadFile(assetKey)
		if err != nil {
			result.State = StateMissing
			result.Message = fmt.Sprintf("reading embedded asset: %v", err)
			results = append(results, result)
			installErrors = append(installErrors, fmt.Sprintf("%s: %v", assetKey, err))
			continue
		}

		targetPath := manifest.TargetFor(assetKey)
		result.TargetPath = targetPath

		embeddedHash := sha256.Sum256(data)
		embeddedHex := hex.EncodeToString(embeddedHash[:])

		// Use Lstat to detect symlinks without following them.
		// os.Stat would follow symlinks, causing a directory target to be
		// misidentified and os.WriteFile to fail with EISDIR.
		info, err := os.Lstat(targetPath)
		if err != nil {
			// File does not exist — use exclusive allocation (atomic, no overwrites).
			if err := WriteExclusiveAtomically(targetPath, data, 0644); err != nil {
				result.State = StateMissing
				result.Message = fmt.Sprintf("writing file: %v", err)
				installErrors = append(installErrors, fmt.Sprintf("%s: %v", assetKey, err))
			} else {
				result.State = StateInstalled
				result.Message = "installed"
				if asset, ok := manifest.Assets[assetKey]; ok {
					asset.InstalledHash = embeddedHex
					asset.Modified = false
					manifest.Assets[assetKey] = asset
				}
			}
		} else if info.Mode()&os.ModeSymlink != 0 {
			// Symlink at target — do not follow or overwrite.
			result.State = StateMissing
			result.Message = "target is a symlink; refusing to install over a symlink"
			results = append(results, result)
			installErrors = append(installErrors, fmt.Sprintf("%s: target is a symlink", assetKey))
			continue
		} else if info.IsDir() {
			result.State = StateMissing
			result.Message = "target is a directory, not a file"
			results = append(results, result)
			installErrors = append(installErrors, fmt.Sprintf("%s: target is a directory", assetKey))
			continue
		} else {
			// File exists — check for drift
			existingData, err := os.ReadFile(targetPath)
			if err != nil {
				result.State = StateMissing
				result.Message = fmt.Sprintf("reading existing file: %v", err)
				results = append(results, result)
				installErrors = append(installErrors, fmt.Sprintf("%s: %v", assetKey, err))
				continue
			}

			existingHash := sha256.Sum256(existingData)
			existingHex := hex.EncodeToString(existingHash[:])

			if existingHex == embeddedHex {
				// Already matches embedded content
				result.State = StateInstalled
				result.Message = "already installed"
				if asset, ok := manifest.Assets[assetKey]; ok {
					asset.InstalledHash = existingHex
					asset.Modified = false
					manifest.Assets[assetKey] = asset
				}
			} else {
				// Content differs — check if it matches the manifest's original hash
				manifestHash := manifest.Assets[assetKey].ContentHash

				if existingHex == manifestHash {
					// Matches what we originally installed — same as installed
					result.State = StateInstalled
					result.Message = "reinstalled"
					if asset, ok := manifest.Assets[assetKey]; ok {
						asset.InstalledHash = existingHex
						asset.Modified = false
						manifest.Assets[assetKey] = asset
					}
				} else {
					// Manual drift — write candidate sibling using exclusive allocation.
					// This ensures no overwrites and rejects symlink attacks.
					candidatePath := CandidateName(targetPath)
					if err := WriteExclusiveAtomically(candidatePath, data, 0644); err != nil {
						// Idempotent retry: if the candidate already exists with matching
						// content, treat it as success. This covers the case where the user
						// re-runs install after receiving the candidate and takes no action;
						// the candidate already holds the correct embedded content.
						if candidateData, readErr := os.ReadFile(candidatePath); readErr == nil {
							candidateHash := sha256.Sum256(candidateData)
							candidateHex := hex.EncodeToString(candidateHash[:])
							if candidateHex == embeddedHex {
								result.State = StateInstalled
								result.Message = "candidate already exists with correct content"
								if asset, ok := manifest.Assets[assetKey]; ok {
									asset.InstalledHash = existingHex
									asset.Modified = true
									manifest.Assets[assetKey] = asset
								}
							} else {
								result.State = StateDrift
								result.Message = fmt.Sprintf("drift detected, failed to write candidate %s: %v", candidatePath, err)
								installErrors = append(installErrors, fmt.Sprintf("%s: %v", assetKey, err))
								if asset, ok := manifest.Assets[assetKey]; ok {
									asset.InstalledHash = existingHex
									asset.Modified = true
									manifest.Assets[assetKey] = asset
								}
							}
						} else {
							result.State = StateDrift
							result.Message = fmt.Sprintf("drift detected, failed to write candidate %s: %v", candidatePath, err)
							installErrors = append(installErrors, fmt.Sprintf("%s: %v", assetKey, err))
							if asset, ok := manifest.Assets[assetKey]; ok {
								asset.InstalledHash = existingHex
								asset.Modified = true
								manifest.Assets[assetKey] = asset
							}
						}
					} else {
						result.State = StateDrift
						result.Message = fmt.Sprintf("drift detected, candidate written to %s", candidatePath)
						if asset, ok := manifest.Assets[assetKey]; ok {
							asset.InstalledHash = existingHex
							asset.Modified = true
							manifest.Assets[assetKey] = asset
						}
					}
				}
			}
		}

		results = append(results, result)
	}

	if err := manifest.Save(); err != nil {
		return results, fmt.Errorf("saving manifest: %w", err)
	}

	// Return error if any asset install failed
	if len(installErrors) > 0 {
		return results, fmt.Errorf("install errors: %v", installErrors)
	}

	return results, nil
}

// CopyFile copies a file from src to dst using io.Copy.
func CopyFile(dst, src string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return err
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dst, srcInfo.Mode())
}
