package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArcKelMiranda/yhat-agent/internal/store"
)

func TestExportMD_WritesAllMemories(t *testing.T) {
	home := t.TempDir()
	t.Setenv("YHAT_HOME", home)
	s, err := store.Open(filepath.Join(home, "yhat.db"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.ProposeMemory(context.Background(), store.Memory{Type: store.MemoryTypeRule, Title: "t", Content: "c", Author: "a"})
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	os.RemoveAll(filepath.Join(home, "memories"))

	if code := runExportMD(); code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if _, err := os.Stat(filepath.Join(home, "memories", m.ID+".md")); err != nil {
		t.Errorf("memory not exported: %v", err)
	}
}

func TestExportMD_FailsWithoutStoreAndDoesNotCreateOne(t *testing.T) {
	home := t.TempDir()
	t.Setenv("YHAT_HOME", home)
	if code := runExportMD(); code != 1 {
		t.Fatalf("exit code %d, want 1", code)
	}
	if _, err := os.Stat(filepath.Join(home, "yhat.db")); err == nil {
		t.Error("export-md created a database")
	}
}
