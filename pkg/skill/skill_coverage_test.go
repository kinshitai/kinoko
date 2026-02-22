package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseFile_Success(t *testing.T) {
	content := `---
name: parse-file-test
description: Test ParseFile function
version: 1
author: test-author
confidence: 0.9
created: 2026-02-20
---

# Parse File Test

## When to Use
When testing the ParseFile function.

## Solution
Write a temp file and call ParseFile.
`
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	skill, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	if skill.Name != "parse-file-test" {
		t.Errorf("name = %q, want parse-file-test", skill.Name)
	}
	if skill.Author != "test-author" {
		t.Errorf("author = %q, want test-author", skill.Author)
	}
	if skill.Confidence != 0.9 {
		t.Errorf("confidence = %f, want 0.9", skill.Confidence)
	}
}

func TestParseFile_NotFound(t *testing.T) {
	_, err := ParseFile("/nonexistent/path/SKILL.md")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestParseFile_InvalidContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte("not a skill file"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ParseFile(path)
	if err == nil {
		t.Fatal("expected error for invalid content")
	}
}
