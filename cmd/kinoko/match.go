package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/kinoko-dev/kinoko/internal/run/injection"
)

var matchCmd = &cobra.Command{
	Use:   "match <query-text>",
	Short: "Find skills matching a query",
	Long: `Query the Kinoko server for skills relevant to the given text.

  kinoko match "I need to fix a database timeout in Go"
  kinoko match --file context.txt --limit 10
  kinoko match "memory leak" --min-score 0.7`,
	RunE: runMatch,
}

var (
	matchAPIURL   string
	matchLimit    int
	matchMinScore float64
	matchFile     string
	matchTimeout  time.Duration
)

func init() {
	matchCmd.Flags().StringVar(&matchAPIURL, "api-url", "", "Kinoko API URL (default: $KINOKO_API_URL or http://127.0.0.1:23233)")
	matchCmd.Flags().IntVar(&matchLimit, "limit", 5, "Maximum number of results")
	matchCmd.Flags().Float64Var(&matchMinScore, "min-score", 0.5, "Minimum match score (0.0-1.0)")
	matchCmd.Flags().StringVar(&matchFile, "file", "", "Read query text from file instead of argument")
	matchCmd.Flags().DurationVar(&matchTimeout, "timeout", 30*time.Second, "Command timeout")
}

func runMatch(cmd *cobra.Command, args []string) error {
	var queryText string
	switch {
	case matchFile != "":
		data, err := os.ReadFile(matchFile)
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
		queryText = string(data)
	case len(args) > 0:
		queryText = args[0]
	default:
		return fmt.Errorf("provide query text as argument or use --file")
	}

	if queryText == "" {
		return fmt.Errorf("query text is empty")
	}

	parentCtx := cmd.Context()
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(parentCtx, matchTimeout)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	apiURL := firstNonEmpty(matchAPIURL, os.Getenv("KINOKO_API_URL"), "http://127.0.0.1:23233")

	client := injection.NewClient(apiURL, logger)
	result, err := client.MatchWithMinScore(ctx, queryText, matchLimit, matchMinScore)
	if err != nil {
		return fmt.Errorf("match failed: %w", err)
	}

	if len(result.Skills) == 0 {
		fmt.Println("No matching skills found.")
		return nil
	}

	fmt.Printf("─── Match Results (%d) ───\n", len(result.Skills))
	for i, s := range result.Skills {
		preview := s.Content
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		fmt.Printf("  %d. %s (score: %.3f)\n", i+1, s.Name, s.Score)
		if preview != "" {
			fmt.Printf("     %s\n", preview)
		}
	}
	fmt.Println("─────────────────────────")

	return nil
}
