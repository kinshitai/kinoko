//go:build integration

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/kinoko-dev/kinoko/internal/shared/config"
)

func TestKinokoInit(t *testing.T) {
	RequireGitBinary(t)

	tempDir, err := os.MkdirTemp("", "kinoko-init-test-*")
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

	t.Logf("kinoko init output: %s", output)

	// Verify directory structure: ~/.kinoko/ and ~/.kinoko/cache/
	kinokoDir := filepath.Join(homeDir, ".kinoko")
	if _, err := os.Stat(kinokoDir); os.IsNotExist(err) {
		t.Error("Expected ~/.kinoko/ directory not created")
	}

	cacheDir := filepath.Join(kinokoDir, "cache")
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		t.Error("Expected ~/.kinoko/cache/ directory not created")
	}

	// Verify config file was created and is valid
	configPath := filepath.Join(kinokoDir, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Config file was not created")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load generated config: %v", err)
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Generated config is invalid: %v", err)
	}

	// Verify SSH key was generated
	keyPath := filepath.Join(kinokoDir, "id_ed25519")
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Error("SSH key was not generated")
	}

	// Test that init is idempotent
	t.Run("idempotent", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "init")
		cmd.Env = append(os.Environ(), "HOME="+homeDir)
		cmd.Dir = tempDir

		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Second kinoko init failed: %v\nOutput: %s", err, output)
		}

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

	tempDir, err := os.MkdirTemp("", "kinoko-init-existing-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	binaryPath := buildKinokoBinary(t, tempDir)
	homeDir := filepath.Join(tempDir, "home")
	kinokoDir := filepath.Join(homeDir, ".kinoko")

	if err := os.MkdirAll(kinokoDir, 0755); err != nil {
		t.Fatalf("Failed to create kinoko dir: %v", err)
	}

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
}

func TestKinokoInitNoGit(t *testing.T) {
	// Test init behavior when git is not available — should still succeed
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

	// Create environment with no git in PATH
	env := []string{}
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "PATH=") {
			continue
		}
		env = append(env, e)
	}
	env = append(env, "HOME="+homeDir)
	// Minimal PATH with only ssh-keygen
	fakeBin := filepath.Join(tempDir, "fakebin")
	os.MkdirAll(fakeBin, 0755)
	if sshKeygen, err := exec.LookPath("ssh-keygen"); err == nil {
		os.Symlink(sshKeygen, filepath.Join(fakeBin, "ssh-keygen"))
	}
	env = append(env, "PATH="+fakeBin)

	cmd := exec.Command(binaryPath, "init")
	cmd.Env = env
	cmd.Dir = tempDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("kinoko init should not fail when git is missing: %v\nOutput: %s", err, output)
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
}

func TestKinokoInitPermissionError(t *testing.T) {
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

	if err := os.MkdirAll(homeDir, 0555); err != nil {
		t.Fatalf("Failed to create readonly home dir: %v", err)
	}
	defer os.Chmod(homeDir, 0755)

	cmd := exec.Command(binaryPath, "init")
	cmd.Env = append(os.Environ(), "HOME="+homeDir)
	cmd.Dir = tempDir

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Errorf("kinoko init should fail when home directory is not writable\nOutput: %s", output)
	}

	if !strings.Contains(string(output), "permission") &&
		!strings.Contains(string(output), "denied") &&
		!strings.Contains(string(output), "failed") {
		t.Errorf("Error message should mention permission issue: %s", output)
	}
}

func TestKinokoInitTildeExpansion(t *testing.T) {
	RequireGitBinary(t)

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

	configPath := filepath.Join(homeDir, ".kinoko", "config.yaml")
	configContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read generated config: %v", err)
	}

	// Config file should contain tilde paths
	if !strings.Contains(string(configContent), "~/.kinoko") {
		t.Error("Generated config should contain tilde paths")
	}

	// When loaded, tildes should be expanded to absolute paths
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if strings.Contains(cfg.Server.DataDir, "~") {
		t.Error("Tilde should be expanded in loaded config")
	}

	if !filepath.IsAbs(cfg.Server.DataDir) {
		t.Errorf("DataDir should be an absolute path, got: %s", cfg.Server.DataDir)
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

	expectedPhrases := []string{
		"Kinoko initialized successfully",
		"kinoko serve",
		"~/.kinoko/",
	}

	for _, phrase := range expectedPhrases {
		if !strings.Contains(outputStr, phrase) {
			t.Errorf("Success message should contain '%s'\nActual output: %s", phrase, outputStr)
		}
	}

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

	kinokoDir := filepath.Join(homeDir, ".kinoko")
	info, err := os.Stat(kinokoDir)
	if err != nil {
		t.Fatalf("Failed to stat kinoko dir: %v", err)
	}
	if info.Mode().Perm() != 0755 {
		t.Errorf("Kinoko directory should have 0755 permissions, got %o", info.Mode().Perm())
	}

	configPath := filepath.Join(kinokoDir, "config.yaml")
	info, err = os.Stat(configPath)
	if err != nil {
		t.Fatalf("Failed to stat config file: %v", err)
	}
	if info.Mode().Perm() != 0644 {
		t.Errorf("Config file should have 0644 permissions, got %o", info.Mode().Perm())
	}

	cacheDir := filepath.Join(kinokoDir, "cache")
	info, err = os.Stat(cacheDir)
	if err != nil {
		t.Fatalf("Failed to stat cache dir: %v", err)
	}
	if info.Mode().Perm() != 0755 {
		t.Errorf("Cache directory should have 0755 permissions, got %o", info.Mode().Perm())
	}
}
