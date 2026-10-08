// F0 prototype tests: in-memory FTS5 search.

package spike

import (
	"context"
	"strings"
	"testing"
)

func TestSearch_CaseInsensitive(t *testing.T) {
	t.Parallel()
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close(context.Background())

	// Case-insensitive match: "DIAGNOSTIC" matches "Diagnostic Fixture Alpha".
	result, err := srv.Search(context.Background(), "DIAGNOSTIC")
	if err != nil {
		t.Fatalf("Search(DIAGNOSTIC): %v", err)
	}
	if len(result.Hits) == 0 {
		t.Error("Search(DIAGNOSTIC): expected hits")
	}
}

func TestSearch_AccentInsensitive(t *testing.T) {
	t.Parallel()
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close(context.Background())

	// Accent-insensitive: "politica" (no accent) matches "Política" (with accent).
	result, err := srv.Search(context.Background(), "politica")
	if err != nil {
		t.Fatalf("Search(politica): %v", err)
	}
	if len(result.Hits) == 0 {
		t.Fatal("Search(politica): expected hits for Política fixture")
	}
	var found bool
	for _, h := range result.Hits {
		if strings.Contains(strings.ToLower(h.Title+h.Text), "pol") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Search(politica): expected Política fixture, got %d hits", len(result.Hits))
	}
}

func TestSearch_AccentInsensitive_Cafe(t *testing.T) {
	t.Parallel()
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close(context.Background())

	// Accent-insensitive: "cafe" matches "Café".
	result, err := srv.Search(context.Background(), "cafe")
	if err != nil {
		t.Fatalf("Search(cafe): %v", err)
	}
	var found bool
	for _, h := range result.Hits {
		if strings.Contains(strings.ToLower(h.Title+h.Text), "caf") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Search(cafe): expected café fixture, got %d hits", len(result.Hits))
	}
}

func TestSearch_NoResults(t *testing.T) {
	t.Parallel()
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close(context.Background())

	result, err := srv.Search(context.Background(), "xyzzy_nonexistent_trebuchet")
	if err != nil {
		t.Fatalf("Search(xyzzy): %v", err)
	}
	if len(result.Hits) != 0 {
		t.Errorf("Search(xyzzy): expected no hits, got %d", len(result.Hits))
	}
}

func TestSearch_EmptyRejected(t *testing.T) {
	t.Parallel()
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close(context.Background())

	for _, input := range []string{"", "   ", "\t"} {
		_, err := srv.Search(context.Background(), input)
		if err == nil {
			t.Errorf("Search(%q): expected error for empty/whitespace", input)
		}
	}
}

func TestSearch_TooLong(t *testing.T) {
	t.Parallel()
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close(context.Background())

	long := strings.Repeat("a", 201)
	_, err = srv.Search(context.Background(), long)
	if err == nil {
		t.Error("Search(201 chars): expected error")
	}
}

func TestSearch_Close(t *testing.T) {
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	srv.Close(context.Background())

	_, err = srv.Search(context.Background(), "test")
	if err == nil {
		t.Error("Search after Close: expected error")
	}
}

func TestSearch_FTSOperators_TreatedLiterally(t *testing.T) {
	t.Parallel()
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close(context.Background())

	// AND, OR, NOT are FTS operators but must be treated as literal text
	// when the query is properly wrapped in double quotes.
	queries := []string{
		"diagnostic AND cafe",
		"diagnostic OR cafe",
		"diagnostic NOT cafe",
		"title:diagnostic",
	}
	for _, q := range queries {
		result, err := srv.Search(context.Background(), q)
		if err != nil {
			t.Errorf("Search(%q): unexpected error: %v", q, err)
			continue
		}
		// These operator-containing queries must return ZERO results
		// because no document contains the literal text "AND", "OR", "NOT", or "title:diagnostic".
		if len(result.Hits) != 0 {
			t.Errorf("Search(%q): expected 0 hits for operator query, got %d", q, len(result.Hits))
		}
	}
}

func TestSearch_UnmatchedQuoteHandled(t *testing.T) {
	t.Parallel()
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close(context.Background())

	// Unmatched quote must not cause errors; FTS5 parses it as a literal phrase.
	// The query "diagnostic"" becomes the literal phrase "diagnostic\" (with quote).
	// This matches "Diagnostic Fixture Alpha" because the phrase is a prefix match
	// and FTS5 considers "diagnostic" within the title.
	result, err := srv.Search(context.Background(), `diagnostic"`)
	if err != nil {
		t.Errorf("Search(unmatched quote): unexpected error: %v", err)
		return
	}
	// The key requirement is NO ERROR - the unmatched quote is handled safely.
	// Results are returned based on FTS5's literal phrase parsing, not a crash.
	if len(result.Hits) == 0 {
		t.Log("Search(unmatched quote): no hits (acceptable - phrase not found)")
	} else {
		t.Logf("Search(unmatched quote): got %d hit(s) - FTS5 literal phrase parsed safely", len(result.Hits))
	}
}

func TestSearch_AccentedRuneLimit_200(t *testing.T) {
	t.Parallel()
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close(context.Background())

	// 200 accented runes must be allowed.
	long200 := strings.Repeat("á", 200)
	result, err := srv.Search(context.Background(), long200)
	if err != nil {
		t.Errorf("Search(200 accented runes): expected success, got %v", err)
		return
	}
	if len(result.Hits) != 0 {
		t.Errorf("Search(200 accented runes): expected 0 hits, got %d", len(result.Hits))
	}
}

func TestSearch_AccentedRuneLimit_201(t *testing.T) {
	t.Parallel()
	srv, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close(context.Background())

	// 201 accented runes must be rejected.
	long201 := strings.Repeat("á", 201)
	_, err = srv.Search(context.Background(), long201)
	if err == nil {
		t.Error("Search(201 accented runes): expected error, got nil")
	}
}
