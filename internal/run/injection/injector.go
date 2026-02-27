// Package injection selects and ranks skills for injection into agent sessions.
// It classifies prompts via LLM, queries the skill store by pattern and embedding
// similarity, and supports degraded (pattern-only) mode when embeddings are
// unavailable. A/B testing is provided by the ABInjector decorator.
package injection

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/kinoko-dev/kinoko/internal/run/extraction"
	"github.com/kinoko-dev/kinoko/internal/run/llm"
	"github.com/kinoko-dev/kinoko/internal/run/llmutil"
	"github.com/kinoko-dev/kinoko/pkg/model"
)

// maxPatterns caps the number of classified patterns forwarded to the query.
const maxPatterns = 3

// defaultMaxSkills is the fallback when InjectionRequest.MaxSkills <= 0.
const defaultMaxSkills = 3

// defaultCandidateLimit bounds the number of rows loaded from the store.
const defaultCandidateLimit = 50

// InjectionEventWriter persists injection events for the feedback loop.
type InjectionEventWriter interface {
	WriteInjectionEvent(ctx context.Context, ev model.InjectionEventRecord) error
}

// Injector selects relevant skills to inject into an agent session.
type Injector interface {
	Inject(ctx context.Context, req model.InjectionRequest) (*model.InjectionResponse, error)
}

// injector implements Injector.
type injector struct {
	embedder    model.Embedder
	store       model.SkillStore
	llm         llm.LLMClient
	eventWriter InjectionEventWriter
	log         *slog.Logger
}

// New creates an Injector. embedder may be nil (fallback mode permanently).
// eventWriter may be nil (injection events will not be logged — not recommended).
func New(
	embedder model.Embedder,
	store model.SkillStore,
	llm llm.LLMClient,
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

func (inj *injector) Inject(ctx context.Context, req model.InjectionRequest) (*model.InjectionResponse, error) {
	maxSkills := req.MaxSkills
	if maxSkills <= 0 {
		maxSkills = defaultMaxSkills
	}

	// Step 1: Classify prompt.
	classification, err := inj.classifyPrompt(ctx, req.Prompt)
	if err != nil {
		inj.log.Warn("prompt classification failed, using empty patterns", "error", err)
		classification = model.PromptClassification{}
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
	query := model.SkillQuery{
		Patterns:   classification.Patterns,
		Embedding:  promptEmbedding,
		LibraryIDs: req.LibraryIDs,
		MinQuality: 0,
		Limit:      defaultCandidateLimit,
	}

	candidates, err := inj.store.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query skill store: %w", err)
	}

	if len(candidates) == 0 {
		return &model.InjectionResponse{
			Skills:         nil,
			Classification: classification,
		}, nil
	}

	// Step 4: Client-side ranking from raw signals.
	// Server returns raw pattern_overlap and cosine_sim. Client ranks locally.
	// TODO(#89): Add local behavioral data (injection count, success) from .kinoko/ files.
	slices.SortFunc(candidates, func(a, b model.ScoredSkill) int {
		scoreA := model.RelevanceScore(a.PatternOverlap, a.CosineSim)
		scoreB := model.RelevanceScore(b.PatternOverlap, b.CosineSim)
		if scoreA > scoreB {
			return -1
		}
		if scoreA < scoreB {
			return 1
		}
		return 0
	})

	// Step 5: Limit to MaxSkills.
	if len(candidates) > maxSkills {
		candidates = candidates[:maxSkills]
	}

	// Step 6: Build response and write injection events.
	now := time.Now().UTC()
	skills := make([]model.InjectedSkill, len(candidates))
	for i, c := range candidates {
		localScore := model.RelevanceScore(c.PatternOverlap, c.CosineSim)
		skills[i] = model.InjectedSkill{
			SkillID:        c.Skill.ID,
			PatternOverlap: c.PatternOverlap,
			CosineSim:      c.CosineSim,
			HistoricalRate: 0, // TODO(#89): populate from .kinoko/ local files
			CompositeScore: localScore,
			RankPosition:   i + 1,
		}

		// Write injection event for the feedback loop (C3).
		if inj.eventWriter != nil && req.SessionID != "" {
			ev := model.InjectionEventRecord{
				ID:             fmt.Sprintf("%s-%s-%d", req.SessionID, c.Skill.ID, i),
				SessionID:      req.SessionID,
				SkillID:        c.Skill.ID,
				RankPosition:   i + 1,
				MatchScore:     localScore,
				PatternOverlap: c.PatternOverlap,
				CosineSim:      c.CosineSim,
				HistoricalRate: 0, // TODO(#89): populate from .kinoko/ local files
				InjectedAt:     now,
			}
			if writeErr := inj.eventWriter.WriteInjectionEvent(ctx, ev); writeErr != nil {
				inj.log.Error("failed to write injection event", "skill_id", c.Skill.ID, "error", writeErr)
				// Non-fatal: don't break injection because logging failed.
			}
		}
	}

	return &model.InjectionResponse{
		Skills:         skills,
		Classification: classification,
	}, nil
}

// classifyPrompt uses the LLM to classify the user prompt.
func (inj *injector) classifyPrompt(ctx context.Context, prompt string) (model.PromptClassification, error) {
	if prompt == "" {
		return model.PromptClassification{}, fmt.Errorf("empty prompt")
	}

	llmPrompt := buildClassificationPrompt(prompt)
	resp, err := inj.llm.Complete(ctx, llmPrompt)
	if err != nil {
		return model.PromptClassification{}, fmt.Errorf("llm classify: %w", err)
	}

	result, err := llmutil.ExtractJSON[classificationResponse](resp)
	if err != nil {
		return model.PromptClassification{}, fmt.Errorf("parse classification: %w", err)
	}

	// Validate intent.
	result.Intent = strings.ToUpper(result.Intent)
	if !validIntents[result.Intent] {
		result.Intent = "BUILD" // default
	}

	// Validate domain (M5).
	result.Domain = model.ValidateDomain(result.Domain)

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

	return model.PromptClassification{
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
