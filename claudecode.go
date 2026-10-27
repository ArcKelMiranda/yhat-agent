package yhatagent

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ClaudeCodeRoot returns the Claude Code configuration root directory.
// It honors CLAUDE_CONFIG_DIR, the same override Claude Code itself uses.
func ClaudeCodeRoot() string {
	if env := os.Getenv("CLAUDE_CONFIG_DIR"); env != "" {
		return env
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.Getenv("HOME"), ".claude")
	}
	return filepath.Join(home, ".claude")
}

// ClaudeCodeSkillTargetPath returns the absolute target path of the skill for Claude Code.
func ClaudeCodeSkillTargetPath() string {
	return filepath.Join(ClaudeCodeRoot(), "skills", "yhat-memory-capture", "SKILL.md")
}

const claudeCodeSkillDescription = "Capture durable YHat operational knowledge (business rules, decisions, patterns, mappings) " +
	"silently into Engram with enrichment queue, hygiene gate and compliance scoring. " +
	"Use when the user states YHat knowledge worth keeping, or asks to capture, audit or bootstrap YHat knowledge."

// ClaudeCodeSkill adapts the embedded skill to Claude Code: the OpenCode
// frontmatter triggers on an agent being selected, which Claude Code lacks,
// so the description is rewritten and the OpenCode-only compatibility key dropped.
func ClaudeCodeSkill(src []byte) ([]byte, error) {
	rest, ok := bytes.CutPrefix(src, []byte("---\n"))
	if !ok {
		return nil, fmt.Errorf("skill has no frontmatter")
	}
	front, body, ok := strings.Cut(string(rest), "\n---\n")
	if !ok {
		return nil, fmt.Errorf("skill frontmatter is not closed")
	}
	var out []string
	for _, line := range strings.Split(front, "\n") {
		switch {
		case strings.HasPrefix(line, "description:"):
			out = append(out, fmt.Sprintf("description: %q", claudeCodeSkillDescription))
		case strings.HasPrefix(line, "compatibility:"):
		default:
			out = append(out, line)
		}
	}
	return []byte("---\n" + strings.Join(out, "\n") + "\n---\n" + body), nil
}

// InstallClaudeCodeSkill writes the Claude Code skill. It never overwrites a
// file that differs: a drifted target gets a CandidateSuffix sibling instead.
func InstallClaudeCodeSkill() (InstallResult, error) {
	result := InstallResult{AssetKey: "claude-code:" + SkillAsset, TargetPath: ClaudeCodeSkillTargetPath()}
	src, err := Assets.ReadFile(SkillAsset)
	if err != nil {
		return result, fmt.Errorf("reading embedded skill: %w", err)
	}
	data, err := ClaudeCodeSkill(src)
	if err != nil {
		return result, err
	}
	if err := EnsureDir(filepath.Dir(result.TargetPath)); err != nil {
		return result, fmt.Errorf("ensuring Claude Code skill dir: %w", err)
	}

	info, err := os.Lstat(result.TargetPath)
	if err != nil {
		if err := WriteExclusiveAtomically(result.TargetPath, data, 0644); err != nil {
			return result, fmt.Errorf("writing skill: %w", err)
		}
		result.State, result.Message = StateInstalled, "installed"
		return result, nil
	}
	if !info.Mode().IsRegular() {
		return result, fmt.Errorf("%s is not a regular file; refusing to install over it", result.TargetPath)
	}
	existing, err := os.ReadFile(result.TargetPath)
	if err != nil {
		return result, fmt.Errorf("reading existing skill: %w", err)
	}
	if bytes.Equal(existing, data) {
		result.State, result.Message = StateInstalled, "already installed"
		return result, nil
	}

	candidate := CandidateName(result.TargetPath)
	result.State = StateCandidate
	if err := WriteExclusiveAtomically(candidate, data, 0644); err != nil {
		if prev, readErr := os.ReadFile(candidate); readErr != nil || !bytes.Equal(prev, data) {
			return result, fmt.Errorf("writing candidate %s: %w", candidate, err)
		}
	}
	result.Message = "target differs; new version written to " + candidate
	return result, nil
}

// ClaudeCodeStatus reports the state of the Claude Code skill without changes.
func ClaudeCodeStatus() (FileInfo, error) {
	info := FileInfo{AssetKey: "claude-code:" + SkillAsset, TargetPath: ClaudeCodeSkillTargetPath()}
	src, err := Assets.ReadFile(SkillAsset)
	if err != nil {
		return info, err
	}
	want, err := ClaudeCodeSkill(src)
	if err != nil {
		return info, err
	}
	got, err := os.ReadFile(info.TargetPath)
	switch {
	case os.IsNotExist(err):
		info.State, info.Message = StateMissing, "not installed"
	case err != nil:
		info.State, info.Message = StateUnknown, fmt.Sprintf("error reading file: %v", err)
	case bytes.Equal(got, want):
		info.State, info.Message = StateInstalled, "up to date"
	default:
		info.State, info.Message = StateDrift, "differs from embedded skill"
	}
	if _, err := os.Stat(CandidateName(info.TargetPath)); err == nil {
		info.State, info.Message = StateCandidate, "candidate exists at "+CandidateName(info.TargetPath)
	}
	return info, nil
}

// UninstallClaudeCodeSkill removes the Claude Code skill only when it still
// matches the embedded version, so user edits are never deleted. It also
// removes a stale candidate sibling. The returned string is a skip reason, or
// empty when the skill was removed or absent.
func UninstallClaudeCodeSkill() (string, error) {
	fi, err := ClaudeCodeStatus()
	if err != nil {
		return "", err
	}
	target := fi.TargetPath
	if _, statErr := os.Stat(target); os.IsNotExist(statErr) {
		return "", nil
	}
	if fi.State == StateDrift || fi.State == StateUnknown || (fi.State == StateCandidate && !fileMatchesClaudeSkill(target)) {
		return "modified or unreadable; left in place", nil
	}
	if err := os.Remove(target); err != nil {
		return "", err
	}
	os.Remove(CandidateName(target))
	removeEmptyParents(target, filepath.Join(ClaudeCodeRoot(), "skills"))
	return "", nil
}

func fileMatchesClaudeSkill(path string) bool {
	src, err := Assets.ReadFile(SkillAsset)
	if err != nil {
		return false
	}
	want, err := ClaudeCodeSkill(src)
	if err != nil {
		return false
	}
	got, err := os.ReadFile(path)
	return err == nil && bytes.Equal(got, want)
}
