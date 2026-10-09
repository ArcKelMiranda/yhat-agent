// F1-B: Read-only query helpers for the MCP tool layer.
// These are added as part of the F1-B work unit (work-unit commit) and do not
// modify the existing store.go file. The Store struct's private db field is
// accessed via a receiver method defined in this file.

package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// EnsureFTS5 creates the memories_fts FTS5 virtual table and its sync triggers
// if they do not already exist. It is safe to call repeatedly (CREATE VIRTUAL TABLE
// is idempotent in SQLite). Call this after the store is opened and before the
// first search.
func (s *Store) EnsureFTS5(ctx context.Context) error {
	// Create the FTS5 virtual table if absent.
	if _, err := s.db.ExecContext(ctx, `
		CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
			title,
			content,
			tokenize='unicode61 remove_diacritics 2'
		);
	`); err != nil {
		return fmt.Errorf("create memories_fts: %w", err)
	}

	// Create sync triggers if absent (check via a simple existence test).
	var triggerCount int
	_ = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name='trg_memories_ai'`,
	).Scan(&triggerCount)
	if triggerCount == 0 {
		triggers := []string{
			`CREATE TRIGGER IF NOT EXISTS trg_memories_ai AFTER INSERT ON memories BEGIN
				INSERT INTO memories_fts(rowid, title, content) VALUES (NEW.rowid, NEW.title, NEW.content);
			END`,
			`CREATE TRIGGER IF NOT EXISTS trg_memories_ad AFTER DELETE ON memories BEGIN
				DELETE FROM memories_fts WHERE rowid = OLD.rowid;
			END`,
			`CREATE TRIGGER IF NOT EXISTS trg_memories_au AFTER UPDATE ON memories BEGIN
				DELETE FROM memories_fts WHERE rowid = OLD.rowid;
				INSERT INTO memories_fts(rowid, title, content) VALUES (NEW.rowid, NEW.title, NEW.content);
			END`,
		}
		for _, trigger := range triggers {
			if _, err := s.db.ExecContext(ctx, trigger); err != nil {
				return fmt.Errorf("create trigger: %w", err)
			}
		}
	}
	return nil
}

// ListPendingMemories returns local 'proposed' memories for the operator,
// oldest first, with an optional limit. The count field reflects the total
// (unpaginated) number of pending records.
func (s *Store) ListPendingMemories(ctx context.Context, limit int) ([]Memory, int, error) {
	if limit < 1 {
		limit = 50
	}
	if limit > 50 {
		limit = 50
	}

	var total int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM memories WHERE origin='local' AND status='proposed'`,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, origin, type, title, content, context, content_hash, author,
		       status, validated_by, validated_at, reject_reason,
		       share_status, shared_at, remote_id, team_status, team_updated_at,
		       is_example, created_at, updated_at
		FROM memories
		WHERE origin='local' AND status='proposed'
		ORDER BY created_at ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []Memory
	for rows.Next() {
		mem, err := scanMemoryRow(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, mem)
	}
	return items, total, rows.Err()
}

// SearchMemoriesFTS5 executes a full-text search against the memories_fts FTS5
// virtual table and returns the matching memories. The query must be a valid
// FTS5 MATCH expression (literal phrase form with outer quotes).
// Returns up to limit results.
func (s *Store) SearchMemoriesFTS5(ctx context.Context, escapedQuery string, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 5
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT m.id, m.origin, m.type, m.title, m.content, m.context, m.content_hash, m.author,
		       m.status, m.validated_by, m.validated_at, m.reject_reason,
		       m.share_status, m.shared_at, m.remote_id, m.team_status, m.team_updated_at,
		       m.is_example, m.created_at, m.updated_at
		FROM memories m
		JOIN memories_fts f ON m.rowid = f.rowid
		WHERE memories_fts MATCH ?
		ORDER BY bm25(memories_fts)
		LIMIT ?
	`, escapedQuery, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Memory
	for rows.Next() {
		mem, err := scanMemoryRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, mem)
	}
	return items, rows.Err()
}

// StoreDB exposes the underlying *sql.DB for read-only FTS5 queries from the
// MCP tool layer. Callers must not mutate data through this connection.
func (s *Store) StoreDB() *sql.DB {
	return s.db
}

// scanMemoryRow scans a memories row into a Memory struct.
// This is a copy of the private helper from store.go, scoped to this file.
func scanMemoryRow(rows *sql.Rows) (Memory, error) {
	var mem Memory
	var contextPtr sql.NullString
	var validatedBy sql.NullString
	var validatedAt sql.NullString
	var rejectReason sql.NullString
	var sharedAt sql.NullString
	var remoteID sql.NullString
	var teamStatus sql.NullString
	var teamUpdatedAt sql.NullString
	var isExample int

	err := rows.Scan(
		&mem.ID, &mem.Origin, &mem.Type, &mem.Title, &mem.Content, &contextPtr,
		&mem.ContentHash, &mem.Author, &mem.Status, &validatedBy, &validatedAt,
		&rejectReason, &mem.ShareStatus, &sharedAt, &remoteID, &teamStatus,
		&teamUpdatedAt, &isExample, &mem.CreatedAt, &mem.UpdatedAt,
	)
	if err != nil {
		return Memory{}, err
	}
	if contextPtr.Valid {
		mem.Context = &contextPtr.String
	}
	if validatedBy.Valid {
		mem.ValidatedBy = &validatedBy.String
	}
	if validatedAt.Valid {
		t, _ := time.Parse(time.RFC3339, validatedAt.String)
		mem.ValidatedAt = &t
	}
	if rejectReason.Valid {
		mem.RejectReason = &rejectReason.String
	}
	if sharedAt.Valid {
		t, _ := time.Parse(time.RFC3339, sharedAt.String)
		mem.SharedAt = &t
	}
	if remoteID.Valid {
		mem.RemoteID = &remoteID.String
	}
	if teamStatus.Valid {
		mem.TeamStatus = &teamStatus.String
	}
	if teamUpdatedAt.Valid {
		t, _ := time.Parse(time.RFC3339, teamUpdatedAt.String)
		mem.TeamUpdatedAt = &t
	}
	mem.IsExample = isExample != 0
	return mem, nil
}
