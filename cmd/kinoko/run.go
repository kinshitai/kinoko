package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/kinoko-dev/kinoko/internal/run/queue"
	"github.com/kinoko-dev/kinoko/internal/run/serverclient"
	"github.com/kinoko-dev/kinoko/internal/shared/config"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start the local Kinoko agent daemon",
	Long: `Starts the local agent daemon that runs alongside your AI agents.

This command starts:
  • Worker pool — extracts knowledge from local session logs
  • Scheduler — decay cron, stale sweep, periodic stats
  • Injection — loads relevant skills into agent sessions

The daemon reads the server URL from config to push extracted skills.
Run 'kinoko serve' separately to start the shared infrastructure server.`,
	RunE: runRun,
}

var (
	runConfigPath string
	runServerURL  string
	runDebug      bool
)

func init() {
	runCmd.Flags().StringVar(&runConfigPath, "config", "", "Config file path (default: ~/.kinoko/config.yaml)")
	runCmd.Flags().StringVar(&runServerURL, "server", "", "Server URL override (e.g. localhost:23231)")
	runCmd.Flags().BoolVar(&runDebug, "debug", false, "Enable pipeline debug tracing")
}

func runRun(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(runConfigPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Apply --server override if provided (host:port format).
	if runServerURL != "" {
		host, port, err := net.SplitHostPort(runServerURL)
		if err != nil {
			// Assume host-only, keep existing port.
			cfg.Server.Host = runServerURL
		} else {
			cfg.Server.Host = host
			if p, err := strconv.Atoi(port); err == nil {
				cfg.Server.Port = p
			}
		}
	}

	// Debug tracing: CLI flag overrides config.
	if runDebug {
		cfg.Debug.Enabled = true
	}
	if cfg.Debug.Enabled && cfg.Debug.Dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			slog.Warn("debug tracing: failed to determine home directory, debug tracing disabled", "error", err)
			cfg.Debug.Enabled = false
		} else {
			cfg.Debug.Dir = filepath.Join(home, ".kinoko", "debug")
		}
	}
	if cfg.Debug.Enabled {
		slog.Info("debug tracing enabled", "dir", cfg.Debug.Dir)
	}

	logger := slog.Default()

	serverURL := cfg.ServerURL()
	logger.Info("Kinoko agent daemon starting", "server", serverURL)

	// Open local queue DB.
	queueDSN := cfg.Client.GetQueueDSN()
	queueStore, err := queue.New(queueDSN)
	if err != nil {
		return fmt.Errorf("open queue store: %w", err)
	}
	defer queueStore.Close()

	// Create server client for HTTP communication.
	serverClient := serverclient.New(serverURL)

	// Start worker system (queue + pool + scheduler).
	_, pool, sched, err := startClientWorkerSystem(cmd.Context(), cfg, queueStore, serverClient, logger)
	if err != nil {
		return fmt.Errorf("start worker system: %w", err)
	}

	logger.Info("Kinoko agent daemon is ready. Use Ctrl+C to shutdown.")

	// Wait for signal.
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		logger.Info("Received shutdown signal", "signal", sig)
	case <-ctx.Done():
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if sched != nil {
		logger.Info("Stopping scheduler...")
		if err := sched.Stop(shutdownCtx); err != nil {
			logger.Error("error stopping scheduler", "error", err)
		}
	}
	if pool != nil {
		logger.Info("Stopping worker pool...")
		if err := pool.Stop(shutdownCtx); err != nil {
			logger.Error("error stopping worker pool", "error", err)
		}
	}

	logger.Info("Kinoko agent daemon stopped")
	return nil
}
