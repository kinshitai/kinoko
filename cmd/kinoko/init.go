package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/kinoko-dev/kinoko/internal/client"
)

var (
	connectURL string
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize Kinoko workspace",
	Long: `Initialize a new Kinoko workspace in ~/.kinoko/.

Without flags, creates the necessary directories, configuration file, and git repository
for managing your local skills (server mode).

With --connect <url>, connects this machine to a remote Kinoko server (client mode).`,
	RunE: initCommand,
}

func init() {
	initCmd.Flags().StringVar(&connectURL, "connect", "", "Connect to a remote Kinoko server (client mode)")
}

// initCommand implements the 'kinoko init' command
func initCommand(cmd *cobra.Command, args []string) error {
	if connectURL != "" {
		return initClientMode(cmd, connectURL)
	}
	slog.Info("Initializing Kinoko workspace...")

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}

	kinokoDir := filepath.Join(homeDir, ".kinoko")
	skillsDir := filepath.Join(kinokoDir, "skills")
	configFile := filepath.Join(kinokoDir, "config.yaml")

	// Create ~/.kinoko/ directory
	if err := os.MkdirAll(kinokoDir, 0755); err != nil {
		return fmt.Errorf("failed to create kinoko directory: %w", err)
	}
	slog.Info("Created directory", "path", kinokoDir)

	// Create ~/.kinoko/skills/ directory
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return fmt.Errorf("failed to create skills directory: %w", err)
	}
	slog.Info("Created skills directory", "path", skillsDir)

	// Create default config.yaml if it doesn't exist
	if err := createDefaultConfig(configFile); err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}

	// Initialize git repo in skills directory if not already a repo
	if err := initGitRepo(skillsDir); err != nil {
		return fmt.Errorf("failed to initialize git repository: %w", err)
	}

	// Print success message and next steps
	printSuccessMessage()

	return nil
}

// createDefaultConfig creates a default config.yaml file if it doesn't exist
func createDefaultConfig(configFile string) error {
	// Check if config file already exists
	if _, err := os.Stat(configFile); err == nil {
		slog.Info("Config file already exists", "path", configFile)
		return nil
	}

	defaultConfig := `# Kinoko Configuration
# This file controls your local Kinoko setup

# Storage configuration
storage:
  driver: sqlite
  dsn: ~/.kinoko/kinoko.db

# Library layers (resolution order: highest priority first)
libraries:
  - name: local
    path: ~/.kinoko/skills
    priority: 100
    description: "Local skills on this machine"

# Server configuration (for 'kinoko serve')
server:
  host: "127.0.0.1"
  port: 23231
  dataDir: ~/.kinoko/data

# Extraction settings
extraction:
  auto_extract: true
  min_confidence: 0.5
  require_validation: true

# Pre-commit hooks
hooks:
  credential_scan: true
  format_validation: true
  llm_critic: false  # Enable when you have an LLM API configured

# Default skill template
defaults:
  author: ""  # Set this to your preferred author identifier
  confidence: 0.7
`

	if err := os.WriteFile(configFile, []byte(defaultConfig), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	slog.Info("Created default config", "path", configFile)
	return nil
}

// initGitRepo initializes a git repository in the given directory if one doesn't exist
func initGitRepo(skillsDir string) error {
	// Check if .git directory exists
	gitDir := filepath.Join(skillsDir, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		slog.Info("Git repository already exists", "path", skillsDir)
		return nil
	}

	// Check if git is available
	if _, err := exec.LookPath("git"); err != nil {
		slog.Warn("Git not found in PATH, skipping git repository initialization")
		return nil
	}

	// Initialize git repository
	cmd := exec.Command("git", "init")
	cmd.Dir = skillsDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to initialize git repository: %w", err)
	}

	slog.Info("Initialized git repository", "path", skillsDir)

	// Create a basic .gitignore
	gitignoreFile := filepath.Join(skillsDir, ".gitignore")
	gitignoreContent := `# Kinoko local files
*.tmp
*.log
.DS_Store
Thumbs.db

# Editor files
*.swp
*.swo
*~
.vscode/
.idea/
`

	if err := os.WriteFile(gitignoreFile, []byte(gitignoreContent), 0644); err != nil {
		slog.Warn("Failed to create .gitignore", "error", err)
		// Not a fatal error
	}

	// Create initial commit
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = skillsDir
	if err := cmd.Run(); err == nil {
		cmd = exec.Command("git", "commit", "-m", "Initial commit: Kinoko skills repository")
		cmd.Dir = skillsDir
		_ = cmd.Run() // Ignore error - commit might fail if git user is not configured
	}

	return nil
}

// initClientMode connects to a remote Kinoko server and saves client config.
func initClientMode(_ *cobra.Command, serverURL string) error {
	fmt.Println("🍄 Connecting to Kinoko server...")

	apiURL, err := client.ParseServerURL(serverURL)
	if err != nil {
		return fmt.Errorf("invalid server URL: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cacheDir := filepath.Join(homeDir, ".kinoko", "cache")

	c := client.New(client.ClientConfig{
		APIURL:   apiURL,
		CacheDir: cacheDir,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Health(ctx); err != nil {
		return fmt.Errorf("cannot reach server: %w", err)
	}
	fmt.Printf("✓ Server reachable at %s\n", apiURL)

	// Save config
	configPath := client.DefaultConfigPath()
	if err := client.SaveClientConfig(configPath, client.ClientSection{
		API:          apiURL,
		Server:       serverURL,
		CacheDir:     cacheDir,
		PullInterval: "5m",
	}); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	fmt.Printf("✓ Config written to %s\n", configPath)

	// Create cache dir
	os.MkdirAll(cacheDir, 0755)

	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  • kinoko pull <skill-repo>  — clone a skill locally\n")
	fmt.Printf("  • kinoko pull --all         — sync all cached skills\n")
	fmt.Println()

	return nil
}

// printSuccessMessage prints the success message and next steps
func printSuccessMessage() {
	fmt.Println()
	fmt.Println("🍄 Kinoko initialized successfully!")
	fmt.Println()
	fmt.Println("Your Kinoko workspace is ready at ~/.kinoko/")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  • Edit ~/.kinoko/config.yaml to configure your setup")
	fmt.Println("  • Set your preferred author in the config file")
	fmt.Println("  • Run 'kinoko serve' to start the git server")
	fmt.Println("  • Agents can then git clone, push, and pull skill repositories over SSH")
	fmt.Println()
	fmt.Println("Your local skills will be stored in ~/.kinoko/skills/")
	fmt.Println("This directory is already a git repository for version control.")
	fmt.Println()
}