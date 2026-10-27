package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	yhatagent "github.com/ArcKelMiranda/yhat-agent"
	"github.com/ArcKelMiranda/yhat-agent/internal/bandeja"
	"github.com/ArcKelMiranda/yhat-agent/internal/config"
	"github.com/ArcKelMiranda/yhat-agent/internal/store"
)

// Global CLI flags (parsed before subcommand routing).
var (
	centralTokenFlag string // --central-token=<token> overrides config.CentralToken
)

// mcpUsageNote is appended to the help text to document the experimental mcp command.
const mcpUsageNote = `
MCP (experimental F0 prototype):
  yhat-agent mcp              Run MCP stdio server
  yhat-agent mcp --selftest  Run self-test and emit JSON diagnostic

  WARNING: This is a diagnostic prototype (F0). Data is synthetic and ephemeral.
  No production features are enabled. Do not use for real tasks.
`

func main() {
	// Pull the global --central-token out (before or after the
	// subcommand) so each subcommand's own FlagSet only sees the flags
	// it defines.
	var rest []string
	centralTokenFlag, rest = extractCentralToken(os.Args[1:])
	if len(rest) < 1 {
		printUsage()
		os.Exit(1)
	}

	cmd := rest[0]
	args := rest[1:]

	switch cmd {
	case "bandeja":
		runBandeja()
	case "install":
		runInstall()
	case "status":
		runStatus()
	case "update":
		runUpdate()
	case "uninstall":
		runUninstall()
	case "mcp":
		exitCode := runMCPCommand(args)
		os.Exit(exitCode)
	case "sync":
		exitCode := runSyncCommand(args)
		os.Exit(exitCode)
	case "version", "--version", "-v":
		runVersion()
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
  update     Download and verify latest release from GitHub (use --check to verify without downloading)
  uninstall  Remove managed files (requires --yes)
  bandeja    Open the local approval inbox in your browser
  mcp        Run MCP stdio server (experimental F0 prototype)
  sync       Drain upload queue and sync with central repository
  version    Show version and build information
  help       Show this help message

Options:
  --yes            Required for uninstall to confirm destructive action
  --json           Output status in JSON format
  --dry-run        Show what would be done without making changes (uninstall only)
  --claude-code    Force Claude Code setup (skill + MCP) on install; otherwise done when ~/.claude exists
  --idle           Close the bandeja after this long without requests (default 10m, e.g. --idle 30m)
  --no-bandeja     Skip opening bandeja after install (install command only)
  --central-token  Override the central bearer token from config.yaml
` + mcpUsageNote)
}

func runInstall() {
	results, err := yhatagent.Install()
	if err != nil {
		fmt.Fprintf(os.Stderr, "install failed: %v\n", err)
		os.Exit(1)
	}

	// Register MCP servers (Windows only; no-op on other platforms).
	if err := runInstallMCP(); err != nil {
		fmt.Fprintf(os.Stderr, "MCP registration failed (legacy assets may already be installed): %v\n", err)
		os.Exit(1)
	}

	// Claude Code: skill + MCP (when ~/.claude exists or --claude-code is set).
	if err := runInstallClaudeCode(); err != nil {
		fmt.Fprintf(os.Stderr, "Claude Code setup failed (OpenCode assets already installed): %v\n", err)
		os.Exit(1)
	}

	// F1-C: write Cerebro config, state, and migrate the store.
	if err := runInstallF1(); err != nil {
		fmt.Fprintf(os.Stderr, "cerebro init failed: %v\n", err)
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

	// F1-D: offer to open bandeja unless --no-bandeja is set.
	if !hasFlag("--no-bandeja") {
		fmt.Println("\nOpening Bandeja YHat...")
		if err := runBandejaPrompt(); err != nil {
			fmt.Fprintf(os.Stderr, "bandeja: %v\n", err)
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

	claude, err := yhatagent.ClaudeCodeStatus()
	if err != nil {
		fmt.Fprintf(os.Stderr, "status failed: %v\n", err)
		os.Exit(1)
	}

	// F1-C: read Cerebro config and state; open store to count memories.
	cerebro := runStatusCerebro(useJSON)

	if useJSON {
		cerebroStatus := make(map[string]interface{})
		cerebroStatus["config_home"] = yhatagent.ConfigHome()
		cerebroStatus["opencode_root"] = yhatagent.OpenCodeRoot()
		cerebroStatus["manifest_path"] = result.ManifestPath
		cerebroStatus["has_manifest"] = result.HasManifest
		cerebroStatus["files"] = result.Files
		cerebroStatus["candidates"] = result.Candidates
		cerebroStatus["has_candidate"] = result.HasCandidate
		cerebroStatus["claude_code"] = claude
		cerebroStatus["cerebro"] = cerebro
		data, _ := json.MarshalIndent(cerebroStatus, "", "  ")
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

	fmt.Printf("  Claude Code skill: %s (%s)\n", claude.TargetPath, claude.Message)

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
		displayName := filepath.Base(f.AssetKey)
		fmt.Printf("    %s %s: %s (%s)\n", icon, displayName, f.State.String(), f.Message)
	}

	// F1-C Cerebro section.
	runStatusCerebroText(cerebro)
}

func runUpdate() {
	checkOnly := hasFlag("--check")

	if checkOnly {
		result, err := yhatagent.CheckForUpdate()
		if err != nil {
			fmt.Fprintf(os.Stderr, "update check failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("yhat-agent %s", result.CurrentVersion)
		if result.LatestVersion != "unknown" && result.LatestVersion != result.CurrentVersion {
			fmt.Printf(" → %s", result.LatestVersion)
		}
		fmt.Println()
		fmt.Println(result.Message)
		return
	}

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

func runVersion() {
	v := yhatagent.Version
	info := yhatagent.BuildInfo
	fmt.Printf("yhat-agent %s", v)
	if info != "" {
		fmt.Printf(" (%s)", info)
	}
	fmt.Println()
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

	if claudeCodeWanted() {
		runUninstallClaudeCode(useJSON)
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

// extractCentralToken removes every --central-token occurrence from args
// (accepting -/-- prefixes, "=value" and "value as next arg" forms) and
// returns the last value seen plus the remaining args in order.
func extractCentralToken(args []string) (string, []string) {
	var token string
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			rest = append(rest, args[i:]...)
			break
		}
		name := strings.TrimLeft(a, "-")
		if name == a || len(a)-len(name) > 2 {
			rest = append(rest, a)
			continue
		}
		switch {
		case strings.HasPrefix(name, "central-token="):
			token = strings.TrimPrefix(name, "central-token=")
		case name == "central-token" && i+1 < len(args):
			i++
			token = args[i]
		case name == "central-token":
			// Trailing flag without a value: drop it, keep the token empty.
		default:
			rest = append(rest, a)
		}
	}
	return token, rest
}

// getCentralToken returns the CLI token flag if set, otherwise the config token.
// If cfg is nil, only the flag is checked.
func getCentralToken(cfg *config.Config) string {
	if centralTokenFlag != "" {
		return centralTokenFlag
	}
	if cfg != nil {
		return cfg.CentralToken
	}
	return ""
}

// cerebroStatus holds the F1-C Cerebro data read by runStatus.
type cerebroStatus struct {
	HomePath    string
	DBPath      string
	DBExists    bool
	SchemaVer   int
	Counts      map[string]int
	Operator    string
	LastSync    string
	ConfigError string
	StateError  string
	StoreError  string
}

// runStatusCerebro reads the Cerebro config, state, and store counts.
// It populates all fields even when individual reads fail (partial output).
func runStatusCerebro(useJSON bool) cerebroStatus {
	cs := cerebroStatus{
		HomePath:  config.DefaultHome(),
		DBPath:    config.DBPath(),
		SchemaVer: store.CurrentSchemaVersion,
		Counts:    map[string]int{},
	}

	// Read config.yaml.
	cfg, err := config.Load(config.ConfigPath())
	if err != nil {
		cs.ConfigError = err.Error()
	} else {
		cs.Operator = cfg.Operator
		cs.LastSync = cfg.LastSync
		if cs.LastSync == "" {
			cs.LastSync = "never"
		}
	}

	// Read state.json (for schema version override if present).
	st, err := config.LoadState(config.StatePath())
	if err != nil {
		cs.StateError = err.Error()
	} else {
		cs.SchemaVer = st.SchemaVersion
	}

	// Count memories.
	if _, err := os.Stat(cs.DBPath); os.IsNotExist(err) {
		cs.DBExists = false
		cs.Counts = map[string]int{
			"proposed":  0,
			"validated": 0,
			"rejected":  0,
			"archived":  0,
		}
	} else {
		cs.DBExists = true
		st_, err := store.Open(cs.DBPath)
		if err != nil {
			cs.StoreError = err.Error()
			cs.Counts = map[string]int{
				"proposed":  0,
				"validated": 0,
				"rejected":  0,
				"archived":  0,
			}
		} else {
			counts, err := st_.CountMemoriesByStatus(context.Background())
			_ = st_.Close()
			if err != nil {
				cs.StoreError = err.Error()
				cs.Counts = map[string]int{
					"proposed":  0,
					"validated": 0,
					"rejected":  0,
					"archived":  0,
				}
			} else {
				cs.Counts = counts
			}
		}
	}

	return cs
}

// runStatusCerebroText prints the Cerebro section for text output.
func runStatusCerebroText(cs cerebroStatus) {
	fmt.Printf("\n  Cerebro:")
	fmt.Printf("\n    YHat home: %s", cs.HomePath)

	dbLabel := cs.DBPath
	if !cs.DBExists {
		dbLabel += " (not initialised)"
	} else {
		dbLabel += fmt.Sprintf(" (schema v%d)", cs.SchemaVer)
	}
	fmt.Printf("\n    DB:           %s", dbLabel)

	proposed := cs.Counts["proposed"]
	validated := cs.Counts["validated"]
	rejected := cs.Counts["rejected"]
	archived := cs.Counts["archived"]
	total := proposed + validated + rejected + archived
	fmt.Printf("\n    Memories:     %d (%d proposed, %d validated, %d rejected, %d archived)",
		total, proposed, validated, rejected, archived)

	op := cs.Operator
	if op == "" && cs.ConfigError != "" {
		op = "?"
	}
	fmt.Printf("\n    Operator:     %s", op)

	lastSync := cs.LastSync
	if lastSync == "" {
		lastSync = "never"
	}
	fmt.Printf("\n    Last sync:    %s\n", lastSync)
}

// runInstallF1 creates the F1 home directory, writes config.yaml and state.json,
// and opens the store to apply migrations.
func runInstallF1() error {
	homeDir := config.DefaultHome()
	if err := config.EnsureDir(homeDir); err != nil {
		return fmt.Errorf("ensure yhat home dir: %w", err)
	}

	// Determine operator name from the environment.
	operator := os.Getenv("USERNAME")
	if operator == "" {
		operator = os.Getenv("USER")
	}
	if operator == "" {
		operator = "unknown"
	}

	// Determine central URL and transport scheme.
	// Priority: YHAT_CENTRAL_URL env > existing central_repo > placeholder.
	centralURL := os.Getenv("YHAT_CENTRAL_URL")
	var centralRepo string
	var centralToken string

	if centralURL != "" {
		// YHAT_CENTRAL_URL set: use as HTTP transport URL.
		// Token from YHAT_CENTRAL_TOKEN env var, or leave change-me placeholder.
		centralToken = os.Getenv("YHAT_CENTRAL_TOKEN")
		if centralToken == "" {
			centralToken = "change-me"
		}
	} else {
		// No YHAT_CENTRAL_URL: use placeholder as central_repo (legacy local-dir path).
		centralRepo = "https://centro.example.invalid"
	}

	cfg := config.Config{
		Operator:     operator,
		CentralRepo:  centralRepo,
		CentralURL:   centralURL,
		CentralToken: centralToken,
		LastSync:     "",
	}
	if err := cfg.Save(config.ConfigPath()); err != nil {
		return fmt.Errorf("write config.yaml: %w", err)
	}

	st := config.State{
		Version:           yhatagent.Version,
		RegisteredClients: []string{"opencode", "claude"},
		SchemaVersion:     store.CurrentSchemaVersion,
		LastSync:          "",
	}
	if err := st.Save(config.StatePath()); err != nil {
		return fmt.Errorf("write state.json: %w", err)
	}

	// Open the store to apply migrations; close immediately.
	st_, err := store.Open(config.DBPath())
	if err != nil {
		return fmt.Errorf("open yhat.db: %w", err)
	}
	_ = st_.Close()

	return nil
}

// runBandeja is the entry point for the bandeja subcommand.
// It opens the HTTP server and the browser, blocking until the user
// presses Ctrl+C or the idle limit (--idle, default 10m) elapses.
func runBandeja() {
	idle, err := idleFlag()
	if err != nil {
		fmt.Fprintf(os.Stderr, "bandeja: %v\n", err)
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	s, err := store.Open(config.DBPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "bandeja: open store: %v\n", err)
		os.Exit(1)
	}
	defer s.Close()

	srv, err := bandeja.StartWithIdle(ctx, s, idle)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bandeja: start server: %v\n", err)
		os.Exit(1)
	}
	defer srv.Close()

	fmt.Println(srv.URL())

	// Try to open the browser; skip silently if tool is unavailable.
	bandeja.OpenBrowser(srv.URL())

	fmt.Println("\nPresiona Ctrl+C para detener el servidor.")

	if err := srv.Wait(); err != nil {
		fmt.Fprintf(os.Stderr, "bandeja: server stopped: %v\n", err)
		srv.Close()
		s.Close()
		os.Exit(1)
	}
}

// runBandejaPrompt is called after install to offer to open bandeja.
// It starts the server, prints the URL, tries to open the browser,
// and blocks until Ctrl+C.
func runBandejaPrompt() error {
	idle, err := idleFlag()
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	s, err := store.Open(config.DBPath())
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	srv, err := bandeja.StartWithIdle(ctx, s, idle)
	if err != nil {
		return fmt.Errorf("start server: %w", err)
	}
	defer srv.Close()

	fmt.Printf("Abre en tu navegador: %s\n", srv.URL())
	fmt.Println("Presiona Ctrl+C para cerrar la Bandeja.")
	bandeja.OpenBrowser(srv.URL())

	// Block until Ctrl+C, the idle limit, or a serve failure; returning
	// earlier would close the server as soon as install finishes.
	if err := srv.Wait(); err != nil {
		return fmt.Errorf("server stopped: %w", err)
	}
	return nil
}

// idleFlag parses --idle <duration> / --idle=<duration> from the subcommand
// args. It defaults to bandeja.DefaultIdle and rejects non-positive values.
func idleFlag() (time.Duration, error) {
	args := os.Args[2:]
	for i, a := range args {
		var raw string
		switch {
		case a == "--idle":
			if i+1 >= len(args) {
				return 0, fmt.Errorf("--idle requires a duration (for example 10m)")
			}
			raw = args[i+1]
		case strings.HasPrefix(a, "--idle="):
			raw = strings.TrimPrefix(a, "--idle=")
		default:
			continue
		}
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 {
			return 0, fmt.Errorf("invalid --idle %q: use a positive duration such as 10m", raw)
		}
		return d, nil
	}
	return bandeja.DefaultIdle, nil
}
