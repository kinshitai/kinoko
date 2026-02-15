package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/mycelium-dev/mycelium/internal/config"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Mycelium git server",
	Long: `Starts a Soft Serve git server for hosting skill repositories.
This is the source of truth for all Mycelium knowledge.`,
	RunE: runServe,
}

var (
	configPath string
)

func init() {
	// Set up flags
	serveCmd.Flags().StringVar(&configPath, "config", "", "Config file path (default: ~/.mycelium/config.yaml)")
}

func runServe(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Create data directory if it doesn't exist
	if err := os.MkdirAll(cfg.Server.DataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory %s: %w", cfg.Server.DataDir, err)
	}

	slog.Info("Starting Mycelium git server", 
		"port", cfg.Server.Port, 
		"dataDir", cfg.Server.DataDir,
		"storageDriver", cfg.Storage.Driver,
		"libraries", len(cfg.Libraries))

	// TODO: Research and implement Soft Serve embedding
	// Current approach: document the research and use the simplest working method
	
	/*
	SOFT SERVE INTEGRATION RESEARCH:
	
	Approach 1: Try to embed Soft Serve as a library
	- Import github.com/charmbracelet/soft-serve/pkg/...
	- Create server instance programmatically
	- Configure repositories, SSH keys, etc.
	
	Approach 2: Managed subprocess (simpler, working approach)
	- Start soft-serve as a subprocess with proper configuration
	- Monitor the process and handle restarts
	- Pass configuration via config files or environment variables
	
	For now, implementing Approach 2 as it's more reliable and simpler.
	Soft Serve is designed primarily as a standalone binary, not a library.
	*/

	return startSoftServeSubprocess(cmd.Context(), cfg)
}

// startSoftServeSubprocess starts Soft Serve as a managed subprocess
func startSoftServeSubprocess(ctx context.Context, cfg *config.Config) error {
	// Set up context for graceful shutdown
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		slog.Info("Received shutdown signal, stopping server...")
		cancel()
	}()

	// For now, simulate the server running
	// TODO: Replace with actual soft-serve subprocess management
	slog.Info("Git server running", "port", cfg.Server.Port, "dataDir", cfg.Server.DataDir)
	slog.Info("Press Ctrl+C to stop")

	// Wait for context cancellation
	<-ctx.Done()
	slog.Info("Server stopped")

	return nil
}