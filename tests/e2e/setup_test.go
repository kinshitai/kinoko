//go:build integration

package e2e

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/kinoko-dev/kinoko/internal/config"
	"github.com/kinoko-dev/kinoko/internal/gitserver"
)

// TestEnvironment manages a complete test environment for e2e testing
type TestEnvironment struct {
	// Directories
	TempDir    string
	ConfigDir  string
	DataDir    string
	SkillsDir  string
	BinaryPath string

	// Configuration
	Config     *config.Config
	ConfigPath string

	// Server management
	Server    *gitserver.Server
	ServerCmd *exec.Cmd
	ServerPID int
	SSHPort   int
	HTTPPort  int

	// SSH keys for testing
	AdminKeyPath    string
	AdminPubKeyPath string

	// Test utilities
	t *testing.T
}

// SetupTestEnvironment creates a complete isolated test environment
func SetupTestEnvironment(t *testing.T) *TestEnvironment {
	t.Helper()

	// Create temporary directory for this test
	tempDir, err := os.MkdirTemp("", "kinoko-e2e-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Build kinoko binary if needed
	binaryPath := buildKinokoBinary(t, tempDir)

	// Find available ports
	sshPort := findAvailablePort(t, 23240) // Start from non-standard port
	httpPort := sshPort + 1

	configDir := filepath.Join(tempDir, "config")
	dataDir := filepath.Join(tempDir, "data")
	skillsDir := filepath.Join(tempDir, "skills")

	// Create directories
	for _, dir := range []string{configDir, dataDir, skillsDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			cleanupTempDir(tempDir)
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}

	// Create test config
	cfg := createTestConfig(t, dataDir, skillsDir, sshPort)
	configPath := filepath.Join(configDir, "config.yaml")
	if err := cfg.Save(configPath); err != nil {
		cleanupTempDir(tempDir)
		t.Fatalf("Failed to save test config: %v", err)
	}

	env := &TestEnvironment{
		TempDir:    tempDir,
		ConfigDir:  configDir,
		DataDir:    dataDir,
		SkillsDir:  skillsDir,
		BinaryPath: binaryPath,
		Config:     cfg,
		ConfigPath: configPath,
		SSHPort:    sshPort,
		HTTPPort:   httpPort,
		t:          t,
	}

	// Setup cleanup
	t.Cleanup(func() {
		env.Cleanup()
	})

	return env
}

// StartServer starts the kinoko server and waits for it to be ready
func (env *TestEnvironment) StartServer() {
	env.t.Helper()

	if env.ServerCmd != nil {
		env.t.Fatal("Server already started")
	}

	env.t.Logf("Starting kinoko server on port %d", env.SSHPort)

	// Start server process
	env.ServerCmd = exec.Command(env.BinaryPath, "serve", "--config", env.ConfigPath)
	env.ServerCmd.Dir = env.TempDir

	// Capture server output for debugging
	env.ServerCmd.Stdout = &testLogWriter{t: env.t, prefix: "[SERVER-OUT]"}
	env.ServerCmd.Stderr = &testLogWriter{t: env.t, prefix: "[SERVER-ERR]"}

	if err := env.ServerCmd.Start(); err != nil {
		env.t.Fatalf("Failed to start server: %v", err)
	}

	env.ServerPID = env.ServerCmd.Process.Pid
	env.t.Logf("Server started with PID %d", env.ServerPID)

	// Wait for server to be ready
	env.waitForServerReady()

	// Setup SSH key paths
	env.AdminKeyPath = filepath.Join(env.DataDir, "kinoko_admin_ed25519")
	env.AdminPubKeyPath = env.AdminKeyPath + ".pub"
}

// StopServer gracefully stops the server
func (env *TestEnvironment) StopServer() {
	env.t.Helper()

	if env.ServerCmd == nil || env.ServerCmd.Process == nil {
		return
	}

	env.t.Logf("Stopping server (PID: %d)", env.ServerPID)

	// Send SIGTERM
	if err := env.ServerCmd.Process.Signal(syscall.SIGTERM); err != nil {
		env.t.Logf("Failed to send SIGTERM: %v", err)
	}

	// Wait for graceful shutdown with timeout
	done := make(chan error, 1)
	go func() {
		done <- env.ServerCmd.Wait()
	}()

	select {
	case <-done:
		env.t.Log("Server stopped gracefully")
	case <-time.After(15 * time.Second):
		env.t.Log("Server shutdown timeout, sending SIGKILL")
		env.ServerCmd.Process.Kill()
		<-done
	}

	env.ServerCmd = nil
	env.ServerPID = 0
}

// RunSSHCommand executes an SSH command against the test server
func (env *TestEnvironment) RunSSHCommand(args ...string) (string, error) {
	env.t.Helper()

	if env.AdminKeyPath == "" {
		return "", fmt.Errorf("server not started or SSH keys not generated")
	}

	// Wait for SSH key to be generated
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(env.AdminKeyPath); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	cmdArgs := []string{
		"-p", strconv.Itoa(env.SSHPort),
		"-i", env.AdminKeyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "GlobalKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"-o", "ConnectTimeout=10",
		"127.0.0.1",
	}
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.Command("ssh", cmdArgs...)
	output, err := cmd.CombinedOutput()

	env.t.Logf("SSH command: ssh %v", cmdArgs)
	env.t.Logf("SSH output: %s", string(output))
	if err != nil {
		env.t.Logf("SSH error: %v", err)
	}

	return string(output), err
}

// GitCloneSSH clones a repository using SSH
func (env *TestEnvironment) GitCloneSSH(repoName, cloneDir string) error {
	env.t.Helper()

	cloneURL := fmt.Sprintf("ssh://127.0.0.1:%d/%s", env.SSHPort, repoName)

	// Set up git SSH command
	gitSSHCmd := fmt.Sprintf("ssh -p %d -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o GlobalKnownHostsFile=/dev/null -o LogLevel=ERROR",
		env.SSHPort, env.AdminKeyPath)

	cmd := exec.Command("git", "clone", cloneURL, cloneDir)
	cmd.Env = append(os.Environ(), fmt.Sprintf("GIT_SSH_COMMAND=%s", gitSSHCmd))
	cmd.Dir = env.TempDir

	output, err := cmd.CombinedOutput()
	env.t.Logf("Git clone output: %s", string(output))

	return err
}

// GitCloneHTTP clones a repository using HTTP
func (env *TestEnvironment) GitCloneHTTP(repoName, cloneDir string) error {
	env.t.Helper()

	cloneURL := fmt.Sprintf("http://127.0.0.1:%d/%s", env.HTTPPort, repoName)

	cmd := exec.Command("git", "clone", cloneURL, cloneDir)
	cmd.Dir = env.TempDir

	output, err := cmd.CombinedOutput()
	env.t.Logf("Git clone HTTP output: %s", string(output))

	return err
}

// CreateSkillFile creates a SKILL.md file with the given content
func (env *TestEnvironment) CreateSkillFile(dir, name, author string, confidence float64, body string) error {
	env.t.Helper()

	skillContent := fmt.Sprintf(`---
name: %s
version: 1
author: %s
confidence: %.2f
created: 2026-02-14
---

%s`, name, author, confidence, body)

	skillPath := filepath.Join(dir, "SKILL.md")
	return os.WriteFile(skillPath, []byte(skillContent), 0644)
}

// Cleanup removes the test environment
func (env *TestEnvironment) Cleanup() {
	env.t.Helper()

	env.StopServer()

	if env.TempDir != "" {
		cleanupTempDir(env.TempDir)
	}
}

// Helper functions

func cleanupTempDir(dir string) {
	os.RemoveAll(dir)
}

func buildKinokoBinary(t *testing.T, tempDir string) string {
	t.Helper()

	// Check if we need to build the binary
	binaryName := "kinoko-test"
	binaryPath := filepath.Join(tempDir, binaryName)

	// Find project root by looking for go.mod
	projectRoot, err := findProjectRoot()
	if err != nil {
		t.Fatalf("Failed to find project root: %v", err)
	}

	t.Logf("Building kinoko binary from %s", projectRoot)

	// Build the binary
	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/kinoko")
	cmd.Dir = projectRoot
	cmd.Env = os.Environ()

	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build kinoko binary: %v\nOutput: %s", err, output)
	}

	return binaryPath
}

func findProjectRoot() (string, error) {
	// Start from current directory and walk up looking for go.mod
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find go.mod in any parent directory")
		}
		dir = parent
	}
}

func createTestConfig(t *testing.T, dataDir, skillsDir string, sshPort int) *config.Config {
	t.Helper()

	cfg := config.DefaultConfig()
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = sshPort
	cfg.Server.DataDir = dataDir
	cfg.Storage.DSN = filepath.Join(dataDir, "test.db")
	cfg.Libraries[0].Path = skillsDir
	cfg.Defaults.Author = "test-user"

	return cfg
}

func findAvailablePort(t *testing.T, startPort int) int {
	t.Helper()

	for port := startPort; port <= startPort+100; port++ {
		if isPortAvailable(port) {
			return port
		}
	}
	t.Fatalf("No available port found starting from %d", startPort)
	return 0
}

func isPortAvailable(port int) bool {
	// Try to bind to the port using net.Listen
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return false // Port not available
	}
	ln.Close()
	return true // Port is available
}

func (env *TestEnvironment) waitForServerReady() {
	env.t.Helper()

	env.t.Log("Waiting for server to be ready...")

	timeout := time.After(60 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			env.t.Fatal("Timeout waiting for server to be ready")
		case <-ticker.C:
			// Check if process is still running
			if env.ServerCmd.ProcessState != nil && env.ServerCmd.ProcessState.Exited() {
				env.t.Fatal("Server process exited unexpectedly")
			}

			// Try SSH connection
			if output, err := env.RunSSHCommand("repo", "list"); err == nil {
				env.t.Logf("Server ready! Initial repo list: %s", output)
				return
			}
		}
	}
}

// testLogWriter captures server output for test logs
type testLogWriter struct {
	t      *testing.T
	prefix string
}

func (w *testLogWriter) Write(p []byte) (n int, err error) {
	w.t.Logf("%s %s", w.prefix, string(p))
	return len(p), nil
}

// Helper to skip tests if soft binary is not available
func RequireSoftBinary(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("soft"); err != nil {
		t.Skip("Skipping test because 'soft' binary is not available")
	}
}

// Helper to skip tests if git binary is not available
func RequireGitBinary(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Skipping test because 'git' binary is not available")
	}
}

// Helper to skip tests if SSH binary is not available
func RequireSSHBinary(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("Skipping test because 'ssh' binary is not available")
	}
}
