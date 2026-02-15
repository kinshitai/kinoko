//go:build integration

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mycelium-dev/mycelium/internal/config"
	"gopkg.in/yaml.v3"
)

func TestMyceliumInit(t *testing.T) {
	RequireGitBinary(t)

	// Create isolated test environment
	tempDir, err := os.MkdirTemp("", "mycelium-init-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Build mycelium binary
	binaryPath := buildMyceliumBinary(t, tempDir)

	// Create fake home directory for testing
	homeDir := filepath.Join(tempDir, "home")
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatalf("Failed to create fake home dir: %v", err)
	}

	// Run mycelium init with custom HOME
	cmd := exec.Command(binaryPath, "init")
	cmd.Env = append(os.Environ(), "HOME="+homeDir)
	cmd.Dir = tempDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mycelium init failed: %v\nOutput: %s", err, output)
	}

	t.Logf("mycelium init output: %s", output)

	// Verify directory structure was created
	expectedDirs := []string{
		".mycelium",
		".mycelium/skills",
	}

	for _, dir := range expectedDirs {
		dirPath := filepath.Join(homeDir, dir)
		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			t.Errorf("Expected directory not created: %s", dirPath)
		}
	}

	// Verify config file was created and is valid
	configPath := filepath.Join(homeDir, ".mycelium", "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Config file was not created")
	}

	// Load and validate config
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load generated config: %v", err)
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Generated config is invalid: %v", err)
	}

	// Verify git repository was initialized
	skillsDir := filepath.Join(homeDir, ".mycelium", "skills")
	gitDir := filepath.Join(skillsDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		t.Error("Git repository was not initialized in skills directory")
	}

	// Verify .gitignore was created
	gitignorePath := filepath.Join(skillsDir, ".gitignore")
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		t.Error(".gitignore file was not created")
	}

	// Test that init is idempotent (can run multiple times safely)
	t.Run("idempotent", func(t *testing.T) {
		// Run init again
		cmd := exec.Command(binaryPath, "init")
		cmd.Env = append(os.Environ(), "HOME="+homeDir)
		cmd.Dir = tempDir

		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Second mycelium init failed: %v\nOutput: %s", err, output)
		}

		// Should not overwrite existing config
		configContent, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("Failed to read config after second init: %v", err)
		}

		var secondConfig config.Config
		if err := yaml.Unmarshal(configContent, &secondConfig); err != nil {
			t.Fatalf("Config corrupted after second init: %v", err)
		}
	})
}

func TestMyceliumInitWithExistingConfig(t *testing.T) {
	RequireGitBinary(t)

	// Create test environment
	tempDir, err := os.MkdirTemp("", "mycelium-init-existing-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	binaryPath := buildMyceliumBinary(t, tempDir)
	homeDir := filepath.Join(tempDir, "home")
	myceliumDir := filepath.Join(homeDir, ".mycelium")

	// Create .mycelium directory and config file first
	if err := os.MkdirAll(myceliumDir, 0755); err != nil {
		t.Fatalf("Failed to create mycelium dir: %v", err)
	}

	// Create existing config with custom values
	existingConfigContent := `server:
  host: "0.0.0.0"
  port: 9999
  dataDir: "/custom/data"

storage:
  driver: "sqlite"
  dsn: "/custom/db.sqlite"

libraries:
  - name: "existing"
    path: "/custom/skills"
    priority: 200

defaults:
  author: "existing-user"`

	configPath := filepath.Join(myceliumDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(existingConfigContent), 0644); err != nil {
		t.Fatalf("Failed to create existing config: %v", err)
	}

	// Run mycelium init
	cmd := exec.Command(binaryPath, "init")
	cmd.Env = append(os.Environ(), "HOME="+homeDir)
	cmd.Dir = tempDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mycelium init failed: %v\nOutput: %s", err, output)
	}

	// Verify existing config was not overwritten
	configContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	if !strings.Contains(string(configContent), "existing-user") {
		t.Error("Existing config was overwritten instead of preserved")
	}

	if !strings.Contains(string(configContent), "port: 9999") {
		t.Error("Existing config port was overwritten")
	}

	// But skills directory should still be created
	skillsDir := filepath.Join(homeDir, ".mycelium", "skills")
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		t.Error("Skills directory was not created when config already existed")
	}
}

func TestMyceliumInitNoGit(t *testing.T) {
	// Test init behavior when git is not available
	tempDir, err := os.MkdirTemp("", "mycelium-init-nogit-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	binaryPath := buildMyceliumBinary(t, tempDir)
	homeDir := filepath.Join(tempDir, "home")
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatalf("Failed to create fake home dir: %v", err)
	}

	// Create environment with git removed from PATH
	env := []string{}
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "PATH=") {
			// Remove git from PATH (crude but effective for testing)
			continue
		}
		env = append(env, e)
	}
	env = append(env, "HOME="+homeDir)
	env = append(env, "PATH=/bin:/usr/bin") // Minimal PATH without git

	cmd := exec.Command(binaryPath, "init")
	cmd.Env = env
	cmd.Dir = tempDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mycelium init should not fail when git is missing: %v\nOutput: %s", err, output)
	}

	// Should still create directories and config
	myceliumDir := filepath.Join(homeDir, ".mycelium")
	if _, err := os.Stat(myceliumDir); os.IsNotExist(err) {
		t.Error("Mycelium directory was not created when git is missing")
	}

	configPath := filepath.Join(myceliumDir, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Config file was not created when git is missing")
	}

	// Skills directory should exist but not be a git repo
	skillsDir := filepath.Join(homeDir, ".mycelium", "skills")
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		t.Error("Skills directory was not created when git is missing")
	}

	gitDir := filepath.Join(skillsDir, ".git")
	if _, err := os.Stat(gitDir); !os.IsNotExist(err) {
		t.Error("Git repository should not be initialized when git binary is missing")
	}
}

func TestMyceliumInitPermissionError(t *testing.T) {
	// Test init behavior when home directory is not writable
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	tempDir, err := os.MkdirTemp("", "mycelium-init-perm-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	binaryPath := buildMyceliumBinary(t, tempDir)
	homeDir := filepath.Join(tempDir, "readonly-home")
	
	// Create readonly home directory
	if err := os.MkdirAll(homeDir, 0555); err != nil {
		t.Fatalf("Failed to create readonly home dir: %v", err)
	}
	defer os.Chmod(homeDir, 0755) // Restore permissions for cleanup

	cmd := exec.Command(binaryPath, "init")
	cmd.Env = append(os.Environ(), "HOME="+homeDir)
	cmd.Dir = tempDir

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Errorf("mycelium init should fail when home directory is not writable\nOutput: %s", output)
	}

	// Error message should be helpful
	if !strings.Contains(string(output), "permission") && 
	   !strings.Contains(string(output), "denied") &&
	   !strings.Contains(string(output), "failed") {
		t.Errorf("Error message should mention permission issue: %s", output)
	}
}

func TestMyceliumInitTildeExpansion(t *testing.T) {
	RequireGitBinary(t)

	// Test that config paths with ~ get expanded correctly
	tempDir, err := os.MkdirTemp("", "mycelium-init-tilde-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	binaryPath := buildMyceliumBinary(t, tempDir)
	homeDir := filepath.Join(tempDir, "home")
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatalf("Failed to create fake home dir: %v", err)
	}

	cmd := exec.Command(binaryPath, "init")
	cmd.Env = append(os.Environ(), "HOME="+homeDir)
	cmd.Dir = tempDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mycelium init failed: %v\nOutput: %s", err, output)
	}

	// Load the generated config
	configPath := filepath.Join(homeDir, ".mycelium", "config.yaml")
	configContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read generated config: %v", err)
	}

	configStr := string(configContent)

	// Config should contain tilde paths (not yet expanded in the file)
	if !strings.Contains(configStr, "~/.mycelium") {
		t.Error("Generated config should contain tilde paths")
	}

	// But when loaded through config package, paths should be expanded
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if strings.Contains(cfg.Server.DataDir, "~") {
		t.Error("Tilde should be expanded in loaded config")
	}

	if !strings.Contains(cfg.Server.DataDir, homeDir) {
		t.Errorf("DataDir should contain home directory path, got: %s", cfg.Server.DataDir)
	}
}

func TestMyceliumInitValidatesSuccessMessage(t *testing.T) {
	RequireGitBinary(t)

	tempDir, err := os.MkdirTemp("", "mycelium-init-message-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	binaryPath := buildMyceliumBinary(t, tempDir)
	homeDir := filepath.Join(tempDir, "home")
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatalf("Failed to create fake home dir: %v", err)
	}

	cmd := exec.Command(binaryPath, "init")
	cmd.Env = append(os.Environ(), "HOME="+homeDir)
	cmd.Dir = tempDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mycelium init failed: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)

	// Verify success message contains expected elements
	expectedPhrases := []string{
		"Mycelium initialized successfully",
		"Next steps",
		"config.yaml",
		"mycelium serve",
		"~/.mycelium/",
	}

	for _, phrase := range expectedPhrases {
		if !strings.Contains(outputStr, phrase) {
			t.Errorf("Success message should contain '%s'\nActual output: %s", phrase, outputStr)
		}
	}

	// Success message should be user-friendly (no debug logs)
	if strings.Contains(outputStr, "DEBUG") || strings.Contains(outputStr, "WARN") {
		t.Errorf("Success message should not contain debug/warning logs: %s", outputStr)
	}
}

func TestMyceliumInitFilePermissions(t *testing.T) {
	RequireGitBinary(t)

	tempDir, err := os.MkdirTemp("", "mycelium-init-perms-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	binaryPath := buildMyceliumBinary(t, tempDir)
	homeDir := filepath.Join(tempDir, "home")
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatalf("Failed to create fake home dir: %v", err)
	}

	cmd := exec.Command(binaryPath, "init")
	cmd.Env = append(os.Environ(), "HOME="+homeDir)
	cmd.Dir = tempDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mycelium init failed: %v\nOutput: %s", err, output)
	}

	// Check directory permissions
	myceliumDir := filepath.Join(homeDir, ".mycelium")
	info, err := os.Stat(myceliumDir)
	if err != nil {
		t.Fatalf("Failed to stat mycelium dir: %v", err)
	}

	if info.Mode().Perm() != 0755 {
		t.Errorf("Mycelium directory should have 0755 permissions, got %o", info.Mode().Perm())
	}

	// Check config file permissions
	configPath := filepath.Join(myceliumDir, "config.yaml")
	info, err = os.Stat(configPath)
	if err != nil {
		t.Fatalf("Failed to stat config file: %v", err)
	}

	if info.Mode().Perm() != 0644 {
		t.Errorf("Config file should have 0644 permissions, got %o", info.Mode().Perm())
	}

	// Check skills directory permissions
	skillsDir := filepath.Join(myceliumDir, "skills")
	info, err = os.Stat(skillsDir)
	if err != nil {
		t.Fatalf("Failed to stat skills dir: %v", err)
	}

	if info.Mode().Perm() != 0755 {
		t.Errorf("Skills directory should have 0755 permissions, got %o", info.Mode().Perm())
	}
}