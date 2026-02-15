package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kinoko-dev/kinoko/internal/config"
	"github.com/kinoko-dev/kinoko/internal/storage"
	"github.com/spf13/cobra"
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
)

func init() {
	runCmd.Flags().StringVar(&runConfigPath, "config", "", "Config file path (default: ~/.kinoko/config.yaml)")
	runCmd.Flags().StringVar(&runServerURL, "server", "", "Server URL override (e.g. localhost:23231)")
}

func runRun(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(runConfigPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// P2-11: Apply --server override if provided.
	if runServerURL != "" {
		cfg.Server.Host = runServerURL
	}

	logger := slog.Default()
	logger.Info("Kinoko agent daemon starting",
		"server", fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port))

	embeddingModel := cfg.Embedding.Model
	if embeddingModel == "" {
		embeddingModel = os.Getenv("KINOKO_EMBEDDING_MODEL")
	}
	if embeddingModel == "" {
		embeddingModel = "text-embedding-3-small"
	}

	store, err := storage.NewSQLiteStore(cfg.Storage.DSN, embeddingModel)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer store.Close()

	// Start worker system (queue + pool + scheduler).
	// Pass nil for gitSrv — the run command doesn't own the git server.
	_, pool, sched, err := startWorkerSystem(cmd.Context(), cfg, store, nil, logger)
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
