package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/mycelium-dev/mycelium/internal/model"
	"github.com/mycelium-dev/mycelium/internal/storage"
)

// ErrBackpressure is returned when the queue depth exceeds the critical threshold.
var ErrBackpressure = fmt.Errorf("queue backpressure: depth exceeds critical threshold")

// SessionQueue manages the extraction work queue.
type SessionQueue interface {
	Enqueue(ctx context.Context, session model.SessionRecord, logContent []byte) error
	Claim(ctx context.Context, workerID string) (*QueueEntry, error)
	Complete(ctx context.Context, sessionID string, result *model.ExtractionResult) error
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

func (q *SQLiteQueue) Enqueue(ctx context.Context, session model.SessionRecord, logContent []byte) error {
	// Write log content to disk first (outside transaction to avoid holding lock during I/O).
	dir := q.queueDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create queue dir: %w", err)
	}
	logPath := filepath.Join(dir, session.ID+".log")
	if err := os.WriteFile(logPath, logContent, 0o644); err != nil {
		return fmt.Errorf("write log file: %w", err)
	}

	session.ExtractionStatus = model.StatusQueued
	session.LogPath = logPath

	// Depth check + INSERT in a single IMMEDIATE transaction to prevent TOCTOU race.
	db := q.store.DB()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		os.Remove(logPath)
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var depth int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE extraction_status = 'queued'`).Scan(&depth); err != nil {
		os.Remove(logPath)
		return fmt.Errorf("check depth: %w", err)
	}
	if depth >= q.cfg.QueueDepthCritical {
		os.Remove(logPath)
		return ErrBackpressure
	}
	if depth >= q.cfg.QueueDepthWarning {
		q.log.Warn("queue depth approaching critical", "depth", depth, "warning", q.cfg.QueueDepthWarning)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO sessions (
			id, started_at, ended_at, duration_minutes, tool_call_count, error_count,
			message_count, error_rate, has_successful_exec, tokens_used, agent_model,
			user_id, library_id, extraction_status, rejected_at_stage, rejection_reason,
			extracted_skill_id, log_content_path
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		session.ID, session.StartedAt, session.EndedAt, session.DurationMinutes,
		session.ToolCallCount, session.ErrorCount, session.MessageCount, session.ErrorRate,
		session.HasSuccessfulExec, session.TokensUsed, session.AgentModel,
		session.UserID, session.LibraryID, string(model.StatusQueued),
		session.RejectedAtStage, session.RejectionReason,
		nil, logPath,
	)
	if err != nil {
		os.Remove(logPath)
		return fmt.Errorf("insert session: %w", err)
	}

	if err := tx.Commit(); err != nil {
		os.Remove(logPath)
		return fmt.Errorf("commit enqueue: %w", err)
	}
	return nil
}

// Claim atomically picks the next claimable session. The UPDATE...WHERE id=(SELECT...)
// is a single SQL statement; combined with SQLite's single-writer guarantee and
// busy_timeout, only one concurrent caller can succeed per row. The rowid tiebreaker
// ensures deterministic FIFO when created_at values collide.
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
			ORDER BY created_at ASC, rowid ASC
			LIMIT 1
		)
		RETURNING id, log_content_path, retry_count, library_id`, workerID)

	var entry QueueEntry
	if err := row.Scan(&entry.SessionID, &entry.LogContentPath, &entry.RetryCount, &entry.LibraryID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("claim: %w", err)
	}
	return &entry, nil
}

func (q *SQLiteQueue) Complete(ctx context.Context, sessionID string, result *model.ExtractionResult) error {
	db := q.store.DB()
	status := model.StatusRejected
	if result.Status == model.StatusExtracted {
		status = model.StatusExtracted
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

	// Single atomic UPDATE: compute backoff inline using current retry_count,
	// then increment. Backoff = initial_backoff_sec * 2^retry_count, capped at max.
	initialSec := int64(q.cfg.InitialBackoff / time.Second)
	maxSec := int64(q.cfg.MaxBackoff / time.Second)

	_, err := db.ExecContext(ctx, `
		UPDATE sessions SET
			extraction_status = 'error',
			retry_count = retry_count + 1,
			last_error = ?,
			next_retry_at = datetime('now', '+' || CAST(MIN(? * (1 << retry_count), ?) AS TEXT) || ' seconds'),
			claimed_by = '',
			claimed_at = NULL
		WHERE id = ?`,
		errMsg, initialSec, maxSec, sessionID)
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
