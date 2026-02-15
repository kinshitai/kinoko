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
	"github.com/mycelium-dev/mycelium/internal/embedding"
	"github.com/mycelium-dev/mycelium/internal/extraction"
	"github.com/mycelium-dev/mycelium/internal/gitserver"
	"github.com/mycelium-dev/mycelium/internal/injection"
	"github.com/mycelium-dev/mycelium/internal/storage"
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

	// Wire extraction hook (post-session).
	if llmAPIKey != "" {
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
			Extractor: "serve-auto-v1",
		})
		if err != nil {
			return nil, fmt.Errorf("build extraction pipeline: %w", err)
		}
		hooks.OnSessionEnd = func(ctx context.Context, session extraction.SessionRecord, logContent []byte) (*extraction.ExtractionResult, error) {
			result, err := pipeline.Extract(ctx, session, logContent)
			if err != nil {
				logger.Error("extraction failed", "session_id", session.ID, "error", err)
				return nil, err
			}
			logger.Info("extraction complete", "session_id", session.ID, "status", result.Status)
			return result, nil
		}
		logger.Info("extraction hook registered")
	} else {
		hooks.OnSessionEnd = func(_ context.Context, _ extraction.SessionRecord, _ []byte) (*extraction.ExtractionResult, error) {
			return &extraction.ExtractionResult{Status: extraction.StatusRejected}, nil
		}
		logger.Warn("extraction hook disabled: no LLM API key")
	}

	return hooks, nil
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

	return waitForShutdown(cmd.Context(), server, logger)
}

// waitForShutdown waits for shutdown signal and gracefully stops the server
func waitForShutdown(ctx context.Context, server *gitserver.Server, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
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
			logger.Info("Received shutdown signal", "signal", sig)
		case <-ctx.Done():
			logger.Info("Context cancelled")
		}
		close(done)
		cancel()
	}()

	logger.Info("Mycelium is ready. Use Ctrl+C to shutdown gracefully.")
	logger.Info("Agents can now git clone, push, and pull over SSH")

	// Wait for shutdown signal or context cancellation
	select {
	case <-done:
		// Signal received, gracefully shut down
	case <-ctx.Done():
		// Context cancelled
	}

	logger.Info("Shutting down git server...")
	if err := server.Stop(); err != nil {
		logger.Error("Error stopping git server", "error", err)
		return err
	}

	logger.Info("Mycelium serve stopped successfully")
	return nil
}
