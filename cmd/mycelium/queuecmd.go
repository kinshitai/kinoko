package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/mycelium-dev/mycelium/internal/config"
	"github.com/mycelium-dev/mycelium/internal/storage"
)

var queueCmd = &cobra.Command{
	Use:   "queue",
	Short: "Queue inspection and management",
}

var queueStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Print queue depth and status counts",
	RunE:  runQueueStats,
}

var queueListCmd = &cobra.Command{
	Use:   "list",
	Short: "List queued, pending, and errored sessions",
	RunE:  runQueueList,
}

var queueRetryCmd = &cobra.Command{
	Use:   "retry <session-id>",
	Short: "Requeue a failed session",
	Args:  cobra.ExactArgs(1),
	RunE:  runQueueRetry,
}

var queueConfigPath string

func init() {
	queueCmd.PersistentFlags().StringVar(&queueConfigPath, "config", "", "Config file path")
	queueCmd.AddCommand(queueStatsCmd)
	queueCmd.AddCommand(queueListCmd)
	queueCmd.AddCommand(queueRetryCmd)
}

func openStoreForQueue() (*storage.SQLiteStore, error) {
	cfg, err := config.Load(queueConfigPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	store, err := storage.NewSQLiteStore(cfg.Storage.DSN, "")
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	return store, nil
}

func runQueueStats(cmd *cobra.Command, args []string) error {
	store, err := openStoreForQueue()
	if err != nil {
		return err
	}
	defer store.Close()

	db := store.DB()
	rows, err := db.QueryContext(cmd.Context(), `
		SELECT extraction_status, COUNT(*) 
		FROM sessions 
		GROUP BY extraction_status 
		ORDER BY extraction_status`)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	total := 0
	fmt.Println("Queue Status:")
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return err
		}
		fmt.Printf("  %-12s %d\n", status, count)
		total += count
	}
	fmt.Printf("  %-12s %d\n", "total", total)
	return rows.Err()
}

func runQueueList(cmd *cobra.Command, args []string) error {
	store, err := openStoreForQueue()
	if err != nil {
		return err
	}
	defer store.Close()

	db := store.DB()
	rows, err := db.QueryContext(cmd.Context(), `
		SELECT id, extraction_status, retry_count, COALESCE(last_error, ''), COALESCE(claimed_by, '')
		FROM sessions
		WHERE extraction_status IN ('queued', 'pending', 'error')
		ORDER BY created_at ASC
		LIMIT 20`)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	fmt.Printf("%-38s %-10s %-7s %-12s %s\n", "ID", "STATUS", "RETRIES", "CLAIMED_BY", "LAST_ERROR")
	for rows.Next() {
		var id, status, lastErr, claimedBy string
		var retries int
		if err := rows.Scan(&id, &status, &retries, &lastErr, &claimedBy); err != nil {
			return err
		}
		if len(lastErr) > 40 {
			lastErr = lastErr[:40] + "..."
		}
		fmt.Printf("%-38s %-10s %-7d %-12s %s\n", id, status, retries, claimedBy, lastErr)
	}
	return rows.Err()
}

func runQueueRetry(cmd *cobra.Command, args []string) error {
	sessionID := args[0]

	store, err := openStoreForQueue()
	if err != nil {
		return err
	}
	defer store.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	_ = logger

	db := store.DB()
	result, err := db.ExecContext(cmd.Context(), `
		UPDATE sessions SET
			extraction_status = 'queued',
			retry_count = 0,
			last_error = NULL,
			next_retry_at = NULL,
			claimed_by = '',
			claimed_at = NULL
		WHERE id = ? AND extraction_status IN ('error', 'failed')`, sessionID)
	if err != nil {
		return fmt.Errorf("requeue: %w", err)
	}

	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("session %s not found or not in error/failed state", sessionID)
	}
	fmt.Printf("Requeued session %s\n", sessionID)
	return nil
}
