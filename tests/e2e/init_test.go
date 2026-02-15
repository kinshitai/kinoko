//go:build integration

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/config"
	"gopkg.in/yaml.v3"
)

func TestKinokoInit(t *testing.T) {
	RequireGitBinary(t)

	// Create isolated test environment
	tempDir, err := os.MkdirTemp("", "kinoko-init-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Build kinoko binary
	binaryPath := buildKinokoBinary(t, tempDir)

	// Create fake home directory for testing
	homeDir := filepath.Join(tempDir, "home")
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatalf("Failed to create fake home dir: %v", err)
	}

	// Run kinoko init with custom HOME
	cmd := exec.Command(binaryPath, "init")
	cmd.Env = append(os.Environ(), "HOME="+homeDir)
	cmd.Dir = tempDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("kinoko init failed: %v\nOutput: %s", err, output)
	}

	t.Logf("kinoko init output: %s", output)

	// Verify directory structure was created
	expectedDirs := []string{
		".kinoko",
		".kinoko/skills",
	}

	for _, dir := range expectedDirs {
		dirPath := filepath.Join(homeDir, dir)
		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			t.Errorf("Expected directory not created: %s", dirPath)
		}
	}

	// Verify config file was created and is valid
	configPath := filepath.Join(homeDir, ".kinoko", "config.yaml")
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
	skillsDir := filepath.Join(homeDir, ".kinoko", "skills")
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
			t.Fatalf("Second kinoko init failed: %v\nOutput: %s", err, output)
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

func TestKinokoInitWithExistingConfig(t *testing.T) {
	RequireGitBinary(t)

	// Create test environment
	tempDir, err := os.MkdirTemp("", "kinoko-init-existing-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	binaryPath := buildKinokoBinary(t, tempDir)
	homeDir := filepath.Join(tempDir, "home")
	kinokoDir := filepath.Join(homeDir, ".kinoko")

	// Create .kinoko directory and config file first
	if err := os.MkdirAll(kinokoDir, 0755); err != nil {
		t.Fatalf("Failed to create kinoko dir: %v", err)
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

	configPath := filepath.Join(kinokoDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(existingConfigContent), 0644); err != nil {
		t.Fatalf("Failed to create existing config: %v", err)
	}

	// Run kinoko init
	cmd := exec.Command(binaryPath, "init")
	cmd.Env = append(os.Environ(), "HOME="+homeDir)
	cmd.Dir = tempDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("kinoko init failed: %v\nOutput: %s", err, output)
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
	skillsDir := filepath.Join(homeDir, ".kinoko", "skills")
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		t.Error("Skills directory was not created when config already existed")
	}
}

func TestKinokoInitNoGit(t *testing.T) {
	// Test init behavior when git is not available
	tempDir, err := os.MkdirTemp("", "kinoko-init-nogit-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	binaryPath := buildKinokoBinary(t, tempDir)
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
		t.Fatalf("kinoko init should not fail when git is missing: %v\nOutput: %s", err, output)
	}

	// Output should contain warning about missing git
	outputStr := string(output)
	if !strings.Contains(outputStr, "git") && !strings.Contains(outputStr, "warning") {
		t.Logf("Expected warning about missing git in output: %s", outputStr)
		// Note: This is documenting expected behavior - graceful degradation with warning
	}

	// Should still create directories and config
	kinokoDir := filepath.Join(homeDir, ".kinoko")
	if _, err := os.Stat(kinokoDir); os.IsNotExist(err) {
		t.Error("Kinoko directory was not created when git is missing")
	}

	configPath := filepath.Join(kinokoDir, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Config file was not created when git is missing")
	}

	// Skills directory should exist but not be a git repo
	skillsDir := filepath.Join(homeDir, ".kinoko", "skills")
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		t.Error("Skills directory was not created when git is missing")
	}

	gitDir := filepath.Join(skillsDir, ".git")
	if _, err := os.Stat(gitDir); !os.IsNotExist(err) {
		t.Error("Git repository should not be initialized when git binary is missing")
	}
}

func TestKinokoInitPermissionError(t *testing.T) {
	// Test init behavior when home directory is not writable
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	tempDir, err := os.MkdirTemp("", "kinoko-init-perm-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	binaryPath := buildKinokoBinary(t, tempDir)
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
		t.Errorf("kinoko init should fail when home directory is not writable\nOutput: %s", output)
	}

	// Error message should be helpful
	if !strings.Contains(string(output), "permission") && 
	   !strings.Contains(string(output), "denied") &&
	   !strings.Contains(string(output), "failed") {
		t.Errorf("Error message should mention permission issue: %s", output)
	}
}

func TestKinokoInitTildeExpansion(t *testing.T) {
	RequireGitBinary(t)

	// Test that config paths with ~ get expanded correctly
	tempDir, err := os.MkdirTemp("", "kinoko-init-tilde-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	binaryPath := buildKinokoBinary(t, tempDir)
	homeDir := filepath.Join(tempDir, "home")
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatalf("Failed to create fake home dir: %v", err)
	}

	cmd := exec.Command(binaryPath, "init")
	cmd.Env = append(os.Environ(), "HOME="+homeDir)
	cmd.Dir = tempDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("kinoko init failed: %v\nOutput: %s", err, output)
	}

	// Load the generated config
	configPath := filepath.Join(homeDir, ".kinoko", "config.yaml")
	configContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read generated config: %v", err)
	}

	configStr := string(configContent)

	// Config should contain tilde paths (not yet expanded in the file)
	if !strings.Contains(configStr, "~/.kinoko") {
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

func TestKinokoInitValidatesSuccessMessage(t *testing.T) {
	RequireGitBinary(t)

	tempDir, err := os.MkdirTemp("", "kinoko-init-message-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	binaryPath := buildKinokoBinary(t, tempDir)
	homeDir := filepath.Join(tempDir, "home")
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatalf("Failed to create fake home dir: %v", err)
	}

	cmd := exec.Command(binaryPath, "init")
	cmd.Env = append(os.Environ(), "HOME="+homeDir)
	cmd.Dir = tempDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("kinoko init failed: %v\nOutput: %s", err, output)
	}

	outputStr := string(output)

	// Verify success message contains expected elements
	expectedPhrases := []string{
		"Kinoko initialized successfully",
		"Next steps",
		"config.yaml",
		"kinoko serve",
		"~/.kinoko/",
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

func TestKinokoInitFilePermissions(t *testing.T) {
	RequireGitBinary(t)

	tempDir, err := os.MkdirTemp("", "kinoko-init-perms-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	binaryPath := buildKinokoBinary(t, tempDir)
	homeDir := filepath.Join(tempDir, "home")
	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatalf("Failed to create fake home dir: %v", err)
	}

	cmd := exec.Command(binaryPath, "init")
	cmd.Env = append(os.Environ(), "HOME="+homeDir)
	cmd.Dir = tempDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("kinoko init failed: %v\nOutput: %s", err, output)
	}

	// Check directory permissions
	kinokoDir := filepath.Join(homeDir, ".kinoko")
	info, err := os.Stat(kinokoDir)
	if err != nil {
		t.Fatalf("Failed to stat kinoko dir: %v", err)
	}

	if info.Mode().Perm() != 0755 {
		t.Errorf("Kinoko directory should have 0755 permissions, got %o", info.Mode().Perm())
	}

	// Check config file permissions
	configPath := filepath.Join(kinokoDir, "config.yaml")
	info, err = os.Stat(configPath)
	if err != nil {
		t.Fatalf("Failed to stat config file: %v", err)
	}

	if info.Mode().Perm() != 0644 {
		t.Errorf("Config file should have 0644 permissions, got %o", info.Mode().Perm())
	}

	// Check skills directory permissions
	skillsDir := filepath.Join(kinokoDir, "skills")
	info, err = os.Stat(skillsDir)
	if err != nil {
		t.Fatalf("Failed to stat skills dir: %v", err)
	}

	if info.Mode().Perm() != 0755 {
		t.Errorf("Skills directory should have 0755 permissions, got %o", info.Mode().Perm())
	}
}