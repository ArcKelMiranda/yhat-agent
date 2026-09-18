package yhatagent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

// StatusResult describes the state of all managed assets.
type StatusResult struct {
	HasManifest  bool
	ManifestPath string
	Files        []FileInfo
	Candidates   []string // paths with .yhat-agent-new siblings
	HasCandidate bool
}

// Status reports the current installation state without making changes.
func Status() (*StatusResult, error) {
	result := &StatusResult{
		ManifestPath: ManifestPath(),
		HasManifest:  HasManifest(),
	}

	// Check for candidate files in standard locations
	standardDirs := []string{AgentDir(), SkillDir()}
	for _, dir := range standardDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if !os.IsNotExist(err) {
				continue
			}
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if len(name) > len(CandidateSuffix) && name[len(name)-len(CandidateSuffix):] == CandidateSuffix {
				candidatePath := dir + "/" + name
				result.Candidates = append(result.Candidates, candidatePath)
				result.HasCandidate = true
			}
		}
	}

	if !result.HasManifest {
		// No manifest — check if files exist but are untracked
		for _, assetKey := range AssetPaths() {
			var targetPath string
			switch assetKey {
			case AgentAsset:
				targetPath = AgentTargetPath()
			case SkillAsset:
				targetPath = SkillTargetPath()
			}

			info := FileInfo{
				AssetKey:   assetKey,
				TargetPath: targetPath,
			}

			if _, err := os.Stat(targetPath); os.IsNotExist(err) {
				info.State = StateMissing
				info.Message = "not installed"
			} else {
				info.State = StateUnknown
				info.Message = "installed but not managed by yhat-agent"
			}
			result.Files = append(result.Files, info)
		}
		return result, nil
	}

	manifest, err := LoadManifest()
	if err != nil {
		return nil, fmt.Errorf("loading manifest: %w", err)
	}

	for _, assetKey := range AssetPaths() {
		info := FileInfo{
			AssetKey: assetKey,
		}

		asset, ok := manifest.Assets[assetKey]
		if !ok {
			info.State = StateUnknown
			info.Message = "not in manifest"
			result.Files = append(result.Files, info)
			continue
		}

		info.TargetPath = asset.TargetPath

		_, err := os.Stat(asset.TargetPath)
		if os.IsNotExist(err) {
			info.State = StateMissing
			info.Message = "file not found"
			result.Files = append(result.Files, info)
			continue
		}

		// File exists — compute current hash
		data, err := os.ReadFile(asset.TargetPath)
		if err != nil {
			info.State = StateUnknown
			info.Message = fmt.Sprintf("error reading file: %v", err)
			result.Files = append(result.Files, info)
			continue
		}

		currentHash := sha256.Sum256(data)
		currentHex := hex.EncodeToString(currentHash[:])
		info.Hash = currentHex

		// Determine state
		if currentHex == asset.ContentHash {
			info.State = StateInstalled
			info.Message = "up to date"
		} else if currentHex == asset.InstalledHash {
			info.State = StateInstalled
			info.Message = "matches original install"
		} else {
			info.State = StateDrift
			info.Message = "manually modified"
		}

		// Check for candidate sibling
		candidatePath := CandidateName(asset.TargetPath)
		if _, err := os.Stat(candidatePath); err == nil {
			info.State = StateCandidate
			info.Message = fmt.Sprintf("candidate exists at %s", candidatePath)
		}

		result.Files = append(result.Files, info)
	}

	return result, nil
}
