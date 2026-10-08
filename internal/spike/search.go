// F0 prototype: isolated in-memory FTS5 search using pure-Go SQLite.
// This package is internal/spike — not production code.
//
// Data is ephemeral (in-memory only), seeded with three synthetic fixtures.
// The FTS5 search tool is explicitly diagnostic and read-only.
// No real knowledge, no production store, no migrations.

package spike

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"

	_ "modernc.org/sqlite" // register sqlite driver
)

const maxQueryLen = 200 // characters

// Hit represents a single search result from the FTS5 index.
type Hit struct {
	Title string  `json:"title"`
	Text  string  `json:"text"`
	Rank  float64 `json:"rank"`
}

// Result wraps FTS5 hits with diagnostic metadata.
type Result struct {
	Hits       []Hit  `json:"hits"`
	TotalFound int    `json:"total_found"`
	Query      string `json:"query"`
}

// Server wraps an in-memory SQLite database with an FTS5 virtual table.
type Server struct {
	db *sql.DB
}

// NewServer creates a new FTS5 search server backed by an in-memory SQLite
// database. The database is seeded with synthetic fixture data on success.
func NewServer() (*Server, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	s := &Server{db: db}
	if err := s.init(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Server) init() error {
	_, err := s.db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS docs USING fts5(
			title,
			text,
			tokenize='unicode61 remove_diacritics 2'
		);
	`)
	if err != nil {
		return err
	}

	// Three explicit synthetic fixtures. "política" and "café" test
	// accent-insensitive matching via unicode61 remove_diacritics.
	fixtures := [][2]string{
		{"Diagnostic Fixture Alpha", "A synthetic record for diagnostic testing."},
		{"Política Fixture Beta", "A synthetic record about policy matters."},
		{"Café Fixture Gamma", "A synthetic record mentioning café culture."},
	}
	for _, f := range fixtures {
		if _, err := s.db.Exec(
			"INSERT INTO docs (title, text) VALUES (?, ?)", f[0], f[1],
		); err != nil {
			return err
		}
	}
	return nil
}

// Search executes an FTS5 full-text search against the in-memory index.
// The query is parameterized to prevent injection; double-quotes in the
// user input are doubled so they are treated as literal characters by FTS5.
func (s *Server) Search(ctx context.Context, query string) (*Result, error) {
	if s.db == nil {
		return nil, errors.New("server is closed")
	}

	q := strings.TrimSpace(query)
	if q == "" {
		return nil, errors.New("query cannot be empty")
	}
	if utf8.RuneCountInString(q) > maxQueryLen {
		return nil, errors.New("query exceeds 200 characters")
	}

	// FTS5 literal phrase: wrap the escaped user string in double quotes.
	// Each double-quote in user input is doubled so they are treated as
	// literal characters by FTS5. The outer quotes ensure FTS operators
	// like AND, OR, NOT are treated as literal text.
	safe := "\"" + strings.ReplaceAll(q, `"`, `""`) + "\""

	// Parameterized query with the escaped literal phrase.
	var total int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM docs WHERE docs MATCH ?`, safe,
	).Scan(&total); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT bm25(docs), title, text
		FROM docs
		WHERE docs MATCH ?
		ORDER BY bm25(docs)
		LIMIT 5
	`, safe)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hits []Hit
	for rows.Next() {
		var rank float64
		var title, text string
		if err := rows.Scan(&rank, &title, &text); err != nil {
			return nil, err
		}
		hits = append(hits, Hit{Title: title, Text: text, Rank: rank})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &Result{Hits: hits, TotalFound: total, Query: q}, nil
}

// Close releases the SQLite connection.
func (s *Server) Close(ctx context.Context) {
	if s.db != nil {
		s.db.Close()
		s.db = nil
	}
}
