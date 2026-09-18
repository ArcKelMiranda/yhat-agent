// Package yhatagent provides a self-contained CLI for installing and managing
// the YHat OpenCode agent and skill assets.
package yhatagent

import (
	"embed"
	_ "embed"
)

// Assets holds the embedded OpenCode agent and skill files.
//
//go:embed assets/*
var Assets embed.FS

// Asset names used for installation targets.
const (
	AgentAsset  = "assets/agents/yhat-memory-capture.md"
	SkillAsset = "assets/skills/yhat-memory-capture/SKILL.md"
)

// AssetPaths returns the list of relative asset paths in the embedded bundle.
func AssetPaths() []string {
	return []string{AgentAsset, SkillAsset}
}
