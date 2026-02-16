package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kinoko-dev/kinoko/internal/model"
)

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// scanSkillFrom scans a skill from any scanner (Row or Rows).
func scanSkillFrom(sc scanner) (*model.SkillRecord, error) {
	var s model.SkillRecord
	var parentID, sourceSessionID sql.NullString
	var lastInjected sql.NullTime
	err := sc.Scan(
		&s.ID, &s.Name, &s.Version, &parentID, &s.LibraryID, &s.Category,
		&s.Quality.ProblemSpecificity, &s.Quality.SolutionCompleteness, &s.Quality.ContextPortability,
		&s.Quality.ReasoningTransparency, &s.Quality.TechnicalAccuracy, &s.Quality.VerificationEvidence,
		&s.Quality.InnovationLevel, &s.Quality.CompositeScore, &s.Quality.CriticConfidence,
		&s.InjectionCount, &lastInjected, &s.SuccessCorrelation, &s.DecayScore,
		&sourceSessionID, &s.ExtractedBy, &s.FilePath, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan skill: %w", err)
	}
	if parentID.Valid {
		s.ParentID = parentID.String
	}
	if sourceSessionID.Valid {
		s.SourceSessionID = sourceSessionID.String
	}
	if lastInjected.Valid {
		s.LastInjectedAt = lastInjected.Time
	}
	return &s, nil
}

func (s *SQLiteStore) Put(ctx context.Context, skill *model.SkillRecord, body []byte) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	if skill.CreatedAt.IsZero() {
		skill.CreatedAt = now
	}
	skill.UpdatedAt = now

	_, err = tx.ExecContext(ctx, `
		INSERT INTO skills (
			id, name, version, parent_id, library_id, category,
			q_problem_specificity, q_solution_completeness, q_context_portability,
			q_reasoning_transparency, q_technical_accuracy, q_verification_evidence,
			q_innovation_level, q_composite_score, q_critic_confidence,
			injection_count, last_injected_at, success_correlation, decay_score,
			source_session_id, extracted_by, file_path, created_at, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		skill.ID, skill.Name, skill.Version, nullString(skill.ParentID), skill.LibraryID, string(skill.Category),
		skill.Quality.ProblemSpecificity, skill.Quality.SolutionCompleteness, skill.Quality.ContextPortability,
		skill.Quality.ReasoningTransparency, skill.Quality.TechnicalAccuracy, skill.Quality.VerificationEvidence,
		skill.Quality.InnovationLevel, skill.Quality.CompositeScore, skill.Quality.CriticConfidence,
		skill.InjectionCount, nullTime(skill.LastInjectedAt), skill.SuccessCorrelation, skill.DecayScore,
		nullString(skill.SourceSessionID), skill.ExtractedBy, skill.FilePath, skill.CreatedAt, skill.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return fmt.Errorf("%w: %s v%d in %s", ErrDuplicate, skill.Name, skill.Version, skill.LibraryID)
		}
		return fmt.Errorf("insert skill: %w", err)
	}

	for _, p := range skill.Patterns {
		if _, err := tx.ExecContext(ctx, `INSERT INTO skill_patterns (skill_id, pattern) VALUES (?, ?)`, skill.ID, p); err != nil {
			return fmt.Errorf("insert pattern: %w", err)
		}
	}

	if len(skill.Embedding) > 0 {
		blob := float32sToBytes(skill.Embedding)
		if _, err := tx.ExecContext(ctx, `INSERT INTO skill_embeddings (skill_id, embedding, model, created_at) VALUES (?, ?, ?, ?)`,
			skill.ID, blob, s.embeddingModel, now); err != nil {
			return fmt.Errorf("insert embedding: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit skill: %w", err)
	}

	if len(body) > 0 && skill.FilePath != "" {
		dir := filepath.Dir(skill.FilePath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create skill dir (post-commit): %w", err)
		}
		if err := os.WriteFile(skill.FilePath, body, 0o644); err != nil {
			return fmt.Errorf("write skill body (post-commit): %w", err)
		}
	}

	return nil
}

func (s *SQLiteStore) Get(ctx context.Context, id string) (*model.SkillRecord, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+skillColumns+` FROM skills WHERE id = ?`, id)
	skill, err := scanSkillFrom(row)
	if err != nil {
		return nil, err
	}

	skill.Patterns, err = s.loadPatterns(ctx, skill.ID)
	if err != nil {
		return nil, err
	}

	skill.Embedding, err = s.loadEmbedding(ctx, skill.ID)
	if err != nil {
		return nil, err
	}

	return skill, nil
}

func (s *SQLiteStore) GetLatestByName(ctx context.Context, name string, libraryID string) (*model.SkillRecord, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+skillColumns+` FROM skills WHERE name = ? AND library_id = ? ORDER BY version DESC LIMIT 1`,
		name, libraryID)
	skill, err := scanSkillFrom(row)
	if err != nil {
		return nil, err
	}

	skill.Patterns, err = s.loadPatterns(ctx, skill.ID)
	if err != nil {
		return nil, err
	}

	skill.Embedding, err = s.loadEmbedding(ctx, skill.ID)
	if err != nil {
		return nil, err
	}

	return skill, nil
}

func (s *SQLiteStore) Query(ctx context.Context, q SkillQuery) ([]ScoredSkill, error) {
	where := []string{"1=1"}
	args := []any{}

	if len(q.LibraryIDs) > 0 {
		placeholders := make([]string, len(q.LibraryIDs))
		for i, id := range q.LibraryIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		where = append(where, "library_id IN ("+strings.Join(placeholders, ",")+") ")
	}
	if q.MinQuality > 0 {
		where = append(where, "q_composite_score >= ?")
		args = append(args, q.MinQuality)
	}
	if q.MinDecay > 0 {
		where = append(where, "decay_score >= ?")
		args = append(args, q.MinDecay)
	}

	query := `SELECT ` + skillColumns + ` FROM skills WHERE ` + strings.Join(where, " AND ") //nolint:gosec // parameterized query, columns are not user input
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query skills: %w", err)
	}
	defer rows.Close()

	var candidates []model.SkillRecord
	var candidateIDs []string
	for rows.Next() {
		skill, err := scanSkillFrom(rows)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, *skill)
		candidateIDs = append(candidateIDs, skill.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	patternMap, err := s.loadPatternsMulti(ctx, candidateIDs)
	if err != nil {
		return nil, err
	}

	embeddingMap, err := s.loadEmbeddingsMulti(ctx, candidateIDs)
	if err != nil {
		return nil, err
	}

	queryPatternSet := make(map[string]struct{}, len(q.Patterns))
	for _, p := range q.Patterns {
		queryPatternSet[p] = struct{}{}
	}

	var results []ScoredSkill
	for i := range candidates {
		skill := &candidates[i]
		skill.Patterns = patternMap[skill.ID]
		skill.Embedding = embeddingMap[skill.ID]

		var patternOverlap float64
		if len(q.Patterns) > 0 {
			matched := 0
			for _, p := range skill.Patterns {
				if _, ok := queryPatternSet[p]; ok {
					matched++
				}
			}
			patternOverlap = float64(matched) / float64(len(q.Patterns))
		}

		var cosineSim float64
		if len(q.Embedding) > 0 && len(skill.Embedding) > 0 {
			cosineSim = cosineSimilarity(q.Embedding, skill.Embedding)
		}

		historicalRate := (skill.SuccessCorrelation + 1.0) / 2.0

		composite := 0.5*patternOverlap + 0.3*cosineSim + 0.2*historicalRate

		results = append(results, ScoredSkill{
			Skill:          *skill,
			PatternOverlap: patternOverlap,
			CosineSim:      cosineSim,
			HistoricalRate: historicalRate,
			CompositeScore: composite,
		})
	}

	slices.SortFunc(results, func(a, b ScoredSkill) int {
		if a.CompositeScore > b.CompositeScore {
			return -1
		}
		if a.CompositeScore < b.CompositeScore {
			return 1
		}
		return 0
	})

	if q.Limit > 0 && len(results) > q.Limit {
		results = results[:q.Limit]
	}

	return results, nil
}

func (s *SQLiteStore) UpdateUsage(ctx context.Context, id string, outcome string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		UPDATE skills SET
			injection_count = injection_count + 1,
			last_injected_at = ?,
			updated_at = ?
		WHERE id = ?`, now, now, id)
	if err != nil {
		return fmt.Errorf("update usage: %w", err)
	}

	if outcome == "success" || outcome == "failure" {
		_, err = s.db.ExecContext(ctx, `
			UPDATE skills SET success_correlation = (
				SELECT COALESCE(
					(CAST(SUM(CASE WHEN session_outcome='success' THEN 1 ELSE 0 END) AS REAL)
					 - CAST(SUM(CASE WHEN session_outcome='failure' THEN 1 ELSE 0 END) AS REAL))
					/ CAST(COUNT(*) AS REAL),
				0.0)
				FROM injection_events WHERE skill_id = ? AND session_outcome IS NOT NULL
			) WHERE id = ?`, id, id)
		if err != nil {
			return fmt.Errorf("update correlation: %w", err)
		}
	}

	return nil
}

func (s *SQLiteStore) UpdateDecay(ctx context.Context, id string, decayScore float64) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `UPDATE skills SET decay_score = ?, updated_at = ? WHERE id = ?`, decayScore, now, id)
	if err != nil {
		return fmt.Errorf("update decay: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListByDecay(ctx context.Context, libraryID string, limit int) ([]model.SkillRecord, error) {
	query := `SELECT ` + skillColumns + ` FROM skills WHERE library_id = ? ORDER BY decay_score ASC`
	args := []any{libraryID}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list by decay: %w", err)
	}
	defer rows.Close()

	var skills []model.SkillRecord
	for rows.Next() {
		skill, err := scanSkillFrom(rows)
		if err != nil {
			return nil, err
		}
		skills = append(skills, *skill)
	}
	return skills, rows.Err()
}

func (s *SQLiteStore) loadPatterns(ctx context.Context, skillID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT pattern FROM skill_patterns WHERE skill_id = ?`, skillID)
	if err != nil {
		return nil, fmt.Errorf("load patterns: %w", err)
	}
	defer rows.Close()
	var patterns []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		patterns = append(patterns, p)
	}
	return patterns, rows.Err()
}

func (s *SQLiteStore) loadPatternsMulti(ctx context.Context, skillIDs []string) (map[string][]string, error) {
	if len(skillIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(skillIDs))
	args := make([]any, len(skillIDs))
	for i, id := range skillIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT skill_id, pattern FROM skill_patterns WHERE skill_id IN (`+strings.Join(placeholders, ",")+`)`, args...) //nolint:gosec // parameterized query, columns are not user input
	if err != nil {
		return nil, fmt.Errorf("load patterns multi: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]string, len(skillIDs))
	for rows.Next() {
		var skillID, pattern string
		if err := rows.Scan(&skillID, &pattern); err != nil {
			return nil, err
		}
		result[skillID] = append(result[skillID], pattern)
	}
	return result, rows.Err()
}

func (s *SQLiteStore) loadEmbeddingsMulti(ctx context.Context, skillIDs []string) (map[string][]float32, error) {
	if len(skillIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(skillIDs))
	args := make([]any, len(skillIDs))
	for i, id := range skillIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT skill_id, embedding FROM skill_embeddings WHERE skill_id IN (`+strings.Join(placeholders, ",")+`)`, args...) //nolint:gosec // parameterized query, columns are not user input
	if err != nil {
		return nil, fmt.Errorf("load embeddings multi: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]float32, len(skillIDs))
	for rows.Next() {
		var skillID string
		var blob []byte
		if err := rows.Scan(&skillID, &blob); err != nil {
			return nil, err
		}
		result[skillID] = bytesToFloat32s(blob)
	}
	return result, rows.Err()
}

func (s *SQLiteStore) loadEmbedding(ctx context.Context, skillID string) ([]float32, error) {
	var blob []byte
	err := s.db.QueryRowContext(ctx, `SELECT embedding FROM skill_embeddings WHERE skill_id = ?`, skillID).Scan(&blob)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("load embedding: %w", err)
	}
	return bytesToFloat32s(blob), nil
}

// CountSkills returns the total number of skills in the store.
func (s *SQLiteStore) CountSkills(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM skills").Scan(&count)
	return count, err
}
