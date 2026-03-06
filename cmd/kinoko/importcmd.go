package main

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/kinoko-dev/kinoko/internal/run/extraction"
	"github.com/kinoko-dev/kinoko/internal/run/queue"
	"github.com/kinoko-dev/kinoko/internal/run/worker"
	"github.com/kinoko-dev/kinoko/internal/shared/config"
)

var importCmd = &cobra.Command{
	Use:   "import [path...]",
	Short: "Import session logs into the extraction queue",
	Long: `Parse session log files and enqueue them for extraction.
Does NOT start workers — sessions wait for 'serve' or 'worker' to process them.

  kinoko import session.log
  kinoko import --dir ./logs/
  kinoko import a.log b.log c.log`,
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

	queueStore, err := queue.New(cfg.Client.GetQueueDSN())
	if err != nil {
		return fmt.Errorf("open queue: %w", err)
	}
	defer queueStore.Close()

	workerCfg := worker.DefaultConfig()
	queueImpl := queue.NewQueue(queueStore, cfg.Server.DataDir, workerCfg, logger)

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

	const maxFileSize int64 = 50 * 1024 * 1024 // 50 MB

	enqueued := 0
	errCount := 0
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %s: %v\n", p, err)
			errCount++
			continue
		}
		if fi.Size() > maxFileSize {
			fmt.Fprintf(os.Stderr, "skip %s: file too large (%d bytes, limit %d)\n", p, fi.Size(), maxFileSize)
			errCount++
			continue
		}
		content, err := os.ReadFile(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %s: %v\n", p, err)
			errCount++
			continue
		}

		rec, parseErr := extraction.ParseSession(bytes.NewReader(content))
		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "skip %s: parse error: %v\n", p, parseErr)
			errCount++
			continue
		}
		id, err := uuid.NewV7()
		if err != nil {
			fmt.Fprintf(os.Stderr, "skip %s: generate ID: %v\n", p, err)
			errCount++
			continue
		}
		rec.ID = id.String()
		rec.LibraryID = libraryID
		session := *rec
		if err := queueImpl.Enqueue(cmd.Context(), session, content); err != nil {
			fmt.Fprintf(os.Stderr, "enqueue %s: %v\n", p, err)
			errCount++
			continue
		}
		enqueued++
	}

	fmt.Printf("Enqueued: %d\n", enqueued)
	if errCount > 0 {
		fmt.Printf("Errors:   %d\n", errCount)
		return fmt.Errorf("%d file(s) failed to import", errCount)
	}
	return nil
}
