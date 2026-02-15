// Package injection selects and ranks skills for injection into agent sessions.
// It classifies prompts via LLM, queries the skill store by pattern and embedding
// similarity, and supports degraded (pattern-only) mode when embeddings are
// unavailable. A/B testing is provided by the ABInjector decorator.
package injection

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/mycelium-dev/mycelium/internal/embedding"
	"github.com/mycelium-dev/mycelium/internal/extraction"
	"github.com/mycelium-dev/mycelium/internal/storage"
)

// maxPatterns caps the number of classified patterns forwarded to the query.
const maxPatterns = 3

// defaultMaxSkills is the fallback when InjectionRequest.MaxSkills <= 0.
const defaultMaxSkills = 3

// defaultMinDecay filters out skills at or below the deprecation threshold (§5.5).
const defaultMinDecay = 0.05

// defaultCandidateLimit bounds the number of rows loaded from the store.
const defaultCandidateLimit = 50

// InjectionEventWriter persists injection events for the feedback loop.
type InjectionEventWriter interface {
	WriteInjectionEvent(ctx context.Context, ev storage.InjectionEventRecord) error
}

// Injector selects relevant skills to inject into an agent session.
type Injector interface {
	Inject(ctx context.Context, req extraction.InjectionRequest) (*extraction.InjectionResponse, error)
}

// injector implements Injector.
type injector struct {
	embedder    embedding.Embedder
	store       storage.SkillStore
	llm         extraction.LLMClient
	eventWriter InjectionEventWriter
	log         *slog.Logger
}

// New creates an Injector. embedder may be nil (fallback mode permanently).
// eventWriter may be nil (injection events will not be logged — not recommended).
func New(
	embedder embedding.Embedder,
	store storage.SkillStore,
	llm extraction.LLMClient,
	eventWriter InjectionEventWriter,
	log *slog.Logger,
) Injector {
	if log == nil {
		log = slog.Default()
	}
	return &injector{
		embedder:    embedder,
		store:       store,
		llm:         llm,
		eventWriter: eventWriter,
		log:         log.With("component", "injector"),
	}
}

func (inj *injector) Inject(ctx context.Context, req extraction.InjectionRequest) (*extraction.InjectionResponse, error) {
	maxSkills := req.MaxSkills
	if maxSkills <= 0 {
		maxSkills = defaultMaxSkills
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

	// Step 3: Query skill store with bounded candidates and dead-skill filter.
	query := storage.SkillQuery{
		Patterns:   classification.Patterns,
		Embedding:  promptEmbedding,
		LibraryIDs: req.LibraryIDs,
		MinQuality: 0,
		MinDecay:   defaultMinDecay,
		Limit:      defaultCandidateLimit,
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

	// Step 4: Re-rank in degraded mode; otherwise use store's composite.
	if degraded {
		for i := range candidates {
			c := &candidates[i]
			c.CompositeScore = 0.7*c.PatternOverlap + 0.3*c.HistoricalRate
		}
		slices.SortFunc(candidates, func(a, b storage.ScoredSkill) int {
			if a.CompositeScore > b.CompositeScore {
				return -1
			}
			if a.CompositeScore < b.CompositeScore {
				return 1
			}
			return 0
		})
	}
	// In normal mode the store already sorted by composite — no recomputation needed.

	// Step 5: Limit to MaxSkills.
	if len(candidates) > maxSkills {
		candidates = candidates[:maxSkills]
	}

	// Step 6: Build response and write injection events.
	now := time.Now().UTC()
	skills := make([]extraction.InjectedSkill, len(candidates))
	for i, c := range candidates {
		skills[i] = extraction.InjectedSkill{
			SkillID:        c.Skill.ID,
			PatternOverlap: c.PatternOverlap,
			CosineSim:      c.CosineSim,
			HistoricalRate: c.HistoricalRate,
			CompositeScore: c.CompositeScore,
			RankPosition:   i + 1,
		}

		// Write injection event for the feedback loop (C3).
		if inj.eventWriter != nil && req.SessionID != "" {
			ev := storage.InjectionEventRecord{
				ID:             fmt.Sprintf("%s-%s-%d", req.SessionID, c.Skill.ID, i),
				SessionID:      req.SessionID,
				SkillID:        c.Skill.ID,
				RankPosition:   i + 1,
				MatchScore:     c.CompositeScore,
				PatternOverlap: c.PatternOverlap,
				CosineSim:      c.CosineSim,
				HistoricalRate: c.HistoricalRate,
				InjectedAt:     now,
			}
			if writeErr := inj.eventWriter.WriteInjectionEvent(ctx, ev); writeErr != nil {
				inj.log.Error("failed to write injection event", "skill_id", c.Skill.ID, "error", writeErr)
				// Non-fatal: don't break injection because logging failed.
			}
		}
	}

	return &extraction.InjectionResponse{
		Skills:         skills,
		Classification: classification,
	}, nil
}

// classifyPrompt uses the LLM to classify the user prompt.
func (inj *injector) classifyPrompt(ctx context.Context, prompt string) (extraction.PromptClassification, error) {
	if prompt == "" {
		return extraction.PromptClassification{}, fmt.Errorf("empty prompt")
	}

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

	// Validate domain (M5).
	result.Domain = extraction.ValidateDomain(result.Domain)

	// Validate patterns against taxonomy (C1).
	var validPats []string
	for _, p := range result.Patterns {
		if extraction.ValidPattern(p) {
			validPats = append(validPats, p)
		}
	}
	if len(validPats) > maxPatterns {
		validPats = validPats[:maxPatterns]
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
	taxonomyStr := strings.Join(extraction.Taxonomy, ", ")
	return fmt.Sprintf(`Classify this user prompt. Respond with ONLY a JSON object.

Determine:
- intent: one of BUILD, FIX, OPTIMIZE, INTEGRATE, CONFIGURE, LEARN
- domain: one of Frontend, Backend, DevOps, Data, Security, Performance
- patterns: 1-3 matching problem patterns from this taxonomy:
  %s

JSON format:
{"intent":"...","domain":"...","patterns":["..."]}

User prompt:
%s`, taxonomyStr, userPrompt)
}

var validIntents = map[string]bool{
	"BUILD":     true,
	"FIX":       true,
	"OPTIMIZE":  true,
	"INTEGRATE": true,
	"CONFIGURE": true,
	"LEARN":     true,
}
