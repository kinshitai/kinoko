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

// noOpSessionWriter implements extraction.SessionWriter by doing nothing.
// Sessions are now tracked via git commits, not separate records.
type noOpSessionWriter struct{}

func (w *noOpSessionWriter) InsertSession(ctx context.Context, session *model.SessionRecord) error {
	// No-op: sessions are tracked via git, not separate HTTP records
	return nil
}

func (w *noOpSessionWriter) UpdateSessionResult(ctx context.Context, session *model.SessionRecord) error {
	// No-op: sessions are tracked via git, not separate HTTP records
	return nil
}

// localFileReviewWriter implements extraction.HumanReviewWriter by writing to local files.
// This keeps review samples local to the client, not sent to server.
type localFileReviewWriter struct {
	logger *slog.Logger
}

func (w *localFileReviewWriter) InsertReviewSample(ctx context.Context, sessionID string, resultJSON []byte) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		w.logger.Warn("failed to get home directory for reviews", "error", err)
		return nil // non-critical, don't fail the pipeline
	}

	reviewDir := homeDir + "/.kinoko/reviews"
	if err := os.MkdirAll(reviewDir, 0755); err != nil {
		w.logger.Warn("failed to create review directory", "dir", reviewDir, "error", err)
		return nil // non-critical, don't fail the pipeline
	}

	filePath := fmt.Sprintf("%s/%s.json", reviewDir, sessionID)
	if err := os.WriteFile(filePath, resultJSON, 0644); err != nil {
		w.logger.Warn("failed to write review sample", "file", filePath, "error", err)
		return nil // non-critical, don't fail the pipeline
	}

	w.logger.Info("wrote human review sample", "session_id", sessionID, "file", filePath)
	return nil
}

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

	rec, err := extraction.ParseSession(bytes.NewReader(content))
	if err != nil {
		return fmt.Errorf("parse session log: %w", err)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("generate session ID: %w", err)
	}
	rec.ID = id.String()
	rec.LibraryID = libraryID
	rec.LogPath = logPath
	session := *rec

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Server URL for novelty checking.
	serverURL := cfg.ServerURL()
	if extractAPIURL != "" {
		serverURL = extractAPIURL
	}

	// Initialize LLM client — Stage2 and Stage3 need it.
	creds, err := llm.ResolveCredentials(cfg.LLM)
	if err != nil {
		return fmt.Errorf("LLM credentials: %w", err)
	}

	// Use config model if set, otherwise use the model from credentials
	llmModel := cfg.LLM.Model
	if llmModel == "" {
		llmModel = creds.Model
	}
	llmClient, err := llm.NewClient(creds.Provider, creds.APIKey, llmModel, creds.BaseURL)
	if err != nil {
		return fmt.Errorf("create LLM client: %w", err)
	}

	// Build pipeline stages
	stage1 := extraction.NewStage1Filter(cfg.Extraction, logger)
	stage2 := extraction.NewStage2Scorer(llmClient, cfg.Extraction, logger)
	stage3 := extraction.NewStage3Critic(llmClient, cfg.Extraction, logger)

	// Novelty checker via server API.
	threshold := cfg.Embedding.GetNoveltyThreshold()
	novelty := extraction.NewNoveltyClient(serverURL, threshold, logger)

	// Git committer via SSH push.
	sshURL := fmt.Sprintf("ssh://%s:%d", cfg.Server.Host, cfg.Server.Port)
	committer := apiclient.NewGitPushCommitter(sshURL, cfg.Server.DataDir, cfg.Client.GetSSHKeyPath(), logger)

	// Session writer and reviewer are client-local concerns (not server endpoints)
	// Sessions are now tracked via git commits; reviews stay local
	var sessions extraction.SessionWriter = &noOpSessionWriter{}
	var reviewer extraction.HumanReviewWriter = &localFileReviewWriter{logger: logger}

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
		DryRun:     extractDryRun,
	})
	if err != nil {
		return fmt.Errorf("create pipeline: %w", err)
	}

	result, err := pipeline.Extract(ctx, session, content)
	if err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	// Print human-readable summary to stderr.
	printExtractionSummary(result, "", extractDryRun)

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

// exitError signals a non-zero exit code without calling os.Exit directly.
type exitError struct {
	code int
	msg  string
}

func (e *exitError) Error() string { return e.msg }
func (e *exitError) ExitCode() int { return e.code }
