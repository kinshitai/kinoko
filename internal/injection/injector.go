package injection

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/mycelium-dev/mycelium/internal/embedding"
	"github.com/mycelium-dev/mycelium/internal/extraction"
	"github.com/mycelium-dev/mycelium/internal/storage"
)

// Injector selects relevant skills to inject into an agent session.
type Injector interface {
	Inject(ctx context.Context, req extraction.InjectionRequest) (*extraction.InjectionResponse, error)
}

// injector implements Injector.
type injector struct {
	embedder embedding.Embedder
	store    storage.SkillStore
	llm      extraction.LLMClient
	log      *slog.Logger
}

// New creates an Injector. embedder may be nil (fallback mode permanently).
func New(
	embedder embedding.Embedder,
	store storage.SkillStore,
	llm extraction.LLMClient,
	log *slog.Logger,
) Injector {
	if log == nil {
		log = slog.Default()
	}
	return &injector{
		embedder: embedder,
		store:    store,
		llm:      llm,
		log:      log.With("component", "injector"),
	}
}

func (inj *injector) Inject(ctx context.Context, req extraction.InjectionRequest) (*extraction.InjectionResponse, error) {
	maxSkills := req.MaxSkills
	if maxSkills <= 0 {
		maxSkills = 3
	}

	// Step 1: Classify prompt.
	classification, err := inj.classifyPrompt(ctx, req.Prompt)
	if err != nil {
		inj.log.Warn("prompt classification failed, using empty patterns", "error", err)
		classification = extraction.PromptClassification{}
	}

	// Step 2: Compute prompt embedding (fallback if unavailable).
	var promptEmbedding []float32
	degraded := false
	if inj.embedder != nil {
		promptEmbedding, err = inj.embedder.Embed(ctx, req.Prompt)
		if err != nil {
			inj.log.Warn("embedding failed, falling back to pattern-only mode", "error", err)
			degraded = true
		}
	} else {
		degraded = true
	}

	if degraded {
		inj.log.Info("injection running in degraded mode (no embeddings)")
	}

	// Step 3: Query skill store.
	query := storage.SkillQuery{
		Patterns:   classification.Patterns,
		Embedding:  promptEmbedding,
		LibraryIDs: req.LibraryIDs,
		MinQuality: 0,
		MinDecay:   0,
		Limit:      0, // get all candidates, we re-rank
	}

	candidates, err := inj.store.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query skill store: %w", err)
	}

	if len(candidates) == 0 {
		return &extraction.InjectionResponse{
			Skills:         nil,
			Classification: classification,
		}, nil
	}

	// Step 4: Re-rank with appropriate weights.
	scored := make([]extraction.ScoredSkill, len(candidates))
	for i, c := range candidates {
		var composite float64
		if degraded {
			composite = 0.7*c.PatternOverlap + 0.3*c.HistoricalRate
		} else {
			composite = 0.5*c.PatternOverlap + 0.3*c.CosineSim + 0.2*c.HistoricalRate
		}
		scored[i] = extraction.ScoredSkill{
			Skill:          c.Skill,
			PatternOverlap: c.PatternOverlap,
			CosineSim:      c.CosineSim,
			HistoricalRate: c.HistoricalRate,
			CompositeScore: composite,
		}
	}

	// Sort descending by composite score.
	sortScoredSkills(scored)

	// Step 5: Limit to MaxSkills.
	if len(scored) > maxSkills {
		scored = scored[:maxSkills]
	}

	return &extraction.InjectionResponse{
		Skills:         scored,
		Classification: classification,
	}, nil
}

// classifyPrompt uses the LLM to classify the user prompt.
func (inj *injector) classifyPrompt(ctx context.Context, prompt string) (extraction.PromptClassification, error) {
	llmPrompt := buildClassificationPrompt(prompt)
	resp, err := inj.llm.Complete(ctx, llmPrompt)
	if err != nil {
		return extraction.PromptClassification{}, fmt.Errorf("llm classify: %w", err)
	}

	var result classificationResponse
	if err := parseClassificationResponse(resp, &result); err != nil {
		return extraction.PromptClassification{}, fmt.Errorf("parse classification: %w", err)
	}

	// Validate intent.
	result.Intent = strings.ToUpper(result.Intent)
	if !validIntents[result.Intent] {
		result.Intent = "BUILD" // default
	}

	// Validate patterns against taxonomy.
	var validPats []string
	for _, p := range result.Patterns {
		if extraction.ValidPattern(p) {
			validPats = append(validPats, p)
		}
	}
	if len(validPats) > 3 {
		validPats = validPats[:3]
	}

	return extraction.PromptClassification{
		Intent:   result.Intent,
		Domain:   result.Domain,
		Patterns: validPats,
	}, nil
}

type classificationResponse struct {
	Intent   string   `json:"intent"`
	Domain   string   `json:"domain"`
	Patterns []string `json:"patterns"`
}

func parseClassificationResponse(resp string, out *classificationResponse) error {
	if err := json.Unmarshal([]byte(resp), out); err == nil {
		return nil
	}
	// Try extracting JSON from markdown fences.
	if start := strings.Index(resp, "{"); start >= 0 {
		if end := strings.LastIndex(resp, "}"); end > start {
			if err := json.Unmarshal([]byte(resp[start:end+1]), out); err == nil {
				return nil
			}
		}
	}
	return fmt.Errorf("could not parse classification JSON from LLM response")
}

func buildClassificationPrompt(userPrompt string) string {
	return fmt.Sprintf(`Classify this user prompt. Respond with ONLY a JSON object.

Determine:
- intent: one of BUILD, FIX, OPTIMIZE, INTEGRATE, CONFIGURE, LEARN
- domain: brief domain description (e.g. "Go backend", "React frontend")
- patterns: 1-3 matching problem patterns from this taxonomy:
  BUILD/Frontend/ComponentDesign, BUILD/Frontend/StateManagement, BUILD/Backend/APIDesign, BUILD/Backend/DataModeling, BUILD/DevOps/CIPipeline, BUILD/DevOps/ContainerSetup, FIX/Frontend/RenderingBug, FIX/Backend/DatabaseConnection, FIX/Backend/AuthFlow, FIX/DevOps/DeploymentFailure, FIX/Performance/MemoryLeak, FIX/Performance/SlowQuery, OPTIMIZE/Performance/Caching, OPTIMIZE/Performance/BundleSize, OPTIMIZE/Backend/QueryOptimization, INTEGRATE/Backend/ThirdPartyAPI, INTEGRATE/DevOps/CloudService, CONFIGURE/DevOps/InfraAsCode, CONFIGURE/Security/AccessControl, LEARN/Data/DataPipeline

JSON format:
{"intent":"...","domain":"...","patterns":["..."]}

User prompt:
%s`, userPrompt)
}

var validIntents = map[string]bool{
	"BUILD":     true,
	"FIX":       true,
	"OPTIMIZE":  true,
	"INTEGRATE": true,
	"CONFIGURE": true,
	"LEARN":     true,
}

// sortScoredSkills sorts descending by CompositeScore.
func sortScoredSkills(skills []extraction.ScoredSkill) {
	for i := 1; i < len(skills); i++ {
		for j := i; j > 0 && skills[j].CompositeScore > skills[j-1].CompositeScore; j-- {
			skills[j], skills[j-1] = skills[j-1], skills[j]
		}
	}
}
