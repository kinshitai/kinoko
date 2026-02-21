package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/kinoko-dev/kinoko/internal/run/debug"
	"github.com/kinoko-dev/kinoko/internal/run/extraction"
	"github.com/kinoko-dev/kinoko/internal/run/llm"
	"github.com/kinoko-dev/kinoko/internal/run/queue"
	"github.com/kinoko-dev/kinoko/internal/run/serverclient"
	"github.com/kinoko-dev/kinoko/internal/run/worker"
	"github.com/kinoko-dev/kinoko/internal/shared/config"
	"github.com/kinoko-dev/kinoko/pkg/model"
)

// libraryIDs extracts library IDs from config.
func libraryIDs(cfg *config.Config) []string {
	ids := make([]string, len(cfg.Libraries))
	for i, lib := range cfg.Libraries {
		ids[i] = lib.Name
	}
	return ids
}

// buildClientPipeline creates an extraction pipeline for kinoko run (client mode).
// Uses serverclient for embedding, skill querying, session writing, review, and git commit.
// Returns nil if no LLM key is configured.
func buildClientPipeline(cfg *config.Config, serverClient *serverclient.Client, logger *slog.Logger) (model.Extractor, error) {
	llmAPIKey := os.Getenv("KINOKO_LLM_API_KEY")
	if llmAPIKey == "" {
		llmAPIKey = os.Getenv("OPENAI_API_KEY")
	}
	if llmAPIKey == "" {
		return nil, nil
	}

	// Embedder via server HTTP API.
	embedder := serverclient.NewHTTPEmbedder(serverClient, cfg.Embedding.GetDims())

	// Skill querier via server HTTP API.
	querier := serverclient.NewHTTPQuerier(serverClient)

	llmModel := cfg.LLM.Model
	if llmModel == "" {
		llmModel = "gpt-4o-mini"
	}
	llmClient, err := llm.NewClient(cfg.LLM.Provider, llmAPIKey, llmModel, cfg.LLM.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("create LLM client: %w", err)
	}

	stage1 := extraction.NewStage1Filter(cfg.Extraction, logger)
	stage2 := extraction.NewStage2Scorer(embedder, querier, llmClient, cfg.Extraction, logger)
	stage3 := extraction.NewStage3Critic(llmClient, cfg.Extraction, logger)

	// Git committer via SSH push.
	sshURL := fmt.Sprintf("ssh://%s:%d", cfg.Server.Host, cfg.Server.Port)
	committer := serverclient.NewGitPushCommitter(sshURL, cfg.Server.DataDir, logger)

	// Session writer and reviewer are client-local concerns (not server endpoints)
	// Sessions are now tracked via git commits; reviews stay local
	var sessions extraction.SessionWriter = &noOpSessionWriter{}
	var reviewer extraction.HumanReviewWriter = &localFileReviewWriter{logger: logger}

	// Debug tracer from config (nil if debug is disabled).
	var tracer *debug.Tracer
	if cfg.Debug.Enabled && cfg.Debug.Dir != "" {
		tracer = debug.NewTracer(cfg.Debug.Dir)
	}

	// Novelty checker via server API.
	threshold := cfg.Embedding.GetNoveltyThreshold()
	serverURL := cfg.ServerURL()
	novelty := extraction.NewNoveltyClient(serverURL, threshold, logger)

	pipeline, err := extraction.NewPipeline(extraction.PipelineConfig{
		Stage1:     stage1,
		Stage2:     stage2,
		Stage3:     stage3,
		Sessions:   sessions,
		Reviewer:   reviewer,
		Novelty:    novelty,
		Committer:  committer,
		Tracer:     tracer,
		Log:        logger,
		SampleRate: cfg.Extraction.SampleRate,
		Extractor:  "worker-v1",
		ExtCfg:     cfg.Extraction,
	})
	if err != nil {
		return nil, fmt.Errorf("build extraction pipeline: %w", err)
	}
	return pipeline, nil
}

// startClientWorkerSystem creates queue, pool, scheduler for kinoko run (client mode).
// Uses local queue DB and server HTTP client. Decay is nil (moves to serve in T7).
func startClientWorkerSystem(
	ctx context.Context,
	cfg *config.Config,
	queueStore *queue.Store,
	serverClient *serverclient.Client,
	logger *slog.Logger,
) (q *queue.Queue, pool worker.Pool, sched worker.Scheduler, err error) {
	workerCfg := worker.DefaultConfig()
	schedCfg := worker.DefaultSchedulerConfig()
	schedCfg.StaleClaimTimeout = workerCfg.StaleClaimTimeout

	q = queue.NewQueue(queueStore, cfg.Server.DataDir, workerCfg, logger)

	pipeline, err := buildClientPipeline(cfg, serverClient, logger)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("build pipeline: %w", err)
	}

	if pipeline == nil {
		// Degraded mode — no LLM key, skip extraction workers.
		logger.Warn("No LLM API key — extraction disabled, running scheduler only.")

		// No decay runner in client mode (decay moves to serve in T7).
		sched = worker.NewScheduler(q, nil, nil, libraryIDs(cfg), schedCfg, logger)
		if err := sched.Start(ctx); err != nil {
			return nil, nil, nil, fmt.Errorf("start scheduler: %w", err)
		}

		return q, nil, sched, nil
	}

	// Session getter reads from local queue DB.
	getSession := func(ctx context.Context, id string) (*model.SessionRecord, error) {
		meta, err := queue.GetSessionMetadata(ctx, queueStore, id)
		if err != nil {
			return nil, err
		}
		return &model.SessionRecord{
			ID:                meta.SessionID,
			StartedAt:         meta.StartedAt,
			EndedAt:           meta.EndedAt,
			DurationMinutes:   meta.DurationMinutes,
			ToolCallCount:     meta.ToolCallCount,
			ErrorCount:        meta.ErrorCount,
			MessageCount:      meta.MessageCount,
			ErrorRate:         meta.ErrorRate,
			HasSuccessfulExec: meta.HasSuccessfulExec,
			TokensUsed:        meta.TokensUsed,
			AgentModel:        meta.AgentModel,
			UserID:            meta.UserID,
			LibraryID:         meta.LibraryID,
		}, nil
	}

	pool = worker.NewPool(q, pipeline, getSession, workerCfg, logger)
	if err := pool.Start(ctx); err != nil {
		return nil, nil, nil, fmt.Errorf("start pool: %w", err)
	}

	// No decay runner in client mode (decay moves to serve in T7).
	sched = worker.NewScheduler(q, pool, nil, libraryIDs(cfg), schedCfg, logger)
	if err := sched.Start(ctx); err != nil {
		_ = pool.Stop(context.Background())
		return nil, nil, nil, fmt.Errorf("start scheduler: %w", err)
	}

	return q, pool, sched, nil
}
