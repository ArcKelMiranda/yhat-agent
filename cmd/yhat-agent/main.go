package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	yhatagent "github.com/ArcKelMiranda/yhat-agent"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]

	switch cmd {
	case "install":
		runInstall()
	case "status":
		runStatus()
	case "update":
		runUpdate()
	case "uninstall":
		runUninstall()
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`yhat-agent - YHat OpenCode agent installer

Usage:
  yhat-agent <command> [options]

Commands:
  install    Install embedded assets to OpenCode config directory
  status     Report installation state without making changes
  update     Download and verify latest release from GitHub
  uninstall  Remove managed files (requires --yes)
  help       Show this help message

Options:
  --yes      Required for uninstall to confirm destructive action
  --json     Output status in JSON format
  --dry-run  Show what would be done without making changes (uninstall only)
`)
}

func runInstall() {
	results, err := yhatagent.Install()
	if err != nil {
		fmt.Fprintf(os.Stderr, "install failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Installation complete:")
	for _, r := range results {
		state := r.State.String()
		if r.State == yhatagent.StateInstalled {
			fmt.Printf("  ✓ %s: %s (%s)\n", r.AssetKey, state, r.Message)
		} else {
			fmt.Printf("  ⚠ %s: %s (%s)\n", r.AssetKey, state, r.Message)
		}
	}
}

func runStatus() {
	useJSON := hasFlag("--json")

	result, err := yhatagent.Status()
	if err != nil {
		if useJSON {
			data, _ := json.MarshalIndent(map[string]interface{}{
				"error": err.Error(),
			}, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Fprintf(os.Stderr, "status failed: %v\n", err)
		}
		os.Exit(1)
	}

	if useJSON {
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Printf("yhat-agent status\n")
	fmt.Printf("  Config home: %s\n", yhatagent.ConfigHome())
	fmt.Printf("  OpenCode root: %s\n", yhatagent.OpenCodeRoot())
	fmt.Printf("  Manifest: %s (%s)\n", result.ManifestPath, boolStr(result.HasManifest))

	if result.HasCandidate {
		fmt.Printf("\n  Candidates found:\n")
		for _, c := range result.Candidates {
			fmt.Printf("    %s\n", c)
		}
	}

	fmt.Printf("\n  Assets:\n")
	for _, f := range result.Files {
		icon := "✓"
		switch f.State {
		case yhatagent.StateMissing:
			icon = "✗"
		case yhatagent.StateDrift:
			icon = "⚠"
		case yhatagent.StateCandidate:
			icon = "◇"
		case yhatagent.StateUnknown:
			icon = "?"
		}
		// Extract human-readable name from asset key (e.g., "yhat-memory-capture.md")
		displayName := filepath.Base(f.AssetKey)
		fmt.Printf("    %s %s: %s (%s)\n", icon, displayName, f.State.String(), f.Message)
	}
}

func runUpdate() {
	fmt.Println("Fetching latest release from GitHub...")

	result, err := yhatagent.Update()
	if err != nil {
		fmt.Fprintf(os.Stderr, "update failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nUpdate summary:\n")
	fmt.Printf("  Current version: %s\n", result.CurrentVersion)
	fmt.Printf("  Latest version:  %s\n", result.LatestVersion)
	fmt.Printf("  Asset:          %s\n", result.AssetName)
	fmt.Printf("  Hash verified:  %s\n", boolStr(result.HashVerified))

	fmt.Printf("\n%s\n", result.Message)
}

func runUninstall() {
	confirmed := hasFlag("--yes")
	dryRun := hasFlag("--dry-run")
	useJSON := hasFlag("--json")

	if !confirmed {
		fmt.Fprintln(os.Stderr, "Error: uninstall requires --yes flag to confirm")
		fmt.Fprintln(os.Stderr, "Usage: yhat-agent uninstall --yes")
		os.Exit(1)
	}

	// Dry-run mode: report planned actions without making changes
	if dryRun {
		// JSON mode: output only valid JSON, no plain banner
		if useJSON {
			plan, err := yhatagent.UninstallPlan()
			if err != nil {
				data, _ := json.MarshalIndent(map[string]interface{}{
					"error": err.Error(),
				}, "", "  ")
				fmt.Println(string(data))
				os.Exit(1)
			}
			data, _ := json.MarshalIndent(plan, "", "  ")
			fmt.Println(string(data))
			return
		}

		// Text mode: show banner and planned actions
		fmt.Println("Dry run mode - no changes will be made")
		plan, err := yhatagent.UninstallPlan()
		if err != nil {
			fmt.Fprintf(os.Stderr, "uninstall plan failed: %v\n", err)
			os.Exit(1)
		}
		if len(plan.Planned) > 0 {
			fmt.Println("Would remove:")
			for _, p := range plan.Planned {
				fmt.Printf("  - %s\n", p)
			}
		}
		if len(plan.Skipped) > 0 {
			fmt.Println("Would skip (modified or not managed):")
			for _, s := range plan.Skipped {
				fmt.Printf("  - %s: %s\n", s.Path, s.Reason)
			}
		}
		if len(plan.Planned) == 0 && len(plan.Skipped) == 0 {
			fmt.Println("No managed files found to remove.")
		}
		return
	}

	result, err := yhatagent.Uninstall(confirmed)
	if err != nil {
		if useJSON {
			data, _ := json.MarshalIndent(map[string]interface{}{
				"error": err.Error(),
			}, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Fprintf(os.Stderr, "uninstall failed: %v\n", err)
		}
		os.Exit(1)
	}

	// For JSON mode: output valid JSON, then exit nonzero if errors exist
	if useJSON {
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
		// Nonzero exit when result contains operational errors, even if
		// some removals succeeded — this signals partial failure to callers.
		if len(result.Errors) > 0 {
			os.Exit(1)
		}
		return
	}

	// Text mode output
	if len(result.Removed) > 0 {
		fmt.Println("Removed:")
		for _, r := range result.Removed {
			fmt.Printf("  - %s\n", r)
		}
	}

	if len(result.Skipped) > 0 {
		fmt.Println("Skipped (modified or not managed):")
		for _, s := range result.Skipped {
			fmt.Printf("  - %s: %s\n", s.Path, s.Reason)
		}
	}

	if len(result.Errors) > 0 {
		fmt.Println("Errors:")
		for _, e := range result.Errors {
			fmt.Printf("  - %s\n", e)
		}
		// Operational failures: exit nonzero regardless of partial success
		os.Exit(1)
	}

	if len(result.Removed) == 0 && len(result.Skipped) == 0 && len(result.Errors) == 0 {
		fmt.Println("No managed files found to remove.")
	}
}

func hasFlag(flag string) bool {
	for _, arg := range os.Args[2:] {
		if arg == flag {
			return true
		}
	}
	return false
}

func boolStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
