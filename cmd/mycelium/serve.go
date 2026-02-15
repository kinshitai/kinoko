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

	slog.Info("Mycelium serve command started")
	slog.Info("Configuration loaded successfully", 
		"host", cfg.Server.Host,
		"port", cfg.Server.Port, 
		"dataDir", cfg.Server.DataDir,
		"storageDriver", cfg.Storage.Driver,
		"libraries", len(cfg.Libraries))

	slog.Warn("Git server integration is pending implementation")
	slog.Info("Currently performing setup and validation only")

	return performSetupAndWait(cmd.Context(), cfg)
}

// performSetupAndWait performs setup tasks and waits for shutdown signal
func performSetupAndWait(ctx context.Context, cfg *config.Config) error {
	// Set up context for graceful shutdown
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Create a done channel to avoid race conditions
	done := make(chan struct{})
	
	go func() {
		select {
		case sig := <-sigCh:
			slog.Info("Received shutdown signal", "signal", sig)
		case <-ctx.Done():
			slog.Info("Context cancelled")
		}
		close(done)
		cancel()
	}()

	slog.Info("Setup complete. Mycelium is ready but git server is not yet implemented.")
	slog.Info("Press Ctrl+C to exit")

	// TODO: When git server is implemented, this is where it would start:
	// - Initialize Soft Serve configuration
	// - Set up SSH keys and repositories
	// - Start the git server on the configured host:port
	// - Monitor server health and handle restarts
	
	// Wait for shutdown signal or context cancellation
	select {
	case <-done:
		// Signal received, gracefully shut down
	case <-ctx.Done():
		// Context cancelled
	}
	
	slog.Info("Mycelium serve stopped")
	return nil
}