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
	dataDir := filepath.Join(kinokoDir, "data")
	configFile := filepath.Join(kinokoDir, "config.yaml")

	// Create ~/.kinoko/ directory
	if err := os.MkdirAll(kinokoDir, 0755); err != nil {
		return fmt.Errorf("failed to create kinoko directory: %w", err)
	}
	slog.Info("Created directory", "path", kinokoDir)

	// Create ~/.kinoko/data/ for Soft Serve git server
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}
	slog.Info("Created data directory", "path", dataDir)

	// Create default config.yaml if it doesn't exist
	if err := createDefaultConfig(configFile); err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}

	// Generate admin SSH keypair for Soft Serve
	if err := generateAdminKeypair(dataDir); err != nil {
		return fmt.Errorf("failed to generate admin keypair: %w", err)
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
# Skills live as individual repos on the Soft Serve git server.
# Use 'kinoko serve' to start the server, then agents push skills via SSH.
libraries: []

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

// generateAdminKeypair creates an ed25519 SSH keypair for the Soft Serve admin.
func generateAdminKeypair(dataDir string) error {
	keyPath := filepath.Join(dataDir, "kinoko_admin_ed25519")
	if _, err := os.Stat(keyPath); err == nil {
		slog.Info("Admin keypair already exists", "path", keyPath)
		return nil
	}

	// Check if ssh-keygen is available
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		slog.Warn("ssh-keygen not found, skipping keypair generation (will be generated on first 'kinoko serve')")
		return nil
	}

	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-f", keyPath, "-N", "", "-C", "kinoko-admin")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ssh-keygen failed: %w", err)
	}

	// Restrict permissions
	os.Chmod(keyPath, 0600)
	os.Chmod(keyPath+".pub", 0644)

	slog.Info("Generated admin keypair", "path", keyPath)
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
	fmt.Println("  • Set OPENAI_API_KEY for extraction and embedding")
	fmt.Println("  • Run 'kinoko serve' to start the server")
	fmt.Println()
	fmt.Println("'kinoko serve' starts:")
	fmt.Println("  • Soft Serve git server (SSH :23231) — skill repos live here")
	fmt.Println("  • Discovery API (HTTP :23232) — clients find relevant skills")
	fmt.Println("  • Worker pool — async extraction from session logs")
	fmt.Println("  • Hooks — credential scanning + auto-indexing on push")
	fmt.Println()
}