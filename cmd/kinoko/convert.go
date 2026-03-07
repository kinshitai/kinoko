package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/kinoko-dev/kinoko/internal/run/apiclient"
	"github.com/kinoko-dev/kinoko/internal/run/debug"
	"github.com/kinoko-dev/kinoko/internal/run/extraction"
	"github.com/kinoko-dev/kinoko/internal/run/llm"
	"github.com/kinoko-dev/kinoko/internal/shared/config"
	"github.com/kinoko-dev/kinoko/pkg/model"
)

var convertCmd = &cobra.Command{
	Use:   "convert <file>",
	Short: "Convert existing documents into SKILL.md format",
	Long: `Reads a document file, runs it through the extraction pipeline (skipping
session filtering), and commits the resulting SKILL.md to the skill repo.

The document is evaluated for reusable knowledge using genre-aware prompts
that don't penalize the absence of tool calls or execution traces.

  kinoko convert my-notes.md
  kinoko convert CLAUDE.md --dry-run
  kinoko convert guide.md --taxonomy "BUILD/Backend/APIDesign"`,
	Args: cobra.ExactArgs(1),
	RunE: runConvert,
}

var (
	convertConfigPath string
	convertLibrary    string
	convertAPIURL     string
	convertDryRun     bool
	convertTaxonomy   string
	convertTimeout    time.Duration
)

func init() {
	convertCmd.Flags().StringVar(&convertConfigPath, "config", "", "Config file path")
	convertCmd.Flags().StringVar(&convertLibrary, "library", "", "Library ID (default: first configured library)")
	convertCmd.Flags().StringVar(&convertAPIURL, "api-url", "", "Kinoko API URL override")
	convertCmd.Flags().BoolVar(&convertDryRun, "dry-run", false, "Run pipeline but skip git commit")
	convertCmd.Flags().StringVar(&convertTaxonomy, "taxonomy", "", "Suggested taxonomy pattern hint (e.g. BUILD/Backend/APIDesign)")
	convertCmd.Flags().DurationVar(&convertTimeout, "timeout", 5*time.Minute, "Command timeout")
}

func runConvert(cmd *cobra.Command, args []string) error {
	parentCtx := cmd.Context()
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(parentCtx, convertTimeout)
	defer cancel()

	filePath := args[0]

	fi, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}
	const maxFileSize = 10 * 1024 * 1024 // 10MB
	if fi.Size() > maxFileSize {
		return fmt.Errorf("file too large: %d bytes (max %d)", fi.Size(), maxFileSize)
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	if len(content) == 0 {
		return fmt.Errorf("file is empty: %s", filePath)
	}

	cfg, err := config.Load(convertConfigPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	libraryID := convertLibrary
	if libraryID == "" && len(cfg.Libraries) > 0 {
		libraryID = cfg.Libraries[0].Name
	}

	// Create a minimal SessionRecord for the pipeline.
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("generate session ID: %w", err)
	}
	session := model.SessionRecord{
		ID:           id.String(),
		LibraryID:    libraryID,
		LogPath:      filePath,
		MessageCount: 1,
	}

	// Try to parse session metadata if the file happens to be a session log.
	// This is best-effort; convert works with any text file.
	if rec, parseErr := extraction.ParseSession(bytes.NewReader(content)); parseErr == nil {
		session.DurationMinutes = rec.DurationMinutes
		session.ToolCallCount = rec.ToolCallCount
		session.ErrorRate = rec.ErrorRate
		session.HasSuccessfulExec = rec.HasSuccessfulExec
		session.MessageCount = rec.MessageCount
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Server URL for novelty checking.
	serverURL := cfg.ServerURL()
	if convertAPIURL != "" {
		serverURL = convertAPIURL
	}

	// Initialize LLM client.
	creds, err := llm.ResolveCredentials(cfg.LLM)
	if err != nil {
		return fmt.Errorf("LLM credentials: %w", err)
	}

	llmModel := cfg.LLM.Model
	if llmModel == "" {
		llmModel = creds.Model
	}
	llmClient, err := llm.NewClient(creds.Provider, creds.APIKey, llmModel, creds.BaseURL)
	if err != nil {
		return fmt.Errorf("create LLM client: %w", err)
	}

	// Build pipeline stages.
	stage1 := extraction.NewStage1Filter(cfg.Extraction, logger)
	stage2 := extraction.NewStage2Scorer(llmClient, cfg.Extraction, logger)
	stage3 := extraction.NewStage3Critic(llmClient, cfg.Extraction, logger)

	// Novelty checker via server API.
	threshold := cfg.Embedding.GetNoveltyThreshold()
	novelty := extraction.NewNoveltyClient(serverURL, threshold, logger)

	// Git committer via SSH push.
	sshURL := fmt.Sprintf("ssh://%s:%d", cfg.Server.Host, cfg.Server.Port)
	committer := apiclient.NewGitPushCommitter(sshURL, cfg.Server.DataDir, cfg.Client.GetSSHKeyPath(), logger)

	var sessions extraction.SessionWriter = &noOpSessionWriter{}
	var reviewer extraction.HumanReviewWriter = &localFileReviewWriter{logger: logger}

	// Debug tracer from config.
	var tracer *debug.Tracer
	if cfg.Debug.Enabled && cfg.Debug.Dir != "" {
		tracer = debug.NewTracer(cfg.Debug.Dir)
	}

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
		SampleRate: 0.01,
		Extractor:  "cli-convert-v1",
		ExtCfg:     cfg.Extraction,
		DryRun:     convertDryRun,
	})
	if err != nil {
		return fmt.Errorf("create pipeline: %w", err)
	}

	result, err := pipeline.ConvertExtract(ctx, session, content, convertTaxonomy)
	if err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}

	// Print summary to stderr.
	printExtractionSummary(result, filePath, convertDryRun)

	// Print JSON for programmatic consumption.
	out, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	fmt.Println(string(out))

	if result.Status == model.StatusRejected {
		return &exitError{code: 2, msg: "conversion rejected"}
	}
	if result.Status == model.StatusError {
		return &exitError{code: 3, msg: "conversion error"}
	}

	return nil
}
