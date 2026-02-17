// queuecmd.go implements the "kinoko queue" command — a client-side CLI tool
// for inspecting and managing the local extraction work queue.

package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kinoko-dev/kinoko/internal/config"
	"github.com/kinoko-dev/kinoko/internal/queue"
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

var queueFlushCmd = &cobra.Command{
	Use:   "flush",
	Short: "Delete all queued sessions from the queue",
	Long:  `Removes all sessions with status = 'queued'. Use --force to skip confirmation.`,
	RunE:  runQueueFlush,
}

var (
	queueConfigPath string
	queueFlushForce bool
)

func init() {
	queueCmd.PersistentFlags().StringVar(&queueConfigPath, "config", "", "Config file path")
	queueFlushCmd.Flags().BoolVar(&queueFlushForce, "force", false, "Skip confirmation prompt")
	queueCmd.AddCommand(queueStatsCmd)
	queueCmd.AddCommand(queueListCmd)
	queueCmd.AddCommand(queueRetryCmd)
	queueCmd.AddCommand(queueFlushCmd)
}

func openQueueStore() (*queue.Store, error) {
	cfg, err := config.Load(queueConfigPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	queueDSN := cfg.Client.GetQueueDSN()
	store, err := queue.New(queueDSN)
	if err != nil {
		return nil, fmt.Errorf("open queue store: %w", err)
	}
	return store, nil
}

func runQueueStats(cmd *cobra.Command, args []string) error {
	store, err := openQueueStore()
	if err != nil {
		return err
	}
	defer store.Close()

	db := store.DB()
	rows, err := db.QueryContext(cmd.Context(), `
		SELECT status, COUNT(*) 
		FROM queue_entries 
		GROUP BY status 
		ORDER BY status`)
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
	store, err := openQueueStore()
	if err != nil {
		return err
	}
	defer store.Close()

	db := store.DB()
	rows, err := db.QueryContext(cmd.Context(), `
		SELECT session_id, status, retry_count, COALESCE(last_error, ''), COALESCE(claimed_by, '')
		FROM queue_entries
		WHERE status IN ('queued', 'pending', 'error')
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

	store, err := openQueueStore()
	if err != nil {
		return err
	}
	defer store.Close()

	db := store.DB()
	result, err := db.ExecContext(cmd.Context(), `
		UPDATE queue_entries SET
			status = 'queued',
			retry_count = 0,
			last_error = NULL,
			next_retry_at = NULL,
			claimed_by = '',
			claimed_at = NULL
		WHERE session_id = ? AND status IN ('error', 'failed')`, sessionID)
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

func runQueueFlush(cmd *cobra.Command, args []string) error {
	store, err := openQueueStore()
	if err != nil {
		return err
	}
	defer store.Close()

	db := store.DB()

	// Count queued entries first.
	var count int
	if err := db.QueryRowContext(cmd.Context(), `SELECT COUNT(*) FROM queue_entries WHERE status = 'queued'`).Scan(&count); err != nil {
		return fmt.Errorf("count queued: %w", err)
	}
	if count == 0 {
		fmt.Println("No queued sessions to flush.")
		return nil
	}

	if !queueFlushForce {
		fmt.Printf("This will delete %d queued session(s). Continue? [y/N] ", count)
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	result, err := db.ExecContext(cmd.Context(), `DELETE FROM queue_entries WHERE status = 'queued'`)
	if err != nil {
		return fmt.Errorf("flush: %w", err)
	}
	n, _ := result.RowsAffected()
	fmt.Printf("Flushed %d queued session(s).\n", n)
	return nil
}
