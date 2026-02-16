package extraction

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/model"

	"github.com/kinoko-dev/kinoko/internal/config"
)

// --- Mocks ---

type mockEmbedder struct {
	embedFn func(ctx context.Context, text string) ([]float32, error)
}

func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return m.embedFn(ctx, text)
}

func (m *mockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	var out [][]float32
	for _, t := range texts {
		v, err := m.embedFn(ctx, t)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (m *mockEmbedder) Dimensions() int { return 1536 }

type mockQuerier struct {
	queryFn func(ctx context.Context, embedding []float32, libraryID string) (*SkillQueryResult, error)
}

func (m *mockQuerier) QueryNearest(ctx context.Context, embedding []float32, libraryID string) (*SkillQueryResult, error) {
	return m.queryFn(ctx, embedding, libraryID)
}

type mockLLM struct {
	completeFn func(ctx context.Context, prompt string) (string, error)
}

func (m *mockLLM) Complete(ctx context.Context, prompt string) (string, error) {
	return m.completeFn(ctx, prompt)
}

// --- Helpers ---

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testConfig() config.ExtractionConfig {
	return config.ExtractionConfig{
		NoveltyMinDistance: 0.15,
		NoveltyMaxDistance: 0.95,
	}
}

func testSession() model.SessionRecord {
	return model.SessionRecord{
		ID:        "sess-1",
		LibraryID: "default",
	}
}

// goodRubricJSON returns a passing rubric response.
func goodRubricJSON() string {
	return `{
		"scores": {
			"problem_specificity": 4,
			"solution_completeness": 4,
			"context_portability": 3,
			"reasoning_transparency": 3,
			"technical_accuracy": 4,
			"verification_evidence": 3,
			"innovation_level": 3
		},
		"category": "tactical",
		"patterns": ["FIX/Backend/DatabaseConnection"]
	}`
}

// failRubricJSON returns a rubric response that fails MinimumViable.
func failRubricJSON() string {
	return `{
		"scores": {
			"problem_specificity": 2,
			"solution_completeness": 2,
			"context_portability": 1,
			"reasoning_transparency": 1,
			"technical_accuracy": 2,
			"verification_evidence": 1,
			"innovation_level": 1
		},
		"category": "contextual",
		"patterns": ["LEARN/Data/DataPipeline"]
	}`
}

// querierWithSimilarity returns a mock querier with the given cosine similarity.
func querierWithSimilarity(sim float64) *mockQuerier {
	return &mockQuerier{
		queryFn: func(_ context.Context, _ []float32, _ string) (*SkillQueryResult, error) {
			return &SkillQueryResult{CosineSim: sim}, nil
		},
	}
}

func emptyQuerier() *mockQuerier {
	return &mockQuerier{
		queryFn: func(_ context.Context, _ []float32, _ string) (*SkillQueryResult, error) {
			return nil, nil
		},
	}
}

func okEmbedder() *mockEmbedder {
	return &mockEmbedder{
		embedFn: func(_ context.Context, _ string) ([]float32, error) {
			return make([]float32, 1536), nil
		},
	}
}

func okLLM(json string) *mockLLM {
	return &mockLLM{
		completeFn: func(_ context.Context, _ string) (string, error) {
			return json, nil
		},
	}
}

// --- Tests ---

func TestStage2Scorer(t *testing.T) {
	tests := []struct {
		name        string
		embedder    *mockEmbedder
		querier     *mockQuerier
		llm         *mockLLM
		wantPassed  bool
		wantErr     bool
		checkResult func(t *testing.T, r *model.Stage2Result)
	}{
		{
			name:       "novelty too low (too similar)",
			embedder:   okEmbedder(),
			querier:    querierWithSimilarity(0.90), // distance = 0.10 < 0.15
			llm:        okLLM(goodRubricJSON()),
			wantPassed: false,
			checkResult: func(t *testing.T, r *model.Stage2Result) {
				if r.EmbeddingDistance >= 0.15 {
					t.Errorf("expected distance < 0.15, got %f", r.EmbeddingDistance)
				}
				if r.Reason == "" {
					t.Error("expected rejection reason")
				}
			},
		},
		{
			name:       "novelty too high (too unrelated)",
			embedder:   okEmbedder(),
			querier:    querierWithSimilarity(0.02), // distance = 0.98 > 0.95
			llm:        okLLM(goodRubricJSON()),
			wantPassed: false,
			checkResult: func(t *testing.T, r *model.Stage2Result) {
				if r.EmbeddingDistance <= 0.95 {
					t.Errorf("expected distance > 0.95, got %f", r.EmbeddingDistance)
				}
			},
		},
		{
			name:       "novelty in range but rubric fails",
			embedder:   okEmbedder(),
			querier:    querierWithSimilarity(0.50), // distance = 0.50, in range
			llm:        okLLM(failRubricJSON()),
			wantPassed: false,
			checkResult: func(t *testing.T, r *model.Stage2Result) {
				if r.RubricScores.MinimumViable() {
					t.Error("expected rubric to fail MinimumViable")
				}
			},
		},
		{
			name:       "full pass",
			embedder:   okEmbedder(),
			querier:    querierWithSimilarity(0.50), // distance = 0.50
			llm:        okLLM(goodRubricJSON()),
			wantPassed: true,
			checkResult: func(t *testing.T, r *model.Stage2Result) {
				if r.ClassifiedCategory != model.CategoryTactical {
					t.Errorf("expected tactical, got %s", r.ClassifiedCategory)
				}
				if len(r.ClassifiedPatterns) != 1 || r.ClassifiedPatterns[0] != "FIX/Backend/DatabaseConnection" {
					t.Errorf("unexpected patterns: %v", r.ClassifiedPatterns)
				}
				if r.RubricScores.CompositeScore == 0 {
					t.Error("expected non-zero composite score")
				}
				if r.NoveltyScore <= 0 {
					t.Errorf("expected positive novelty score, got %f", r.NoveltyScore)
				}
			},
		},
		{
			name:       "full pass with no existing skills",
			embedder:   okEmbedder(),
			querier:    emptyQuerier(),
			llm:        okLLM(goodRubricJSON()),
			wantPassed: false, // distance=1.0 > maxDist=0.95
			checkResult: func(t *testing.T, r *model.Stage2Result) {
				if r.EmbeddingDistance != 1.0 {
					t.Errorf("expected distance 1.0, got %f", r.EmbeddingDistance)
				}
			},
		},
		{
			name:     "LLM returns bad JSON",
			embedder: okEmbedder(),
			querier:  querierWithSimilarity(0.50),
			llm: &mockLLM{
				completeFn: func(_ context.Context, _ string) (string, error) {
					return "this is not json at all", nil
				},
			},
			wantErr: true,
		},
		{
			name: "embedder error",
			embedder: &mockEmbedder{
				embedFn: func(_ context.Context, _ string) ([]float32, error) {
					return nil, errors.New("embedding service unavailable")
				},
			},
			querier: querierWithSimilarity(0.50),
			llm:     okLLM(goodRubricJSON()),
			wantErr: true,
		},
		{
			name:     "store error",
			embedder: okEmbedder(),
			querier: &mockQuerier{
				queryFn: func(_ context.Context, _ []float32, _ string) (*SkillQueryResult, error) {
					return nil, errors.New("db locked")
				},
			},
			llm:     okLLM(goodRubricJSON()),
			wantErr: true,
		},
		{
			name:     "LLM error",
			embedder: okEmbedder(),
			querier:  querierWithSimilarity(0.50),
			llm: &mockLLM{
				completeFn: func(_ context.Context, _ string) (string, error) {
					return "", errors.New("rate limited")
				},
			},
			wantErr: true,
		},
		{
			name:       "LLM response wrapped in markdown code block",
			embedder:   okEmbedder(),
			querier:    querierWithSimilarity(0.50),
			llm:        okLLM(fmt.Sprintf("```json\n%s\n```", goodRubricJSON())),
			wantPassed: true,
		},
		{
			name:       "boundary: distance exactly at min",
			embedder:   okEmbedder(),
			querier:    querierWithSimilarity(0.85), // distance = 0.15 == minDist
			llm:        okLLM(goodRubricJSON()),
			wantPassed: true,
		},
		{
			name:       "boundary: distance exactly at max",
			embedder:   okEmbedder(),
			querier:    querierWithSimilarity(0.05), // distance = 0.95 == maxDist
			llm:        okLLM(goodRubricJSON()),
			wantPassed: true,
		},
		{
			name:     "P1.1: out-of-range rubric scores rejected",
			embedder: okEmbedder(),
			querier:  querierWithSimilarity(0.50),
			llm: okLLM(`{
				"scores": {
					"problem_specificity": 47,
					"solution_completeness": 4,
					"context_portability": 3,
					"reasoning_transparency": 3,
					"technical_accuracy": 4,
					"verification_evidence": 3,
					"innovation_level": 3
				},
				"category": "tactical",
				"patterns": ["FIX/Backend/DatabaseConnection"]
			}`),
			wantErr: true,
		},
		{
			name:     "P1.1: zero score rejected",
			embedder: okEmbedder(),
			querier:  querierWithSimilarity(0.50),
			llm: okLLM(`{
				"scores": {
					"problem_specificity": 0,
					"solution_completeness": 4,
					"context_portability": 3,
					"reasoning_transparency": 3,
					"technical_accuracy": 4,
					"verification_evidence": 3,
					"innovation_level": 3
				},
				"category": "tactical",
				"patterns": ["FIX/Backend/DatabaseConnection"]
			}`),
			wantErr: true,
		},
		{
			name:     "P1.1: negative score rejected",
			embedder: okEmbedder(),
			querier:  querierWithSimilarity(0.50),
			llm: okLLM(`{
				"scores": {
					"problem_specificity": 4,
					"solution_completeness": -3,
					"context_portability": 3,
					"reasoning_transparency": 3,
					"technical_accuracy": 4,
					"verification_evidence": 3,
					"innovation_level": 3
				},
				"category": "tactical",
				"patterns": ["FIX/Backend/DatabaseConnection"]
			}`),
			wantErr: true,
		},
		{
			name:     "P1.2: invalid category defaults to tactical",
			embedder: okEmbedder(),
			querier:  querierWithSimilarity(0.50),
			llm: okLLM(`{
				"scores": {
					"problem_specificity": 4,
					"solution_completeness": 4,
					"context_portability": 3,
					"reasoning_transparency": 3,
					"technical_accuracy": 4,
					"verification_evidence": 3,
					"innovation_level": 3
				},
				"category": "strategic",
				"patterns": ["FIX/Backend/DatabaseConnection"]
			}`),
			wantPassed: true,
			checkResult: func(t *testing.T, r *model.Stage2Result) {
				if r.ClassifiedCategory != model.CategoryTactical {
					t.Errorf("expected tactical (default), got %s", r.ClassifiedCategory)
				}
			},
		},
		{
			name:     "P1.3: invalid patterns stripped",
			embedder: okEmbedder(),
			querier:  querierWithSimilarity(0.50),
			llm: okLLM(`{
				"scores": {
					"problem_specificity": 4,
					"solution_completeness": 4,
					"context_portability": 3,
					"reasoning_transparency": 3,
					"technical_accuracy": 4,
					"verification_evidence": 3,
					"innovation_level": 3
				},
				"category": "tactical",
				"patterns": ["YOLO/Backend/Everything", "FIX/Backend/DatabaseConnection", "MADE_UP"]
			}`),
			wantPassed: true,
			checkResult: func(t *testing.T, r *model.Stage2Result) {
				if len(r.ClassifiedPatterns) != 1 || r.ClassifiedPatterns[0] != "FIX/Backend/DatabaseConnection" {
					t.Errorf("expected only valid pattern, got %v", r.ClassifiedPatterns)
				}
			},
		},
		{
			name:     "P1.3: all patterns invalid yields empty list",
			embedder: okEmbedder(),
			querier:  querierWithSimilarity(0.50),
			llm: okLLM(`{
				"scores": {
					"problem_specificity": 4,
					"solution_completeness": 4,
					"context_portability": 3,
					"reasoning_transparency": 3,
					"technical_accuracy": 4,
					"verification_evidence": 3,
					"innovation_level": 3
				},
				"category": "tactical",
				"patterns": ["FAKE/Pattern/One"]
			}`),
			wantPassed: true,
			checkResult: func(t *testing.T, r *model.Stage2Result) {
				if len(r.ClassifiedPatterns) != 0 {
					t.Errorf("expected empty patterns, got %v", r.ClassifiedPatterns)
				}
			},
		},
		{
			name:       "P2.1: JSON with preamble containing braces",
			embedder:   okEmbedder(),
			querier:    querierWithSimilarity(0.50),
			llm:        okLLM(fmt.Sprintf("Here's my analysis:\n```json\n%s\n```", goodRubricJSON())),
			wantPassed: true,
		},
		{
			name:       "P2.2: novelty score at boundary is nonzero",
			embedder:   okEmbedder(),
			querier:    querierWithSimilarity(0.85), // distance = 0.15 = minDist
			llm:        okLLM(goodRubricJSON()),
			wantPassed: true,
			checkResult: func(t *testing.T, r *model.Stage2Result) {
				if r.NoveltyScore < 0.05 {
					t.Errorf("expected novelty score >= 0.05 at boundary, got %f", r.NoveltyScore)
				}
			},
		},
		{
			name:       "P2.2: novelty score at midpoint is 1.0",
			embedder:   okEmbedder(),
			querier:    querierWithSimilarity(0.45), // distance = 0.55 ≈ midpoint of [0.15, 0.95]
			llm:        okLLM(goodRubricJSON()),
			wantPassed: true,
			checkResult: func(t *testing.T, r *model.Stage2Result) {
				if r.NoveltyScore < 0.95 {
					t.Errorf("expected novelty score near 1.0 at midpoint, got %f", r.NoveltyScore)
				}
			},
		},
		{
			name:     "P2.3: composite score is weighted not flat average",
			embedder: okEmbedder(),
			querier:  querierWithSimilarity(0.50),
			llm: okLLM(`{
				"scores": {
					"problem_specificity": 5,
					"solution_completeness": 5,
					"context_portability": 1,
					"reasoning_transparency": 1,
					"technical_accuracy": 5,
					"verification_evidence": 1,
					"innovation_level": 1
				},
				"category": "tactical",
				"patterns": ["BUILD/Backend/APIDesign"]
			}`),
			wantPassed: true,
			checkResult: func(t *testing.T, r *model.Stage2Result) {
				// Weighted: 5*0.15 + 5*0.20 + 1*0.15 + 1*0.10 + 5*0.20 + 1*0.10 + 1*0.10 = 3.20
				// Flat avg: 19/7 ≈ 2.714
				expected := 3.20
				if diff := r.RubricScores.CompositeScore - expected; diff < -0.01 || diff > 0.01 {
					t.Errorf("expected weighted composite %.2f, got %.2f", expected, r.RubricScores.CompositeScore)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scorer := NewStage2Scorer(tt.embedder, tt.querier, tt.llm, testConfig(), testLogger())
			result, err := scorer.Score(context.Background(), testSession(), []byte("session content"))

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Passed != tt.wantPassed {
				t.Errorf("Passed = %v, want %v (reason: %s)", result.Passed, tt.wantPassed, result.Reason)
			}

			if tt.checkResult != nil {
				tt.checkResult(t, result)
			}
		})
	}
}
