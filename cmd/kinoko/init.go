package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/kinoko-dev/kinoko/internal/client"
	"github.com/spf13/cobra"
)

var (
	connectURL string
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize Kinoko workspace",
	Long: `Initialize a new Kinoko workspace in ~/.kinoko/.

Creates the necessary directories, configuration file, and client SSH key
for connecting to a Kinoko server.

With --connect <url>, connects this machine to a specific Kinoko server
(default: localhost:23231).`,
	RunE: initCommand,
}

func init() {
	initCmd.Flags().StringVar(&connectURL, "connect", "", "Kinoko server URL (default: localhost:23231)")
}

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
	cacheDir := filepath.Join(kinokoDir, "cache")
	configFile := filepath.Join(kinokoDir, "config.yaml")

	// Create ~/.kinoko/ directory
	if err := os.MkdirAll(kinokoDir, 0755); err != nil {
		return fmt.Errorf("failed to create kinoko directory: %w", err)
	}
	slog.Info("Created directory", "path", kinokoDir)

	// Create ~/.kinoko/cache/ for local skill cache
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}
	slog.Info("Created cache directory", "path", cacheDir)

	// Create default config.yaml if it doesn't exist
	if err := createDefaultConfig(configFile); err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}

	// Generate client SSH key for authenticating with the server
	if err := generateClientKey(kinokoDir); err != nil {
		return fmt.Errorf("failed to generate client SSH key: %w", err)
	}

	printSuccessMessage()
	return nil
}

// createDefaultConfig creates a default config.yaml file if it doesn't exist
func createDefaultConfig(configFile string) error {
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
libraries: []

# Server configuration
# The server URL that 'kinoko run' pushes extracted skills to
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

// generateClientKey creates an ed25519 SSH key for authenticating with the Kinoko server.
func generateClientKey(kinokoDir string) error {
	keyPath := filepath.Join(kinokoDir, "id_ed25519")
	if _, err := os.Stat(keyPath); err == nil {
		slog.Info("Client SSH key already exists", "path", keyPath)
		return nil
	}

	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		slog.Warn("ssh-keygen not found, skipping client key generation")
		return nil
	}

	slog.Info("Generating client SSH key...")
	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-f", keyPath, "-N", "", "-C", "kinoko-client")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ssh-keygen failed: %w", err)
	}

	os.Chmod(keyPath, 0600)
	os.Chmod(keyPath+".pub", 0644)

	slog.Info("Generated client SSH key", "path", keyPath)
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Health(ctx); err != nil {
		return fmt.Errorf("cannot reach server: %w", err)
	}
	fmt.Printf("✓ Server reachable at %s\n", apiURL)

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

	os.MkdirAll(cacheDir, 0755)

	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  • kinoko run   — start the local agent daemon\n")
	fmt.Printf("  • kinoko pull  — sync skills from the server\n")
	fmt.Println()

	return nil
}

func printSuccessMessage() {
	fmt.Println()
	fmt.Println("🍄 Kinoko initialized successfully!")
	fmt.Println()
	fmt.Println("Your workspace is ready at ~/.kinoko/")
	fmt.Println()
	fmt.Println("Three commands to know:")
	fmt.Println()
	fmt.Println("  kinoko serve   — Start the shared infrastructure server")
	fmt.Println("                   (git server, API, hooks, indexer)")
	fmt.Println()
	fmt.Println("  kinoko run     — Start the local agent daemon")
	fmt.Println("                   (worker pool, scheduler, injection)")
	fmt.Println()
	fmt.Println("  kinoko init    — You just ran this :)")
	fmt.Println()
	fmt.Println("Solo use:  'kinoko serve' + 'kinoko run' in separate terminals")
	fmt.Println("Team use:  One shared 'kinoko serve', each machine runs 'kinoko run'")
	fmt.Println()
	fmt.Println("Set OPENAI_API_KEY for extraction and embedding.")
	fmt.Println()
}
