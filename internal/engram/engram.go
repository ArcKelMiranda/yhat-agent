// Package engram reads prospects for the source of truth from a person's local
// Engram database. Access is strictly read-only: the Engram file is opened with
// mode=ro and never written, migrated or locked beyond a normal reader.
package engram

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "modernc.org/sqlite" // register sqlite driver
)

// DefaultProject is the Engram project that holds YHat knowledge.
const DefaultProject = "yhat-knowledge"

// DefaultKeyPrefix marks observations saved with a YHat topic key.
const DefaultKeyPrefix = "yhat."

// ErrNotFound means the Engram database file does not exist.
var ErrNotFound = errors.New("engram database not found")

// Observation is the subset of an Engram observation the importer needs.
type Observation struct {
	ID        int64
	SyncID    string
	Project   string
	TopicKey  string
	Type      string
	Title     string
	Content   string
	UpdatedAt string
}

// Criteria selects which observations are prospects.
type Criteria struct {
	Project   string // Engram project name; empty disables the project rule
	KeyPrefix string // topic_key prefix; empty disables the key rule
}

// DefaultCriteria is the criterion agreed for YHat: project yhat-knowledge or a
// yhat.* topic key.
func DefaultCriteria() Criteria {
	return Criteria{Project: DefaultProject, KeyPrefix: DefaultKeyPrefix}
}

// Cursor marks how far a previous import read. The zero value reads everything.
type Cursor struct {
	UpdatedAt string
	ID        int64
}

// String encodes the cursor for storage ("updated_at|id"); the zero value is "".
func (c Cursor) String() string {
	if c == (Cursor{}) {
		return ""
	}
	return c.UpdatedAt + "|" + strconv.FormatInt(c.ID, 10)
}

// ParseCursor decodes a stored cursor. An empty string is the zero cursor.
func ParseCursor(s string) (Cursor, error) {
	if s == "" {
		return Cursor{}, nil
	}
	i := strings.LastIndex(s, "|")
	if i < 0 {
		return Cursor{}, fmt.Errorf("invalid cursor %q", s)
	}
	id, err := strconv.ParseInt(s[i+1:], 10, 64)
	if err != nil {
		return Cursor{}, fmt.Errorf("invalid cursor %q: %w", s, err)
	}
	return Cursor{UpdatedAt: s[:i], ID: id}, nil
}

// DefaultPath returns the Engram database path: $ENGRAM_DATA_DIR/engram.db when
// the variable is set, otherwise ~/.engram/engram.db.
func DefaultPath() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("ENGRAM_DATA_DIR")); dir != "" {
		return filepath.Join(dir, "engram.db"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	return filepath.Join(home, ".engram", "engram.db"), nil
}

// Read returns the prospects after cur, oldest first, and a cursor pointing at
// the last one returned (cur itself when there are none). Soft-deleted
// observations and observations with an empty title or content are skipped.
func Read(ctx context.Context, path string, crit Criteria, cur Cursor) ([]Observation, Cursor, error) {
	if crit.Project == "" && crit.KeyPrefix == "" {
		return nil, cur, errors.New("criteria select nothing: set a project or a key prefix")
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, cur, fmt.Errorf("%w: %s", ErrNotFound, path)
		}
		return nil, cur, err
	}

	u := url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro&_pragma=busy_timeout(5000)"}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, cur, fmt.Errorf("open engram: %w", err)
	}
	defer db.Close()

	var where []string
	var args []any
	if crit.Project != "" {
		where = append(where, "project = ?")
		args = append(args, crit.Project)
	}
	if crit.KeyPrefix != "" {
		// substr instead of LIKE so the prefix is literal (no % or _ wildcards).
		where = append(where, "substr(topic_key, 1, ?) = ?")
		args = append(args, len([]rune(crit.KeyPrefix)), crit.KeyPrefix)
	}
	q := `SELECT id, COALESCE(sync_id,''), COALESCE(project,''), COALESCE(topic_key,''),
	             type, title, content, updated_at
	      FROM observations
	      WHERE deleted_at IS NULL AND trim(title) <> '' AND trim(content) <> ''
	        AND (` + strings.Join(where, " OR ") + `)
	        AND (updated_at > ? OR (updated_at = ? AND id > ?))
	      ORDER BY updated_at, id`
	args = append(args, cur.UpdatedAt, cur.UpdatedAt, cur.ID)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, cur, fmt.Errorf("query engram observations: %w", err)
	}
	defer rows.Close()

	var out []Observation
	next := cur
	for rows.Next() {
		var o Observation
		if err := rows.Scan(&o.ID, &o.SyncID, &o.Project, &o.TopicKey, &o.Type, &o.Title, &o.Content, &o.UpdatedAt); err != nil {
			return nil, cur, fmt.Errorf("scan engram observation: %w", err)
		}
		out = append(out, o)
		next = Cursor{UpdatedAt: o.UpdatedAt, ID: o.ID}
	}
	if err := rows.Err(); err != nil {
		return nil, cur, fmt.Errorf("read engram observations: %w", err)
	}
	return out, next, nil
}
