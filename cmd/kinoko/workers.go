package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

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
)

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
// If no LLM API key is configured, it starts in degraded mode: scheduler +
// injection only, no extraction workers.
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
		// P0-3: Degraded mode — no LLM key, skip extraction workers.
		logger.Warn("No LLM API key — extraction disabled, running scheduler and injection only.")

		decayCfg := decayConfigFromYAML(cfg.Decay)
		decayRunner, err := decay.NewRunner(store, store, decayCfg, logger)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("create decay runner: %w", err)
		}

		sched = worker.NewScheduler(queue, nil, decayRunner, libraryIDs(cfg), schedCfg, logger)
		if err := sched.Start(ctx); err != nil {
			return nil, nil, nil, fmt.Errorf("start scheduler: %w", err)
		}

		return queue, nil, sched, nil
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

// SessionHooks holds callbacks for session lifecycle events.
type SessionHooks struct {
	OnSessionStart func(ctx context.Context, req model.InjectionRequest) (*model.InjectionResponse, error)
	OnSessionEnd   func(ctx context.Context, session model.SessionRecord, logContent []byte) (*model.ExtractionResult, error)
}

// buildSessionHooks wires the injection pipeline into session lifecycle hooks.
// This is a run-side concern — serve should not call this.
func buildSessionHooks(cfg *config.Config, store *storage.SQLiteStore, logger *slog.Logger) (*SessionHooks, error) {
	hooks := &SessionHooks{}

	embCfg := embedding.DefaultConfig()
	embCfg.APIKey = os.Getenv("KINOKO_EMBEDDING_API_KEY")
	if embCfg.APIKey == "" {
		embCfg.APIKey = os.Getenv("OPENAI_API_KEY")
	}

	llmAPIKey := os.Getenv("KINOKO_LLM_API_KEY")
	if llmAPIKey == "" {
		llmAPIKey = os.Getenv("OPENAI_API_KEY")
	}

	if embCfg.APIKey != "" {
		embedder := embedding.New(embCfg, logger)
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

	// Extraction hook is a no-op here — extraction happens via worker pool.
	hooks.OnSessionEnd = func(_ context.Context, _ model.SessionRecord, _ []byte) (*model.ExtractionResult, error) {
		return &model.ExtractionResult{Status: model.StatusRejected}, nil
	}

	return hooks, nil
}
