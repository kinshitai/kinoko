// Package main implements the kinoko CLI — the primary entry point for the
// Kinoko knowledge-sharing infrastructure. Subcommands include serve (shared
// infrastructure server), run (local agent daemon), extract (single-session
// extraction), import (queue ingestion), and various management utilities.
package main

import (
	"log/slog"
	"os"
)

func main() {
	// Set up structured logging
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Execute the root command
	if err := rootCmd.Execute(); err != nil {
		slog.Error("command failed", "error", err)
		code := 1
		if ee, ok := err.(*exitError); ok {
			code = ee.ExitCode()
		}
		os.Exit(code)
	}
}
