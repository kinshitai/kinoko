package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/kinoko-dev/kinoko/internal/model"
)

// Compile-time check.
var _ model.SkillIndexer = (*SQLiteIndexer)(nil)

// SQLiteIndexer implements model.SkillIndexer by upserting into SQLite.
type SQLiteIndexer struct {
	store *SQLiteStore
}

// NewSQLiteIndexer returns a new indexer backed by the given store.
func NewSQLiteIndexer(store *SQLiteStore) *SQLiteIndexer {
	return &SQLiteIndexer{store: store}
}

// IndexSkill upserts skill metadata, patterns, and embedding into SQLite.
// It does not mutate the caller's *SkillRecord.
func (idx *SQLiteIndexer) IndexSkill(ctx context.Context, skill *model.SkillRecord, embedding []float32) error {
	if skill.ID == "" {
		return fmt.Errorf("skill ID is required")
	}
	if skill.Name == "" {
		return fmt.Errorf("skill name is required")
	}

	tx, err := idx.store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	createdAt := skill.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	updatedAt := now

	// Pre-clean child rows for any existing skill with same name+library.
	// Required because the upsert may change the skill ID, which would violate
	// foreign key constraints on skill_patterns and skill_embeddings (no ON UPDATE CASCADE).
	if _, err = tx.ExecContext(ctx, `DELETE FROM skill_patterns WHERE skill_id IN (SELECT id FROM skills WHERE name = ? AND library_id = ?)`, skill.Name, skill.LibraryID); err != nil {
		return fmt.Errorf("delete old patterns: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM skill_embeddings WHERE skill_id IN (SELECT id FROM skills WHERE name = ? AND library_id = ?)`, skill.Name, skill.LibraryID); err != nil {
		return fmt.Errorf("delete old embeddings: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO skills (
			id, name, version, parent_id, library_id, category,
			q_problem_specificity, q_solution_completeness, q_context_portability,
			q_reasoning_transparency, q_technical_accuracy, q_verification_evidence,
			q_innovation_level, q_composite_score, q_critic_confidence,
			injection_count, last_injected_at, success_correlation, decay_score,
			source_session_id, extracted_by, file_path, created_at, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(name, library_id) DO UPDATE SET
			id = excluded.id,
			version = excluded.version,
			parent_id = excluded.parent_id,
			library_id = excluded.library_id,
			category = excluded.category,
			q_problem_specificity = excluded.q_problem_specificity,
			q_solution_completeness = excluded.q_solution_completeness,
			q_context_portability = excluded.q_context_portability,
			q_reasoning_transparency = excluded.q_reasoning_transparency,
			q_technical_accuracy = excluded.q_technical_accuracy,
			q_verification_evidence = excluded.q_verification_evidence,
			q_innovation_level = excluded.q_innovation_level,
			q_composite_score = excluded.q_composite_score,
			q_critic_confidence = excluded.q_critic_confidence,
			injection_count = excluded.injection_count,
			last_injected_at = excluded.last_injected_at,
			success_correlation = excluded.success_correlation,
			decay_score = excluded.decay_score,
			source_session_id = excluded.source_session_id,
			extracted_by = excluded.extracted_by,
			file_path = excluded.file_path,
			updated_at = excluded.updated_at`,
		skill.ID, skill.Name, skill.Version, nullString(skill.ParentID), skill.LibraryID, string(skill.Category),
		skill.Quality.ProblemSpecificity, skill.Quality.SolutionCompleteness, skill.Quality.ContextPortability,
		skill.Quality.ReasoningTransparency, skill.Quality.TechnicalAccuracy, skill.Quality.VerificationEvidence,
		skill.Quality.InnovationLevel, skill.Quality.CompositeScore, skill.Quality.CriticConfidence,
		skill.InjectionCount, nullTime(skill.LastInjectedAt), skill.SuccessCorrelation, skill.DecayScore,
		nullString(skill.SourceSessionID), skill.ExtractedBy, skill.FilePath, createdAt, updatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert skill: %w", err)
	}

	// Look up the actual stored ID (may differ from skill.ID if upsert updated it).
	var storedID string
	err = tx.QueryRowContext(ctx, `SELECT id FROM skills WHERE name = ? AND library_id = ?`, skill.Name, skill.LibraryID).Scan(&storedID)
	if err != nil {
		return fmt.Errorf("lookup stored skill id: %w", err)
	}

	// Replace patterns.
	if _, err := tx.ExecContext(ctx, `DELETE FROM skill_patterns WHERE skill_id = ?`, storedID); err != nil {
		return fmt.Errorf("delete patterns: %w", err)
	}
	for _, p := range skill.Patterns {
		if _, err := tx.ExecContext(ctx, `INSERT INTO skill_patterns (skill_id, pattern) VALUES (?, ?)`, storedID, p); err != nil {
			return fmt.Errorf("insert pattern: %w", err)
		}
	}

	// Replace embedding.
	if len(embedding) > 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM skill_embeddings WHERE skill_id = ?`, storedID); err != nil {
			return fmt.Errorf("delete embedding: %w", err)
		}
		blob := float32sToBytes(embedding)
		if _, err := tx.ExecContext(ctx, `INSERT INTO skill_embeddings (skill_id, embedding, model, created_at) VALUES (?, ?, ?, ?)`,
			storedID, blob, idx.store.embeddingModel, now); err != nil {
			return fmt.Errorf("insert embedding: %w", err)
		}
	}

	return tx.Commit()
}
