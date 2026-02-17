package extraction

import (
	"context"
	"fmt"
	"log/slog"
	"math"

	"github.com/kinoko-dev/kinoko/internal/config"
	"github.com/kinoko-dev/kinoko/internal/llm"
	"github.com/kinoko-dev/kinoko/internal/llmutil"
	"github.com/kinoko-dev/kinoko/internal/model"
)

// Taxonomy is the canonical list of problem patterns from Appendix B.
// Exported so the injection package can share the same list for prompt building.
var Taxonomy = []string{
	"BUILD/Frontend/ComponentDesign",
	"BUILD/Frontend/StateManagement",
	"BUILD/Backend/APIDesign",
	"BUILD/Backend/DataModeling",
	"BUILD/DevOps/CIPipeline",
	"BUILD/DevOps/ContainerSetup",
	"FIX/Frontend/RenderingBug",
	"FIX/Backend/DatabaseConnection",
	"FIX/Backend/AuthFlow",
	"FIX/DevOps/DeploymentFailure",
	"FIX/Performance/MemoryLeak",
	"FIX/Performance/SlowQuery",
	"OPTIMIZE/Performance/Caching",
	"OPTIMIZE/Performance/BundleSize",
	"OPTIMIZE/Backend/QueryOptimization",
	"INTEGRATE/Backend/ThirdPartyAPI",
	"INTEGRATE/DevOps/CloudService",
	"CONFIGURE/DevOps/InfraAsCode",
	"CONFIGURE/Security/AccessControl",
	"LEARN/Data/DataPipeline",
}

// validPatterns is built from Taxonomy for O(1) lookups.
var validPatterns map[string]bool

func init() {
	validPatterns = make(map[string]bool, len(Taxonomy))
	for _, p := range Taxonomy {
		validPatterns[p] = true
	}
}

// Stage2Scorer runs embedding novelty + structured rubric scoring.
type Stage2Scorer interface {
	Score(ctx context.Context, session model.SessionRecord, content []byte) (*model.Stage2Result, error)
}

type stage2Scorer struct {
	embedder model.Embedder
	querier  model.SkillQuerier
	llm      llm.LLMClient
	minDist  float64
	maxDist  float64
	log      *slog.Logger
}

// NewStage2Scorer creates a Stage2Scorer from dependencies and config.
func NewStage2Scorer(
	embedder model.Embedder,
	querier model.SkillQuerier,
	llmClient llm.LLMClient,
	cfg config.ExtractionConfig,
	log *slog.Logger,
) Stage2Scorer {
	return &stage2Scorer{
		embedder: embedder,
		querier:  querier,
		llm:      llmClient,
		minDist:  cfg.NoveltyMinDistance,
		maxDist:  cfg.NoveltyMaxDistance,
		log:      log,
	}
}

func (s *stage2Scorer) Score(ctx context.Context, session model.SessionRecord, content []byte) (*model.Stage2Result, error) {
	result := &model.Stage2Result{}

	// Classifier 1: Embedding Novelty
	emb, err := s.embedder.Embed(ctx, string(content))
	if err != nil {
		return nil, fmt.Errorf("stage2: embed content: %w", err)
	}

	nearest, err := s.querier.QueryNearest(ctx, emb, session.LibraryID)
	if err != nil {
		return nil, fmt.Errorf("stage2: query skill store: %w", err)
	}

	var distance float64
	if nearest == nil {
		// No existing skills — maximum novelty.
		distance = 1.0
	} else {
		// CosineSim is in [0,1] for normalized embeddings; distance = 1 - similarity.
		distance = 1.0 - nearest.CosineSim
	}

	result.EmbeddingDistance = distance
	if nearest != nil {
		result.NearestSkillName = nearest.SkillName
	}
	// Novelty score: 0 at boundaries, 1 at midpoint of valid range.
	if distance < s.minDist {
		result.NoveltyScore = 0
		result.Reason = fmt.Sprintf("too similar to existing skill (distance %.3f < min %.3f)", distance, s.minDist)
		s.log.Info("stage2 reject: novelty too low", "session_id", session.ID, "distance", distance)
		return result, nil
	}
	if distance > s.maxDist {
		result.NoveltyScore = 0
		result.Reason = fmt.Sprintf("too unrelated to existing skills (distance %.3f > max %.3f)", distance, s.maxDist)
		s.log.Info("stage2 reject: novelty too high", "session_id", session.ID, "distance", distance)
		return result, nil
	}

	// Normalize novelty into [0,1] within the valid range using a linear ramp
	// from boundaries to midpoint (triangle function). Boundaries get a small
	// epsilon floor so that edge-of-range sessions still carry a nonzero score.
	mid := (s.minDist + s.maxDist) / 2
	halfRange := (s.maxDist - s.minDist) / 2
	if halfRange > 0 {
		raw := 1.0 - math.Abs(distance-mid)/halfRange
		// Floor at 0.05 so boundary distances are not zero.
		if raw < 0.05 {
			raw = 0.05
		}
		result.NoveltyScore = raw
	} else {
		result.NoveltyScore = 1.0
	}

	s.log.Info("stage2 novelty pass", "session_id", session.ID, "distance", distance, "novelty_score", result.NoveltyScore)

	// Classifier 2: Structured Rubric Scoring via LLM
	prompt := buildRubricPrompt(content)
	resp, err := s.llm.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("stage2: llm rubric call: %w", err)
	}

	rubric, err := llmutil.ExtractJSON[rubricResponse](resp)
	if err != nil {
		return nil, fmt.Errorf("stage2: parse rubric response: %w", err)
	}

	// Validate rubric scores are in [1,5].
	if err := rubric.Scores.validate(); err != nil {
		return nil, fmt.Errorf("stage2: invalid rubric scores: %w", err)
	}

	// Validate category; default to tactical if invalid.
	rubric.Category = validateCategory(rubric.Category)

	// Strip patterns not in the taxonomy.
	rubric.Patterns = validatePatterns(rubric.Patterns)

	result.RubricScores = rubric.Scores.toQualityScores()
	result.RubricScores.CompositeScore = compositeScore(result.RubricScores)
	result.ClassifiedCategory = rubric.Category
	result.ClassifiedPatterns = rubric.Patterns

	if !result.RubricScores.MinimumViable() {
		result.Reason = "rubric scores below minimum viable threshold"
		s.log.Info("stage2 reject: rubric", "session_id", session.ID, "scores", result.RubricScores)
		return result, nil
	}

	result.Passed = true
	s.log.Info("stage2 pass", "session_id", session.ID, "category", result.ClassifiedCategory, "patterns", result.ClassifiedPatterns)
	return result, nil
}

// rubricScoresJSON maps the snake_case JSON from the LLM to model.QualityScores.
type rubricScoresJSON struct {
	ProblemSpecificity    int `json:"problem_specificity"`
	SolutionCompleteness  int `json:"solution_completeness"`
	ContextPortability    int `json:"context_portability"`
	ReasoningTransparency int `json:"reasoning_transparency"`
	TechnicalAccuracy     int `json:"technical_accuracy"`
	VerificationEvidence  int `json:"verification_evidence"`
	InnovationLevel       int `json:"innovation_level"`
}

func (r rubricScoresJSON) toQualityScores() model.QualityScores {
	return model.QualityScores{
		ProblemSpecificity:    r.ProblemSpecificity,
		SolutionCompleteness:  r.SolutionCompleteness,
		ContextPortability:    r.ContextPortability,
		ReasoningTransparency: r.ReasoningTransparency,
		TechnicalAccuracy:     r.TechnicalAccuracy,
		VerificationEvidence:  r.VerificationEvidence,
		InnovationLevel:       r.InnovationLevel,
	}
}

// rubricResponse is the expected JSON structure from the LLM.
type rubricResponse struct {
	Scores   rubricScoresJSON    `json:"scores"`
	Category model.SkillCategory `json:"category"`
	Patterns []string            `json:"patterns"`
}

// Weighted composite score per spec §1.1 model.QualityScores.
// Weights: problem_specificity=0.15, solution_completeness=0.20, context_portability=0.15,
// reasoning_transparency=0.10, technical_accuracy=0.20, verification_evidence=0.10, innovation_level=0.10.
func compositeScore(q model.QualityScores) float64 {
	return float64(q.ProblemSpecificity)*0.15 +
		float64(q.SolutionCompleteness)*0.20 +
		float64(q.ContextPortability)*0.15 +
		float64(q.ReasoningTransparency)*0.10 +
		float64(q.TechnicalAccuracy)*0.20 +
		float64(q.VerificationEvidence)*0.10 +
		float64(q.InnovationLevel)*0.10
}

// validate checks all 7 rubric scores are in [1,5].
func (r rubricScoresJSON) validate() error {
	scores := map[string]int{
		"problem_specificity":    r.ProblemSpecificity,
		"solution_completeness":  r.SolutionCompleteness,
		"context_portability":    r.ContextPortability,
		"reasoning_transparency": r.ReasoningTransparency,
		"technical_accuracy":     r.TechnicalAccuracy,
		"verification_evidence":  r.VerificationEvidence,
		"innovation_level":       r.InnovationLevel,
	}
	for name, v := range scores {
		if v < 1 || v > 5 {
			return fmt.Errorf("%s score %d out of range [1,5]", name, v)
		}
	}
	return nil
}

// validateCategory returns the category if valid, or model.CategoryTactical as default.
func validateCategory(c model.SkillCategory) model.SkillCategory {
	switch c {
	case model.CategoryFoundational, model.CategoryTactical, model.CategoryContextual:
		return c
	default:
		return model.CategoryTactical
	}
}

// ValidPattern reports whether p is in the taxonomy.
func ValidPattern(p string) bool {
	return validPatterns[p]
}

// validatePatterns strips patterns not in the taxonomy.
func validatePatterns(patterns []string) []string {
	var valid []string
	for _, p := range patterns {
		if validPatterns[p] {
			valid = append(valid, p)
		}
	}
	return valid
}

func buildRubricPrompt(content []byte) string {
	return fmt.Sprintf(`Analyze this agent session and respond with ONLY a JSON object (no markdown, no explanation).

Score each dimension 1-5:
- problem_specificity: How specific and well-defined is the problem?
- solution_completeness: How complete is the solution?
- context_portability: How reusable is this outside its original context?
- reasoning_transparency: How clear is the reasoning chain?
- technical_accuracy: How technically correct is the solution?
- verification_evidence: Is there evidence the solution works?
- innovation_level: How novel is the approach?

Classify the category as one of: foundational, tactical, contextual.

Pick 1-3 matching problem patterns from this taxonomy:
BUILD/Frontend/ComponentDesign, BUILD/Frontend/StateManagement, BUILD/Backend/APIDesign, BUILD/Backend/DataModeling, BUILD/DevOps/CIPipeline, BUILD/DevOps/ContainerSetup, FIX/Frontend/RenderingBug, FIX/Backend/DatabaseConnection, FIX/Backend/AuthFlow, FIX/DevOps/DeploymentFailure, FIX/Performance/MemoryLeak, FIX/Performance/SlowQuery, OPTIMIZE/Performance/Caching, OPTIMIZE/Performance/BundleSize, OPTIMIZE/Backend/QueryOptimization, INTEGRATE/Backend/ThirdPartyAPI, INTEGRATE/DevOps/CloudService, CONFIGURE/DevOps/InfraAsCode, CONFIGURE/Security/AccessControl, LEARN/Data/DataPipeline

JSON format:
{
  "scores": {
    "problem_specificity": N,
    "solution_completeness": N,
    "context_portability": N,
    "reasoning_transparency": N,
    "technical_accuracy": N,
    "verification_evidence": N,
    "innovation_level": N
  },
  "category": "foundational|tactical|contextual",
  "patterns": ["PATTERN/..."]
}

Session content:
%s`, string(content))
}
