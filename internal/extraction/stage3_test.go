package extraction

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/mycelium-dev/mycelium/internal/config"
)

// --- Mock LLM ---

type mockLLM3 struct {
	completeFn func(ctx context.Context, prompt string) (string, error)
}

func (m *mockLLM3) Complete(ctx context.Context, prompt string) (string, error) {
	return m.completeFn(ctx, prompt)
}

func s3okLLM(response string) LLMClient {
	return &mockLLM3{completeFn: func(_ context.Context, _ string) (string, error) {
		return response, nil
	}}
}

func s3errLLM(err error) LLMClient {
	return &mockLLM3{completeFn: func(_ context.Context, _ string) (string, error) {
		return "", err
	}}
}

// --- Test fixtures ---

func s3testConfig() config.ExtractionConfig {
	return config.ExtractionConfig{MinConfidence: 0.5}
}

func s3testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func s3testSession() SessionRecord {
	return SessionRecord{
		ID:        "test-session-123",
		LibraryID: "test-lib",
	}
}

func passingStage2() *Stage2Result {
	return &Stage2Result{
		Passed:            true,
		EmbeddingDistance:  0.55,
		NoveltyScore:      0.85,
		RubricScores: QualityScores{
			ProblemSpecificity:    4,
			SolutionCompleteness:  4,
			ContextPortability:    3,
			ReasoningTransparency: 3,
			TechnicalAccuracy:     4,
			VerificationEvidence:  3,
			InnovationLevel:       3,
			CompositeScore:        3.55,
		},
		ClassifiedCategory: CategoryTactical,
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
		"contradicts_best_practices": false
	}`
}

func rejectVerdictJSON() string {
	return `{
		"verdict": "reject",
		"reasoning": "Session is too trivial.",
		"refined_scores": {
			"problem_specificity": 2,
			"solution_completeness": 2,
			"context_portability": 1,
			"reasoning_transparency": 2,
			"technical_accuracy": 2,
			"verification_evidence": 1,
			"innovation_level": 1
		},
		"confidence": 0.92,
		"reusable_pattern": false,
		"explicit_reasoning": false,
		"contradicts_best_practices": false
	}`
}

func extractVerdictWithFlags(reusable, explicit, contradicts bool) string {
	return fmt.Sprintf(`{
		"verdict": "extract",
		"reasoning": "Good session.",
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
		"reusable_pattern": %t,
		"explicit_reasoning": %t,
		"contradicts_best_practices": %t
	}`, reusable, explicit, contradicts)
}

func contradictoryVerdictJSON(verdict string, ps, sc, cp, rt, ta, ve, il int) string {
	return fmt.Sprintf(`{
		"verdict": %q,
		"reasoning": "Analysis complete.",
		"refined_scores": {
			"problem_specificity": %d,
			"solution_completeness": %d,
			"context_portability": %d,
			"reasoning_transparency": %d,
			"technical_accuracy": %d,
			"verification_evidence": %d,
			"innovation_level": %d
		},
		"confidence": 0.8,
		"reusable_pattern": false,
		"explicit_reasoning": false,
		"contradicts_best_practices": false
	}`, verdict, ps, sc, cp, rt, ta, ve, il)
}

func verdictWithInvalidScore(score int) string {
	return fmt.Sprintf(`{
		"verdict": "extract",
		"reasoning": "Good.",
		"refined_scores": {
			"problem_specificity": %d,
			"solution_completeness": 4,
			"context_portability": 3,
			"reasoning_transparency": 4,
			"technical_accuracy": 4,
			"verification_evidence": 3,
			"innovation_level": 3
		},
		"confidence": 0.87,
		"reusable_pattern": false,
		"explicit_reasoning": false,
		"contradicts_best_practices": false
	}`, score)
}

func verdictWithConfidence(conf float64) string {
	return fmt.Sprintf(`{
		"verdict": "extract",
		"reasoning": "Good.",
		"refined_scores": {
			"problem_specificity": 4,
			"solution_completeness": 4,
			"context_portability": 3,
			"reasoning_transparency": 4,
			"technical_accuracy": 4,
			"verification_evidence": 3,
			"innovation_level": 3
		},
		"confidence": %f,
		"reusable_pattern": false,
		"explicit_reasoning": false,
		"contradicts_best_practices": false
	}`, conf)
}

func verdictWithString(verdict string) string {
	return fmt.Sprintf(`{
		"verdict": %q,
		"reasoning": "Analysis.",
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
		"reusable_pattern": false,
		"explicit_reasoning": false,
		"contradicts_best_practices": false
	}`, verdict)
}

func verdictWithEmptyReasoning() string {
	return `{
		"verdict": "extract",
		"reasoning": "",
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
		"reusable_pattern": false,
		"explicit_reasoning": false,
		"contradicts_best_practices": false
	}`
}

// --- Core Verdict Tests ---

func TestStage3Critic(t *testing.T) {
	tests := []struct {
		name        string
		llmResponse string
		llmErr      error
		stage2      *Stage2Result
		content     []byte
		wantPassed  *bool // nil = don't check
		wantVerdict string
		wantErr     bool
		checkResult func(t *testing.T, r *Stage3Result)
	}{
		{
			name:        "extract verdict with high scores",
			llmResponse: extractVerdictJSON(),
			stage2:      passingStage2(),
			content:     []byte("meaningful session content"),
			wantPassed:  boolPtr(true),
			wantVerdict: "extract",
		},
		{
			name:        "reject verdict with low scores",
			llmResponse: rejectVerdictJSON(),
			stage2:      passingStage2(),
			content:     []byte("trivial session"),
			wantPassed:  boolPtr(false),
			wantVerdict: "reject",
		},
		{
			name:        "extract with flags",
			llmResponse: extractVerdictWithFlags(true, true, false),
			stage2:      passingStage2(),
			content:     []byte("session"),
			wantPassed:  boolPtr(true),
			checkResult: func(t *testing.T, r *Stage3Result) {
				if !r.ReusablePattern {
					t.Error("expected ReusablePattern=true")
				}
				if !r.ExplicitReasoning {
					t.Error("expected ExplicitReasoning=true")
				}
				if r.ContradictsBestPractices {
					t.Error("expected ContradictsBestPractices=false")
				}
			},
		},
		// JSON parsing strategies
		{
			name:        "response wrapped in json block",
			llmResponse: "```json\n" + extractVerdictJSON() + "\n```",
			stage2:      passingStage2(),
			content:     []byte("session"),
			wantPassed:  boolPtr(true),
			wantVerdict: "extract",
		},
		{
			name:        "response with preamble text",
			llmResponse: "Here is my analysis:\n\n" + extractVerdictJSON(),
			stage2:      passingStage2(),
			content:     []byte("session"),
			wantPassed:  boolPtr(true),
			wantVerdict: "extract",
		},
		{
			name:        "malformed JSON treated as rejection",
			llmResponse: "I think this is good {broken",
			stage2:      passingStage2(),
			content:     []byte("session"),
			wantPassed:  boolPtr(false),
			checkResult: func(t *testing.T, r *Stage3Result) {
				if r.CriticVerdict != "reject" {
					t.Errorf("expected reject, got %s", r.CriticVerdict)
				}
			},
		},
		{
			name:        "empty LLM response",
			llmResponse: "",
			stage2:      passingStage2(),
			content:     []byte("session"),
			wantPassed:  boolPtr(false),
		},
		{
			name:        "valid JSON missing required fields",
			llmResponse: `{"verdict": "extract"}`,
			stage2:      passingStage2(),
			content:     []byte("session"),
			wantPassed:  boolPtr(false), // scores will be 0 → invalid
		},
		// Contradictions
		{
			name:        "verdict=extract but all scores are 1",
			llmResponse: contradictoryVerdictJSON("extract", 1, 1, 1, 1, 1, 1, 1),
			stage2:      passingStage2(),
			content:     []byte("session"),
			wantPassed:  boolPtr(false),
			checkResult: func(t *testing.T, r *Stage3Result) {
				if r.Passed {
					t.Error("should not pass with all-1 scores")
				}
			},
		},
		{
			name:        "verdict=reject but all scores are 5",
			llmResponse: contradictoryVerdictJSON("reject", 5, 5, 5, 5, 5, 5, 5),
			stage2:      passingStage2(),
			content:     []byte("session"),
			wantPassed:  boolPtr(false),
			wantVerdict: "reject",
		},
		{
			name:        "empty reasoning string",
			llmResponse: verdictWithEmptyReasoning(),
			stage2:      passingStage2(),
			content:     []byte("session"),
			wantPassed:  boolPtr(true),
		},
		// Score validation
		{
			name:        "score out of range 47",
			llmResponse: verdictWithInvalidScore(47),
			stage2:      passingStage2(),
			content:     []byte("session"),
			wantPassed:  boolPtr(false),
		},
		{
			name:        "score zero",
			llmResponse: verdictWithInvalidScore(0),
			stage2:      passingStage2(),
			content:     []byte("session"),
			wantPassed:  boolPtr(false),
		},
		{
			name:        "score negative",
			llmResponse: verdictWithInvalidScore(-1),
			stage2:      passingStage2(),
			content:     []byte("session"),
			wantPassed:  boolPtr(false),
		},
		{
			name:        "confidence > 1.0 clamped",
			llmResponse: verdictWithConfidence(1.5),
			stage2:      passingStage2(),
			content:     []byte("session"),
			checkResult: func(t *testing.T, r *Stage3Result) {
				if r.RefinedScores.CriticConfidence > 1.0 {
					t.Error("confidence must be clamped to [0, 1]")
				}
			},
		},
		{
			name:        "confidence negative clamped",
			llmResponse: verdictWithConfidence(-0.5),
			stage2:      passingStage2(),
			content:     []byte("session"),
			checkResult: func(t *testing.T, r *Stage3Result) {
				if r.RefinedScores.CriticConfidence < 0 {
					t.Error("confidence must be clamped to [0, 1]")
				}
			},
		},
		// Error propagation
		{
			name:    "LLM returns error",
			llmErr:  errors.New("service unavailable"),
			stage2:  passingStage2(),
			content: []byte("session"),
			wantErr: true,
		},
		{
			name:        "nil stage2 input",
			llmResponse: extractVerdictJSON(),
			stage2:      nil,
			content:     []byte("session"),
			wantErr:     true,
		},
		{
			name:        "nil content",
			llmResponse: extractVerdictJSON(),
			stage2:      passingStage2(),
			content:     nil,
			wantErr:     true,
		},
		{
			name:        "empty content",
			llmResponse: extractVerdictJSON(),
			stage2:      passingStage2(),
			content:     []byte(""),
			wantErr:     true,
		},
		// Invalid verdict strings
		{
			name:        "verdict EXTRACT normalized to lowercase",
			llmResponse: verdictWithString("EXTRACT"),
			stage2:      passingStage2(),
			content:     []byte("session"),
			wantPassed:  boolPtr(true),
			wantVerdict: "extract",
		},
		{
			name:        "verdict maybe treated as rejection",
			llmResponse: verdictWithString("maybe"),
			stage2:      passingStage2(),
			content:     []byte("session"),
			wantPassed:  boolPtr(false),
		},
		{
			name:        "stage2.Passed=false",
			llmResponse: extractVerdictJSON(),
			stage2:      &Stage2Result{Passed: false},
			content:     []byte("session"),
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var llm LLMClient
			if tt.llmErr != nil {
				llm = s3errLLM(tt.llmErr)
			} else {
				llm = s3okLLM(tt.llmResponse)
			}

			critic := newTestCritic(llm)
			result, err := critic.Evaluate(context.Background(), s3testSession(), tt.content, tt.stage2)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantPassed != nil && result.Passed != *tt.wantPassed {
				t.Errorf("passed: got %v, want %v", result.Passed, *tt.wantPassed)
			}
			if tt.wantVerdict != "" && result.CriticVerdict != tt.wantVerdict {
				t.Errorf("verdict: got %q, want %q", result.CriticVerdict, tt.wantVerdict)
			}
			// Passed must be consistent with verdict
			if result.CriticVerdict == "extract" && !result.Passed {
				// OK only if contradiction override happened
			} else if result.CriticVerdict == "reject" && result.Passed {
				t.Error("Passed=true but verdict=reject")
			}

			if tt.checkResult != nil {
				tt.checkResult(t, result)
			}
		})
	}
}

// --- Retry Tests ---

func TestStage3Critic_Retry(t *testing.T) {
	tests := []struct {
		name       string
		failures   int
		failErr    error
		wantCalls  int
		wantErr    bool
		wantPassed bool
	}{
		{"succeeds first try", 0, nil, 1, false, true},
		{"fails once then succeeds", 1, errors.New("timeout"), 2, false, true},
		{"fails 3 then succeeds", 3, errors.New("timeout"), 4, false, true},
		{"fails 4 exceeds max retries", 4, errors.New("timeout"), 4, true, false},
		{"rate limit 429 gets 5 retries", 5, errors.New("429 rate limit"), 6, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			llm := &mockLLM3{
				completeFn: func(_ context.Context, _ string) (string, error) {
					callCount++
					if callCount <= tt.failures {
						return "", tt.failErr
					}
					return extractVerdictJSON(), nil
				},
			}

			critic := newTestCritic(llm)
			result, err := critic.Evaluate(context.Background(), s3testSession(), []byte("content"), passingStage2())

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if result.Passed != tt.wantPassed {
					t.Errorf("passed: got %v, want %v", result.Passed, tt.wantPassed)
				}
			}
			if callCount != tt.wantCalls {
				t.Errorf("calls: got %d, want %d", callCount, tt.wantCalls)
			}
		})
	}
}

// --- Circuit Breaker Tests ---

func TestStage3Critic_CircuitBreaker(t *testing.T) {
	t.Run("opens after 5 consecutive failures", func(t *testing.T) {
		llm := s3errLLM(errors.New("unavailable"))
		critic := newTestCritic(llm)

		// 5 calls fail (each retries internally, but all fail)
		for i := 0; i < 5; i++ {
			_, err := critic.Evaluate(context.Background(), s3testSession(), []byte("content"), passingStage2())
			if err == nil {
				t.Fatal("expected error")
			}
		}

		// 6th should be circuit open
		_, err := critic.Evaluate(context.Background(), s3testSession(), []byte("content"), passingStage2())
		if !errors.Is(err, ErrCircuitOpen) {
			t.Errorf("expected ErrCircuitOpen, got %v", err)
		}
	})

	t.Run("half-open after duration success closes", func(t *testing.T) {
		now := time.Now()
		callCount := 0
		shouldFail := true

		llm := &mockLLM3{completeFn: func(_ context.Context, _ string) (string, error) {
			callCount++
			if shouldFail {
				return "", errors.New("unavailable")
			}
			return extractVerdictJSON(), nil
		}}

		critic := newTestCriticWithClock(llm, func() time.Time { return now })

		// Open circuit
		for i := 0; i < 5; i++ {
			critic.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2())
		}

		// Advance past open duration
		now = now.Add(6 * time.Minute)
		shouldFail = false

		result, err := critic.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2())
		if err != nil {
			t.Fatalf("half-open should succeed: %v", err)
		}
		if !result.Passed {
			t.Error("expected pass")
		}

		// Circuit should be closed now
		result2, err := critic.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2())
		if err != nil {
			t.Fatalf("closed circuit should work: %v", err)
		}
		if !result2.Passed {
			t.Error("expected pass")
		}
	})

	t.Run("success resets failure counter", func(t *testing.T) {
		callCount := 0
		llm := &mockLLM3{completeFn: func(_ context.Context, _ string) (string, error) {
			callCount++
			// Fail first 4, succeed 5th, fail next 4
			switch {
			case callCount <= 4:
				return "", errors.New("unavailable")
			case callCount == 5:
				return extractVerdictJSON(), nil
			default:
				return "", errors.New("unavailable")
			}
		}}

		_ = newTestCritic(llm)

		// Adjusted: use smarter mock below

		failCount := 0
		succeedNext := false
		llm2 := &mockLLM3{completeFn: func(_ context.Context, _ string) (string, error) {
			if succeedNext {
				return extractVerdictJSON(), nil
			}
			failCount++
			return "", errors.New("unavailable")
		}}

		critic2 := newTestCritic(llm2)

		// 4 evaluate calls that fail
		for i := 0; i < 4; i++ {
			critic2.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2())
		}

		// 1 success
		succeedNext = true
		_, err := critic2.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2())
		if err != nil {
			t.Fatalf("expected success: %v", err)
		}

		// 4 more failures — circuit should NOT open
		succeedNext = false
		for i := 0; i < 4; i++ {
			_, err := critic2.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2())
			if errors.Is(err, ErrCircuitOpen) {
				t.Fatalf("circuit should not be open after reset, failed on call %d", i+1)
			}
		}
	})
}

// --- Context Cancellation ---

func TestStage3Critic_ContextCancellation(t *testing.T) {
	t.Run("already cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		critic := newTestCritic(s3okLLM(extractVerdictJSON()))
		_, err := critic.Evaluate(ctx, s3testSession(), []byte("content"), passingStage2())
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	})

	t.Run("cancelled during LLM call", func(t *testing.T) {
		llm := &mockLLM3{completeFn: func(ctx context.Context, _ string) (string, error) {
			return "", context.DeadlineExceeded
		}}

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
		defer cancel()
		time.Sleep(2 * time.Millisecond) // let it expire

		critic := newTestCritic(llm)
		_, err := critic.Evaluate(ctx, s3testSession(), []byte("content"), passingStage2())
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

// --- Content Edge Cases ---

func TestStage3Critic_ContentEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		wantErr bool
		check   func(t *testing.T, r *Stage3Result)
	}{
		{"null bytes", []byte("fix\x00bug"), false, nil},
		{"pure JSON content", []byte(`{"key":"value"}`), false, nil},
		{"emoji content", []byte("fixed the bug 🎉🔥"), false, nil},
		{"150KB content", bytes.Repeat([]byte("x"), 150*1024), false, nil},
		{"content with code blocks", []byte("Fixed:\n```json\n{\"key\": \"value\"}\n```\nDone."), false, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			critic := newTestCritic(s3okLLM(extractVerdictJSON()))
			result, err := critic.Evaluate(context.Background(), s3testSession(), tt.content, passingStage2())
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}

// --- Logging Tests ---

func TestStage3Critic_Logging(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	critic := NewStage3Critic(s3okLLM(extractVerdictJSON()), s3testConfig(), log)
	critic.Evaluate(context.Background(), s3testSession(), []byte("content"), passingStage2())

	logOutput := buf.String()

	// Must log
	for _, want := range []string{"session_id", "verdict", "latency_ms"} {
		if !strings.Contains(logOutput, want) {
			t.Errorf("log missing %q", want)
		}
	}
}

func TestStage3Critic_NoSecretsInLogs(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	content := []byte("connecting with AKIA1234567890ABCDEF and secret=wJalrXUtnFEMI/K7MDENG password=hunter2")
	critic := NewStage3Critic(s3okLLM(extractVerdictJSON()), s3testConfig(), log)
	critic.Evaluate(context.Background(), s3testSession(), content, passingStage2())

	logOutput := buf.String()
	for _, secret := range []string{"AKIA1234567890", "wJalrXUtnFEMI", "hunter2", "password"} {
		if strings.Contains(logOutput, secret) {
			t.Errorf("secret %q found in log output", secret)
		}
	}
}

// --- Prompt Security ---

func TestStage3Critic_PromptSecurity(t *testing.T) {
	var capturedPrompt string
	llm := &mockLLM3{completeFn: func(_ context.Context, prompt string) (string, error) {
		capturedPrompt = prompt
		return extractVerdictJSON(), nil
	}}

	critic := NewStage3Critic(llm, s3testConfig(), s3testLogger())
	critic.Evaluate(context.Background(), s3testSession(), []byte("api key sk-proj-abc123"), passingStage2())

	if !strings.Contains(capturedPrompt, "---BEGIN SESSION---") {
		t.Error("prompt should delimit session content")
	}
	if !strings.Contains(capturedPrompt, "---END SESSION---") {
		t.Error("prompt should have end delimiter")
	}
}

// --- Latency ---

func TestStage3Critic_LatencyTracking(t *testing.T) {
	slowLLM := &mockLLM3{completeFn: func(_ context.Context, _ string) (string, error) {
		time.Sleep(50 * time.Millisecond)
		return extractVerdictJSON(), nil
	}}

	critic := NewStage3Critic(slowLLM, s3testConfig(), s3testLogger())
	result, err := critic.Evaluate(context.Background(), s3testSession(), []byte("content"), passingStage2())
	if err != nil {
		t.Fatal(err)
	}
	if result.LatencyMs < 50 {
		t.Errorf("expected latency >= 50ms, got %d", result.LatencyMs)
	}
}

// --- Consistency ---

func TestStage3Critic_Consistency(t *testing.T) {
	critic := newTestCritic(s3okLLM(extractVerdictJSON()))

	var verdicts []string
	for i := 0; i < 10; i++ {
		result, err := critic.Evaluate(context.Background(), s3testSession(), []byte("consistent"), passingStage2())
		if err != nil {
			t.Fatal(err)
		}
		verdicts = append(verdicts, result.CriticVerdict)
	}

	for i := 1; i < len(verdicts); i++ {
		if verdicts[i] != verdicts[0] {
			t.Errorf("inconsistent verdict on call %d: got %s, expected %s", i, verdicts[i], verdicts[0])
		}
	}
}

// --- Composite Score Recomputation ---

func TestStage3Critic_CompositeScoreRecomputed(t *testing.T) {
	critic := newTestCritic(s3okLLM(extractVerdictJSON()))
	result, err := critic.Evaluate(context.Background(), s3testSession(), []byte("content"), passingStage2())
	if err != nil {
		t.Fatal(err)
	}

	expected := compositeScore(result.RefinedScores)
	if result.RefinedScores.CompositeScore != expected {
		t.Errorf("composite: got %f, want %f", result.RefinedScores.CompositeScore, expected)
	}
}

// --- Passed/Verdict Consistency ---

func TestStage3Critic_PassedVerdictConsistency(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantPass bool
	}{
		{"extract", extractVerdictJSON(), true},
		{"reject", rejectVerdictJSON(), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			critic := newTestCritic(s3okLLM(tt.response))
			result, err := critic.Evaluate(context.Background(), s3testSession(), []byte("c"), passingStage2())
			if err != nil {
				t.Fatal(err)
			}
			if result.Passed != tt.wantPass {
				t.Errorf("Passed=%v but verdict=%s", result.Passed, result.CriticVerdict)
			}
		})
	}
}

// --- Truncation ---

func TestTruncateContent(t *testing.T) {
	// Basic truncation
	long := bytes.Repeat([]byte("a"), 200*1024)
	result := truncateContent(long, maxContentBytes)
	if len(result) > maxContentBytes {
		t.Errorf("not truncated: %d", len(result))
	}

	// Mid-rune safety
	// 3-byte UTF-8 char: ä = 0xC3 0xA4 (2 bytes actually), let's use € = 0xE2 0x82 0xAC
	content := []byte("aaa€") // 3 + 3 = 6 bytes
	trunc := truncateContent(content, 5) // cuts into €
	if !bytes.Equal(trunc, []byte("aaa")) {
		t.Errorf("expected 'aaa', got %q", string(trunc))
	}
}

// --- Helpers ---

func boolPtr(b bool) *bool { return &b }

func newTestCritic(llm LLMClient) Stage3Critic {
	c := NewStage3Critic(llm, s3testConfig(), s3testLogger()).(*stage3Critic)
	c.sleep = func(d time.Duration) {} // no-op sleep for tests
	return c
}

func newTestCriticWithClock(llm LLMClient, clock func() time.Time) *stage3Critic {
	c := NewStage3Critic(llm, s3testConfig(), s3testLogger()).(*stage3Critic)
	c.clock = clock
	c.sleep = func(d time.Duration) {}
	return c
}
