package model

import "time"

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
	CommitHash  string           `json:"commit_hash,omitempty"`
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

// Stage2Result is the output of rubric scoring.
type Stage2Result struct {
	Passed             bool          `json:"passed"`
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
	Retries                  int           `json:"retries"`
	ModelName                string        `json:"model_name,omitempty"`
	CircuitBreakerState      string        `json:"circuit_breaker_state,omitempty"`
	SkillMD                  string        `json:"skill_md,omitempty"`
}
