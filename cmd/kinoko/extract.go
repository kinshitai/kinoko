package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/kinoko-dev/kinoko/internal/config"
	"github.com/kinoko-dev/kinoko/internal/embedding"
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
)

func init() {
	extractCmd.Flags().StringVar(&extractConfigPath, "config", "", "Config file path")
	extractCmd.Flags().StringVar(&extractLibrary, "library", "", "Library ID (default: first configured library)")
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

	// Initialize embedder
	embCfg := embedding.DefaultConfig()
	embCfg.APIKey = os.Getenv("KINOKO_EMBEDDING_API_KEY")
	if embCfg.APIKey == "" {
		embCfg.APIKey = os.Getenv("OPENAI_API_KEY")
	}
	embedder := embedding.New(embCfg, logger)

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

	pipeline, err := extraction.NewPipeline(extraction.PipelineConfig{
		Stage1:     stage1,
		Stage2:     stage2,
		Stage3:     stage3,
		Writer:     store,
		Sessions:   store,
		Embedder:   embedder,
		Reviewer:   store,
		Log:        logger,
		SampleRate: 0.01, // 1% sampling per spec
		Extractor:  "cli-extract-v1",
	})
	if err != nil {
		return fmt.Errorf("create pipeline: %w", err)
	}

	result, err := pipeline.Extract(cmd.Context(), session, content)
	if err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	// Print result as JSON
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

// exitError signals a non-zero exit code without calling os.Exit directly.
type exitError struct {
	code int
	msg  string
}

func (e *exitError) Error() string { return e.msg }
func (e *exitError) ExitCode() int { return e.code }
