package extraction

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kinoko-dev/kinoko/internal/circuitbreaker"
	"github.com/kinoko-dev/kinoko/internal/config"
	"github.com/kinoko-dev/kinoko/internal/llm"
	"github.com/kinoko-dev/kinoko/internal/model"
)

// --- Mock LLM ---

type mockLLM3 struct {
	completeFn func(ctx context.Context, prompt string) (string, error)
}

func (m *mockLLM3) Complete(ctx context.Context, prompt string) (string, error) {
	return m.completeFn(ctx, prompt)
}

type mockLLMV2 struct {
	completeFn            func(ctx context.Context, prompt string) (string, error)
	completeWithTimeoutFn func(ctx context.Context, prompt string, timeout time.Duration) (*llm.LLMCompleteResult, error)
}

func (m *mockLLMV2) Complete(ctx context.Context, prompt string) (string, error) {
	return m.completeFn(ctx, prompt)
}

func (m *mockLLMV2) CompleteWithTimeout(ctx context.Context, prompt string, timeout time.Duration) (*llm.LLMCompleteResult, error) {
	if m.completeWithTimeoutFn != nil {
		return m.completeWithTimeoutFn(ctx, prompt, timeout)
	}
	resp, err := m.completeFn(ctx, prompt)
	if err != nil {
		return nil, err
	}
	return &llm.LLMCompleteResult{Content: resp, TokensIn: 100, TokensOut: 50}, nil
}

func s3okLLM(response string) llm.LLMClient {
	return &mockLLM3{completeFn: func(_ context.Context, _ string) (string, error) {
		return response, nil
	}}
}

func s3errLLM(err error) llm.LLMClient {
	return &mockLLM3{completeFn: func(_ context.Context, _ string) (string, error) {
		return "", err
	}}
}

func s3testConfig() config.ExtractionConfig {
	return config.ExtractionConfig{MinConfidence: 0.5}
}

func s3testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func s3testSession() model.SessionRecord {
	return model.SessionRecord{ID: "test-session-123", LibraryID: "test-lib"}
}

func passingStage2() *model.Stage2Result {
	return &model.Stage2Result{
		Passed: true, EmbeddingDistance: 0.55, NoveltyScore: 0.85,
		RubricScores: model.QualityScores{
			ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
			ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
			InnovationLevel: 3, CompositeScore: 3.55,
		},
		ClassifiedCategory: model.CategoryTactical,
		ClassifiedPatterns: []string{"FIX/Backend/DatabaseConnection"},
	}
}

func extractVerdictJSON() string {
	return `{
		"verdict": "extract",
		"reasoning": "This session demonstrates a clear problem-solution pattern with verified results.",
		"refined_scores": {
			"problem_specificity": 4,
			"solution_completeness": 4,
			"context_portability": 3,
			"reasoning_transparency": 4,
			"technical_accuracy": 4,
			"verification_evidence": 3,
			"innovation_level": 3
		},
		"confidence": 0.87,
		"reusable_pattern": true,
		"explicit_reasoning": true,
		"contradicts_best_practices": false,
		"skill_md": "---\nname: fix-db-timeout\nversion: 1\ncategory: FIX\ntags:\n  - databases/timeouts\n---\n\n# Fix DB Timeout\n\n## Problem\nConnection pool exhaustion.\n\n## Solution\nIncrease pool size and add retry logic.\n\n## Why It Works\nMore connections handle burst traffic.\n\n## Pitfalls\nToo many connections can overwhelm the DB.\n\n## References\nNone."
	}`
}

func extractVerdictJSONNoSkillMD() string {
	return `{
		"verdict": "extract",
		"reasoning": "This session demonstrates a clear problem-solution pattern with verified results.",
		"refined_scores": {
			"problem_specificity": 4, "solution_completeness": 4, "context_portability": 3,
			"reasoning_transparency": 4, "technical_accuracy": 4, "verification_evidence": 3,
			"innovation_level": 3
		},
		"confidence": 0.87, "reusable_pattern": true, "explicit_reasoning": true,
		"contradicts_best_practices": false
	}`
}

func rejectVerdictJSON() string {
	return `{
		"verdict": "reject",
		"reasoning": "Session is too trivial.",
		"refined_scores": {
			"problem_specificity": 2, "solution_completeness": 2, "context_portability": 1,
			"reasoning_transparency": 2, "technical_accuracy": 2, "verification_evidence": 1,
			"innovation_level": 1
		},
		"confidence": 0.92, "reusable_pattern": false, "explicit_reasoning": false,
		"contradicts_best_practices": false
	}`
}

func extractVerdictWithFlags(reusable, explicit, contradicts bool) string {
	return fmt.Sprintf(`{
		"verdict": "extract", "reasoning": "Good session.",
		"refined_scores": {
			"problem_specificity": 4, "solution_completeness": 4, "context_portability": 3,
			"reasoning_transparency": 4, "technical_accuracy": 4, "verification_evidence": 3,
			"innovation_level": 3
		},
		"confidence": 0.87, "reusable_pattern": %t, "explicit_reasoning": %t,
		"contradicts_best_practices": %t
	}`, reusable, explicit, contradicts)
}

func contradictoryVerdictJSON(verdict string, ps, sc, cp, rt, ta, ve, il int) string {
	return fmt.Sprintf(`{
		"verdict": %q, "reasoning": "Analysis complete.",
		"refined_scores": {
			"problem_specificity": %d, "solution_completeness": %d, "context_portability": %d,
			"reasoning_transparency": %d, "technical_accuracy": %d, "verification_evidence": %d,
			"innovation_level": %d
		},
		"confidence": 0.8, "reusable_pattern": false, "explicit_reasoning": false,
		"contradicts_best_practices": false
	}`, verdict, ps, sc, cp, rt, ta, ve, il)
}

func verdictWithInvalidScore(score int) string {
	return fmt.Sprintf(`{
		"verdict": "extract", "reasoning": "Good.",
		"refined_scores": {
			"problem_specificity": %d, "solution_completeness": 4, "context_portability": 3,
			"reasoning_transparency": 4, "technical_accuracy": 4, "verification_evidence": 3,
			"innovation_level": 3
		},
		"confidence": 0.87, "reusable_pattern": false, "explicit_reasoning": false,
		"contradicts_best_practices": false
	}`, score)
}

func verdictWithConfidence(conf float64) string {
	return fmt.Sprintf(`{
		"verdict": "extract", "reasoning": "Good.",
		"refined_scores": {
			"problem_specificity": 4, "solution_completeness": 4, "context_portability": 3,
			"reasoning_transparency": 4, "technical_accuracy": 4, "verification_evidence": 3,
			"innovation_level": 3
		},
		"confidence": %f, "reusable_pattern": false, "explicit_reasoning": false,
		"contradicts_best_practices": false
	}`, conf)
}

func verdictWithString(verdict string) string {
	return fmt.Sprintf(`{
		"verdict": %q, "reasoning": "Analysis.",
		"refined_scores": {
			"problem_specificity": 4, "solution_completeness": 4, "context_portability": 3,
			"reasoning_transparency": 4, "technical_accuracy": 4, "verification_evidence": 3,
			"innovation_level": 3
		},
		"confidence": 0.87, "reusable_pattern": false, "explicit_reasoning": false,
		"contradicts_best_practices": false
	}`, verdict)
}

func verdictWithEmptyReasoning() string {
	return `{
		"verdict": "extract", "reasoning": "",
		"refined_scores": {
			"problem_specificity": 4, "solution_completeness": 4, "context_portability": 3,
			"reasoning_transparency": 4, "technical_accuracy": 4, "verification_evidence": 3,
			"innovation_level": 3
		},
		"confidence": 0.87, "reusable_pattern": false, "explicit_reasoning": false,
		"contradicts_best_practices": false
	}`
}

func boolPtr(b bool) *bool { return &b }

func newTestCritic(l llm.LLMClient) Stage3Critic {
	c := NewStage3Critic(l, s3testConfig(), s3testLogger()).(*stage3Critic)
	c.sleep = func(d time.Duration) {}
	return c
}

type clockAdapter struct{ fn func() time.Time }

func (a clockAdapter) Now() time.Time { return a.fn() }

func newTestCriticWithClock(l llm.LLMClient, clock func() time.Time) *stage3Critic {
	c := NewStage3Critic(l, s3testConfig(), s3testLogger()).(*stage3Critic)
	c.clock = clock
	c.sleep = func(d time.Duration) {}
	cb, err := circuitbreaker.New(circuitbreaker.Config{
		Threshold: 5, BaseDuration: 5 * time.Minute, MaxDuration: 30 * time.Minute,
	}, clockAdapter{fn: clock})
	if err != nil {
		panic(err)
	}
	c.cb = cb
	return c
}
