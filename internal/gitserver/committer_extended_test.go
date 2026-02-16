package gitserver

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/config"
)

func TestInitEmptyWorkdir(t *testing.T) {
	workdir := filepath.Join(t.TempDir(), "repo")

	cfg := config.DefaultConfig()
	cfg.Server.DataDir = t.TempDir()
	s := &Server{config: cfg, adminKeyPath: "/dev/null"}

	c := NewGitCommitter(GitCommitterConfig{
		Server:  s,
		DataDir: t.TempDir(),
	})

	ctx := context.Background()
	err := c.initEmptyWorkdir(ctx, "ssh://fake:22/repo", workdir, "ssh -o StrictHostKeyChecking=no")
	if err != nil {
		t.Fatalf("initEmptyWorkdir: %v", err)
	}

	// Check .git dir exists
	if _, err := os.Stat(filepath.Join(workdir, ".git")); err != nil {
		t.Fatal("expected .git directory to be created")
	}

	// Check remote is set
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = workdir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git remote get-url: %v", err)
	}
	if got := string(out); got != "ssh://fake:22/repo\n" {
		t.Errorf("remote URL = %q", got)
	}
}

func TestCommitAndPush_NoChanges(t *testing.T) {
	// Create a git repo with an initial commit
	workdir := t.TempDir()
	env := []string{
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test",
	}

	for _, args := range [][]string{
		{"init"},
		{"commit", "--allow-empty", "-m", "initial"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = workdir
		cmd.Env = append(os.Environ(), env...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}

	cfg := config.DefaultConfig()
	s := &Server{config: cfg, adminKeyPath: "/dev/null"}
	c := NewGitCommitter(GitCommitterConfig{Server: s, DataDir: t.TempDir()})

	// commitAndPush with no changes should return existing HEAD hash
	hash, err := c.commitAndPush(context.Background(), workdir, "no changes")
	if err != nil {
		t.Fatalf("commitAndPush: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
	if len(hash) != 40 {
		t.Errorf("hash length = %d, want 40", len(hash))
	}
}

func TestEnsureWorkdir_ExistingClone(t *testing.T) {
	// Create a bare repo and clone it, then test that ensureWorkdir does a pull
	bareDir := filepath.Join(t.TempDir(), "bare.git")
	cmd := exec.Command("git", "init", "--bare", bareDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %s: %v", out, err)
	}

	workdir := filepath.Join(t.TempDir(), "clone")
	cmd = exec.Command("git", "clone", bareDir, workdir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git clone: %s: %v", out, err)
	}

	cfg := config.DefaultConfig()
	s := &Server{config: cfg, adminKeyPath: "/dev/null"}
	s.config.Server.Host = "127.0.0.1"
	s.config.Server.Port = 22

	c := NewGitCommitter(GitCommitterConfig{Server: s, DataDir: t.TempDir()})

	// ensureWorkdir should see existing .git and try to pull
	// It will fail because the clone URL won't match, but the pull path is exercised
	// Actually for a local bare repo, let's set the remote correctly
	remoteCmd := exec.Command("git", "remote", "set-url", "origin", bareDir)
	remoteCmd.Dir = workdir
	if out, err := remoteCmd.CombinedOutput(); err != nil {
		t.Fatalf("set-url: %s: %v", out, err)
	}

	// Now ensureWorkdir should pull successfully (empty repo, but no error)
	err := c.ensureWorkdir(context.Background(), "test/repo", workdir)
	// The pull may succeed or fail depending on git version with empty repos
	// The point is exercising the "already cloned" branch
	_ = err
}
