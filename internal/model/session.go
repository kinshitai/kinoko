package model

import "time"

// SessionRecord captures metadata about an agent session for extraction evaluation.
type SessionRecord struct {
	ID                string    `db:"id"`
	StartedAt         time.Time `db:"started_at"`
	EndedAt           time.Time `db:"ended_at"`
	DurationMinutes   float64   `db:"duration_minutes"`
	ToolCallCount     int       `db:"tool_call_count"`
	ErrorCount        int       `db:"error_count"`
	MessageCount      int       `db:"message_count"`
	ErrorRate         float64   `db:"error_rate"`
	HasSuccessfulExec bool      `db:"has_successful_exec"`
	TokensUsed        int       `db:"tokens_used"`
	AgentModel        string    `db:"agent_model"`
	UserID            string    `db:"user_id"`
	LibraryID         string    `db:"library_id"`

	ExtractionStatus ExtractionStatus `db:"extraction_status"`
	RejectedAtStage  int              `db:"rejected_at_stage"`
	RejectionReason  string           `db:"rejection_reason"`
	ExtractedSkillID string           `db:"extracted_skill_id"`

	LogPath string `db:"-"`
}

// ExtractionStatus represents the pipeline state of a session.
type ExtractionStatus string

const (
	StatusQueued    ExtractionStatus = "queued"
	StatusPending   ExtractionStatus = "pending"
	StatusExtracted ExtractionStatus = "extracted"
	StatusRejected  ExtractionStatus = "rejected"
	StatusError     ExtractionStatus = "error"
	StatusFailed    ExtractionStatus = "failed"
)
