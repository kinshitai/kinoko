package skill

import (
	"os"
	"path/filepath"
	"testing"
)

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
