package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/mycelium-dev/mycelium/internal/config"
	"github.com/mycelium-dev/mycelium/internal/storage"
	"github.com/mycelium-dev/mycelium/internal/worker"
)

var importCmd = &cobra.Command{
	Use:   "import [path...]",
	Short: "Import session logs into the extraction queue",
	Long: `Parse session log files and enqueue them for extraction.
Does NOT start workers — sessions wait for 'serve' or 'worker' to process them.

  mycelium import session.log
  mycelium import --dir ./logs/
  mycelium import a.log b.log c.log`,
	RunE: runImport,
}

var (
	importConfigPath string
	importLibrary    string
	importDir        string
)

func init() {
	importCmd.Flags().StringVar(&importConfigPath, "config", "", "Config file path")
	importCmd.Flags().StringVar(&importLibrary, "library", "", "Library ID (default: first configured library)")
	importCmd.Flags().StringVar(&importDir, "dir", "", "Directory of log files to import")
}

func runImport(cmd *cobra.Command, args []string) error {
	if len(args) == 0 && importDir == "" {
		return fmt.Errorf("provide log file paths or --dir")
	}

	cfg, err := config.Load(importConfigPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	libraryID := importLibrary
	if libraryID == "" && len(cfg.Libraries) > 0 {
		libraryID = cfg.Libraries[0].Name
	}
	if libraryID == "" {
		return fmt.Errorf("no library specified and none configured")
	}

	if err := os.MkdirAll(cfg.Server.DataDir, 0755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	store, err := storage.NewSQLiteStore(cfg.Storage.DSN, "")
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer store.Close()

	workerCfg := worker.DefaultConfig()
	queue := worker.NewSQLiteQueue(store, cfg.Server.DataDir, workerCfg, logger)

	// Collect file paths.
	var paths []string
	paths = append(paths, args...)
	if importDir != "" {
		entries, err := os.ReadDir(importDir)
		if err != nil {
			return fmt.Errorf("read dir %s: %w", importDir, err)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if strings.HasSuffix(e.Name(), ".log") || strings.HasSuffix(e.Name(), ".txt") || strings.HasSuffix(e.Name(), ".json") {
				paths = append(paths, filepath.Join(importDir, e.Name()))
			}
		}
	}

	enqueued := 0
	errCount := 0
	for _, p := range paths {
		content, err := os.ReadFile(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %s: %v\n", p, err)
			errCount++
			continue
		}

		session := parseSessionFromLog(content, libraryID)
		if err := queue.Enqueue(cmd.Context(), session, content); err != nil {
			fmt.Fprintf(os.Stderr, "enqueue %s: %v\n", p, err)
			errCount++
			continue
		}
		enqueued++
	}

	fmt.Printf("Enqueued: %d\n", enqueued)
	if errCount > 0 {
		fmt.Printf("Errors:   %d\n", errCount)
	}
	return nil
}
