package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/kinoko-dev/kinoko/internal/config"
	embpkg "github.com/kinoko-dev/kinoko/internal/embedding"
	"github.com/kinoko-dev/kinoko/internal/extraction"
	"github.com/kinoko-dev/kinoko/internal/llm"
	"github.com/kinoko-dev/kinoko/internal/model"
	"github.com/kinoko-dev/kinoko/internal/storage"
)

var extractCmd = &cobra.Command{
	Use:   "extract <session-log>",
	Short: "Run extraction pipeline on a session log file",
	Long:  `Reads a session log, parses metadata, runs the 3-stage extraction pipeline, and prints the result. For manual testing and debugging.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runExtract,
}

var (
	extractConfigPath string
	extractLibrary    string
	extractAPIURL     string
	extractDryRun     bool
)

func init() {
	extractCmd.Flags().StringVar(&extractConfigPath, "config", "", "Config file path")
	extractCmd.Flags().StringVar(&extractLibrary, "library", "", "Library ID (default: first configured library)")
	extractCmd.Flags().StringVar(&extractAPIURL, "api-url", "", "Kinoko API URL (default: $KINOKO_API_URL or http://127.0.0.1:23233)")
	extractCmd.Flags().BoolVar(&extractDryRun, "dry-run", false, "Run pipeline but skip git push")
}

func runExtract(cmd *cobra.Command, args []string) error {
	logPath := args[0]

	content, err := os.ReadFile(logPath)
	if err != nil {
		return fmt.Errorf("read session log: %w", err)
	}

	cfg, err := config.Load(extractConfigPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	libraryID := extractLibrary
	if libraryID == "" && len(cfg.Libraries) > 0 {
		libraryID = cfg.Libraries[0].Name
	}

	session := extraction.ParseSessionFromLog(content, libraryID)
	session.LogPath = logPath

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Initialize store
	store, err := storage.NewSQLiteStore(cfg.Storage.DSN, "")
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer store.Close()

	apiURL := firstNonEmpty(extractAPIURL, os.Getenv("KINOKO_API_URL"), "http://127.0.0.1:23233")

	// Initialize embedder — try API-key-based first, degrade gracefully.
	embCfg := embpkg.DefaultConfig()
	embCfg.APIKey = os.Getenv("KINOKO_EMBEDDING_API_KEY")
	if embCfg.APIKey == "" {
		embCfg.APIKey = os.Getenv("OPENAI_API_KEY")
	}
	var embedder embpkg.Embedder
	if embCfg.APIKey != "" {
		embedder = embpkg.New(embCfg, logger)
	} else {
		logger.Warn("no embedding API key set, Stage2 scoring will use server embed endpoint or be skipped")
		// Use HTTP embedder via server API as fallback.
		embedder = &httpEmbedder{apiURL: apiURL, logger: logger}
	}

	// Initialize LLM client — Stage2 and Stage3 need it.
	llmAPIKey := cfg.LLM.APIKey
	if llmAPIKey == "" {
		llmAPIKey = os.Getenv("KINOKO_LLM_API_KEY")
	}
	if llmAPIKey == "" {
		llmAPIKey = os.Getenv("OPENAI_API_KEY")
	}
	if llmAPIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY or KINOKO_LLM_API_KEY required for extraction")
	}
	llmModel := cfg.LLM.Model
	if llmModel == "" {
		llmModel = "gpt-4o-mini"
	}
	llmClient, err := llm.NewClient(cfg.LLM.Provider, llmAPIKey, llmModel, cfg.LLM.BaseURL)
	if err != nil {
		return fmt.Errorf("create LLM client: %w", err)
	}

	// Build pipeline stages
	stage1 := extraction.NewStage1Filter(cfg.Extraction, logger)
	stage2 := extraction.NewStage2Scorer(embedder, storage.NewSkillQuerier(store), llmClient, cfg.Extraction, logger)
	stage3 := extraction.NewStage3Critic(llmClient, cfg.Extraction, logger)

	// Novelty checker via server API.
	threshold := cfg.Embedding.GetNoveltyThreshold()
	novelty := extraction.NewNoveltyClient(apiURL, threshold, logger)

	// Git pusher — skip if --dry-run.
	var pusher extraction.SkillPusher
	if !extractDryRun {
		serverAddr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
		home, _ := os.UserHomeDir()
		keyPath := filepath.Join(home, ".kinoko", "id_ed25519")
		if p, err := extraction.NewGitPusher(serverAddr, keyPath, logger); err != nil {
			logger.Warn("git pusher unavailable, skills will not be pushed", "error", err)
		} else {
			pusher = p
		}
	}

	pipeline, err := extraction.NewPipeline(extraction.PipelineConfig{
		Stage1:     stage1,
		Stage2:     stage2,
		Stage3:     stage3,
		Writer:     store,
		Sessions:   store,
		Embedder:   embedder,
		Reviewer:   store,
		Novelty:    novelty,
		Pusher:     pusher,
		Log:        logger,
		SampleRate: 0.01,
		Extractor:  "cli-extract-v1",
	})
	if err != nil {
		return fmt.Errorf("create pipeline: %w", err)
	}

	result, err := pipeline.Extract(cmd.Context(), session, content)
	if err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	// Print human-readable summary.
	printExtractSummary(result, extractDryRun)

	// Also print JSON for programmatic consumption.
	out, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	fmt.Println(string(out))

	if result.Status == model.StatusRejected {
		return &exitError{code: 2, msg: "extraction rejected"}
	}
	if result.Status == model.StatusError {
		return &exitError{code: 3, msg: "extraction error"}
	}

	return nil
}

// printExtractSummary prints a human-readable extraction summary.
func printExtractSummary(result *model.ExtractionResult, dryRun bool) {
	fmt.Println("─── Extraction Summary ───")
	fmt.Printf("  Status:  %s\n", result.Status)

	switch result.Status {
	case model.StatusExtracted:
		if result.Skill != nil {
			fmt.Printf("  Skill:   %s\n", result.Skill.Name)
			fmt.Printf("  Version: %d\n", result.Skill.Version)
			fmt.Printf("  Quality: %.2f\n", result.Skill.Quality.CompositeScore)
		}
		if dryRun {
			fmt.Println("  Pushed:  no (dry-run)")
		} else if result.CommitHash != "" {
			fmt.Printf("  Pushed:  yes (%s)\n", result.CommitHash)
		} else {
			fmt.Println("  Pushed:  no")
		}
	case model.StatusRejected:
		if result.Stage1 != nil && !result.Stage1.Passed {
			fmt.Printf("  Rejected at: Stage 1 — %s\n", result.Stage1.Reason)
		} else if result.Stage2 != nil && !result.Stage2.Passed {
			fmt.Printf("  Rejected at: Stage 2 — %s\n", result.Stage2.Reason)
		} else if result.Stage3 != nil && !result.Stage3.Passed {
			fmt.Printf("  Rejected at: Stage 3 — %s\n", result.Stage3.CriticReasoning)
		}
	case model.StatusError:
		fmt.Printf("  Error:   %s\n", result.Error)
	}
	fmt.Printf("  Duration: %dms\n", result.DurationMs)
	fmt.Println("──────────────────────────")
}

// httpEmbedder implements embedding.Embedder via the server's /api/v1/embed endpoint.
type httpEmbedder struct {
	apiURL string
	logger *slog.Logger
}

func (e *httpEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	vec, err := fetchEmbedding(e.apiURL, text)
	if err != nil {
		e.logger.Warn("HTTP embedding failed", "error", err)
		return nil, err
	}
	return vec, nil
}

func (e *httpEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, t := range texts {
		vec, err := e.Embed(ctx, t)
		if err != nil {
			return nil, err
		}
		results[i] = vec
	}
	return results, nil
}

func (e *httpEmbedder) Dimensions() int {
	return 1536 // Default OpenAI text-embedding-3-small dimensions.
}

// exitError signals a non-zero exit code without calling os.Exit directly.
type exitError struct {
	code int
	msg  string
}

func (e *exitError) Error() string { return e.msg }
func (e *exitError) ExitCode() int { return e.code }
