package main

import (
	"fmt"
	"os"
	"os/exec"

	yhatagent "github.com/ArcKelMiranda/yhat-agent"
)

// claudeRunner runs the claude CLI; replaced in tests.
var claudeRunner = func(args ...string) error {
	return exec.Command("claude", args...).Run()
}

// claudeCodeWanted reports whether Claude Code support should be installed:
// forced by --claude-code, or implied by an existing Claude Code config dir.
func claudeCodeWanted() bool {
	if hasFlag("--claude-code") {
		return true
	}
	info, err := os.Stat(yhatagent.ClaudeCodeRoot())
	return err == nil && info.IsDir()
}

// runInstallClaudeCode installs the skill and registers the MCP server with
// Claude Code. MCP registration goes through `claude mcp add` because
// ~/.claude.json is owned and rewritten by Claude Code itself.
func runInstallClaudeCode() error {
	if !claudeCodeWanted() {
		return nil
	}
	res, err := yhatagent.InstallClaudeCodeSkill()
	if err != nil {
		return fmt.Errorf("claude code skill: %w", err)
	}
	fmt.Printf("  ✓ claude-code skill: %s (%s)\n", res.TargetPath, res.Message)

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable path: %w", err)
	}
	if _, err := exec.LookPath("claude"); err != nil {
		fmt.Printf("  ⚠ claude-code mcp: `claude` not found on PATH; register manually:\n    claude mcp add --scope user yhat -- %s mcp\n", exe)
		return nil
	}
	if claudeRunner("mcp", "get", "yhat") == nil {
		fmt.Println("  ✓ claude-code mcp: yhat already registered")
		return nil
	}
	if err := claudeRunner("mcp", "add", "--scope", "user", "yhat", "--", exe, "mcp"); err != nil {
		return fmt.Errorf("claude mcp add: %w", err)
	}
	fmt.Println("  ✓ claude-code mcp: yhat registered (user scope)")
	return nil
}

// runUninstallClaudeCode removes the Claude Code skill (if unmodified) and the
// user-scope MCP registration. Quiet unless there is something to report.
func runUninstallClaudeCode(quiet bool) {
	say := func(format string, a ...any) {
		if !quiet {
			fmt.Printf(format, a...)
		}
	}
	reason, err := yhatagent.UninstallClaudeCodeSkill()
	switch {
	case err != nil:
		fmt.Fprintf(os.Stderr, "claude code skill: %v\n", err)
	case reason != "":
		say("Skipped claude-code skill: %s\n", reason)
	}
	if _, err := exec.LookPath("claude"); err == nil && claudeRunner("mcp", "get", "yhat") == nil {
		if err := claudeRunner("mcp", "remove", "--scope", "user", "yhat"); err != nil {
			fmt.Fprintf(os.Stderr, "claude mcp remove: %v\n", err)
		} else {
			say("Removed claude-code mcp: yhat\n")
		}
	}
}
