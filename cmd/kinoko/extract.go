// extract.go implements the "kinoko extract" CLI command — a client-side
// one-shot tool that runs the extraction pipeline on a single session log.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/kinoko-dev/kinoko/internal/config"
	"github.com/kinoko-dev/kinoko/internal/debug"
	"github.com/kinoko-dev/kinoko/internal/extraction"
	"github.com/kinoko-dev/kinoko/internal/llm"
	"github.com/kinoko-dev/kinoko/internal/model"
	"github.com/kinoko-dev/kinoko/internal/serverclient"
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
	extractTimeout    time.Duration
)

func init() {
	extractCmd.Flags().StringVar(&extractConfigPath, "config", "", "Config file path")
	extractCmd.Flags().StringVar(&extractLibrary, "library", "", "Library ID (default: first configured library)")
	extractCmd.Flags().StringVar(&extractAPIURL, "api-url", "", "Kinoko API URL override")
	extractCmd.Flags().BoolVar(&extractDryRun, "dry-run", false, "Run pipeline but skip git push")
	extractCmd.Flags().DurationVar(&extractTimeout, "timeout", 5*time.Minute, "Command timeout")
}

func runExtract(cmd *cobra.Command, args []string) error {
	parentCtx := cmd.Context()
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(parentCtx, extractTimeout)
	defer cancel()

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

	// Server client for embeddings, skill querying, sessions, reviews.
	serverURL := cfg.ServerURL()
	if extractAPIURL != "" {
		serverURL = extractAPIURL
	}
	serverClient := serverclient.New(serverURL)

	// Embedder via server HTTP API.
	embedder := serverclient.NewHTTPEmbedder(serverClient, cfg.Embedding.GetDims())

	// Skill querier via server HTTP API.
	querier := serverclient.NewHTTPQuerier(serverClient)

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
	stage2 := extraction.NewStage2Scorer(embedder, querier, llmClient, cfg.Extraction, logger)
	stage3 := extraction.NewStage3Critic(llmClient, cfg.Extraction, logger)

	// Novelty checker via server API.
	threshold := cfg.Embedding.GetNoveltyThreshold()
	novelty := extraction.NewNoveltyClient(serverURL, threshold, logger)

	// Git committer via SSH push.
	sshURL := fmt.Sprintf("ssh://%s:%d", cfg.Server.Host, cfg.Server.Port)
	committer := serverclient.NewGitPushCommitter(sshURL, cfg.Server.DataDir, logger)

	// Session writer and reviewer via server HTTP API.
	sessions := serverclient.NewHTTPSessionWriter(serverClient)
	reviewer := serverclient.NewHTTPReviewer(serverClient)

	// Debug tracer from config (nil if debug is disabled).
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
		Extractor:  "cli-extract-v1",
	})
	if err != nil {
		return fmt.Errorf("create pipeline: %w", err)
	}

	result, err := pipeline.Extract(ctx, session, content)
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
		switch {
		case dryRun:
			fmt.Println("  Pushed:  no (dry-run)")
		case result.CommitHash != "":
			fmt.Printf("  Pushed:  yes (%s)\n", result.CommitHash)
		default:
			fmt.Println("  Pushed:  no")
		}
	case model.StatusRejected:
		switch {
		case result.Stage1 != nil && !result.Stage1.Passed:
			fmt.Printf("  Rejected at: Stage 1 — %s\n", result.Stage1.Reason)
		case result.Stage2 != nil && !result.Stage2.Passed:
			fmt.Printf("  Rejected at: Stage 2 — %s\n", result.Stage2.Reason)
		case result.Stage3 != nil && !result.Stage3.Passed:
			fmt.Printf("  Rejected at: Stage 3 — %s\n", result.Stage3.CriticReasoning)
		}
	case model.StatusError:
		fmt.Printf("  Error:   %s\n", result.Error)
	}
	fmt.Printf("  Duration: %dms\n", result.DurationMs)
	fmt.Println("──────────────────────────")
}

// exitError signals a non-zero exit code without calling os.Exit directly.
type exitError struct {
	code int
	msg  string
}

func (e *exitError) Error() string { return e.msg }
func (e *exitError) ExitCode() int { return e.code }
