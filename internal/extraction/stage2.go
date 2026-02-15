package extraction

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/mycelium-dev/mycelium/internal/config"
	"github.com/mycelium-dev/mycelium/internal/embedding"
)

// LLMClient is a lightweight LLM interface for rubric scoring.
type LLMClient interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// SkillQueryResult holds the nearest-neighbor result from a skill store query.
type SkillQueryResult struct {
	CosineSim float64
}

// SkillQuerier finds nearest-neighbor skills by embedding.
type SkillQuerier interface {
	QueryNearest(ctx context.Context, embedding []float32, libraryID string) (*SkillQueryResult, error)
}

// Stage2Scorer runs embedding novelty + structured rubric scoring.
type Stage2Scorer interface {
	Score(ctx context.Context, session SessionRecord, content []byte) (*Stage2Result, error)
}

type stage2Scorer struct {
	embedder embedding.Embedder
	querier  SkillQuerier
	llm      LLMClient
	minDist  float64
	maxDist  float64
	log      *slog.Logger
}

// NewStage2Scorer creates a Stage2Scorer from dependencies and config.
func NewStage2Scorer(
	embedder embedding.Embedder,
	querier SkillQuerier,
	llm LLMClient,
	cfg config.ExtractionConfig,
	log *slog.Logger,
) Stage2Scorer {
	return &stage2Scorer{
		embedder: embedder,
		querier:  querier,
		llm:      llm,
		minDist:  cfg.NoveltyMinDistance,
		maxDist:  cfg.NoveltyMaxDistance,
		log:      log,
	}
}

func (s *stage2Scorer) Score(ctx context.Context, session SessionRecord, content []byte) (*Stage2Result, error) {
	result := &Stage2Result{}

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

	// Normalize novelty into [0,1] within the valid range.
	mid := (s.minDist + s.maxDist) / 2
	halfRange := (s.maxDist - s.minDist) / 2
	if halfRange > 0 {
		result.NoveltyScore = 1.0 - abs(distance-mid)/halfRange
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

	var rubric rubricResponse
	if err := parseRubricResponse(resp, &rubric); err != nil {
		return nil, fmt.Errorf("stage2: parse rubric response: %w", err)
	}

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

// rubricScoresJSON maps the snake_case JSON from the LLM to QualityScores.
type rubricScoresJSON struct {
	ProblemSpecificity    int `json:"problem_specificity"`
	SolutionCompleteness  int `json:"solution_completeness"`
	ContextPortability    int `json:"context_portability"`
	ReasoningTransparency int `json:"reasoning_transparency"`
	TechnicalAccuracy     int `json:"technical_accuracy"`
	VerificationEvidence  int `json:"verification_evidence"`
	InnovationLevel       int `json:"innovation_level"`
}

func (r rubricScoresJSON) toQualityScores() QualityScores {
	return QualityScores{
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
	Scores   rubricScoresJSON `json:"scores"`
	Category SkillCategory    `json:"category"`
	Patterns []string         `json:"patterns"`
}

func compositeScore(q QualityScores) float64 {
	sum := q.ProblemSpecificity + q.SolutionCompleteness + q.ContextPortability +
		q.ReasoningTransparency + q.TechnicalAccuracy + q.VerificationEvidence +
		q.InnovationLevel
	return float64(sum) / 7.0
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func parseRubricResponse(resp string, out *rubricResponse) error {
	// Try to extract JSON from the response (LLM may wrap in markdown code blocks).
	cleaned := resp
	if idx := strings.Index(cleaned, "{"); idx >= 0 {
		cleaned = cleaned[idx:]
	}
	if idx := strings.LastIndex(cleaned, "}"); idx >= 0 {
		cleaned = cleaned[:idx+1]
	}

	if err := json.Unmarshal([]byte(cleaned), out); err != nil {
		return fmt.Errorf("invalid JSON in LLM response: %w", err)
	}
	return nil
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
