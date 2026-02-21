package queue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SessionMetadata holds the lightweight session info stored client-side.
type SessionMetadata struct {
	SessionID         string
	StartedAt         time.Time
	EndedAt           time.Time
	DurationMinutes   float64
	ToolCallCount     int
	ErrorCount        int
	MessageCount      int
	ErrorRate         float64
	HasSuccessfulExec bool
	TokensUsed        int
	AgentModel        string
	UserID            string
	LibraryID         string
}

// GetSessionMetadata retrieves session metadata by ID.
func GetSessionMetadata(ctx context.Context, store *Store, id string) (*SessionMetadata, error) {
	var m SessionMetadata
	err := store.DB().QueryRowContext(ctx, `
		SELECT session_id, started_at, ended_at, duration_minutes, tool_call_count,
			error_count, message_count, error_rate, has_successful_exec,
			tokens_used, agent_model, user_id, library_id
		FROM session_metadata WHERE session_id = ?`, id).Scan(
		&m.SessionID, &m.StartedAt, &m.EndedAt, &m.DurationMinutes,
		&m.ToolCallCount, &m.ErrorCount, &m.MessageCount, &m.ErrorRate,
		&m.HasSuccessfulExec, &m.TokensUsed, &m.AgentModel, &m.UserID, &m.LibraryID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("session metadata %s: not found", id)
		}
		return nil, fmt.Errorf("get session metadata %s: %w", id, err)
	}
	return &m, nil
}

// PutSessionMetadata inserts or replaces session metadata.
func PutSessionMetadata(ctx context.Context, store *Store, m *SessionMetadata) error {
	_, err := store.DB().ExecContext(ctx, `
		INSERT OR REPLACE INTO session_metadata (
			session_id, started_at, ended_at, duration_minutes, tool_call_count,
			error_count, message_count, error_rate, has_successful_exec,
			tokens_used, agent_model, user_id, library_id
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		m.SessionID, m.StartedAt, m.EndedAt, m.DurationMinutes,
		m.ToolCallCount, m.ErrorCount, m.MessageCount, m.ErrorRate,
		m.HasSuccessfulExec, m.TokensUsed, m.AgentModel, m.UserID, m.LibraryID,
	)
	if err != nil {
		return fmt.Errorf("put session metadata %s: %w", m.SessionID, err)
	}
	return nil
}
