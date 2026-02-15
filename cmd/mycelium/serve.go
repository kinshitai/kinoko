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
	"github.com/mycelium-dev/mycelium/internal/gitserver"
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

	// Create and start the git server
	server, err := gitserver.NewServer(cfg)
	if err != nil {
		return fmt.Errorf("failed to create git server: %w", err)
	}

	if err := server.Start(); err != nil {
		return fmt.Errorf("failed to start git server: %w", err)
	}

	// Get connection info for logging
	connInfo := server.GetConnectionInfo()
	slog.Info("Mycelium git server is ready",
		"ssh_url", connInfo.SSHUrl,
		"host", connInfo.SSHHost,
		"port", connInfo.SSHPort)

	return waitForShutdown(cmd.Context(), server)
}

// waitForShutdown waits for shutdown signal and gracefully stops the server
func waitForShutdown(ctx context.Context, server *gitserver.Server) error {
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

	// TODO(extraction): Post-session hook integration point.
	// After a session completes (detected via git push or session API),
	// read the session log, construct a SessionRecord, and run the extraction
	// pipeline asynchronously. Wire here:
	//   go func() { pipeline.Extract(ctx, session, content) }()
	//
	// TODO(injection): Pre-session hook integration point.
	// Before a session starts (or on first prompt), run the injector to
	// select relevant skills and prepend them to the agent context. Wire here:
	//   resp, _ := injector.Inject(ctx, extraction.InjectionRequest{...})

	slog.Info("Mycelium is ready. Use Ctrl+C to shutdown gracefully.")
	slog.Info("Agents can now git clone, push, and pull over SSH")
	
	// Wait for shutdown signal or context cancellation
	select {
	case <-done:
		// Signal received, gracefully shut down
	case <-ctx.Done():
		// Context cancelled
	}
	
	slog.Info("Shutting down git server...")
	if err := server.Stop(); err != nil {
		slog.Error("Error stopping git server", "error", err)
		return err
	}
	
	slog.Info("Mycelium serve stopped successfully")
	return nil
}