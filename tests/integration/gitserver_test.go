package integration

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// ---------------------------------------------------------------------------
// GitTestServer — test helper that starts a real Soft Serve subprocess
// ---------------------------------------------------------------------------

// GitTestServer wraps a Soft Serve process for integration testing.
type GitTestServer struct {
	Port     int
	HTTPPort int
	DataDir  string
	AdminKey string // path to private key
	cmd      *exec.Cmd
	t        *testing.T
}

// requireSoftBinary skips the test if the `soft` binary is not on PATH.
func requireSoftBinary(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("soft")
	if err != nil {
		t.Skip("soft binary not found, skipping git integration tests")
	}
	return path
}

// randomFreePort asks the OS for a free TCP port.
func randomFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// generateTestSSHKey creates an ed25519 keypair in dir, returns private key path.
func generateTestSSHKey(t *testing.T, dir string) string {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	// Write private key (OpenSSH format)
	privBytes, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	privPath := filepath.Join(dir, "test_admin_ed25519")
	if err := os.WriteFile(privPath, pem.EncodeToMemory(privBytes), 0600); err != nil {
		t.Fatalf("write private key: %v", err)
	}

	// Write public key
	pubKey, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("new public key: %v", err)
	}
	pubPath := privPath + ".pub"
	if err := os.WriteFile(pubPath, ssh.MarshalAuthorizedKey(pubKey), 0644); err != nil {
		t.Fatalf("write public key: %v", err)
	}

	return privPath
}

// StartGitTestServer starts Soft Serve on a random port with a temp data dir.
// It registers t.Cleanup to stop the server and remove temp files.
// Skips the test if `soft` is not installed.
func StartGitTestServer(t *testing.T) *GitTestServer {
	t.Helper()
	softBin := requireSoftBinary(t)

	dataDir := t.TempDir()
	sshPort := randomFreePort(t)
	httpPort := randomFreePort(t)
	adminKeyPath := generateTestSSHKey(t, dataDir)

	// Read public key for admin
	pubBytes, err := os.ReadFile(adminKeyPath + ".pub")
	if err != nil {
		t.Fatalf("read public key: %v", err)
	}

	cmd := exec.Command(softBin, "serve")
	cmd.Dir = dataDir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("SOFT_SERVE_DATA_PATH=%s", dataDir),
		fmt.Sprintf("SOFT_SERVE_INITIAL_ADMIN_KEYS=%s", strings.TrimSpace(string(pubBytes))),
		fmt.Sprintf("SOFT_SERVE_SSH_LISTEN_ADDR=127.0.0.1:%d", sshPort),
		fmt.Sprintf("SOFT_SERVE_HTTP_LISTEN_ADDR=127.0.0.1:%d", httpPort),
	)

	// Capture stderr for debugging
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start soft serve: %v", err)
	}

	gs := &GitTestServer{
		Port:     sshPort,
		HTTPPort: httpPort,
		DataDir:  dataDir,
		AdminKey: adminKeyPath,
		cmd:      cmd,
		t:        t,
	}

	t.Cleanup(func() {
		if gs.cmd.Process != nil {
			_ = syscall.Kill(-gs.cmd.Process.Pid, syscall.SIGKILL)
			gs.cmd.Wait()
		}
	})

	// Wait for server to be ready
	gs.waitForReady()

	return gs
}

// waitForReady polls SSH until Soft Serve responds or times out.
func (gs *GitTestServer) waitForReady() {
	gs.t.Helper()
	deadline := time.Now().Add(15 * time.Second)

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp",
			fmt.Sprintf("127.0.0.1:%d", gs.Port), 500*time.Millisecond)
		if err == nil {
			conn.Close()
			// Give it a moment after port is open
			time.Sleep(500 * time.Millisecond)
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	gs.t.Fatalf("soft serve did not become ready on port %d within 15s", gs.Port)
}

// SSHCommand runs an SSH command against the test server and returns output.
func (gs *GitTestServer) SSHCommand(args ...string) (string, error) {
	cmdArgs := []string{
		"-p", fmt.Sprintf("%d", gs.Port),
		"-i", gs.AdminKey,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "GlobalKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"127.0.0.1",
	}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.Command("ssh", cmdArgs...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// CloneURL returns the SSH clone URL for a repo on this test server.
func (gs *GitTestServer) CloneURL(repo string) string {
	return fmt.Sprintf("ssh://127.0.0.1:%d/%s", gs.Port, repo)
}

// GitEnv returns environment variables for git commands to use this server's key.
func (gs *GitTestServer) GitEnv() []string {
	sshCmd := fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o GlobalKnownHostsFile=/dev/null -o LogLevel=ERROR -p %d",
		gs.AdminKey, gs.Port)
	return append(os.Environ(), fmt.Sprintf("GIT_SSH_COMMAND=%s", sshCmd))
}

// ---------------------------------------------------------------------------
// Integration Tests
// ---------------------------------------------------------------------------

func TestGitServer_StartStop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration test in -short mode")
	}

	gs := StartGitTestServer(t)

	// Verify we can talk to it
	_, err := gs.SSHCommand("help")
	if err != nil {
		// Some versions don't have "help", try "repo list" instead
		out, err := gs.SSHCommand("repo", "list")
		if err != nil {
			t.Fatalf("cannot communicate with soft serve: %v\nOutput: %s", err, out)
		}
	}
	t.Logf("Soft Serve responding on port %d", gs.Port)
}

func TestGitServer_CreateAndListRepo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration test in -short mode")
	}

	gs := StartGitTestServer(t)

	// Create a repo
	out, err := gs.SSHCommand("repo", "create", "test-skill")
	if err != nil {
		t.Fatalf("failed to create repo: %v\nOutput: %s", err, out)
	}

	// List repos — should contain our repo
	out, err = gs.SSHCommand("repo", "list")
	if err != nil {
		t.Fatalf("failed to list repos: %v\nOutput: %s", err, out)
	}

	if !strings.Contains(out, "test-skill") {
		t.Errorf("repo list should contain 'test-skill', got: %s", out)
	}
}

func TestGitServer_ClonePushVerify(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration test in -short mode")
	}

	gs := StartGitTestServer(t)

	// Create repo on server
	if out, err := gs.SSHCommand("repo", "create", "test-roundtrip"); err != nil {
		t.Fatalf("create repo: %v\n%s", err, out)
	}

	// Clone it
	cloneDir := filepath.Join(t.TempDir(), "clone")
	cloneCmd := exec.Command("git", "clone", gs.CloneURL("test-roundtrip"), cloneDir)
	cloneCmd.Env = gs.GitEnv()
	if out, err := cloneCmd.CombinedOutput(); err != nil {
		t.Fatalf("git clone failed: %v\n%s", err, string(out))
	}

	// Write a SKILL.md
	skillContent := []byte("# Fix N+1 Queries\n\nUse eager loading.\n")
	skillDir := filepath.Join(cloneDir, "v1")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), skillContent, 0644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	// Configure git user for commit
	for _, cfg := range [][]string{
		{"config", "user.email", "test@kinoko.dev"},
		{"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", cfg...)
		cmd.Dir = cloneDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git config: %v\n%s", err, string(out))
		}
	}

	// Add, commit, push
	for _, step := range []struct {
		name string
		args []string
		env  []string
	}{
		{"add", []string{"add", "."}, nil},
		{"commit", []string{"commit", "-m", "v1: fix-n-plus-one"}, nil},
		{"push", []string{"push", "origin", "HEAD"}, gs.GitEnv()},
	} {
		cmd := exec.Command("git", step.args...)
		cmd.Dir = cloneDir
		if step.env != nil {
			cmd.Env = step.env
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s failed: %v\n%s", step.name, err, string(out))
		}
	}

	// Verify: clone again to a fresh directory and check file exists
	verifyDir := filepath.Join(t.TempDir(), "verify")
	verifyCmd := exec.Command("git", "clone", gs.CloneURL("test-roundtrip"), verifyDir)
	verifyCmd.Env = gs.GitEnv()
	if out, err := verifyCmd.CombinedOutput(); err != nil {
		t.Fatalf("verify clone failed: %v\n%s", err, string(out))
	}

	got, err := os.ReadFile(filepath.Join(verifyDir, "v1", "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md from clone: %v", err)
	}

	if string(got) != string(skillContent) {
		t.Errorf("SKILL.md content mismatch\ngot:  %q\nwant: %q", string(got), string(skillContent))
	}

	t.Logf("Full roundtrip verified: create → clone → push → verify ✓")
}

func TestGitServer_DeleteRepo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping git integration test in -short mode")
	}

	gs := StartGitTestServer(t)

	// Create then delete
	if out, err := gs.SSHCommand("repo", "create", "ephemeral"); err != nil {
		t.Fatalf("create: %v\n%s", err, out)
	}
	if out, err := gs.SSHCommand("repo", "delete", "ephemeral", "-y"); err != nil {
		// Some versions need --yes or -f; try without flag
		if out2, err2 := gs.SSHCommand("repo", "delete", "ephemeral"); err2 != nil {
			t.Fatalf("delete: %v\n%s\nalso tried without -y: %v\n%s", err, out, err2, out2)
		}
	}

	// Verify gone
	out, err := gs.SSHCommand("repo", "list")
	if err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	if strings.Contains(out, "ephemeral") {
		t.Errorf("repo 'ephemeral' still in list after delete: %s", out)
	}
}
