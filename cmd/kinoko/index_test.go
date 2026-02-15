package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindLatestSkillMD(t *testing.T) {
	dir := t.TempDir()

	// Create version directories with SKILL.md files.
	for _, v := range []string{"v1", "v2", "v3"} {
		vdir := filepath.Join(dir, v)
		os.MkdirAll(vdir, 0o755)
		os.WriteFile(filepath.Join(vdir, "SKILL.md"), []byte("# "+v), 0o644)
	}

	got, err := findLatestSkillMD(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Should find v3 (latest when sorted descending).
	if filepath.Base(filepath.Dir(got)) != "v3" {
		t.Errorf("got %q, want v3/SKILL.md", got)
	}
}

func TestFindLatestSkillMD_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := findLatestSkillMD(dir)
	if err == nil {
		t.Fatal("expected error for empty dir")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "", "c"); got != "c" {
		t.Errorf("got %q, want c", got)
	}
	if got := firstNonEmpty("a", "b"); got != "a" {
		t.Errorf("got %q, want a", got)
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
