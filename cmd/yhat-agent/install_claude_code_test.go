package main

import (
	"os"
	"strings"
	"testing"
)

func stubClaude(t *testing.T, getErr error) *[]string {
	t.Helper()
	var calls []string
	old := claudeRunner
	claudeRunner = func(args ...string) error {
		calls = append(calls, strings.Join(args, " "))
		if args[1] == "get" {
			return getErr
		}
		return nil
	}
	t.Cleanup(func() { claudeRunner = old })
	return &calls
}

func TestInstallClaudeCode_NoopWithoutClaudeDir(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir()+"/missing")
	calls := stubClaude(t, nil)
	if err := runInstallClaudeCode(); err != nil || len(*calls) != 0 {
		t.Fatalf("err=%v calls=%v", err, *calls)
	}
}

func TestInstallClaudeCode_RegistersWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	t.Setenv("PATH", t.TempDir())
	os.WriteFile(t.TempDir()+"/x", nil, 0644)
	// claude must be on PATH for registration; fake one.
	bin := t.TempDir()
	os.WriteFile(bin+"/claude", []byte("#!/bin/sh\n"), 0755)
	t.Setenv("PATH", bin)
	calls := stubClaude(t, os.ErrNotExist)
	if err := runInstallClaudeCode(); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 2 || !strings.HasPrefix((*calls)[1], "mcp add --scope user yhat -- ") {
		t.Fatalf("calls=%v", *calls)
	}
	if _, err := os.Stat(dir + "/skills/yhat-memory-capture/SKILL.md"); err != nil {
		t.Error("skill not installed")
	}
}

func TestInstallClaudeCode_SkipsWhenRegistered(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	bin := t.TempDir()
	os.WriteFile(bin+"/claude", []byte("#!/bin/sh\n"), 0755)
	t.Setenv("PATH", bin)
	calls := stubClaude(t, nil)
	if err := runInstallClaudeCode(); err != nil || len(*calls) != 1 {
		t.Fatalf("err=%v calls=%v", err, *calls)
	}
}

func TestUninstallClaudeCode_RemovesMCPWhenRegistered(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	bin := t.TempDir()
	os.WriteFile(bin+"/claude", []byte("#!/bin/sh\n"), 0755)
	t.Setenv("PATH", bin)
	calls := stubClaude(t, nil)
	runUninstallClaudeCode(true)
	if len(*calls) != 2 || (*calls)[1] != "mcp remove --scope user yhat" {
		t.Fatalf("calls=%v", *calls)
	}
}
