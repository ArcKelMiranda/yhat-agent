// Package sensitive provides detection of secrets and sensitive content
// in text. It ships a default set of high-confidence credential patterns
// and exposes Scan for single-pass matching against those patterns.
//
// All patterns are compiled once at package init as regexp.Regexp values,
// so there is no per-call compilation overhead.
//
// Match positions are byte indices into the original text (compatible with
// Go's string indexing). No matches returns an empty (not nil) slice.
package sensitive

import (
	"regexp"
)

// Match describes a detected secret in the input text.
type Match struct {
	// Category is the short human-readable label, e.g. "aws_access_key".
	Category string
	// Pattern is the descriptive pattern name used for the match.
	Pattern string
	// Start is the byte offset of the first character of the match.
	Start int
	// End is the byte offset one-past the last character of the match.
	End int
}

// NamedPattern is a compiled pattern with a display name.
type NamedPattern struct {
	Name    string
	Regexp  *regexp.Regexp
	Pattern string // the source pattern string for documentation
}

// DefaultPatterns returns the default credential patterns in the order
// they should be displayed to the user. The patterns are:
//
//   - aws_access_key   : AKIA[0-9A-Z]{16}
//   - github_pat       : ghp_|gho_|ghu_|ghs_|ghr_ followed by ≥20 chars
//   - pem_private_key  : -----BEGIN (RSA | EC | OPENSSH | ) PRIVATE KEY-----
//   - bearer_jwt       : Bearer eyJ
//   - slack_token      : xox[baprs]-
//   - anthropic_key    : sk-ant-
//   - openai_key       : sk-
//
// Patterns are compiled once at package init.
func DefaultPatterns() []NamedPattern {
	return defaultPatterns
}

// Scan runs text against all DefaultPatterns and returns every match.
// Returns an empty slice when nothing matches. Positions are byte indices.
func Scan(text string) []Match {
	var matches []Match
	for _, np := range defaultPatterns {
		// FindAllStringIndex with n=-1 returns all non-overlapping matches.
		for _, loc := range np.Regexp.FindAllStringIndex(text, -1) {
			matches = append(matches, Match{
				Category: np.Name,
				Pattern:  np.Pattern,
				Start:    loc[0],
				End:      loc[1],
			})
		}
	}
	return matches
}

// ─── Package init: compile all patterns once ─────────────────────────────────

var defaultPatterns []NamedPattern

func init() {
	defaultPatterns = []NamedPattern{
		{
			Name:    "aws_access_key",
			Pattern: "AKIA[0-9A-Z]{16}",
			Regexp:  mustCompile(`AKIA[0-9A-Z]{16}`),
		},
		{
			Name:    "github_pat",
			Pattern: "ghp_|gho_|ghu_|ghs_|ghr_ + ≥20 chars",
			// Character class excludes _ so the token boundary is respected.
			Regexp: mustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`),
		},
		{
			Name:    "pem_private_key",
			Pattern: "-----BEGIN ... PRIVATE KEY-----",
			Regexp: mustCompile(
				`-----BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY-----`,
			),
		},
		{
			Name:    "bearer_jwt",
			Pattern: "Bearer eyJ",
			Regexp:  mustCompile(`Bearer eyJ`),
		},
		{
			Name:    "slack_token",
			Pattern: "xox[baprs]-",
			Regexp:  mustCompile(`xox[baprs]-[A-Za-z0-9]+`),
		},
		{
			Name:    "anthropic_key",
			Pattern: "sk-ant-",
			Regexp:  mustCompile(`sk-ant-[A-Za-z0-9_-]+`),
		},
		{
			Name:    "openai_key",
			Pattern: "sk-",
			Regexp:  mustCompile(`sk-[A-Za-z0-9_-]{10,}`),
		},
	}
}

func mustCompile(pattern string) *regexp.Regexp {
	re, err := regexp.Compile(pattern)
	if err != nil {
		panic("sensitive: invalid pattern: " + pattern + ": " + err.Error())
	}
	return re
}
