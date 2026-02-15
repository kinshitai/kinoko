package worker

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/mycelium-dev/mycelium/internal/extraction"
	"github.com/mycelium-dev/mycelium/internal/storage"
)

// ErrBackpressure is returned when the queue depth exceeds the critical threshold.
var ErrBackpressure = fmt.Errorf("queue backpressure: depth exceeds critical threshold")

// SessionQueue manages the extraction work queue.
type SessionQueue interface {
	Enqueue(ctx context.Context, session extraction.SessionRecord, logContent []byte) error
	Claim(ctx context.Context, workerID string) (*QueueEntry, error)
	Complete(ctx context.Context, sessionID string, result *extraction.ExtractionResult) error
	Fail(ctx context.Context, sessionID string, err error) error
	FailPermanent(ctx context.Context, sessionID string, err error) error
	Depth(ctx context.Context) (int, error)
	RequeueStale(ctx context.Context, staleDuration time.Duration) (int, error)
}

// QueueEntry is returned by Claim.
type QueueEntry struct {
	SessionID      string
	LogContentPath string
	RetryCount     int
	LibraryID      string
}

// SQLiteQueue implements SessionQueue backed by SQLite.
type SQLiteQueue struct {
	store   *storage.SQLiteStore
	dataDir string
	cfg     Config
	log     *slog.Logger
}

// NewSQLiteQueue creates a new queue. dataDir is the base directory for log files.
func NewSQLiteQueue(store *storage.SQLiteStore, dataDir string, cfg Config, log *slog.Logger) *SQLiteQueue {
	return &SQLiteQueue{
		store:   store,
		dataDir: dataDir,
		cfg:     cfg,
		log:     log,
	}
}

func (q *SQLiteQueue) queueDir() string {
	return filepath.Join(q.dataDir, "queue")
}

func (q *SQLiteQueue) Enqueue(ctx context.Context, session extraction.SessionRecord, logContent []byte) error {
	// Check backpressure.
	depth, err := q.Depth(ctx)
	if err != nil {
		return fmt.Errorf("check depth: %w", err)
	}
	if depth >= q.cfg.QueueDepthCritical {
		return ErrBackpressure
	}
	if depth >= q.cfg.QueueDepthWarning {
		q.log.Warn("queue depth approaching critical", "depth", depth, "warning", q.cfg.QueueDepthWarning)
	}

	// Write log content to disk.
	dir := q.queueDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create queue dir: %w", err)
	}
	logPath := filepath.Join(dir, session.ID+".log")
	if err := os.WriteFile(logPath, logContent, 0o644); err != nil {
		return fmt.Errorf("write log file: %w", err)
	}

	// Insert session with StatusQueued.
	session.ExtractionStatus = extraction.StatusQueued
	session.LogPath = logPath

	db := q.store.DB()
	_, err = db.ExecContext(ctx, `
		INSERT INTO sessions (
			id, started_at, ended_at, duration_minutes, tool_call_count, error_count,
			message_count, error_rate, has_successful_exec, tokens_used, agent_model,
			user_id, library_id, extraction_status, rejected_at_stage, rejection_reason,
			extracted_skill_id, log_content_path
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		session.ID, session.StartedAt, session.EndedAt, session.DurationMinutes,
		session.ToolCallCount, session.ErrorCount, session.MessageCount, session.ErrorRate,
		session.HasSuccessfulExec, session.TokensUsed, session.AgentModel,
		session.UserID, session.LibraryID, string(extraction.StatusQueued),
		session.RejectedAtStage, session.RejectionReason,
		nil, logPath,
	)
	if err != nil {
		// Clean up log file on DB failure.
		os.Remove(logPath)
		return fmt.Errorf("insert session: %w", err)
	}

	return nil
}

func (q *SQLiteQueue) Claim(ctx context.Context, workerID string) (*QueueEntry, error) {
	db := q.store.DB()
	row := db.QueryRowContext(ctx, `
		UPDATE sessions
		SET extraction_status = 'pending',
		    claimed_by = ?,
		    claimed_at = CURRENT_TIMESTAMP
		WHERE id = (
			SELECT id FROM sessions
			WHERE (extraction_status = 'queued')
			   OR (extraction_status = 'error' AND next_retry_at <= CURRENT_TIMESTAMP)
			ORDER BY created_at ASC
			LIMIT 1
		)
		RETURNING id, log_content_path, retry_count, library_id`, workerID)

	var entry QueueEntry
	err := row.Scan(&entry.SessionID, &entry.LogContentPath, &entry.RetryCount, &entry.LibraryID)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("claim: %w", err)
	}
	return &entry, nil
}

func (q *SQLiteQueue) Complete(ctx context.Context, sessionID string, result *extraction.ExtractionResult) error {
	db := q.store.DB()
	status := extraction.StatusRejected
	if result.Status == extraction.StatusExtracted {
		status = extraction.StatusExtracted
	}

	var skillID interface{}
	if result.Skill != nil {
		skillID = result.Skill.ID
	}

	rejectedStage := 0
	rejectionReason := ""
	if result.Stage1 != nil && !result.Stage1.Passed {
		rejectedStage = 1
		rejectionReason = result.Stage1.Reason
	} else if result.Stage2 != nil && !result.Stage2.Passed {
		rejectedStage = 2
		rejectionReason = result.Stage2.Reason
	} else if result.Stage3 != nil && !result.Stage3.Passed {
		rejectedStage = 3
		rejectionReason = result.Stage3.CriticReasoning
	}

	_, err := db.ExecContext(ctx, `
		UPDATE sessions SET
			extraction_status = ?,
			rejected_at_stage = ?,
			rejection_reason = ?,
			extracted_skill_id = ?,
			claimed_by = '',
			claimed_at = NULL
		WHERE id = ?`,
		string(status), rejectedStage, rejectionReason, skillID, sessionID)
	if err != nil {
		return fmt.Errorf("complete session %s: %w", sessionID, err)
	}
	return nil
}

func (q *SQLiteQueue) Fail(ctx context.Context, sessionID string, failErr error) error {
	db := q.store.DB()
	errMsg := ""
	if failErr != nil {
		errMsg = failErr.Error()
	}

	// Exponential backoff: initial_backoff * 2^(retry_count)
	// retry_count is the current value BEFORE increment, so the delay for
	// the Nth retry (1-indexed) uses 2^(N-1).
	var retryCount int
	err := db.QueryRowContext(ctx, `SELECT retry_count FROM sessions WHERE id = ?`, sessionID).Scan(&retryCount)
	if err != nil {
		return fmt.Errorf("get retry count: %w", err)
	}

	backoff := q.cfg.InitialBackoff * (1 << retryCount)
	if backoff > q.cfg.MaxBackoff {
		backoff = q.cfg.MaxBackoff
	}
	nextRetry := time.Now().UTC().Add(backoff)

	_, err = db.ExecContext(ctx, `
		UPDATE sessions SET
			extraction_status = 'error',
			retry_count = retry_count + 1,
			last_error = ?,
			next_retry_at = ?,
			claimed_by = '',
			claimed_at = NULL
		WHERE id = ?`,
		errMsg, nextRetry, sessionID)
	if err != nil {
		return fmt.Errorf("fail session %s: %w", sessionID, err)
	}
	return nil
}

func (q *SQLiteQueue) FailPermanent(ctx context.Context, sessionID string, failErr error) error {
	db := q.store.DB()
	errMsg := ""
	if failErr != nil {
		errMsg = failErr.Error()
	}

	_, err := db.ExecContext(ctx, `
		UPDATE sessions SET
			extraction_status = 'failed',
			last_error = ?,
			claimed_by = '',
			claimed_at = NULL
		WHERE id = ?`,
		errMsg, sessionID)
	if err != nil {
		return fmt.Errorf("fail permanent session %s: %w", sessionID, err)
	}
	return nil
}

func (q *SQLiteQueue) Depth(ctx context.Context) (int, error) {
	db := q.store.DB()
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE extraction_status = 'queued'`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("depth: %w", err)
	}
	return count, nil
}

func (q *SQLiteQueue) RequeueStale(ctx context.Context, staleDuration time.Duration) (int, error) {
	db := q.store.DB()
	cutoff := time.Now().UTC().Add(-staleDuration)
	result, err := db.ExecContext(ctx, `
		UPDATE sessions SET
			extraction_status = 'queued',
			claimed_by = '',
			claimed_at = NULL
		WHERE extraction_status = 'pending'
		  AND claimed_at IS NOT NULL
		  AND claimed_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("requeue stale: %w", err)
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}
