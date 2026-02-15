package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/mycelium-dev/mycelium/internal/config"
	"github.com/mycelium-dev/mycelium/internal/decay"
	"github.com/mycelium-dev/mycelium/internal/embedding"
	"github.com/mycelium-dev/mycelium/internal/extraction"
	"github.com/mycelium-dev/mycelium/internal/gitserver"
	"github.com/mycelium-dev/mycelium/internal/injection"
	"github.com/mycelium-dev/mycelium/internal/storage"
	"github.com/mycelium-dev/mycelium/internal/worker"
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

// SessionHooks holds callbacks for session lifecycle events.
type SessionHooks struct {
	// OnSessionStart is called when a new agent session begins.
	// It runs the injection pipeline to select relevant skills.
	OnSessionStart func(ctx context.Context, req extraction.InjectionRequest) (*extraction.InjectionResponse, error)

	// OnSessionEnd is called when an agent session completes.
	// It runs the extraction pipeline on the session log.
	OnSessionEnd func(ctx context.Context, session extraction.SessionRecord, logContent []byte) (*extraction.ExtractionResult, error)
}

// buildSessionHooks wires the extraction and injection pipelines into session
// lifecycle hooks. Returns hooks ready for registration with the server.
func buildSessionHooks(cfg *config.Config, store *storage.SQLiteStore, logger *slog.Logger) (*SessionHooks, error) {
	hooks := &SessionHooks{}

	// Embedding client (shared by both pipelines).
	embCfg := embedding.DefaultConfig()
	embCfg.APIKey = os.Getenv("MYCELIUM_EMBEDDING_API_KEY")
	if embCfg.APIKey == "" {
		embCfg.APIKey = os.Getenv("OPENAI_API_KEY")
	}
	embedder := embedding.New(embCfg, logger)

	// LLM client for extraction stages.
	llmAPIKey := os.Getenv("MYCELIUM_LLM_API_KEY")
	if llmAPIKey == "" {
		llmAPIKey = os.Getenv("OPENAI_API_KEY")
	}

	// Wire injection hook (pre-session).
	if embCfg.APIKey != "" {
		var llmClient extraction.LLMClient
		if llmAPIKey != "" {
			llmClient = &openAILLMClient{apiKey: llmAPIKey, model: "gpt-4o-mini"}
		}

		// When A/B testing is enabled, ABInjector writes events (with group info).
		// Base injector gets nil eventWriter to prevent double-writing.
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

		hooks.OnSessionStart = func(ctx context.Context, req extraction.InjectionRequest) (*extraction.InjectionResponse, error) {
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
		hooks.OnSessionStart = func(_ context.Context, _ extraction.InjectionRequest) (*extraction.InjectionResponse, error) {
			return &extraction.InjectionResponse{}, nil
		}
		logger.Warn("injection hook disabled: no embedding API key")
	}

	// Wire extraction hook (post-session) — enqueue instead of synchronous extraction.
	// The queue parameter is injected by the caller (nil-safe: if nil, hook is a no-op).
	hooks.OnSessionEnd = func(_ context.Context, _ extraction.SessionRecord, _ []byte) (*extraction.ExtractionResult, error) {
		return &extraction.ExtractionResult{Status: extraction.StatusRejected}, nil
	}
	logger.Info("extraction hook: enqueue mode (set via setEnqueueHook)")

	return hooks, nil
}

// buildPipeline creates an extraction pipeline from config. Returns nil if no LLM key.
func buildPipeline(cfg *config.Config, store *storage.SQLiteStore, logger *slog.Logger) (extraction.Extractor, error) {
	llmAPIKey := os.Getenv("MYCELIUM_LLM_API_KEY")
	if llmAPIKey == "" {
		llmAPIKey = os.Getenv("OPENAI_API_KEY")
	}
	if llmAPIKey == "" {
		return nil, nil
	}

	embCfg := embedding.DefaultConfig()
	embCfg.APIKey = os.Getenv("MYCELIUM_EMBEDDING_API_KEY")
	if embCfg.APIKey == "" {
		embCfg.APIKey = os.Getenv("OPENAI_API_KEY")
	}
	embedder := embedding.New(embCfg, logger)

	llm := &openAILLMClient{apiKey: llmAPIKey, model: "gpt-4o-mini"}
	stage1 := extraction.NewStage1Filter(cfg.Extraction, logger)
	stage2 := extraction.NewStage2Scorer(embedder, &storeQuerier{store: store}, llm, cfg.Extraction, logger)
	stage3 := extraction.NewStage3Critic(llm, cfg.Extraction, logger)
	pipeline, err := extraction.NewPipeline(extraction.PipelineConfig{
		Stage1:    stage1,
		Stage2:    stage2,
		Stage3:    stage3,
		Writer:    store,
		Sessions:  store,
		Embedder:  embedder,
		Reviewer:  store,
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
// Returns cleanup function for graceful shutdown.
func startWorkerSystem(
	ctx context.Context,
	cfg *config.Config,
	store *storage.SQLiteStore,
	logger *slog.Logger,
) (queue *worker.SQLiteQueue, pool worker.Pool, sched worker.Scheduler, err error) {
	workerCfg := worker.DefaultConfig()
	schedCfg := worker.DefaultSchedulerConfig()

	queue = worker.NewSQLiteQueue(store, cfg.Server.DataDir, workerCfg, logger)

	pipeline, err := buildPipeline(cfg, store, logger)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("build pipeline: %w", err)
	}
	if pipeline == nil {
		return nil, nil, nil, fmt.Errorf("no LLM API key configured; workers require extraction pipeline")
	}

	getSession := func(ctx context.Context, id string) (*extraction.SessionRecord, error) {
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
	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Create data directory if it doesn't exist
	if err := os.MkdirAll(cfg.Server.DataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory %s: %w", cfg.Server.DataDir, err)
	}

	logger := slog.Default()
	logger.Info("Mycelium serve command started")
	logger.Info("Configuration loaded successfully",
		"host", cfg.Server.Host,
		"port", cfg.Server.Port,
		"dataDir", cfg.Server.DataDir,
		"storageDriver", cfg.Storage.Driver,
		"libraries", len(cfg.Libraries))

	// Open store for hooks.
	store, err := storage.NewSQLiteStore(cfg.Storage.DSN, "")
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer store.Close()

	// Build and register session hooks.
	hooks, err := buildSessionHooks(cfg, store, logger)
	if err != nil {
		return fmt.Errorf("build session hooks: %w", err)
	}

	// Create and start the git server
	server, err := gitserver.NewServer(cfg)
	if err != nil {
		return fmt.Errorf("failed to create git server: %w", err)
	}

	// Start worker system (queue + pool + scheduler).
	queue, pool, sched, err := startWorkerSystem(cmd.Context(), cfg, store, logger)
	if err != nil {
		logger.Warn("worker system not started, extraction will be disabled", "error", err)
	}

	// Replace synchronous extraction hook with async enqueue.
	if queue != nil {
		hooks.OnSessionEnd = func(ctx context.Context, session extraction.SessionRecord, logContent []byte) (*extraction.ExtractionResult, error) {
			if err := queue.Enqueue(ctx, session, logContent); err != nil {
				logger.Error("enqueue failed", "session_id", session.ID, "error", err)
				return nil, err
			}
			logger.Info("session enqueued", "session_id", session.ID)
			return &extraction.ExtractionResult{Status: extraction.StatusQueued}, nil
		}
	}

	// Register hooks with the git server for session lifecycle events.
	server.SetSessionHooks(hooks.OnSessionStart, hooks.OnSessionEnd)

	if err := server.Start(); err != nil {
		return fmt.Errorf("failed to start git server: %w", err)
	}

	// Get connection info for logging
	connInfo := server.GetConnectionInfo()
	logger.Info("Mycelium git server is ready",
		"ssh_url", connInfo.SSHUrl,
		"host", connInfo.SSHHost,
		"port", connInfo.SSHPort)

	return waitForShutdown(cmd.Context(), server, sched, pool, store, logger)
}

// waitForShutdown waits for shutdown signal and gracefully stops all components.
// Shutdown order: scheduler → pool → git server → store.
func waitForShutdown(ctx context.Context, server *gitserver.Server, sched worker.Scheduler, pool worker.Pool, store *storage.SQLiteStore, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(ctx)
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

	logger.Info("Mycelium is ready. Use Ctrl+C to shutdown gracefully.")
	logger.Info("Agents can now git clone, push, and pull over SSH")

	select {
	case <-done:
	case <-ctx.Done():
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// 1. Stop scheduler first (no new sweeps/decay).
	if sched != nil {
		logger.Info("Stopping scheduler...")
		if err := sched.Stop(shutdownCtx); err != nil {
			logger.Error("Error stopping scheduler", "error", err)
		}
	}

	// 2. Stop pool (drain in-flight workers).
	if pool != nil {
		logger.Info("Stopping worker pool...")
		if err := pool.Stop(shutdownCtx); err != nil {
			logger.Error("Error stopping worker pool", "error", err)
		}
	}

	// 3. Stop git server.
	logger.Info("Stopping git server...")
	if err := server.Stop(); err != nil {
		logger.Error("Error stopping git server", "error", err)
		return err
	}

	// 4. Store is closed by deferred store.Close() in runServe.
	logger.Info("Mycelium serve stopped successfully")
	return nil
}
