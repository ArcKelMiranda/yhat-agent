// F2: sync subcommand and background scheduler.
// The sync command drains the upload queue, sends memories through the transport,
// and pulls team-approved memories from the central repository.

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ArcKelMiranda/yhat-agent/internal/config"
	"github.com/ArcKelMiranda/yhat-agent/internal/share"
	"github.com/ArcKelMiranda/yhat-agent/internal/store"
)

// ---------------------------------------------------------------------------
// Background scheduler (singleton)
// ---------------------------------------------------------------------------

// schedulerState guards against double-start of the background goroutine.
var (
	schedulerState struct {
		mu       sync.Mutex
		running  bool
		cancelFn context.CancelFunc
	}
)

// startBackgroundScheduler launches the background sync goroutine if it is not
// already running. It uses a sync.Mutex singleton to guard against double-start.
// The scheduler sleeps 15 minutes between sync runs, waking on context cancel.
func startBackgroundScheduler(ctx context.Context) {
	schedulerState.mu.Lock()
	defer schedulerState.mu.Unlock()
	if schedulerState.running {
		return
	}

	childCtx, cancel := context.WithCancel(ctx)
	schedulerState.cancelFn = cancel
	schedulerState.running = true

	go func() {
		defer func() {
			schedulerState.mu.Lock()
			schedulerState.running = false
			schedulerState.cancelFn = nil
			schedulerState.mu.Unlock()
		}()

		// Run once immediately.
		runBackgroundSyncOnce(childCtx)

		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-childCtx.Done():
				return
			case <-ticker.C:
				runBackgroundSyncOnce(childCtx)
			}
		}
	}()
}

// runBackgroundSyncOnce performs one sync cycle and records the last_sync time.
// It uses the shared mcpStore singleton when available.
// If mcpStore is nil (not yet initialized by an MCP tool), it skips this cycle.
// This avoids a race where the background goroutine opens the store while an
// MCP tool handler is also trying to open it (SQLITE_BUSY).
func runBackgroundSyncOnce(ctx context.Context) {
	// Only use the store if it's already initialized by an MCP tool handler.
	// This prevents SQLITE_BUSY when ensureStore() has the store open.
	mcpStoreMu.Lock()
	s := mcpStore
	mcpStoreMu.Unlock()
	if s == nil {
		// Store not yet initialized; skip this cycle.
		// The next cycle (in 15 min) will likely have it initialized.
		return
	}

	// Read central_repo from config.
	cfg, err := config.Load(config.ConfigPath())
	if err != nil {
		return
	}
	if cfg.CentralRepo == "" || cfg.CentralRepo == "https://centro.example.invalid" {
		return
	}

	lt, err := share.NewLocalDirTransport(cfg.CentralRepo)
	if err != nil {
		return
	}

	// Run sync.
	report, _ := share.Sync(ctx, s, lt)
	// Run pull.
	share.PullAndInsert(ctx, s, lt)

	// Update last_sync in config.
	if report.Sent > 0 || report.Failed > 0 {
		cfg.LastSync = time.Now().UTC().Format(time.RFC3339)
		_ = cfg.Save(config.ConfigPath())
	}
}

// ---------------------------------------------------------------------------
// Sync subcommand
// ---------------------------------------------------------------------------

// runSyncCommand dispatches the sync subcommand.
func runSyncCommand(args []string) int {
	fs := flag.NewFlagSet("yhat-agent sync", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: yhat-agent sync [--central-repo=<dir>]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  Drains the upload queue, sends memories to the central repository,")
		fmt.Fprintln(os.Stderr, "  and pulls team-approved memories. Prints a summary on stdout.")
	}

	var centralRepoFlag string
	fs.StringVar(&centralRepoFlag, "central-repo", "", "override central repository directory")

	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "yhat-agent sync: unexpected argument: %s\n", fs.Arg(0))
		return 1
	}

	ctx := context.Background()

	// Resolve central_repo: flag > config > placeholder.
	centralRepo := centralRepoFlag
	if centralRepo == "" {
		cfg, err := config.Load(config.ConfigPath())
		if err != nil {
			fmt.Fprintf(os.Stderr, "sync: cannot load config: %v\n", err)
			return 1
		}
		centralRepo = cfg.CentralRepo
	}
	if centralRepo == "" || centralRepo == "https://centro.example.invalid" {
		// No central repo configured; nothing to do.
		fmt.Println("0 sent, 0 failed, 0 skipped")
		return 0
	}

	// Open store.
	dbPath := config.DBPath()
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Println("0 sent, 0 failed, 0 skipped")
		return 0
	}
	s, err := store.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sync: open store: %v\n", err)
		return 1
	}
	defer s.Close()

	// Create transport.
	lt, err := share.NewLocalDirTransport(centralRepo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sync: create transport: %v\n", err)
		return 1
	}

	// Run sync (drain upload queue).
	report, err := share.Sync(ctx, s, lt)
	if err != nil {
		// Sync-level error: return non-zero so the caller knows to retry.
		fmt.Fprintf(os.Stderr, "sync: %v\n", err)
		return 1
	}

	// Pull team-approved memories.
	pulled, pullErr := share.PullAndInsert(ctx, s, lt)
	if pullErr != nil {
		// Pull error is non-fatal; print a warning.
		fmt.Fprintf(os.Stderr, "sync: pull warning: %v\n", pullErr)
	}
	if pulled > 0 {
		fmt.Printf("sync: pulled %d team memories\n", pulled)
	}

	fmt.Printf("%d sent, %d failed, %d skipped\n", report.Sent, report.Failed, report.Skipped)

	// Return non-zero if any failures occurred (retry-worthy).
	if report.Failed > 0 {
		return 2
	}
	return 0
}
