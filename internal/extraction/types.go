package extraction

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

// SessionRecord captures metadata about an agent session for extraction evaluation.
type SessionRecord struct {
	ID                string           `db:"id"`
	StartedAt         time.Time        `db:"started_at"`
	EndedAt           time.Time        `db:"ended_at"`
	DurationMinutes   float64          `db:"duration_minutes"`
	ToolCallCount     int              `db:"tool_call_count"`
	ErrorCount        int              `db:"error_count"`
	MessageCount      int              `db:"message_count"`
	ErrorRate         float64          `db:"error_rate"`
	HasSuccessfulExec bool             `db:"has_successful_exec"`
	TokensUsed        int              `db:"tokens_used"`
	AgentModel        string           `db:"agent_model"`
	UserID            string           `db:"user_id"`
	LibraryID         string           `db:"library_id"`

	ExtractionStatus ExtractionStatus `db:"extraction_status"`
	RejectedAtStage  int              `db:"rejected_at_stage"`
	RejectionReason  string           `db:"rejection_reason"`
	ExtractedSkillID string           `db:"extracted_skill_id"`

	LogPath string `db:"-"`
}

// ExtractionStatus represents the pipeline state of a session.
type ExtractionStatus string

const (
	StatusPending   ExtractionStatus = "pending"
	StatusStage1    ExtractionStatus = "stage1"
	StatusStage2    ExtractionStatus = "stage2"
	StatusStage3    ExtractionStatus = "stage3"
	StatusExtracted ExtractionStatus = "extracted"
	StatusRejected  ExtractionStatus = "rejected"
	StatusError     ExtractionStatus = "error"
)

// ExtractionResult is the output of the full pipeline for a single session.
type ExtractionResult struct {
	SessionID   string           `json:"session_id"`
	Status      ExtractionStatus `json:"status"`
	Stage1      *Stage1Result    `json:"stage1,omitempty"`
	Stage2      *Stage2Result    `json:"stage2,omitempty"`
	Stage3      *Stage3Result    `json:"stage3,omitempty"`
	Skill       *SkillRecord     `json:"skill,omitempty"`
	ProcessedAt time.Time        `json:"processed_at"`
	DurationMs  int64            `json:"duration_ms"`
	Error       string           `json:"error,omitempty"`
}

// Stage1Result is the output of metadata pre-filtering.
type Stage1Result struct {
	Passed          bool   `json:"passed"`
	DurationOK      bool   `json:"duration_ok"`
	ToolCallCountOK bool   `json:"tool_call_count_ok"`
	ErrorRateOK     bool   `json:"error_rate_ok"`
	HasSuccessExec  bool   `json:"has_success_exec"`
	Reason          string `json:"reason,omitempty"`
}

// Stage2Result is the output of embedding + rubric scoring.
type Stage2Result struct {
	Passed             bool          `json:"passed"`
	EmbeddingDistance   float64       `json:"embedding_distance"`
	NoveltyScore       float64       `json:"novelty_score"`
	RubricScores       QualityScores `json:"rubric_scores"`
	ClassifiedCategory SkillCategory `json:"classified_category"`
	ClassifiedPatterns []string      `json:"classified_patterns"`
	Reason             string        `json:"reason,omitempty"`
}

// Stage3Result is the output of the LLM critic.
type Stage3Result struct {
	Passed                   bool          `json:"passed"`
	CriticVerdict            string        `json:"critic_verdict"`
	CriticReasoning          string        `json:"critic_reasoning"`
	RefinedScores            QualityScores `json:"refined_scores"`
	ReusablePattern          bool          `json:"reusable_pattern"`
	ExplicitReasoning        bool          `json:"explicit_reasoning"`
	ContradictsBestPractices bool          `json:"contradicts_best_practices"`
	TokensUsed               int           `json:"tokens_used"`
	LatencyMs                int64         `json:"latency_ms"`
}

// InjectionRequest is the input to the Injector.
type InjectionRequest struct {
	Prompt     string
	LibraryIDs []string
	MaxSkills  int
	SessionID  string
}

// InjectionResponse is the output of the Injector.
type InjectionResponse struct {
	Skills         []ScoredSkill
	Classification PromptClassification
}

// ScoredSkill is a skill with match scores (used in injection responses).
type ScoredSkill struct {
	Skill          SkillRecord
	PatternOverlap float64
	CosineSim      float64
	HistoricalRate float64
	CompositeScore float64
}

// PromptClassification describes how a prompt was parsed for injection.
type PromptClassification struct {
	Intent   string
	Domain   string
	Patterns []string
}
