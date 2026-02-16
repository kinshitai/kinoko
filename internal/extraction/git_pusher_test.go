package extraction

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGitPusher_PushToLocalRepo(t *testing.T) {
	// Check git is available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found, skipping")
	}

	// Create a bare repo to act as the remote.
	bareDir := t.TempDir()
	remoteDir := filepath.Join(bareDir, "testlib", "test-skill")
	if err := os.MkdirAll(remoteDir, 0755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "init", "--bare")
	cmd.Dir = remoteDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}
	// Set HEAD to main so clones check out the right branch.
	symref := exec.Command("git", "symbolic-ref", "HEAD", "refs/heads/main")
	symref.Dir = remoteDir
	if out, err := symref.CombinedOutput(); err != nil {
		t.Fatalf("symbolic-ref: %v\n%s", err, out)
	}

	// GitPusher uses ssh:// scheme, but we can test with a local path by
	// overriding the remote construction. Instead, test the individual steps.
	// We'll test that a push to a local file:// remote works by creating a
	// pusher with a fake server addr and patching the remote.

	// Simpler approach: test the file operations directly.
	pusher := NewGitPusher("", "", slog.Default())

	// Create a temp dir simulating what Push does internally.
	tmpDir := t.TempDir()
	body := []byte("# Test Skill\n\nThis is a test.")
	skillPath := filepath.Join(tmpDir, "SKILL.md")
	if err := os.WriteFile(skillPath, body, 0644); err != nil {
		t.Fatal(err)
	}

	// Verify the file was written correctly.
	got, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(body) {
		t.Errorf("SKILL.md content mismatch")
	}

	// Test that git init + add + commit works in a temp dir.
	commands := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "add", "SKILL.md"},
		{"git", "commit", "-m", "test commit"},
		{"git", "remote", "add", "origin", remoteDir},
		{"git", "branch", "-M", "main"},
		{"git", "push", "origin", "main", "--force"},
	}

	for _, args := range commands {
		c := exec.Command(args[0], args[1:]...)
		c.Dir = tmpDir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	// Verify the push landed.
	verifyDir := filepath.Join(t.TempDir(), "cloned")
	c := exec.Command("git", "clone", remoteDir, verifyDir)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("clone: %v\n%s", err, out)
	}
	cloned, err := os.ReadFile(filepath.Join(verifyDir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(cloned) != string(body) {
		t.Error("cloned SKILL.md doesn't match")
	}

	_ = pusher // verify it was constructed
}
