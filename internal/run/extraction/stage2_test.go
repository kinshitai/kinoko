package extraction

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/kinoko-dev/kinoko/pkg/model"

	"github.com/kinoko-dev/kinoko/internal/shared/config"
)

// --- Mocks ---

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
	return config.ExtractionConfig{}
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
		llm         *mockLLM
		wantPassed  bool
		wantErr     bool
		checkResult func(t *testing.T, r *model.Stage2Result)
	}{
		{
			name:       "rubric fails minimum viable",
			llm:        okLLM(failRubricJSON()),
			wantPassed: false,
			checkResult: func(t *testing.T, r *model.Stage2Result) {
				if r.RubricScores.MinimumViable() {
					t.Error("expected rubric to fail MinimumViable")
				}
			},
		},
		{
			name:       "rubric passes",
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
			},
		},
		{
			name: "LLM returns bad JSON",
			llm: &mockLLM{
				completeFn: func(_ context.Context, _ string) (string, error) {
					return "this is not json at all", nil
				},
			},
			wantErr: true,
		},
		{
			name: "LLM error",
			llm: &mockLLM{
				completeFn: func(_ context.Context, _ string) (string, error) {
					return "", errors.New("rate limited")
				},
			},
			wantErr: true,
		},
		{
			name:       "LLM response wrapped in markdown code block",
			llm:        okLLM(fmt.Sprintf("```json\n%s\n```", goodRubricJSON())),
			wantPassed: true,
		},
		{
			name: "out-of-range rubric scores rejected",
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
			name: "zero score rejected",
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
			name: "negative score rejected",
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
			name: "invalid category defaults to tactical",
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
			name: "invalid patterns stripped",
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
			name: "all patterns invalid yields empty list",
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
			name:       "JSON with preamble containing braces",
			llm:        okLLM(fmt.Sprintf("Here's my analysis:\n```json\n%s\n```", goodRubricJSON())),
			wantPassed: true,
		},
		{
			name: "composite score is weighted not flat average",
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
			scorer := NewStage2Scorer(tt.llm, testConfig(), testLogger())
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
