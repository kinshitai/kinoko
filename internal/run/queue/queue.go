package queue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/kinoko-dev/kinoko/internal/run/worker"
	"github.com/kinoko-dev/kinoko/pkg/model"
)

// Compile-time interface check.
var _ worker.SessionQueue = (*Queue)(nil)

// Queue implements worker.SessionQueue backed by the queue Store.
type Queue struct {
	store   *Store
	dataDir string
	cfg     worker.Config
	log     *slog.Logger
}

// NewQueue creates a Queue. dataDir is the base directory for log files on disk.
func NewQueue(store *Store, dataDir string, cfg worker.Config, log *slog.Logger) *Queue {
	return &Queue{store: store, dataDir: dataDir, cfg: cfg, log: log}
}

func (q *Queue) queueDir() string {
	return filepath.Join(q.dataDir, "queue")
}

// Enqueue adds a session to the work queue. It writes the log content to disk
// and inserts both session_metadata and queue_entries rows inside one tx.
func (q *Queue) Enqueue(ctx context.Context, session model.SessionRecord, logContent []byte) error {
	dir := q.queueDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create queue dir: %w", err)
	}
	logPath := filepath.Join(dir, session.ID+".log")
	if err := os.WriteFile(logPath, logContent, 0o600); err != nil {
		return fmt.Errorf("write log file: %w", err)
	}

	db := q.store.DB()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		os.Remove(logPath)
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Backpressure check.
	var depth int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM queue_entries WHERE status = 'queued'`).Scan(&depth); err != nil {
		os.Remove(logPath)
		return fmt.Errorf("check depth: %w", err)
	}
	if depth >= q.cfg.QueueDepthCritical {
		os.Remove(logPath)
		return worker.ErrBackpressure
	}
	if depth >= q.cfg.QueueDepthWarning {
		q.log.Warn("queue depth approaching critical", "depth", depth, "warning", q.cfg.QueueDepthWarning)
	}

	// Insert session metadata.
	_, err = tx.ExecContext(ctx, `
		INSERT INTO session_metadata (
			session_id, started_at, ended_at, duration_minutes, tool_call_count,
			error_count, message_count, error_rate, has_successful_exec,
			tokens_used, agent_model, user_id, library_id
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		session.ID, session.StartedAt, session.EndedAt, session.DurationMinutes,
		session.ToolCallCount, session.ErrorCount, session.MessageCount, session.ErrorRate,
		session.HasSuccessfulExec, session.TokensUsed, session.AgentModel,
		session.UserID, session.LibraryID,
	)
	if err != nil {
		os.Remove(logPath)
		return fmt.Errorf("insert session_metadata: %w", err)
	}

	// Insert queue entry.
	_, err = tx.ExecContext(ctx, `
		INSERT INTO queue_entries (session_id, library_id, log_content_path, status)
		VALUES (?, ?, ?, 'queued')`,
		session.ID, session.LibraryID, logPath,
	)
	if err != nil {
		os.Remove(logPath)
		return fmt.Errorf("insert queue_entry: %w", err)
	}

	if err := tx.Commit(); err != nil {
		os.Remove(logPath)
		return fmt.Errorf("commit enqueue: %w", err)
	}
	return nil
}

// Claim atomically picks the next claimable entry.
func (q *Queue) Claim(ctx context.Context, workerID string) (*worker.QueueEntry, error) {
	db := q.store.DB()
	row := db.QueryRowContext(ctx, `
		UPDATE queue_entries
		SET status = 'pending',
		    claimed_by = ?,
		    claimed_at = CURRENT_TIMESTAMP
		WHERE session_id = (
			SELECT session_id FROM queue_entries
			WHERE (status = 'queued')
			   OR (status = 'error' AND next_retry_at <= CURRENT_TIMESTAMP)
			ORDER BY created_at ASC, rowid ASC
			LIMIT 1
		)
		RETURNING session_id, log_content_path, retry_count, library_id`, workerID)

	var entry worker.QueueEntry
	if err := row.Scan(&entry.SessionID, &entry.LogContentPath, &entry.RetryCount, &entry.LibraryID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("claim: %w", err)
	}
	return &entry, nil
}

// Complete marks a queue entry as done (extracted or rejected).
func (q *Queue) Complete(ctx context.Context, sessionID string, result *model.ExtractionResult) error {
	db := q.store.DB()
	status := "rejected"
	if result.Status == model.StatusExtracted {
		status = "extracted"
	}
	_, err := db.ExecContext(ctx, `
		UPDATE queue_entries SET
			status = ?,
			claimed_by = '',
			claimed_at = NULL
		WHERE session_id = ? AND status = 'pending'`,
		status, sessionID)
	if err != nil {
		return fmt.Errorf("complete session %s: %w", sessionID, err)
	}
	return nil
}

// Fail marks a queue entry as errored with exponential backoff retry.
func (q *Queue) Fail(ctx context.Context, sessionID string, failErr error) error {
	db := q.store.DB()
	errMsg := ""
	if failErr != nil {
		errMsg = failErr.Error()
	}
	initialSec := int64(q.cfg.InitialBackoff / time.Second)
	maxSec := int64(q.cfg.MaxBackoff / time.Second)

	_, err := db.ExecContext(ctx, `
		UPDATE queue_entries SET
			status = 'error',
			retry_count = retry_count + 1,
			last_error = ?,
			next_retry_at = datetime('now', '+' || CAST(MIN(? * (1 << retry_count), ?) AS TEXT) || ' seconds'),
			claimed_by = '',
			claimed_at = NULL
		WHERE session_id = ?
		  AND status = 'pending'`,
		errMsg, initialSec, maxSec, sessionID)
	if err != nil {
		return fmt.Errorf("fail session %s: %w", sessionID, err)
	}
	return nil
}

// FailPermanent marks a queue entry as permanently failed (no retry).
func (q *Queue) FailPermanent(ctx context.Context, sessionID string, failErr error) error {
	db := q.store.DB()
	errMsg := ""
	if failErr != nil {
		errMsg = failErr.Error()
	}
	_, err := db.ExecContext(ctx, `
		UPDATE queue_entries SET
			status = 'failed',
			last_error = ?,
			claimed_by = '',
			claimed_at = NULL
		WHERE session_id = ?
		  AND status = 'pending'`,
		errMsg, sessionID)
	if err != nil {
		return fmt.Errorf("fail permanent session %s: %w", sessionID, err)
	}
	return nil
}

// Depth returns the number of queued entries.
func (q *Queue) Depth(ctx context.Context) (int, error) {
	db := q.store.DB()
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM queue_entries WHERE status = 'queued'`).Scan(&count); err != nil {
		return 0, fmt.Errorf("depth: %w", err)
	}
	return count, nil
}

// RequeueStale resets entries that have been claimed too long back to queued.
func (q *Queue) RequeueStale(ctx context.Context, staleDuration time.Duration) (int, error) {
	db := q.store.DB()
	cutoff := time.Now().UTC().Add(-staleDuration)
	result, err := db.ExecContext(ctx, `
		UPDATE queue_entries SET
			status = 'queued',
			claimed_by = '',
			claimed_at = NULL
		WHERE status = 'pending'
		  AND claimed_at IS NOT NULL
		  AND claimed_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("requeue stale: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return int(n), nil
}
