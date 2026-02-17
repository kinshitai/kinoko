package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kinoko-dev/kinoko/internal/model"
)

// GetSession retrieves a session record by ID.
func (s *SQLiteStore) GetSession(ctx context.Context, id string) (*model.SessionRecord, error) {
	var sr model.SessionRecord
	var extractedSkillID sql.NullString
	var status string
	var logContentPath, lastError, claimedBy string
	var retryCount int
	var nextRetryAt, claimedAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT id, started_at, ended_at, duration_minutes, tool_call_count, error_count,
			message_count, error_rate, has_successful_exec, tokens_used, agent_model,
			user_id, library_id, extraction_status, rejected_at_stage, rejection_reason,
			extracted_skill_id, log_content_path, retry_count, last_error, next_retry_at,
			claimed_by, claimed_at
		FROM sessions WHERE id = ?`, id).Scan(
		&sr.ID, &sr.StartedAt, &sr.EndedAt, &sr.DurationMinutes, &sr.ToolCallCount,
		&sr.ErrorCount, &sr.MessageCount, &sr.ErrorRate, &sr.HasSuccessfulExec,
		&sr.TokensUsed, &sr.AgentModel, &sr.UserID, &sr.LibraryID, &status,
		&sr.RejectedAtStage, &sr.RejectionReason, &extractedSkillID,
		&logContentPath, &retryCount, &lastError, &nextRetryAt, &claimedBy, &claimedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get session %s: %w", id, err)
	}
	sr.ExtractionStatus = model.ExtractionStatus(status)
	if extractedSkillID.Valid {
		sr.ExtractedSkillID = extractedSkillID.String
	}
	sr.LogPath = logContentPath
	return &sr, nil
}

// InsertSession inserts a session record into the sessions table.
func (s *SQLiteStore) InsertSession(ctx context.Context, session *model.SessionRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (
			id, started_at, ended_at, duration_minutes, tool_call_count, error_count,
			message_count, error_rate, has_successful_exec, tokens_used, agent_model,
			user_id, library_id, extraction_status, rejected_at_stage, rejection_reason,
			extracted_skill_id, log_content_path
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		session.ID, session.StartedAt, session.EndedAt, session.DurationMinutes,
		session.ToolCallCount, session.ErrorCount, session.MessageCount, session.ErrorRate,
		session.HasSuccessfulExec, session.TokensUsed, session.AgentModel,
		session.UserID, session.LibraryID, string(session.ExtractionStatus),
		session.RejectedAtStage, session.RejectionReason,
		nullString(session.ExtractedSkillID), session.LogPath,
	)
	if err != nil {
		return fmt.Errorf("insert session %s: %w", session.ID, err)
	}
	return nil
}

// UpdateSessionResult updates extraction results on an existing session row.
func (s *SQLiteStore) UpdateSessionResult(ctx context.Context, session *model.SessionRecord) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE sessions SET
			extraction_status = ?,
			rejected_at_stage = ?,
			rejection_reason = ?,
			extracted_skill_id = ?
		WHERE id = ?`,
		string(session.ExtractionStatus),
		session.RejectedAtStage,
		session.RejectionReason,
		nullString(session.ExtractedSkillID),
		session.ID,
	)
	if err != nil {
		return fmt.Errorf("update session %s result: %w", session.ID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update session %s rows affected: %w", session.ID, err)
	}
	if n == 0 {
		return model.ErrNotFound
	}
	return nil
}
