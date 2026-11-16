package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/ArcKelMiranda/yhat-agent/internal/config"
	"github.com/ArcKelMiranda/yhat-agent/internal/engram"
	"github.com/ArcKelMiranda/yhat-agent/internal/sensitive"
	"github.com/ArcKelMiranda/yhat-agent/internal/store"
)

// engramCursorKey is the sync_state key that remembers how far the last
// import-engram run read.
const engramCursorKey = "engram_import_cursor"

// keyTypes maps the type segment of a yhat.{type}.{domain}.{concept} topic key
// (short and long forms) to a memory type.
var keyTypes = map[string]string{
	"decision": store.MemoryTypeDecision, "rule": store.MemoryTypeRule,
	"obs": store.MemoryTypeObservation, "observation": store.MemoryTypeObservation,
	"assum": store.MemoryTypeAssumption, "assumption": store.MemoryTypeAssumption,
	"process": store.MemoryTypeProcess,
	"map":     store.MemoryTypeMapping, "mapping": store.MemoryTypeMapping,
	"excep": store.MemoryTypeException, "exception": store.MemoryTypeException,
	"def": store.MemoryTypeEntityDefinition, "entity-definition": store.MemoryTypeEntityDefinition,
	"intg": store.MemoryTypeIntegration, "integration": store.MemoryTypeIntegration,
	"anom": store.MemoryTypeAnomaly, "anomaly": store.MemoryTypeAnomaly,
	"tech": store.MemoryTypeTechnicalRule, "technical-rule": store.MemoryTypeTechnicalRule,
	"improvement": store.MemoryTypeImprovement,
}

// engramTypes maps an Engram observation type to a memory type when the topic
// key does not say. Anything unlisted is an observation.
var engramTypes = map[string]string{
	"decision": store.MemoryTypeDecision,
	"bugfix":   store.MemoryTypeAnomaly,
	"config":   store.MemoryTypeTechnicalRule,
	"pattern":  store.MemoryTypeProcess,
}

// engramMemoryType picks the memory type of a prospect: first the type segment
// of a yhat.* topic key, then the Engram observation type.
func engramMemoryType(topicKey, engramType string) string {
	if parts := strings.Split(topicKey, "."); len(parts) >= 2 && parts[0] == "yhat" {
		if t, ok := keyTypes[parts[1]]; ok {
			return t
		}
	}
	if t, ok := engramTypes[engramType]; ok {
		return t
	}
	return store.MemoryTypeObservation
}

// engramContext records where a prospect came from.
func engramContext(o engram.Observation) string {
	parts := []string{"Fuente: Engram", "proyecto " + o.Project}
	if o.TopicKey != "" {
		parts = append(parts, "clave "+o.TopicKey)
	}
	if o.SyncID != "" {
		parts = append(parts, "observación "+o.SyncID)
	}
	return strings.Join(parts, " · ")
}

// runImportEngram reads the person's Engram, turns the prospects into proposed
// memories and prints a summary. It returns the process exit code.
func runImportEngram(args []string, out io.Writer) int {
	fs := flag.NewFlagSet("import-engram", flag.ContinueOnError)
	fs.SetOutput(out)
	dbFlag := fs.String("db", "", "path to the Engram database (default: $ENGRAM_DATA_DIR or ~/.engram/engram.db)")
	dryRun := fs.Bool("dry-run", false, "list the prospects without storing them or moving the read position")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	storePath := config.DBPath()
	if _, err := os.Stat(storePath); err != nil {
		fmt.Fprintf(out, "import-engram: no local store at %s (run yhat-agent install first)\n", storePath)
		return 1
	}
	engramPath := *dbFlag
	if engramPath == "" {
		p, err := engram.DefaultPath()
		if err != nil {
			fmt.Fprintf(out, "import-engram: %v\n", err)
			return 1
		}
		engramPath = p
	}

	ctx := context.Background()
	s, err := store.Open(storePath)
	if err != nil {
		fmt.Fprintf(out, "import-engram: open store: %v\n", err)
		return 1
	}
	defer s.Close()
	if !*dryRun {
		if err := s.EnsureFTS5(ctx); err != nil {
			fmt.Fprintf(out, "import-engram: %v\n", err)
			return 1
		}
	}

	stored, err := s.GetSyncState(ctx, engramCursorKey)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		fmt.Fprintf(out, "import-engram: read position: %v\n", err)
		return 1
	}
	cur, err := engram.ParseCursor(stored)
	if err != nil {
		fmt.Fprintf(out, "import-engram: %v\n", err)
		return 1
	}

	obs, _, err := engram.Read(ctx, engramPath, engram.DefaultCriteria(), cur)
	if err != nil {
		fmt.Fprintf(out, "import-engram: %v\n", err)
		return 1
	}

	author := operatorAuthor()
	var imported, duplicates int
	var skipped []string
	last := cur
	for _, o := range obs {
		memType := engramMemoryType(o.TopicKey, o.Type)
		ctxText := engramContext(o)

		reason := ""
		switch {
		case utf8.RuneCountInString(o.Title) > 200:
			reason = "title longer than 200 characters"
		case utf8.RuneCountInString(o.Content) > 10000:
			reason = "content longer than 10000 characters"
		case len(sensitive.Scan(o.Title+"\n"+o.Content+"\n"+ctxText)) > 0:
			reason = "looks like it contains a secret"
		}
		if reason != "" {
			skipped = append(skipped, fmt.Sprintf("  skipped %q (%s)", clip(o.Title), reason))
			last = engram.Cursor{UpdatedAt: o.UpdatedAt, ID: o.ID}
			continue
		}
		if *dryRun {
			fmt.Fprintf(out, "  would import [%s] %q\n", memType, clip(o.Title))
			imported++
			continue
		}
		_, err := s.ProposeMemory(ctx, store.Memory{
			Type: memType, Title: o.Title, Content: o.Content, Context: &ctxText, Author: author,
		})
		switch {
		case err == nil:
			imported++
		case errors.Is(err, store.ErrDuplicate):
			duplicates++
		default:
			fmt.Fprintf(out, "import-engram: stopped at %q: %v\n", clip(o.Title), err)
			saveCursor(ctx, s, last, cur)
			return 1
		}
		last = engram.Cursor{UpdatedAt: o.UpdatedAt, ID: o.ID}
	}

	for _, line := range skipped {
		fmt.Fprintln(out, line)
	}
	if *dryRun {
		fmt.Fprintf(out, "dry run: %d prospects would be imported, %d skipped; nothing was stored\n", imported, len(skipped))
		return 0
	}
	if err := saveCursor(ctx, s, last, cur); err != nil {
		fmt.Fprintf(out, "import-engram: save position: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "Engram %s: %d imported, %d duplicate, %d skipped. Review them in the bandeja (yhat-agent bandeja).\n",
		engramPath, imported, duplicates, len(skipped))
	return 0
}

// saveCursor stores the read position when it moved.
func saveCursor(ctx context.Context, s *store.Store, last, prev engram.Cursor) error {
	if last == prev {
		return nil
	}
	return s.SetSyncState(ctx, engramCursorKey, last.String())
}

// clip shortens a title for terminal output.
func clip(s string) string {
	if utf8.RuneCountInString(s) <= 80 {
		return s
	}
	r := []rune(s)
	return string(r[:80]) + "…"
}
