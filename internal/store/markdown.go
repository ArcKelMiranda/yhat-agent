package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Markdown mirror: every memory is also written as <dir>/<id>.md next to the
// SQLite file (~/.yhat/memories). SQLite stays the source of truth; the files
// are a human-readable, git-friendly copy and are rewritten on every change.
// A failed write is logged and never fails the database operation.

var safeMemoryID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// MarkdownDir returns the mirror directory, or "" when mirroring is disabled
// (relative or in-memory database paths).
func (s *Store) MarkdownDir() string { return s.mdDir }

// markdownDirFor derives the mirror directory from the database path.
func markdownDirFor(dbPath string) string {
	if !filepath.IsAbs(dbPath) {
		return ""
	}
	return filepath.Join(filepath.Dir(dbPath), "memories")
}

// RenderMarkdown renders a memory as a Markdown document with YAML frontmatter.
func RenderMarkdown(m Memory) []byte {
	var b bytes.Buffer
	field := func(k string, v any) { fmt.Fprintf(&b, "%s: %s\n", k, yamlValue(v)) }
	ts := func(t time.Time) string { return t.UTC().Format(time.RFC3339) }

	b.WriteString("---\n")
	field("id", m.ID)
	fmt.Fprintf(&b, "type: %s\nstatus: %s\norigin: %s\n", m.Type, m.Status, m.Origin)
	field("title", m.Title)
	field("author", m.Author)
	fmt.Fprintf(&b, "share_status: %s\n", m.ShareStatus)
	fmt.Fprintf(&b, "created_at: %s\nupdated_at: %s\n", ts(m.CreatedAt), ts(m.UpdatedAt))
	if m.ValidatedBy != nil {
		field("validated_by", *m.ValidatedBy)
	}
	if m.ValidatedAt != nil {
		fmt.Fprintf(&b, "validated_at: %s\n", ts(*m.ValidatedAt))
	}
	if m.RejectReason != nil {
		field("reject_reason", *m.RejectReason)
	}
	if m.SharedAt != nil {
		fmt.Fprintf(&b, "shared_at: %s\n", ts(*m.SharedAt))
	}
	if m.RemoteID != nil {
		field("remote_id", *m.RemoteID)
	}
	if m.TeamStatus != nil {
		fmt.Fprintf(&b, "team_status: %s\n", *m.TeamStatus)
	}
	fmt.Fprintf(&b, "content_hash: %s\n---\n\n", m.ContentHash)

	fmt.Fprintf(&b, "# %s\n\n%s\n", singleLine(m.Title), strings.TrimRight(m.Content, "\n"))
	if m.Context != nil && *m.Context != "" {
		fmt.Fprintf(&b, "\n## Contexto\n\n%s\n", strings.TrimRight(*m.Context, "\n"))
	}
	return b.Bytes()
}

// yamlValue quotes v as a JSON string, which is valid single-line YAML.
func yamlValue(v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return strings.TrimRight(b.String(), "\n")
}

func singleLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// WriteMarkdown atomically writes the memory to <dir>/<id>.md (dir 0700, file 0600).
func WriteMarkdown(dir string, m Memory) error {
	if !safeMemoryID.MatchString(m.ID) {
		return fmt.Errorf("unsafe memory id %q", m.ID)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(RenderMarkdown(m)); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, m.ID+".md"))
}

// mirror writes m to the mirror directory; failures are logged only.
func (s *Store) mirror(m Memory) {
	if s.mdDir == "" {
		return
	}
	if err := WriteMarkdown(s.mdDir, m); err != nil {
		log.Printf("yhat: markdown mirror %s: %v", m.ID, err)
	}
}

// mirrorID re-reads the memory and mirrors it.
func (s *Store) mirrorID(ctx context.Context, id string) {
	if s.mdDir == "" {
		return
	}
	m, err := s.getMemoryByID(ctx, id)
	if err != nil {
		log.Printf("yhat: markdown mirror %s: %v", id, err)
		return
	}
	s.mirror(m)
}

// ExportMarkdown (re)writes every memory to the mirror directory and returns
// how many files were written. It keeps going after a failure and returns the
// first error.
func (s *Store) ExportMarkdown(ctx context.Context) (int, error) {
	if s.mdDir == "" {
		return 0, fmt.Errorf("markdown mirror is disabled for this database path")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM memories ORDER BY created_at`)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	var first error
	n := 0
	for _, id := range ids {
		m, err := s.getMemoryByID(ctx, id)
		if err == nil {
			err = WriteMarkdown(s.mdDir, m)
		}
		if err != nil {
			if first == nil {
				first = fmt.Errorf("%s: %w", id, err)
			}
			continue
		}
		n++
	}
	return n, first
}
