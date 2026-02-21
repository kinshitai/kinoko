package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/kinoko-dev/kinoko/internal/api"
	"github.com/kinoko-dev/kinoko/internal/config"
	"github.com/kinoko-dev/kinoko/internal/embedding"
	"github.com/kinoko-dev/kinoko/internal/gitserver"
	"github.com/kinoko-dev/kinoko/internal/storage"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Kinoko infrastructure server",
	Long: `Starts the shared Kinoko infrastructure server.

This command starts:
  • Soft Serve git server (SSH) — skill repos live here
  • Discovery API (HTTP) — /api/v1/discover, /api/v1/health, /api/v1/ingest
  • Hooks — credential scanning + auto-indexing on push
  • SQLite indexer — derived cache from git repos

Self-bootstrapping: creates data dir and admin keypair on first run.

Use 'kinoko run' in a separate terminal to start the local agent daemon
(worker pool, scheduler, injection).`,
	RunE: runServe,
}

var (
	configPath string
)

func init() {
	serveCmd.Flags().StringVar(&configPath, "config", "", "Config file path (default: ~/.kinoko/config.yaml)")
}

// bootstrapServer ensures server data dir and admin keypair exist.
func bootstrapServer(dataDir string, logger *slog.Logger) error {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("create data directory %s: %w", dataDir, err)
	}

	keyPath := filepath.Join(dataDir, "kinoko_admin_ed25519")
	if _, err := os.Stat(keyPath); err == nil {
		return nil // already exists
	}

	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		logger.Warn("ssh-keygen not found, skipping admin keypair generation")
		return nil
	}

	logger.Info("Generating admin keypair for Soft Serve...", "path", keyPath)
	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-f", keyPath, "-N", "", "-C", "kinoko-admin")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ssh-keygen failed: %w", err)
	}
	if err := os.Chmod(keyPath, 0600); err != nil {
		return fmt.Errorf("chmod private key: %w", err)
	}
	if err := os.Chmod(keyPath+".pub", 0644); err != nil {
		return fmt.Errorf("chmod public key: %w", err)
	}
	logger.Info("Admin keypair generated", "path", keyPath)
	return nil
}

// buildIndexFn returns a function that triggers skill indexing for a given repo+rev.
// It shells out to `kinoko index` so the indexing logic stays in one place.
func buildIndexFn(_ *storage.SQLiteStore, dataDir string, logger *slog.Logger) func(ctx context.Context, repo, rev string) error {
	kinokoBin, err := os.Executable()
	if err != nil {
		kinokoBin = "kinoko"
	}
	return func(ctx context.Context, repo, rev string) error {
		args := []string{"index", "--repo", repo, "--data-dir", dataDir}
		if rev != "" {
			args = append(args, "--rev", rev)
		}
		cmd := exec.CommandContext(ctx, kinokoBin, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		logger.Info("triggering index", "repo", repo, "rev", rev)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("kinoko index: %w", err)
		}
		return nil
	}
}

// libraryIDs is defined in workers_run.go.
// decayConfigFromYAML is defined in decay.go.

func runServe(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	logger := slog.Default()

	// Self-bootstrap: create data dir + admin keypair if needed.
	if err := bootstrapServer(cfg.Server.DataDir, logger); err != nil {
		return fmt.Errorf("bootstrap server: %w", err)
	}

	logger.Info("Kinoko server starting",
		"host", cfg.Server.Host,
		"port", cfg.Server.Port,
		"dataDir", cfg.Server.DataDir,
		"storageDriver", cfg.Storage.Driver,
		"libraries", len(cfg.Libraries))

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

	// Create and start the git server.
	server, err := gitserver.NewServer(cfg)
	if err != nil {
		return fmt.Errorf("failed to create git server: %w", err)
	}

	// Install Soft Serve hooks for post-receive indexing.
	kinokoBin, err := os.Executable()
	if err != nil {
		kinokoBin = "kinoko" // fallback to PATH
	}
	if err := gitserver.InstallHooks(cfg.Server.DataDir, kinokoBin, cfg.Server.GetAPIPort()); err != nil {
		logger.Warn("failed to install git hooks", "error", err)
	} else {
		logger.Info("git hooks installed", "data_dir", cfg.Server.DataDir)
	}

	if err := server.Start(); err != nil {
		return fmt.Errorf("failed to start git server: %w", err)
	}

	connInfo := server.GetConnectionInfo()
	logger.Info("Kinoko git server is ready",
		"ssh_url", connInfo.SSHUrl,
		"host", connInfo.SSHHost,
		"port", connInfo.SSHPort)

	// Start HTTP API server for discovery + ingestion.
	embCfgAPI := embedding.DefaultConfig()
	embCfgAPI.APIKey = os.Getenv("KINOKO_EMBEDDING_API_KEY")
	if embCfgAPI.APIKey == "" {
		embCfgAPI.APIKey = os.Getenv("OPENAI_API_KEY")
	}
	var apiEmbedder embedding.Embedder
	if embCfgAPI.APIKey != "" {
		apiEmbedder = embedding.New(embCfgAPI, logger)
	} else {
		logger.Warn("No embedding API key — /discover will return 503")
	}
	apiPort := cfg.Server.GetAPIPort()
	// Build indexFn that reuses the store and embedder for post-receive hook triggers.
	indexFn := buildIndexFn(store, cfg.Server.DataDir, logger)
	apiSrv := api.New(api.Config{
		Host:     cfg.Server.Host,
		Port:     apiPort,
		Store:    store,
		Embedder: apiEmbedder,
		SSHURL:   connInfo.SSHUrl,
		Logger:   logger,
		IndexFn:  indexFn,
	})
	// Wire embedding engine for /api/v1/embed endpoint.
	// Built with -tags embedding: real ONNX engine. Without: nil (503).
	embedEngine, err := initEmbedEngine(cfg, logger)
	switch {
	case err != nil:
		logger.Error("failed to init embedding engine", "error", err)
	case embedEngine != nil:
		apiSrv.SetEmbedEngine(embedEngine)
		logger.Info("Embedding engine enabled", "model", embedEngine.ModelID(), "dims", embedEngine.Dims())
	default:
		logger.Info("Embedding engine disabled (built without native deps)")
	}

	if err := apiSrv.Start(); err != nil {
		logger.Error("failed to start API server", "error", err)
	} else {
		logger.Info("API server ready", "port", apiPort)
	}

	// Start decay scheduler (server-side concern: reads/writes skill scores in index DB).
	decaySched, err := newDecayScheduler(store, cfg, logger)
	if err != nil {
		logger.Error("failed to create decay scheduler", "error", err)
	} else {
		decaySched.Start(cmd.Context())
		logger.Info("Decay scheduler started", "interval", decaySched.interval)
	}

	// Wait for shutdown signal.
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	done := make(chan struct{})
	go func() {
		select {
		case sig := <-sigCh:
			logger.Info("Received shutdown signal", "signal", sig)
		case <-ctx.Done():
			logger.Info("Context cancelled")
		}
		close(done)
		cancel()
	}()

	logger.Info("Kinoko server is ready. Use Ctrl+C to shutdown gracefully.")
	logger.Info("Run 'kinoko run' in another terminal to start workers and scheduler.")

	select {
	case <-done:
	case <-ctx.Done():
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if apiSrv != nil {
		if err := apiSrv.Stop(shutdownCtx); err != nil {
			logger.Error("error stopping API server", "error", err)
		}
	}

	if decaySched != nil {
		logger.Info("Stopping decay scheduler...")
		decaySched.Stop()
	}

	logger.Info("Stopping git server...")
	if err := server.Stop(); err != nil {
		logger.Error("Error stopping git server", "error", err)
		return err
	}

	logger.Info("Kinoko server stopped successfully")
	return nil
}
