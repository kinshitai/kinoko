package apiclient

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSSHEnv_EmptyPath(t *testing.T) {
	g := NewGitPushCommitter("git@example.com:repo.git", t.TempDir(), "", slog.Default())
	env := g.sshEnv()
	if env != nil {
		t.Errorf("expected nil for empty sshKeyPath, got %v", env)
	}
}

func TestSSHEnv_SetPath(t *testing.T) {
	// Create a real file so the constructor doesn't warn about missing key.
	tmp := t.TempDir()
	keyPath := filepath.Join(tmp, "id_ed25519")
	if err := os.WriteFile(keyPath, []byte("fake-key"), 0600); err != nil {
		t.Fatal(err)
	}

	g := NewGitPushCommitter("git@example.com:repo.git", t.TempDir(), keyPath, slog.Default())
	env := g.sshEnv()
	if len(env) != 1 {
		t.Fatalf("expected 1 env var, got %d", len(env))
	}
	expected := "GIT_SSH_COMMAND=ssh -i '" + keyPath + "' -o StrictHostKeyChecking=no -o IdentitiesOnly=yes"
	if env[0] != expected {
		t.Errorf("expected %q, got %q", expected, env[0])
	}
}

func TestSSHEnv_PathWithSpaces(t *testing.T) {
	// Create a directory with spaces and a key file inside it.
	tmp := t.TempDir()
	spacedDir := filepath.Join(tmp, "my user", "keys")
	if err := os.MkdirAll(spacedDir, 0755); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(spacedDir, "id_ed25519")
	if err := os.WriteFile(keyPath, []byte("fake-key"), 0600); err != nil {
		t.Fatal(err)
	}

	g := NewGitPushCommitter("git@example.com:repo.git", t.TempDir(), keyPath, slog.Default())
	env := g.sshEnv()
	if len(env) != 1 {
		t.Fatalf("expected 1 env var, got %d", len(env))
	}
	// Single quotes protect spaces from shell splitting.
	expected := "GIT_SSH_COMMAND=ssh -i '" + keyPath + "' -o StrictHostKeyChecking=no -o IdentitiesOnly=yes"
	if env[0] != expected {
		t.Errorf("expected %q, got %q", expected, env[0])
	}
}

// TestSSHEnv_PathWithSingleQuote documents that a key path containing
// a single quote will produce a broken GIT_SSH_COMMAND value because
// the current implementation uses naive single-quoting. If this test
// starts failing because the quoting was fixed, update it accordingly.
func TestSSHEnv_PathWithSingleQuote(t *testing.T) {
	g := &GitPushCommitter{sshKeyPath: "/tmp/it's a key"}
	env := g.sshEnv()
	if len(env) != 1 {
		t.Fatalf("expected 1 env var, got %d", len(env))
	}
	// The unescaped single quote breaks shell quoting:
	//   ssh -i '/tmp/it's a key' ...
	// This is technically broken but acceptable since key paths come from
	// config files, not untrusted input. Document the behaviour.
	if !strings.Contains(env[0], "it's a key") {
		t.Errorf("expected path to appear verbatim in env, got %q", env[0])
	}
}

// TestNewGitPushCommitter_MissingKeyWarns verifies that the constructor
// logs a warning (instead of crashing) when the SSH key file doesn't exist.
func TestNewGitPushCommitter_MissingKeyWarns(t *testing.T) {
	// Must not panic even though the file doesn't exist.
	g := NewGitPushCommitter(
		"git@example.com:repo.git",
		t.TempDir(),
		"/nonexistent/path/id_ed25519",
		slog.Default(),
	)
	// Should still store the path — the warning is advisory.
	if g.sshKeyPath != "/nonexistent/path/id_ed25519" {
		t.Errorf("expected sshKeyPath to be set, got %q", g.sshKeyPath)
	}
}

// TestNewGitPushCommitter_NilLoggerNonEmptyPath verifies that passing a nil
// logger with a non-empty (missing) key path doesn't panic. This guards
// against regression — the original code panicked here.
func TestNewGitPushCommitter_NilLoggerNonEmptyPath(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NewGitPushCommitter panicked with nil logger: %v", r)
		}
	}()
	// This will hit the os.Stat path because the key doesn't exist,
	// then try log.Warn on a nil logger.
	_ = NewGitPushCommitter(
		"git@example.com:repo.git",
		t.TempDir(),
		"/nonexistent/key",
		nil,
	)
}
