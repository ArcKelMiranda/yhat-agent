package sensitive

import (
	"strings"
	"testing"
)

func TestScan_NoMatch_ReturnsEmpty(t *testing.T) {
	clean := []string{
		"Hola mundo",
		"El servidor está en AWS us-east-1",
		"Mi título de decisión",
		"Contenido normal sin secretos",
		"",
		"AKIAIOSFODNN7", // too short, 13 chars instead of 20
		"ghp_",          // prefix only, no 20 chars
		"sk-ant-",       // prefix only, no continuation
		"sk-",           // just the prefix
		"xoxb-",         // Slack prefix only, missing after dash
		"-----BEGIN CERTIFICATE-----",
		"-----BEGIN RSA PUBLIC KEY-----",
		"-----BEGIN EC PUBLIC KEY-----",
		"Authorization: Bearer", // no JWT payload
	}
	for _, text := range clean {
		matches := Scan(text)
		if len(matches) != 0 {
			t.Errorf("Scan(%q): expected 0 matches, got %d: %v", text, len(matches), matches)
		}
	}
}

func TestScan_AWS(t *testing.T) {
	// Valid AWS key: AKIA + 16 alphanumeric chars (total 20)
	text := "AWS_SECRET=AKIAIOSFODNN7EXMPL00"

	matches := Scan(text)
	if len(matches) != 1 {
		t.Fatalf("Scan(AWS): expected 1 match, got %d", len(matches))
	}
	if matches[0].Category != "aws_access_key" {
		t.Errorf("Category: want aws_access_key, got %q", matches[0].Category)
	}
	if matches[0].Pattern != "AKIA[0-9A-Z]{16}" {
		t.Errorf("Pattern: want %q, got %q", "AKIA[0-9A-Z]{16}", matches[0].Pattern)
	}
	// Positions must be byte indices into the original string.
	idx := strings.Index(text, "AKIAIOSFODNN7EXMPL00")
	if idx == -1 {
		t.Fatalf("AKIAIOSFODNN7EXMPL00 not found in %q", text)
	}
	if matches[0].Start != idx || matches[0].End != idx+20 {
		t.Errorf("Start/End: want (%d,%d), got (%d,%d)", idx, idx+20, matches[0].Start, matches[0].End)
	}
}

func TestScan_AWS_InvalidTooShort(t *testing.T) {
	// AKIA + fewer than 16 chars should NOT match.
	texts := []string{
		"AKIAIOSFODNN7",
		"AKIA1234X7",
		"AKIA1234X7890EXMPL", // 24 chars - should still match
		"AKIAXXAA",              // too short
	}
	for _, text := range texts {
		matches := Scan(text)
		// Only the ones with AKIA + exactly 16 alphanumeric should match.
		if len(matches) != 0 && text == "AKIAIOSFODNN7EXMPL00" {
			continue
		}
		if len(matches) != 0 && strings.HasPrefix(text, "AKIA") && len(strings.TrimPrefix(text, "AKIA")) == 16 {
			continue
		}
		// Any other text with AKIA prefix should not match if wrong length
		if strings.HasPrefix(text, "AKIA") {
			// Only exact 20-char AKIA keys should match
			rest := strings.TrimPrefix(text, "AKIA")
			if len(rest) != 16 || !isAlphanumeric16(rest) {
				if len(matches) != 0 {
					t.Errorf("Scan(%q): expected 0 matches (invalid key length), got %d", text, len(matches))
				}
			}
		}
	}
}

func isAlphanumeric16(s string) bool {
	if len(s) != 16 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')) {
			return false
		}
	}
	return true
}

func TestScan_GitHubPAT_ghp(t *testing.T) {
	text := "ghp_EXAMPLE0000000000000000EXAMPLEX"

	matches := Scan(text)
	if len(matches) != 1 {
		t.Fatalf("Scan(ghp_): expected 1 match, got %d", len(matches))
	}
	if matches[0].Category != "github_pat" {
		t.Errorf("Category: want github_pat, got %q", matches[0].Category)
	}
	if matches[0].Start != 0 || matches[0].End != len(text) {
		t.Errorf("Start/End: want (0,%d), got (%d,%d)", len(text), matches[0].Start, matches[0].End)
	}
}

func TestScan_GitHubPAT_AllPrefixes(t *testing.T) {
	prefixes := []string{"ghp_", "gho_", "ghu_", "ghs_", "ghr_"}
	for _, prefix := range prefixes {
		// 20 chars of 'x' after prefix = 24-char string.
		text := prefix + strings.Repeat("X", 20)
		matches := Scan(text)
		if len(matches) != 1 {
			t.Errorf("Scan(%q): expected 1 match, got %d", text, len(matches))
			continue
		}
		if matches[0].Category != "github_pat" {
			t.Errorf("Scan(%q): Category want github_pat, got %q", text, matches[0].Category)
		}
	}
}

func TestScan_GitHubPAT_TooShort_NoMatch(t *testing.T) {
	// ghp_ followed by only 19 chars should NOT match.
	text := "ghp_" + strings.Repeat("x", 19)
	matches := Scan(text)
	if len(matches) != 0 {
		t.Errorf("Scan(ghp_ with 19 chars): expected 0 matches, got %d", len(matches))
	}
}

func TestScan_PEMPrivateKey(t *testing.T) {
	text := "-----BEGIN PRIVATE KEY-----\nMFCo...\n-----END PRIVATE KEY-----"

	matches := Scan(text)
	if len(matches) != 1 {
		t.Fatalf("Scan(PEM): expected 1 match, got %d", len(matches))
	}
	if matches[0].Category != "pem_private_key" {
		t.Errorf("Category: want pem_private_key, got %q", matches[0].Category)
	}
	if matches[0].Start != 0 {
		t.Errorf("Start: want 0, got %d", matches[0].Start)
	}
	if matches[0].End != len("-----BEGIN PRIVATE KEY-----") {
		t.Errorf("End: want %d, got %d", len("-----BEGIN PRIVATE KEY-----"), matches[0].End)
	}
}

func TestScan_PEM_RSAPrivateKey(t *testing.T) {
	text := "-----BEGIN RSA PRIVATE KEY-----\ndata\n-----END RSA PRIVATE KEY-----"

	matches := Scan(text)
	if len(matches) != 1 {
		t.Fatalf("Scan(RSA PEM): expected 1 match, got %d", len(matches))
	}
	if matches[0].Category != "pem_private_key" {
		t.Errorf("Category: want pem_private_key, got %q", matches[0].Category)
	}
}

func TestScan_PEM_EC(t *testing.T) {
	text := "-----BEGIN EC PRIVATE KEY-----\ndata\n-----END EC PRIVATE KEY-----"

	matches := Scan(text)
	if len(matches) != 1 {
		t.Fatalf("Scan(EC PEM): expected 1 match, got %d", len(matches))
	}
	if matches[0].Category != "pem_private_key" {
		t.Errorf("Category: want pem_private_key, got %q", matches[0].Category)
	}
}

func TestScan_PEM_OpenSSH(t *testing.T) {
	text := "-----BEGIN OPENSSH PRIVATE KEY-----\ndata\n-----END OPENSSH PRIVATE KEY-----"

	matches := Scan(text)
	if len(matches) != 1 {
		t.Fatalf("Scan(OpenSSH PEM): expected 1 match, got %d", len(matches))
	}
	if matches[0].Category != "pem_private_key" {
		t.Errorf("Category: want pem_private_key, got %q", matches[0].Category)
	}
}

func TestScan_PEM_Certificate_NotBlocked(t *testing.T) {
	text := "-----BEGIN CERTIFICATE-----\ncert\n-----END CERTIFICATE-----"

	matches := Scan(text)
	if len(matches) != 0 {
		t.Errorf("Scan(CERTIFICATE): expected 0 matches, got %d", len(matches))
	}
}

func TestScan_BearerJWT(t *testing.T) {
	text := "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.EXAMPLE-SIGNATURE-EXMP"

	matches := Scan(text)
	if len(matches) != 1 {
		t.Fatalf("Scan(Bearer JWT): expected 1 match, got %d", len(matches))
	}
	if matches[0].Category != "bearer_jwt" {
		t.Errorf("Category: want bearer_jwt, got %q", matches[0].Category)
	}
}

func TestScan_BearerJWT_Alone(t *testing.T) {
	text := "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.EXAMPLE-SIGNATURE-EXMP"

	matches := Scan(text)
	if len(matches) != 1 {
		t.Fatalf("Scan(Bearer eyJ): expected 1 match, got %d", len(matches))
	}
	if matches[0].Category != "bearer_jwt" {
		t.Errorf("Category: want bearer_jwt, got %q", matches[0].Category)
	}
}

func TestScan_Bearer_NoJWT_NoMatch(t *testing.T) {
	texts := []string{
		"Authorization: Bearer",
		"Authorization: Bearer ",
		"Authorization: Basic dXNlcjpwYXNz",
	}
	for _, text := range texts {
		matches := Scan(text)
		if len(matches) != 0 {
			t.Errorf("Scan(%q): expected 0 matches, got %d", text, len(matches))
		}
	}
}

func TestScan_Slack(t *testing.T) {
	text := "xoxb-EXAMPLE-EXAMPLE-EXAMPLE-EXAMPLE-EXMP"

	matches := Scan(text)
	if len(matches) != 1 {
		t.Fatalf("Scan(Slack xoxb): expected 1 match, got %d", len(matches))
	}
	if matches[0].Category != "slack_token" {
		t.Errorf("Category: want slack_token, got %q", matches[0].Category)
	}
}

func TestScan_Slack_AllTypes(t *testing.T) {
	tokens := []string{
		"xoxb-EXAMPLE-EXAMPLE-EXAMPLE-EXMP",
		"xoxa-EXAMPLE-EXAMPLE-EXAMPLE-EXMP",
		"xoxp-EXAMPLE-EXAMPLE-EXAMPLE-EXMP",
		"xoxr-EXAMPLE-EXAMPLE-EXAMPLE-EXMP",
		"xoxs-EXAMPLE-EXAMPLE-EXAMPLE-EXMP",
	}
	for _, token := range tokens {
		matches := Scan(token)
		if len(matches) != 1 {
			t.Errorf("Scan(%q): expected 1 match, got %d", token, len(matches))
			continue
		}
		if matches[0].Category != "slack_token" {
			t.Errorf("Scan(%q): Category want slack_token, got %q", token, matches[0].Category)
		}
	}
}

func TestScan_Anthropic(t *testing.T) {
	text := "sk-ant-api03-EXAMPLE-EXAMPLE-EXAMPLE-EXAMPLE-EXMP"

	matches := Scan(text)
	if len(matches) < 1 {
		t.Fatalf("Scan(Anthropic): expected at least 1 match, got %d", len(matches))
	}
	// First match must be anthropic_key (pattern order: anthropic before openai).
	if matches[0].Category != "anthropic_key" {
		t.Errorf("Category: want anthropic_key, got %q", matches[0].Category)
	}
}

func TestScan_OpenAI(t *testing.T) {
	text := "sk-proj-EXAMPLE-EXAMPLE-EXAMPLE-EXAMPLE-EXAMPLE-EXAMPLE-EXAMPLE-EXAMPLE-EXMP"

	matches := Scan(text)
	if len(matches) != 1 {
		t.Fatalf("Scan(OpenAI): expected 1 match, got %d", len(matches))
	}
	if matches[0].Category != "openai_key" {
		t.Errorf("Category: want openai_key, got %q", matches[0].Category)
	}
}

func TestScan_OpenAI_skPrefix(t *testing.T) {
	text := "sk-EXAMPLE-EXAMPLE-EXAMPLE-EXAMPLE-EXAMPLE-EXMP"

	matches := Scan(text)
	if len(matches) != 1 {
		t.Fatalf("Scan(sk-): expected 1 match, got %d", len(matches))
	}
	if matches[0].Category != "openai_key" {
		t.Errorf("Category: want openai_key, got %q", matches[0].Category)
	}
}

func TestScan_MultipleMatches(t *testing.T) {
	text := "ghp_EXAMPLE0000000000000000EXAMPLEX and AKIAIOSFODNN7EXMPL00"

	matches := Scan(text)
	if len(matches) != 2 {
		t.Errorf("Scan(multiple): expected 2 matches, got %d: %v", len(matches), matches)
	}
}

func TestScan_UTF8Positions(t *testing.T) {
	// Spanish text with accented characters before an AWS key.
	text := "Decisión: AKIAIOSFODNN7EXMPL00"

	matches := Scan(text)
	if len(matches) != 1 {
		t.Fatalf("Scan(UTF8): expected 1 match, got %d", len(matches))
	}
	// The "A" of "AKIA" is at byte offset 11 (0-indexed).
	// "Decisión: " = 11 bytes:
	// D(1)+e(1)+c(1)+i(1)+s(1)+i(1)+ó(2)+n(1)+:(1)+space(1) = 11 bytes.
	if matches[0].Start != 11 {
		t.Errorf("UTF8 Start: want 11, got %d", matches[0].Start)
	}
}

func TestScan_OnlyMatchesFirst(t *testing.T) {
	// When multiple matches exist, we return all of them.
	// But the MCP handler will use the first one to build the message.
	text := "ghp_EXAMPLE0000000000000000EXAMPLEX and another ghp_EXAMPLE0000000000YYYYYYY"

	matches := Scan(text)
	if len(matches) != 2 {
		t.Errorf("Scan(multiple ghp): expected 2 matches, got %d", len(matches))
	}
	if matches[0].Category != "github_pat" {
		t.Errorf("First match category: want github_pat, got %q", matches[0].Category)
	}
}

func TestDefaultPatterns(t *testing.T) {
	patterns := DefaultPatterns()
	if len(patterns) == 0 {
		t.Fatal("DefaultPatterns: expected non-empty slice")
	}

	// Each pattern must have a non-empty Name and a valid compiled Regexp.
	for _, p := range patterns {
		if p.Name == "" {
			t.Error("Pattern with empty Name")
		}
		if p.Regexp == nil {
			t.Errorf("Pattern %q: Regexp is nil", p.Name)
			continue
		}
		// Try matching on an empty string — should not panic.
		p.Regexp.MatchString("")
	}

	// Verify we have patterns for all expected categories.
	categorySet := make(map[string]bool)
	for _, p := range patterns {
		categorySet[p.Name] = true
	}
	expected := []string{
		"aws_access_key",
		"github_pat",
		"pem_private_key",
		"bearer_jwt",
		"slack_token",
		"anthropic_key",
		"openai_key",
	}
	for _, cat := range expected {
		if !categorySet[cat] {
			t.Errorf("DefaultPatterns: missing category %q", cat)
		}
	}
}
