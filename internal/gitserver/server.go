// Package gitserver manages a Soft Serve git server subprocess for hosting
// skill repositories. It handles SSH key generation, process lifecycle,
// repository CRUD via SSH commands, and session hook registration.
package gitserver

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/kinoko-dev/kinoko/internal/config"
	"github.com/kinoko-dev/kinoko/internal/model"
)

// Server wraps the Soft Serve git server with Kinoko-specific functionality
type Server struct {
	config         *config.Config
	dataDir        string
	cmd            *exec.Cmd
	cmdDone        chan error // result of cmd.Wait(), populated by Start
	logger         *slog.Logger
	softBinary     string
	adminKeyPath   string
	onSessionStart SessionStartHook
	onSessionEnd   SessionEndHook
}

// SessionStartHook is called when a new agent session begins to run injection.
type SessionStartHook func(ctx context.Context, req model.InjectionRequest) (*model.InjectionResponse, error)

// SessionEndHook is called when an agent session completes to run extraction.
type SessionEndHook func(ctx context.Context, session model.SessionRecord, logContent []byte) (*model.ExtractionResult, error)

// SetSessionHooks registers session lifecycle callbacks.
// The hooks are called during git push events to trigger injection (pre-session)
// and extraction (post-session) pipelines.
func (s *Server) SetSessionHooks(onStart SessionStartHook, onEnd SessionEndHook) {
	s.onSessionStart = onStart
	s.onSessionEnd = onEnd
	s.logger.Info("session hooks registered")
}

// NewServer creates a new git server instance
func NewServer(cfg *config.Config) (*Server, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Check if soft binary is available
	softBinary, err := CheckSoftBinary()
	if err != nil {
		return nil, err
	}

	// Ensure data directory exists
	dataDir := cfg.Server.DataDir
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory %s: %w", dataDir, err)
	}

	return &Server{
		config:     cfg,
		dataDir:    dataDir,
		logger:     slog.Default(),
		softBinary: softBinary,
	}, nil
}

// Start starts the git server
func (s *Server) Start() error {
	s.logger.Info("Starting Kinoko git server with Soft Serve",
		"host", s.config.Server.Host,
		"port", s.config.Server.Port,
		"dataDir", s.dataDir)

	// Generate admin SSH keys if they don't exist
	adminKeyPath, err := s.ensureAdminKeys()
	if err != nil {
		return fmt.Errorf("failed to setup admin SSH keys: %w", err)
	}
	s.adminKeyPath = adminKeyPath

	// Get admin public key for SOFT_SERVE_INITIAL_ADMIN_KEYS
	adminPublicKey, err := s.getAdminPublicKey()
	if err != nil {
		return fmt.Errorf("failed to read admin public key: %w", err)
	}

	// Setup environment variables for Soft Serve
	env := os.Environ()
	env = append(env,
		fmt.Sprintf("SOFT_SERVE_DATA_PATH=%s", s.dataDir),
		fmt.Sprintf("SOFT_SERVE_INITIAL_ADMIN_KEYS=%s", adminPublicKey),
		fmt.Sprintf("SOFT_SERVE_SSH_LISTEN_ADDR=:%d", s.config.Server.Port),
		fmt.Sprintf("SOFT_SERVE_HTTP_LISTEN_ADDR=:%d", s.config.Server.Port+1),
	)

	// Create command to start Soft Serve
	s.cmd = exec.Command(s.softBinary, "serve") //nolint:gosec // controlled input from config
	s.cmd.Env = env
	s.cmd.Dir = s.dataDir
	s.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Start the server
	if err := s.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start soft serve: %w", err)
	}

	s.logger.Info("Soft Serve process started", "pid", s.cmd.Process.Pid)

	// Monitor subprocess in background (used by waitForReady and Stop).
	s.cmdDone = make(chan error, 1)
	go func() {
		s.cmdDone <- s.cmd.Wait()
	}()

	// Wait for the server to be ready
	if err := s.waitForReady(); err != nil {
		// Kill the process group if it's still running
		if s.cmd.Process != nil {
			_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
		}
		return fmt.Errorf("soft serve failed to start properly: %w", err)
	}

	s.logger.Info("Git server started successfully",
		"ssh_url", fmt.Sprintf("ssh://%s:%d", s.config.Server.Host, s.config.Server.Port),
		"http_url", fmt.Sprintf("http://%s:%d", s.config.Server.Host, s.config.Server.Port+1))

	return nil
}

// waitForReady waits for Soft Serve to be ready by attempting SSH connections.
// It also monitors the subprocess for early exit (e.g. port conflict).
func (s *Server) waitForReady() error {
	timeout := 10 * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Check if process already exited (port conflict, etc.).
		select {
		case err := <-s.cmdDone:
			if err != nil {
				return fmt.Errorf("soft serve exited early: %w", err)
			}
			return fmt.Errorf("soft serve exited early with status 0")
		default:
		}

		// Try to connect via SSH to test if server is ready
		testCmd := exec.Command("ssh", //nolint:gosec // controlled input from config
			"-p", strconv.Itoa(s.config.Server.Port),
			"-i", s.adminKeyPath,
			"-o", "StrictHostKeyChecking=no",
			"-o", "ConnectTimeout=2",
			s.config.Server.Host,
			"repo", "list")

		if err := testCmd.Run(); err == nil {
			return nil // Server is ready
		}

		time.Sleep(500 * time.Millisecond)
	}

	// Timed out — kill the process group.
	if s.cmd.Process != nil {
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
	}
	return fmt.Errorf("timeout after %s waiting for soft serve to be ready", timeout)
}

// Stop gracefully shuts down the git server
func (s *Server) Stop() error {
	s.logger.Info("Stopping Kinoko git server")

	if s.cmd == nil || s.cmd.Process == nil {
		s.logger.Info("Git server was not running")
		return nil
	}

	// Send SIGTERM to the entire process group for graceful shutdown
	if err := syscall.Kill(-s.cmd.Process.Pid, syscall.SIGTERM); err != nil {
		s.logger.Warn("Failed to send SIGTERM", "error", err)
	}

	// Wait for graceful shutdown with timeout
	select {
	case <-s.cmdDone:
		s.logger.Info("Git server stopped gracefully")
	case <-time.After(10 * time.Second):
		s.logger.Warn("Graceful shutdown timed out, sending SIGKILL")
		if err := syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL); err != nil {
			s.logger.Error("Failed to kill process", "error", err)
			return err
		}
		<-s.cmdDone // Wait for the process to actually exit
		s.logger.Info("Git server stopped forcefully")
	}

	return nil
}

// runSSHCommand executes an SSH command against the Soft Serve server
func (s *Server) runSSHCommand(args ...string) (string, error) {
	if s.adminKeyPath == "" {
		return "", fmt.Errorf("admin key path not set")
	}

	cmdArgs := []string{
		"-p", strconv.Itoa(s.config.Server.Port),
		"-i", s.adminKeyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "GlobalKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		s.config.Server.Host,
	}
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.Command("ssh", cmdArgs...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// CreateRepo creates a new repository programmatically
func (s *Server) CreateRepo(name, description string) error {
	if name == "" {
		return fmt.Errorf("repository name cannot be empty")
	}

	s.logger.Info("Creating repository", "name", name, "description", description)

	// Create the repository via SSH
	output, err := s.runSSHCommand("repo", "create", name)
	if err != nil {
		return fmt.Errorf("failed to create repository %s: %w\nOutput: %s", name, err, output)
	}

	// Set description if provided
	if description != "" {
		descOutput, descErr := s.runSSHCommand("repo", "description", name, description)
		if descErr != nil {
			s.logger.Warn("Failed to set repository description", "name", name, "error", descErr, "output", descOutput)
			// Don't fail the entire operation if description setting fails
		}
	}

	s.logger.Info("Repository created successfully", "name", name)
	return nil
}

// ListRepos returns a list of all repositories
func (s *Server) ListRepos() ([]string, error) {
	s.logger.Debug("Listing repositories")

	output, err := s.runSSHCommand("repo", "list")
	if err != nil {
		return nil, fmt.Errorf("failed to list repositories: %w\nOutput: %s", err, output)
	}

	// Parse the output to extract repository names
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var repos []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// The output format may vary, but typically repo names are the first word
		// We'll split by whitespace and take the first part
		parts := strings.Fields(line)
		if len(parts) > 0 {
			repos = append(repos, parts[0])
		}
	}

	s.logger.Debug("Found repositories", "count", len(repos), "repos", repos)
	return repos, nil
}

// DeleteRepo removes a repository
func (s *Server) DeleteRepo(name string) error {
	if name == "" {
		return fmt.Errorf("repository name cannot be empty")
	}

	s.logger.Info("Deleting repository", "name", name)

	output, err := s.runSSHCommand("repo", "delete", name)
	if err != nil {
		return fmt.Errorf("failed to delete repository %s: %w\nOutput: %s", name, err, output)
	}

	s.logger.Info("Repository deleted successfully", "name", name)
	return nil
}

// GetCloneURL returns the SSH clone URL for a repository
func (s *Server) GetCloneURL(name string) string {
	return fmt.Sprintf("ssh://%s:%d/%s", s.config.Server.Host, s.config.Server.Port, name)
}

// GetConnectionInfo returns the SSH connection information for clients
func (s *Server) GetConnectionInfo() ConnectionInfo {
	return ConnectionInfo{
		SSHHost:  s.config.Server.Host,
		SSHPort:  s.config.Server.Port,
		SSHUrl:   fmt.Sprintf("ssh://%s:%d", s.config.Server.Host, s.config.Server.Port),
		HTTPUrl:  fmt.Sprintf("http://%s:%d", s.config.Server.Host, s.config.Server.Port+1),
		CloneSSH: func(repo string) string { return s.GetCloneURL(repo) },
		CloneHTTP: func(repo string) string {
			return fmt.Sprintf("http://%s:%d/%s", s.config.Server.Host, s.config.Server.Port+1, repo)
		},
	}
}

// ConnectionInfo contains information needed to connect to the git server
type ConnectionInfo struct {
	SSHHost   string              `json:"ssh_host"`
	SSHPort   int                 `json:"ssh_port"`
	SSHUrl    string              `json:"ssh_url"`
	HTTPUrl   string              `json:"http_url"`
	CloneSSH  func(string) string `json:"-"`
	CloneHTTP func(string) string `json:"-"`
}
