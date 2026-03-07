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
  kinoko convert guide.md --min-quality 0.7 --taxonomy "BUILD/Backend/APIDesign"`,
	Args: cobra.ExactArgs(1),
	RunE: runConvert,
}

var (
	convertConfigPath string
	convertLibrary    string
	convertAPIURL     string
	convertDryRun     bool
	convertMinQuality float64
	convertTaxonomy   string
	convertTimeout    time.Duration
)

func init() {
	convertCmd.Flags().StringVar(&convertConfigPath, "config", "", "Config file path")
	convertCmd.Flags().StringVar(&convertLibrary, "library", "", "Library ID (default: first configured library)")
	convertCmd.Flags().StringVar(&convertAPIURL, "api-url", "", "Kinoko API URL override")
	convertCmd.Flags().BoolVar(&convertDryRun, "dry-run", false, "Run pipeline but skip git commit")
	convertCmd.Flags().Float64Var(&convertMinQuality, "min-quality", 0, "Minimum composite quality score (default: 0.65)")
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

	// min-quality flag — logged for visibility but quality gating is handled
	// by the pipeline's rubric scoring (MinimumViable check in Stage 2).
	if convertMinQuality > 0 {
		cfg.Extraction.MinConfidence = convertMinQuality
	}

	libraryID := convertLibrary
	if libraryID == "" && len(cfg.Libraries) > 0 {
		libraryID = cfg.Libraries[0].Name
	}

	// Prepend taxonomy hint to content if provided.
	pipelineContent := content
	if convertTaxonomy != "" {
		hint := fmt.Sprintf("Suggested taxonomy pattern: %s. Use this as a hint but override if content clearly fits elsewhere.\n\n", convertTaxonomy)
		pipelineContent = append([]byte(hint), content...)
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
	})
	if err != nil {
		return fmt.Errorf("create pipeline: %w", err)
	}

	result, err := pipeline.ConvertExtract(ctx, session, pipelineContent)
	if err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}

	// Print summary.
	printConvertSummary(result, filePath, convertDryRun)

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

// printConvertSummary prints a human-readable conversion summary.
func printConvertSummary(result *model.ExtractionResult, sourcePath string, dryRun bool) {
	fmt.Println("─── Convert Summary ───")
	fmt.Printf("  Source:   %s\n", sourcePath)
	fmt.Printf("  Status:   %s\n", result.Status)

	// Print Stage 2 rubric scores if available.
	if result.Stage2 != nil {
		s := result.Stage2.RubricScores
		fmt.Println()
		fmt.Println("  Stage 2 Scores:")
		fmt.Printf("    problem_specificity:    %d\n", s.ProblemSpecificity)
		fmt.Printf("    solution_completeness:  %d\n", s.SolutionCompleteness)
		fmt.Printf("    context_portability:    %d\n", s.ContextPortability)
		fmt.Printf("    reasoning_transparency: %d\n", s.ReasoningTransparency)
		fmt.Printf("    technical_accuracy:     %d\n", s.TechnicalAccuracy)
		fmt.Printf("    verification_evidence:  %d\n", s.VerificationEvidence)
		fmt.Printf("    innovation_level:       %d\n", s.InnovationLevel)
		fmt.Printf("    composite:              %.2f\n", s.CompositeScore)
	}

	// Print Stage 3 verdict if available.
	if result.Stage3 != nil {
		fmt.Println()
		fmt.Printf("  Stage 3 Verdict: %s\n", result.Stage3.CriticVerdict)
		fmt.Printf("    confidence: %.2f\n", result.Stage3.RefinedScores.CriticConfidence)
		fmt.Printf("    reasoning: %s\n", result.Stage3.CriticReasoning)
	}

	switch result.Status {
	case model.StatusExtracted:
		if result.Skill != nil {
			fmt.Println()
			fmt.Printf("  Skill:    %s\n", result.Skill.Name)
			fmt.Printf("  Version:  %d\n", result.Skill.Version)
			fmt.Printf("  Quality:  %.2f\n", result.Skill.Quality.CompositeScore)
		}
		switch {
		case dryRun:
			fmt.Println("  Committed: no (dry-run)")
		case result.CommitHash != "":
			fmt.Printf("  Committed: yes (%s)\n", result.CommitHash)
		default:
			fmt.Println("  Committed: no")
		}
	case model.StatusRejected:
		switch {
		case result.Stage2 != nil && !result.Stage2.Passed:
			fmt.Printf("  Rejected at: Stage 2 — %s\n", result.Stage2.Reason)
		case result.Stage3 != nil && !result.Stage3.Passed:
			fmt.Printf("  Rejected at: Stage 3 — %s\n", result.Stage3.CriticReasoning)
		}
	case model.StatusError:
		fmt.Printf("  Error:    %s\n", result.Error)
	}

	fmt.Printf("  Duration: %dms\n", result.DurationMs)
	fmt.Println("───────────────────────")
}
