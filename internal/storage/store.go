package storage

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/mycelium-dev/mycelium/internal/extraction"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaDDL string

// SkillStore persists and retrieves skills.
type SkillStore interface {
	Put(ctx context.Context, skill *extraction.SkillRecord, body []byte) error
	Get(ctx context.Context, id string) (*extraction.SkillRecord, error)
	GetLatestByName(ctx context.Context, name string, libraryID string) (*extraction.SkillRecord, error)
	Query(ctx context.Context, q SkillQuery) ([]ScoredSkill, error)
	UpdateUsage(ctx context.Context, id string, outcome string) error
	UpdateDecay(ctx context.Context, id string, decayScore float64) error
	ListByDecay(ctx context.Context, libraryID string, limit int) ([]extraction.SkillRecord, error)
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
	Skill          extraction.SkillRecord
	PatternOverlap float64
	CosineSim      float64
	HistoricalRate float64
	CompositeScore float64
}

// SQLiteStore implements SkillStore with SQLite.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens (or creates) a SQLite database and runs migrations.
func NewSQLiteStore(dsn string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	// Foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	// Integrity check
	var result string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		db.Close()
		return nil, fmt.Errorf("integrity check: %w", err)
	}
	if result != "ok" {
		db.Close()
		return nil, fmt.Errorf("database integrity check failed: %s", result)
	}
	slog.Info("sqlite integrity check passed")

	// Run schema migration
	if _, err := db.Exec(schemaDDL); err != nil {
		db.Close()
		return nil, fmt.Errorf("run schema: %w", err)
	}
	slog.Info("sqlite schema applied")

	return &SQLiteStore{db: db}, nil
}

// Close closes the database.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) Put(ctx context.Context, skill *extraction.SkillRecord, body []byte) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

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
		return fmt.Errorf("insert skill: %w", err)
	}

	// Insert patterns
	for _, p := range skill.Patterns {
		if _, err := tx.ExecContext(ctx, `INSERT INTO skill_patterns (skill_id, pattern) VALUES (?, ?)`, skill.ID, p); err != nil {
			return fmt.Errorf("insert pattern: %w", err)
		}
	}

	// Insert embedding if present
	if len(skill.Embedding) > 0 {
		blob := float32sToBytes(skill.Embedding)
		// Spec says model is stored; default to a sensible value since SkillRecord doesn't carry model info.
		if _, err := tx.ExecContext(ctx, `INSERT INTO skill_embeddings (skill_id, embedding, model, created_at) VALUES (?, ?, ?, ?)`,
			skill.ID, blob, "text-embedding-3-small", now); err != nil {
			return fmt.Errorf("insert embedding: %w", err)
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) Get(ctx context.Context, id string) (*extraction.SkillRecord, error) {
	row := s.db.QueryRowContext(ctx, `SELECT * FROM skills WHERE id = ?`, id)
	skill, err := scanSkill(row)
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

func (s *SQLiteStore) GetLatestByName(ctx context.Context, name string, libraryID string) (*extraction.SkillRecord, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT * FROM skills WHERE name = ? AND library_id = ? ORDER BY version DESC LIMIT 1`,
		name, libraryID)
	skill, err := scanSkill(row)
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
	// Build query to fetch candidate skills
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

	query := `SELECT * FROM skills WHERE ` + strings.Join(where, " AND ")
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query skills: %w", err)
	}
	defer rows.Close()

	var candidates []extraction.SkillRecord
	for rows.Next() {
		skill, err := scanSkillRows(rows)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, *skill)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Load patterns and embeddings, compute scores
	queryPatternSet := make(map[string]struct{}, len(q.Patterns))
	for _, p := range q.Patterns {
		queryPatternSet[p] = struct{}{}
	}

	var results []ScoredSkill
	for _, skill := range candidates {
		patterns, err := s.loadPatterns(ctx, skill.ID)
		if err != nil {
			return nil, err
		}
		skill.Patterns = patterns

		// Pattern overlap: fraction of query patterns matched
		var patternOverlap float64
		if len(q.Patterns) > 0 {
			matched := 0
			for _, p := range patterns {
				if _, ok := queryPatternSet[p]; ok {
					matched++
				}
			}
			patternOverlap = float64(matched) / float64(len(q.Patterns))
		}

		// Cosine similarity
		var cosineSim float64
		if len(q.Embedding) > 0 {
			emb, err := s.loadEmbedding(ctx, skill.ID)
			if err != nil {
				return nil, err
			}
			skill.Embedding = emb
			if len(emb) > 0 {
				cosineSim = cosineSimilarity(q.Embedding, emb)
			}
		}

		// Historical rate: success_correlation normalized to 0-1 range
		historicalRate := (skill.SuccessCorrelation + 1.0) / 2.0

		composite := 0.5*patternOverlap + 0.3*cosineSim + 0.2*historicalRate

		results = append(results, ScoredSkill{
			Skill:          skill,
			PatternOverlap: patternOverlap,
			CosineSim:      cosineSim,
			HistoricalRate: historicalRate,
			CompositeScore: composite,
		})
	}

	// Sort descending by composite score
	sortScoredSkills(results)

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

	// Recompute success_correlation from injection_events
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

func (s *SQLiteStore) ListByDecay(ctx context.Context, libraryID string, limit int) ([]extraction.SkillRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT * FROM skills WHERE library_id = ? ORDER BY decay_score ASC LIMIT ?`,
		libraryID, limit)
	if err != nil {
		return nil, fmt.Errorf("list by decay: %w", err)
	}
	defer rows.Close()

	var skills []extraction.SkillRecord
	for rows.Next() {
		skill, err := scanSkillRows(rows)
		if err != nil {
			return nil, err
		}
		skills = append(skills, *skill)
	}
	return skills, rows.Err()
}

// --- helpers ---

func scanSkill(row *sql.Row) (*extraction.SkillRecord, error) {
	var s extraction.SkillRecord
	var parentID, sourceSessionID sql.NullString
	var lastInjected sql.NullTime
	err := row.Scan(
		&s.ID, &s.Name, &s.Version, &parentID, &s.LibraryID, &s.Category,
		&s.Quality.ProblemSpecificity, &s.Quality.SolutionCompleteness, &s.Quality.ContextPortability,
		&s.Quality.ReasoningTransparency, &s.Quality.TechnicalAccuracy, &s.Quality.VerificationEvidence,
		&s.Quality.InnovationLevel, &s.Quality.CompositeScore, &s.Quality.CriticConfidence,
		&s.InjectionCount, &lastInjected, &s.SuccessCorrelation, &s.DecayScore,
		&sourceSessionID, &s.ExtractedBy, &s.FilePath, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("skill not found")
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

func scanSkillRows(rows *sql.Rows) (*extraction.SkillRecord, error) {
	var s extraction.SkillRecord
	var parentID, sourceSessionID sql.NullString
	var lastInjected sql.NullTime
	err := rows.Scan(
		&s.ID, &s.Name, &s.Version, &parentID, &s.LibraryID, &s.Category,
		&s.Quality.ProblemSpecificity, &s.Quality.SolutionCompleteness, &s.Quality.ContextPortability,
		&s.Quality.ReasoningTransparency, &s.Quality.TechnicalAccuracy, &s.Quality.VerificationEvidence,
		&s.Quality.InnovationLevel, &s.Quality.CompositeScore, &s.Quality.CriticConfidence,
		&s.InjectionCount, &lastInjected, &s.SuccessCorrelation, &s.DecayScore,
		&sourceSessionID, &s.ExtractedBy, &s.FilePath, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan skill row: %w", err)
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

func (s *SQLiteStore) loadEmbedding(ctx context.Context, skillID string) ([]float32, error) {
	var blob []byte
	err := s.db.QueryRowContext(ctx, `SELECT embedding FROM skill_embeddings WHERE skill_id = ?`, skillID).Scan(&blob)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("load embedding: %w", err)
	}
	return bytesToFloat32s(blob), nil
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullTime(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t, Valid: true}
}

func float32sToBytes(fs []float32) []byte {
	buf := make([]byte, len(fs)*4)
	for i, f := range fs {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

func bytesToFloat32s(b []byte) []float32 {
	fs := make([]float32, len(b)/4)
	for i := range fs {
		fs[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return fs
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		ai, bi := float64(a[i]), float64(b[i])
		dot += ai * bi
		normA += ai * ai
		normB += bi * bi
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}

func sortScoredSkills(s []ScoredSkill) {
	// Simple insertion sort — result sets are small (bounded by limit, typically <100)
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j].CompositeScore > s[j-1].CompositeScore; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
