package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"context"

	"github.com/spf13/cobra"
	"github.com/mycelium-dev/mycelium/internal/config"
	"github.com/mycelium-dev/mycelium/internal/storage"
	"github.com/mycelium-dev/mycelium/internal/worker"
)

var workerCmd = &cobra.Command{
	Use:   "worker",
	Short: "Run standalone extraction workers",
	Long:  `Starts a worker pool and scheduler without the git server. Use for scaling extraction workers separately.`,
	RunE:  runWorker,
}

var workerConfigPath string

func init() {
	workerCmd.Flags().StringVar(&workerConfigPath, "config", "", "Config file path")
}

func runWorker(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(workerConfigPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := os.MkdirAll(cfg.Server.DataDir, 0755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	logger := slog.Default()
	logger.Info("Mycelium worker starting")

	store, err := storage.NewSQLiteStore(cfg.Storage.DSN, "")
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer store.Close()

	_, pool, sched, err := startWorkerSystem(cmd.Context(), cfg, store, logger)
	if err != nil {
		return fmt.Errorf("start worker system: %w", err)
	}

	logger.Info("Worker system running. Use Ctrl+C to shutdown.")

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

	_ = ignoreNil(sched, func(s worker.Scheduler) error { return s.Stop(shutdownCtx) })
	_ = ignoreNil(pool, func(p worker.Pool) error { return p.Stop(shutdownCtx) })

	logger.Info("Worker stopped")
	return nil
}

// ignoreNil calls fn only if v is non-nil.
func ignoreNil[T any](v T, fn func(T) error) error {
	// Use interface check since T might be an interface.
	if any(v) == nil {
		return nil
	}
	return fn(v)
}
