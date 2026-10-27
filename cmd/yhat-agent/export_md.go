package main

import (
	"context"
	"fmt"
	"os"

	"github.com/ArcKelMiranda/yhat-agent/internal/config"
	"github.com/ArcKelMiranda/yhat-agent/internal/store"
)

// runExportMD writes every memory in the local store to <yhat home>/memories
// as Markdown. It returns the process exit code.
func runExportMD() int {
	dbPath := config.DBPath()
	if _, err := os.Stat(dbPath); err != nil {
		fmt.Fprintf(os.Stderr, "export-md: no local store at %s (run yhat-agent install first)\n", dbPath)
		return 1
	}
	s, err := store.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "export-md: open store: %v\n", err)
		return 1
	}
	defer s.Close()

	n, err := s.ExportMarkdown(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "export-md: %d written to %s, first error: %v\n", n, s.MarkdownDir(), err)
		return 1
	}
	fmt.Printf("Exported %d memories to %s\n", n, s.MarkdownDir())
	return 0
}
