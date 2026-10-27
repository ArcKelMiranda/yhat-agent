package yhatagent

import (
	"os"
	"strings"
	"testing"
)

func setupClaudeCodeEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	return dir
}

func TestClaudeCodeSkill_AdaptsFrontmatter(t *testing.T) {
	src, err := Assets.ReadFile(SkillAsset)
	if err != nil {
		t.Fatal(err)
	}
	out, err := ClaudeCodeSkill(src)
	if err != nil {
		t.Fatal(err)
	}
	front, body, _ := strings.Cut(strings.TrimPrefix(string(out), "---\n"), "\n---\n")
	if strings.Contains(front, "compatibility:") || strings.Contains(front, "agent is selected") {
		t.Errorf("OpenCode-only frontmatter leaked:\n%s", front)
	}
	for _, want := range []string{"name: yhat-memory-capture", "description: ", "version:"} {
		if !strings.Contains(front, want) {
			t.Errorf("frontmatter missing %q:\n%s", want, front)
		}
	}
	if !strings.Contains(string(src), body) {
		t.Error("skill body was altered")
	}
}

func TestClaudeCodeSkill_RejectsBadFrontmatter(t *testing.T) {
	for _, in := range []string{"no frontmatter", "---\nname: x\nno close"} {
		if _, err := ClaudeCodeSkill([]byte(in)); err == nil {
			t.Errorf("expected error for %q", in)
		}
	}
}

func TestInstallClaudeCodeSkill_InstallIdempotentAndDrift(t *testing.T) {
	root := setupClaudeCodeEnv(t)
	target := root + "/skills/yhat-memory-capture/SKILL.md"

	r, err := InstallClaudeCodeSkill()
	if err != nil || r.State != StateInstalled || r.TargetPath != target {
		t.Fatalf("first install: %+v, %v", r, err)
	}
	if r, err = InstallClaudeCodeSkill(); err != nil || r.Message != "already installed" {
		t.Fatalf("second install: %+v, %v", r, err)
	}

	if err := os.WriteFile(target, []byte("user edit"), 0644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // second run is idempotent
		r, err = InstallClaudeCodeSkill()
		if err != nil || r.State != StateCandidate {
			t.Fatalf("drift run %d: %+v, %v", i, r, err)
		}
	}
	if got, _ := os.ReadFile(target); string(got) != "user edit" {
		t.Error("drifted user file was overwritten")
	}
	if !fileExists(CandidateName(target)) {
		t.Error("candidate not written")
	}
}

func TestInstallClaudeCodeSkill_DoesNotTouchOpenCodeAssets(t *testing.T) {
	setupClaudeCodeEnv(t)
	cfg, cleanup := setupTestEnv(t)
	defer cleanup()
	if _, err := InstallClaudeCodeSkill(); err != nil {
		t.Fatal(err)
	}
	if fileExists(cfg + "/opencode") {
		t.Error("Claude Code install created OpenCode files")
	}
}

func TestClaudeCodeStatus_States(t *testing.T) {
	setupClaudeCodeEnv(t)
	if fi, _ := ClaudeCodeStatus(); fi.State != StateMissing {
		t.Fatalf("want missing, got %v", fi.State)
	}
	if _, err := InstallClaudeCodeSkill(); err != nil {
		t.Fatal(err)
	}
	if fi, _ := ClaudeCodeStatus(); fi.State != StateInstalled {
		t.Fatalf("want installed, got %v", fi.State)
	}
	os.WriteFile(ClaudeCodeSkillTargetPath(), []byte("edit"), 0644)
	if fi, _ := ClaudeCodeStatus(); fi.State != StateDrift {
		t.Fatalf("want drift, got %v", fi.State)
	}
	InstallClaudeCodeSkill()
	if fi, _ := ClaudeCodeStatus(); fi.State != StateCandidate {
		t.Fatalf("want candidate, got %v", fi.State)
	}
}

func TestUninstallClaudeCodeSkill(t *testing.T) {
	root := setupClaudeCodeEnv(t)
	if reason, err := UninstallClaudeCodeSkill(); reason != "" || err != nil {
		t.Fatalf("absent: %q %v", reason, err)
	}
	InstallClaudeCodeSkill()
	if reason, err := UninstallClaudeCodeSkill(); reason != "" || err != nil {
		t.Fatalf("pristine: %q %v", reason, err)
	}
	if fileExists(root + "/skills/yhat-memory-capture") {
		t.Error("empty skill dir left behind")
	}

	InstallClaudeCodeSkill()
	os.WriteFile(ClaudeCodeSkillTargetPath(), []byte("edit"), 0644)
	if reason, _ := UninstallClaudeCodeSkill(); reason == "" {
		t.Error("expected skip reason for modified skill")
	}
	if got, _ := os.ReadFile(ClaudeCodeSkillTargetPath()); string(got) != "edit" {
		t.Error("user-modified skill was removed")
	}
}
