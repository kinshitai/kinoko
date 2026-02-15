// Package storage provides SQLite-backed persistence for skills, sessions,
// injection events, and human review samples. It implements the SkillStore
// and SessionStore interfaces consumed by the extraction and injection pipelines.
package storage

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/kinoko-dev/kinoko/internal/model"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaDDL string

// Sentinel errors for callers to check with errors.Is.
var (
	ErrNotFound  = errors.New("skill not found")
	ErrDuplicate = errors.New("duplicate skill")
)

// skillColumns is the canonical column list for the skills table.
const skillColumns = `id, name, version, parent_id, library_id, category,
	q_problem_specificity, q_solution_completeness, q_context_portability,
	q_reasoning_transparency, q_technical_accuracy, q_verification_evidence,
	q_innovation_level, q_composite_score, q_critic_confidence,
	injection_count, last_injected_at, success_correlation, decay_score,
	source_session_id, extracted_by, file_path, created_at, updated_at`

// SessionStore persists and updates session records.
type SessionStore interface {
	InsertSession(ctx context.Context, session *model.SessionRecord) error
	UpdateSessionResult(ctx context.Context, session *model.SessionRecord) error
}

// SkillStore persists and retrieves skills.
type SkillStore interface {
	Put(ctx context.Context, skill *model.SkillRecord, body []byte) error
	Get(ctx context.Context, id string) (*model.SkillRecord, error)
	GetLatestByName(ctx context.Context, name string, libraryID string) (*model.SkillRecord, error)
	Query(ctx context.Context, q SkillQuery) ([]ScoredSkill, error)
	UpdateUsage(ctx context.Context, id string, outcome string) error
	UpdateDecay(ctx context.Context, id string, decayScore float64) error
	ListByDecay(ctx context.Context, libraryID string, limit int) ([]model.SkillRecord, error)
}

// SkillQuery defines query parameters for skill search.
type SkillQuery struct {
	Patterns   []string
	Embedding  []float32
	LibraryIDs []string
	MinQuality float64
	MinDecay   float64
	Limit      int
}

// ScoredSkill is a skill with match scores.
type ScoredSkill struct {
	Skill          model.SkillRecord
	PatternOverlap float64
	CosineSim      float64
	HistoricalRate float64
	CompositeScore float64
}

// SQLiteStore implements SkillStore with SQLite.
type SQLiteStore struct {
	db             *sql.DB
	embeddingModel string
}

// NewSQLiteStore opens (or creates) a SQLite database and runs migrations.
// embeddingModel specifies the model name stored with embeddings (e.g. "text-embedding-3-small").
func NewSQLiteStore(dsn string, embeddingModel string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}

	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	var result string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		db.Close()
		return nil, fmt.Errorf("integrity check: %w", err)
	}
	if result != "ok" {
		db.Close()
		return nil, fmt.Errorf("database integrity check failed: %s", result)
	}
	slog.Info("sqlite integrity check passed", "dsn", dsn)

	if _, err := db.Exec(schemaDDL); err != nil {
		db.Close()
		return nil, fmt.Errorf("run schema: %w", err)
	}

	for _, col := range []struct{ name, ddl string }{
		{"log_content_path", "ALTER TABLE sessions ADD COLUMN log_content_path TEXT NOT NULL DEFAULT ''"},
		{"retry_count", "ALTER TABLE sessions ADD COLUMN retry_count INTEGER NOT NULL DEFAULT 0"},
		{"last_error", "ALTER TABLE sessions ADD COLUMN last_error TEXT NOT NULL DEFAULT ''"},
		{"next_retry_at", "ALTER TABLE sessions ADD COLUMN next_retry_at TIMESTAMP"},
		{"claimed_by", "ALTER TABLE sessions ADD COLUMN claimed_by TEXT NOT NULL DEFAULT ''"},
		{"claimed_at", "ALTER TABLE sessions ADD COLUMN claimed_at TIMESTAMP"},
	} {
		if _, err := db.Exec(col.ddl); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				db.Close()
				return nil, fmt.Errorf("migrate %s: %w", col.name, err)
			}
		}
	}
	slog.Info("sqlite schema applied")

	if embeddingModel == "" {
		embeddingModel = "text-embedding-3-small"
	}

	return &SQLiteStore{db: db, embeddingModel: embeddingModel}, nil
}

// Close closes the database.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// DB returns the underlying *sql.DB for direct queries (stats, migrations, etc.).
func (s *SQLiteStore) DB() *sql.DB {
	return s.db
}
