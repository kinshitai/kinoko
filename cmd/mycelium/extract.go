package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/mycelium-dev/mycelium/internal/config"
	"github.com/mycelium-dev/mycelium/internal/embedding"
	"github.com/mycelium-dev/mycelium/internal/extraction"
	"github.com/mycelium-dev/mycelium/internal/storage"
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

	session := parseSessionFromLog(content, libraryID)
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
	embCfg.APIKey = os.Getenv("MYCELIUM_EMBEDDING_API_KEY")
	if embCfg.APIKey == "" {
		embCfg.APIKey = os.Getenv("OPENAI_API_KEY")
	}
	embedder := embedding.New(embCfg, logger)

	// Initialize LLM client — Stage2 and Stage3 need it.
	// For CLI extract, we use the embedder's API key with a simple LLM shim.
	llmAPIKey := os.Getenv("MYCELIUM_LLM_API_KEY")
	if llmAPIKey == "" {
		llmAPIKey = os.Getenv("OPENAI_API_KEY")
	}
	if llmAPIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY or MYCELIUM_LLM_API_KEY required for extraction")
	}
	llm := &openAILLMClient{apiKey: llmAPIKey, model: "gpt-4o-mini"}

	// Build pipeline stages
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
		SampleRate: 0.01, // 1% sampling per spec
		Extractor: "cli-extract-v1",
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

	if result.Status == extraction.StatusRejected {
		return &exitError{code: 2, msg: "extraction rejected"}
	}
	if result.Status == extraction.StatusError {
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

// parseSessionFromLog extracts metadata from a session log file.
// Looks for common patterns: timestamps, tool calls, errors, model info.
func parseSessionFromLog(content []byte, libraryID string) extraction.SessionRecord {
	lines := strings.Split(string(content), "\n")

	session := extraction.SessionRecord{
		ID:        uuid.Must(uuid.NewV7()).String(),
		LibraryID: libraryID,
	}

	var timestamps []time.Time
	toolCalls := 0
	errorCount := 0
	msgCount := len(lines)
	hasExec := false

	// Patterns for common log formats
	tsPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2})`),
	}
	toolPattern := regexp.MustCompile(`(tool_call|function_call|<tool_use>|<invoke|"type"\s*:\s*"function")`)
	errorPattern := regexp.MustCompile(`((?:^|\s)error[:\s=]|(?:^|\s)ERROR[:\s=]|traceback \(most recent|panic:|fatal:|FAILED|exit status [1-9])`)
	execPattern := regexp.MustCompile(`(tool_call.*exec|<exec|command_output|shell_exec|"name"\s*:\s*"exec")`)
	modelPattern := regexp.MustCompile(`(?i)model[=: ]+([a-zA-Z0-9._-]+)`)

	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, len(buf))

	for scanner.Scan() {
		line := scanner.Text()

		for _, pat := range tsPatterns {
			if m := pat.FindString(line); m != "" {
				for _, layout := range []string{
					"2006-01-02T15:04:05",
					"2006-01-02 15:04:05",
				} {
					if t, err := time.Parse(layout, m); err == nil {
						timestamps = append(timestamps, t)
						break
					}
				}
			}
		}

		if toolPattern.MatchString(line) {
			toolCalls++
		}
		if errorPattern.MatchString(line) {
			errorCount++
		}
		if execPattern.MatchString(line) {
			hasExec = true
		}
		if m := modelPattern.FindStringSubmatch(line); len(m) > 1 && session.AgentModel == "" {
			session.AgentModel = m[1]
		}
	}

	now := time.Now()
	if len(timestamps) >= 2 {
		session.StartedAt = timestamps[0]
		session.EndedAt = timestamps[len(timestamps)-1]
	} else {
		session.StartedAt = now.Add(-10 * time.Minute)
		session.EndedAt = now
	}

	session.DurationMinutes = session.EndedAt.Sub(session.StartedAt).Minutes()
	if session.DurationMinutes < 0 {
		session.DurationMinutes = 0
	}

	session.ToolCallCount = toolCalls
	session.ErrorCount = errorCount
	session.MessageCount = msgCount
	session.HasSuccessfulExec = hasExec

	if session.ToolCallCount > 0 {
		session.ErrorRate = float64(session.ErrorCount) / float64(session.ToolCallCount)
	}

	session.TokensUsed = estimateTokens(content)

	return session
}

func estimateTokens(content []byte) int {
	// Rough estimate: ~4 chars per token
	return len(content) / 4
}

// storeQuerier adapts storage.SQLiteStore to extraction.SkillQuerier.
type storeQuerier struct {
	store *storage.SQLiteStore
}

func (sq *storeQuerier) QueryNearest(ctx context.Context, emb []float32, libraryID string) (*extraction.SkillQueryResult, error) {
	results, err := sq.store.Query(ctx, storage.SkillQuery{
		Embedding:  emb,
		LibraryIDs: []string{libraryID},
		Limit:      1,
	})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return &extraction.SkillQueryResult{CosineSim: results[0].CosineSim}, nil
}

// openAILLMClient is a minimal LLM client for CLI use.
type openAILLMClient struct {
	apiKey string
	model  string
}

func (c *openAILLMClient) Complete(ctx context.Context, prompt string) (string, error) {
	// Use the same HTTP approach as the embedding client
	return openAIComplete(ctx, c.apiKey, c.model, prompt)
}

func openAIComplete(ctx context.Context, apiKey, model, prompt string) (string, error) {
	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens": 2048,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := newHTTPRequest(ctx, "POST", "https://api.openai.com/v1/chat/completions", data)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body := make([]byte, 512)
		n, _ := resp.Body.Read(body)
		return "", fmt.Errorf("openai API %d: %s", resp.StatusCode, string(body[:n]))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	return result.Choices[0].Message.Content, nil
}

