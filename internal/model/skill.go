package model

import "time"

// SkillRecord is the database representation of an extracted skill.
type SkillRecord struct {
	ID        string        `db:"id"`
	Name      string        `db:"name"`
	Version   int           `db:"version"`
	ParentID  string        `db:"parent_id"`
	LibraryID string        `db:"library_id"`
	Category  SkillCategory `db:"category"`
	Patterns  []string      `db:"-"`

	Quality QualityScores `db:"-"`

	Embedding []float32 `db:"-"`

	InjectionCount     int       `db:"injection_count"`
	LastInjectedAt     time.Time `db:"last_injected_at"`
	SuccessCorrelation float64   `db:"success_correlation"`
	DecayScore         float64   `db:"decay_score"`

	SourceSessionID string    `db:"source_session_id"`
	ExtractedBy     string    `db:"extracted_by"`
	CreatedAt       time.Time `db:"created_at"`
	UpdatedAt       time.Time `db:"updated_at"`

	FilePath string `db:"file_path"`
}

// SkillCategory classifies skills by durability.
type SkillCategory string

const (
	CategoryFoundational SkillCategory = "foundational"
	CategoryTactical     SkillCategory = "tactical"
	CategoryContextual   SkillCategory = "contextual"
)

// QualityScores holds the 7-dimensional evaluation from Stage 2/3.
type QualityScores struct {
	ProblemSpecificity    int     `db:"q_problem_specificity"`
	SolutionCompleteness  int     `db:"q_solution_completeness"`
	ContextPortability    int     `db:"q_context_portability"`
	ReasoningTransparency int     `db:"q_reasoning_transparency"`
	TechnicalAccuracy     int     `db:"q_technical_accuracy"`
	VerificationEvidence  int     `db:"q_verification_evidence"`
	InnovationLevel       int     `db:"q_innovation_level"`
	CompositeScore        float64 `db:"q_composite_score"`
	CriticConfidence      float64 `db:"q_critic_confidence"`
}

// MinimumViable returns true if the skill meets minimum thresholds.
func (q QualityScores) MinimumViable() bool {
	return q.ProblemSpecificity >= 3 &&
		q.SolutionCompleteness >= 3 &&
		q.TechnicalAccuracy >= 3
}

// HighValue returns true if average across all dimensions >= 4.
func (q QualityScores) HighValue() bool {
	sum := q.ProblemSpecificity + q.SolutionCompleteness + q.ContextPortability +
		q.ReasoningTransparency + q.TechnicalAccuracy + q.VerificationEvidence +
		q.InnovationLevel
	return float64(sum)/7.0 >= 4.0
}

// InjectionPriority returns the injection ranking weight.
func (q QualityScores) InjectionPriority() float64 {
	return float64(q.ContextPortability)*0.6 + float64(q.VerificationEvidence)*0.4
}
