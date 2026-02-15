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

	"github.com/kinoko-dev/kinoko/internal/api"
	"github.com/kinoko-dev/kinoko/internal/config"
	"github.com/kinoko-dev/kinoko/internal/decay"
	"github.com/kinoko-dev/kinoko/internal/embedding"
	"github.com/kinoko-dev/kinoko/internal/extraction"
	"github.com/kinoko-dev/kinoko/internal/gitserver"
	"github.com/kinoko-dev/kinoko/internal/injection"
	"github.com/kinoko-dev/kinoko/internal/llm"
	"github.com/kinoko-dev/kinoko/internal/model"
	"github.com/kinoko-dev/kinoko/internal/storage"
	"github.com/kinoko-dev/kinoko/internal/worker"
	"github.com/spf13/cobra"
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
	os.Chmod(keyPath, 0600)
	os.Chmod(keyPath+".pub", 0644)
	logger.Info("Admin keypair generated", "path", keyPath)
	return nil
}

// SessionHooks holds callbacks for session lifecycle events.
type SessionHooks struct {
	OnSessionStart func(ctx context.Context, req model.InjectionRequest) (*model.InjectionResponse, error)
	OnSessionEnd   func(ctx context.Context, session model.SessionRecord, logContent []byte) (*model.ExtractionResult, error)
}

// buildSessionHooks wires the injection pipeline into session lifecycle hooks.
func buildSessionHooks(cfg *config.Config, store *storage.SQLiteStore, logger *slog.Logger) (*SessionHooks, error) {
	hooks := &SessionHooks{}

	embCfg := embedding.DefaultConfig()
	embCfg.APIKey = os.Getenv("KINOKO_EMBEDDING_API_KEY")
	if embCfg.APIKey == "" {
		embCfg.APIKey = os.Getenv("OPENAI_API_KEY")
	}
	embedder := embedding.New(embCfg, logger)

	llmAPIKey := os.Getenv("KINOKO_LLM_API_KEY")
	if llmAPIKey == "" {
		llmAPIKey = os.Getenv("OPENAI_API_KEY")
	}

	if embCfg.APIKey != "" {
		var llmClient llm.LLMClient
		if llmAPIKey != "" {
			llmClient = llm.NewOpenAIClient(llmAPIKey, "gpt-4o-mini")
		}

		abCfg := injection.ABConfig{
			Enabled:       cfg.Extraction.ABTest.Enabled,
			ControlRatio:  cfg.Extraction.ABTest.ControlRatio,
			MinSampleSize: cfg.Extraction.ABTest.MinSampleSize,
		}
		var inj injection.Injector
		if abCfg.Enabled {
			baseInj := injection.New(embedder, store, llmClient, nil, logger)
			inj = injection.NewABInjector(baseInj, store, abCfg, logger)
		} else {
			inj = injection.New(embedder, store, llmClient, store, logger)
		}

		hooks.OnSessionStart = func(ctx context.Context, req model.InjectionRequest) (*model.InjectionResponse, error) {
			resp, err := inj.Inject(ctx, req)
			if err != nil {
				logger.Error("injection failed", "error", err)
				return nil, err
			}
			logger.Info("injection complete", "skills_injected", len(resp.Skills))
			return resp, nil
		}
		logger.Info("injection hook registered")
	} else {
		hooks.OnSessionStart = func(_ context.Context, _ model.InjectionRequest) (*model.InjectionResponse, error) {
			return &model.InjectionResponse{}, nil
		}
		logger.Warn("injection hook disabled: no embedding API key")
	}

	// Extraction hook is a no-op on the server — extraction happens in 'kinoko run'.
	hooks.OnSessionEnd = func(_ context.Context, _ model.SessionRecord, _ []byte) (*model.ExtractionResult, error) {
		return &model.ExtractionResult{Status: model.StatusRejected}, nil
	}

	return hooks, nil
}

// buildPipeline creates an extraction pipeline from config. Returns nil if no LLM key.
func buildPipeline(cfg *config.Config, store *storage.SQLiteStore, gitSrv *gitserver.Server, logger *slog.Logger) (model.Extractor, error) {
	llmAPIKey := os.Getenv("KINOKO_LLM_API_KEY")
	if llmAPIKey == "" {
		llmAPIKey = os.Getenv("OPENAI_API_KEY")
	}
	if llmAPIKey == "" {
		return nil, nil
	}

	embCfg := embedding.DefaultConfig()
	embCfg.APIKey = os.Getenv("KINOKO_EMBEDDING_API_KEY")
	if embCfg.APIKey == "" {
		embCfg.APIKey = os.Getenv("OPENAI_API_KEY")
	}
	embedder := embedding.New(embCfg, logger)

	llmClient := llm.NewOpenAIClient(llmAPIKey, "gpt-4o-mini")
	stage1 := extraction.NewStage1Filter(cfg.Extraction, logger)
	stage2 := extraction.NewStage2Scorer(embedder, storage.NewSkillQuerier(store), llmClient, cfg.Extraction, logger)
	stage3 := extraction.NewStage3Critic(llmClient, cfg.Extraction, logger)

	var committer model.SkillCommitter
	if gitSrv != nil {
		committer = gitserver.NewGitCommitter(gitserver.GitCommitterConfig{
			Server:  gitSrv,
			DataDir: cfg.Server.DataDir,
			Logger:  logger,
		})
	}

	pipeline, err := extraction.NewPipeline(extraction.PipelineConfig{
		Stage1:    stage1,
		Stage2:    stage2,
		Stage3:    stage3,
		Writer:    store,
		Sessions:  store,
		Embedder:  embedder,
		Reviewer:  store,
		Committer: committer,
		Log:       logger,
		Extractor: "worker-v1",
	})
	if err != nil {
		return nil, fmt.Errorf("build extraction pipeline: %w", err)
	}
	return pipeline, nil
}

// libraryIDs extracts library IDs from config.
func libraryIDs(cfg *config.Config) []string {
	ids := make([]string, len(cfg.Libraries))
	for i, lib := range cfg.Libraries {
		ids[i] = lib.Name
	}
	return ids
}

// startWorkerSystem creates queue, pool, scheduler and starts them.
func startWorkerSystem(
	ctx context.Context,
	cfg *config.Config,
	store *storage.SQLiteStore,
	gitSrv *gitserver.Server,
	logger *slog.Logger,
) (queue *worker.SQLiteQueue, pool worker.Pool, sched worker.Scheduler, err error) {
	workerCfg := worker.DefaultConfig()
	schedCfg := worker.DefaultSchedulerConfig()
	schedCfg.StaleClaimTimeout = workerCfg.StaleClaimTimeout

	queue = worker.NewSQLiteQueue(store, cfg.Server.DataDir, workerCfg, logger)

	pipeline, err := buildPipeline(cfg, store, gitSrv, logger)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("build pipeline: %w", err)
	}
	if pipeline == nil {
		return nil, nil, nil, fmt.Errorf("no LLM API key configured; workers require extraction pipeline")
	}

	getSession := func(ctx context.Context, id string) (*model.SessionRecord, error) {
		return store.GetSession(ctx, id)
	}

	pool = worker.NewPool(queue, pipeline, getSession, workerCfg, logger)
	if err := pool.Start(ctx); err != nil {
		return nil, nil, nil, fmt.Errorf("start pool: %w", err)
	}

	decayCfg := decayConfigFromYAML(cfg.Decay)
	decayRunner, err := decay.NewRunner(store, store, decayCfg, logger)
	if err != nil {
		pool.Stop(context.Background())
		return nil, nil, nil, fmt.Errorf("create decay runner: %w", err)
	}

	sched = worker.NewScheduler(queue, pool, decayRunner, libraryIDs(cfg), schedCfg, logger)
	if err := sched.Start(ctx); err != nil {
		pool.Stop(context.Background())
		return nil, nil, nil, fmt.Errorf("start scheduler: %w", err)
	}

	return queue, pool, sched, nil
}

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

	// Build session hooks (injection only — extraction is handled by 'kinoko run').
	hooks, err := buildSessionHooks(cfg, store, logger)
	if err != nil {
		return fmt.Errorf("build session hooks: %w", err)
	}

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
	if err := gitserver.InstallHooks(cfg.Server.DataDir, kinokoBin); err != nil {
		logger.Warn("failed to install git hooks", "error", err)
	} else {
		logger.Info("git hooks installed", "data_dir", cfg.Server.DataDir)
	}

	// Register hooks with the git server.
	server.SetSessionHooks(hooks.OnSessionStart, hooks.OnSessionEnd)

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
	var apiSrv *api.Server
	if embCfgAPI.APIKey != "" {
		apiEmbedder := embedding.New(embCfgAPI, logger)
		apiPort := cfg.Server.GetAPIPort()
		apiSrv = api.New(api.Config{
			Host:     cfg.Server.Host,
			Port:     apiPort,
			Store:    store,
			Embedder: apiEmbedder,
			SSHURL:   connInfo.SSHUrl,
			Logger:   logger,
			Enqueue: func(ctx context.Context, session model.SessionRecord, logContent []byte) error {
				// Ingestion via API enqueues to the store; 'kinoko run' picks it up.
				return nil
			},
		})
		if err := apiSrv.Start(); err != nil {
			logger.Error("failed to start API server", "error", err)
		} else {
			logger.Info("API server ready", "port", apiPort)
		}
	} else {
		logger.Warn("API server disabled: no embedding API key")
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
	_ = shutdownCtx

	if apiSrv != nil {
		apiSrv.Stop(context.Background())
	}

	logger.Info("Stopping git server...")
	if err := server.Stop(); err != nil {
		logger.Error("Error stopping git server", "error", err)
		return err
	}

	logger.Info("Kinoko server stopped successfully")
	return nil
}
