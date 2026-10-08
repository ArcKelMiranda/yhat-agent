// Package main provides the yhat-agent CLI.
package main

import (
	"fmt"
	"os"
	"runtime"

	yhatagent "github.com/ArcKelMiranda/yhat-agent"
	yhatclients "github.com/ArcKelMiranda/yhat-agent/internal/clients"
)

// realEnv implements yhatclients.Env using real OS queries.
type realEnv struct{}

// OpenCodeRoot delegates to the library function to avoid duplicating logic.
func (realEnv) OpenCodeRoot() string {
	return yhatagent.OpenCodeRoot()
}

func (realEnv) OpenCodeConfig() string {
	return os.Getenv("OPENCODE_CONFIG")
}

func (realEnv) AppData() string {
	return os.Getenv("APPDATA")
}

func (realEnv) LocalAppData() string {
	return os.Getenv("LOCALAPPDATA")
}

func (realEnv) Executable() (string, error) {
	return os.Executable()
}

// runInstallMCP registers the MCP server with OpenCode and Claude Desktop.
// It is called after the legacy asset install succeeds.
// On non-Windows, this is a no-op (no config files are created).
func runInstallMCP() error {
	if runtime.GOOS != "windows" {
		return nil
	}
	env := realEnv{}
	results, err := yhatclients.RunInstallMCP(env)
	if err != nil {
		return fmt.Errorf("MCP registration failed: %v", err)
	}

	for _, r := range results {
		switch r.State {
		case "configured":
			fmt.Printf("  ✓ %s: configured (%s)\n", r.Client, r.Config)
			if r.Backup != "" {
				fmt.Printf("    backup: %s\n", r.Backup)
			}
		case "unchanged":
			fmt.Printf("  ✓ %s: unchanged (%s)\n", r.Client, r.Config)
		case "skipped":
			fmt.Printf("  ⚠ %s: skipped (%s)\n", r.Client, r.Message)
		case "error":
			fmt.Printf("  ✗ %s: error (%s)\n", r.Client, r.Message)
		default:
			fmt.Printf("  ? %s: %s\n", r.Client, r.Message)
		}
	}

	// Check for errors in results
	for _, r := range results {
		if r.State == "error" {
			return fmt.Errorf("MCP registration error for %s", r.Client)
		}
	}

	return nil
}
