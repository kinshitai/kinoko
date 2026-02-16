package api

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadSkillContent_PathTraversal(t *testing.T) {
	// Create a file that should NOT be accessible via traversal.
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(secret, []byte("top secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{"empty path", "", ""},
		{"dot-dot traversal", "../../../etc/passwd", ""},
		{"dot-dot in middle", "skills/../../../etc/passwd", ""},
		// Note: /tmp/../etc/passwd cleans to /etc/passwd (no ".." remains),
		// so it's not blocked by traversal check — it just fails to read.
		{"valid path", filepath.Join(dir, "nonexistent.md"), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := readSkillContent(tt.path)
			if got != tt.want {
				t.Errorf("readSkillContent(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestReadSkillContent_ValidFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "SKILL.md")
	content := "# Test Skill\nSome content."
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got := readSkillContent(p)
	if got != content {
		t.Errorf("got %q, want %q", got, content)
	}
}
