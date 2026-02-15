package integration

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"testing"
	"time"

	"github.com/mycelium-dev/mycelium/internal/config"
	"github.com/mycelium-dev/mycelium/internal/extraction"
	"github.com/mycelium-dev/mycelium/internal/storage"
)

// --- Mock LLM ---

// predictableLLM returns canned responses based on prompt content keywords.
type predictableLLM struct {
	// rubricResponse is returned for Stage2 rubric calls.
	rubricResponse string
	// criticResponse is returned for Stage3 critic calls.
	criticResponse string
	// classifyResponse is returned for injection classification calls.
	classifyResponse string
	// callLog records all prompts received.
	callLog []string
	// failAfter makes Complete fail after N successful calls. 0 = never fail.
	failAfter int
	callCount int
}

func (m *predictableLLM) Complete(_ context.Context, prompt string) (string, error) {
	m.callLog = append(m.callLog, prompt)
	m.callCount++
	if m.failAfter > 0 && m.callCount > m.failAfter {
		return "", fmt.Errorf("simulated LLM failure after %d calls", m.failAfter)
	}
	// Detect which component is calling based on prompt content.
	// Injection classifier: "Classify this user prompt"
	if contains(prompt, "Classify this user prompt") {
		if m.classifyResponse != "" {
			return m.classifyResponse, nil
		}
	}
	// Stage3 critic: "critical evaluator" or "BEGIN SESSION"
	if contains(prompt, "critical evaluator") || contains(prompt, "BEGIN SESSION") {
		return m.criticResponse, nil
	}
	// Stage2 rubric: everything else with LLM call (rubric scoring)
	return m.rubricResponse, nil
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchString(s, sub)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// --- Mock Embedder ---

// predictableEmbedder returns deterministic embeddings based on text hash.
type predictableEmbedder struct {
	dims      int
	failAfter int // -1 = always fail, 0 = never fail, N = fail after N successes
	callCount int
}

func newPredictableEmbedder(dims int) *predictableEmbedder {
	return &predictableEmbedder{dims: dims}
}

func (e *predictableEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	e.callCount++
	if e.failAfter == -1 {
		return nil, fmt.Errorf("simulated embedding failure (always)")
	}
	if e.failAfter > 0 && e.callCount > e.failAfter {
		return nil, fmt.Errorf("simulated embedding failure after %d calls", e.failAfter)
	}
	return e.deterministicVector(text), nil
}

func (e *predictableEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v, err := e.Embed(context.Background(), t)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

func (e *predictableEmbedder) Dimensions() int { return e.dims }

// deterministicVector creates a normalized vector from text hash.
func (e *predictableEmbedder) deterministicVector(text string) []float32 {
	v := make([]float32, e.dims)
	h := uint32(0)
	for _, b := range []byte(text) {
		h = h*31 + uint32(b)
	}
	var norm float64
	for i := range v {
		seed := h + uint32(i)*2654435761
		v[i] = float32(seed%1000)/500.0 - 1.0
		norm += float64(v[i]) * float64(v[i])
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range v {
			v[i] /= float32(norm)
		}
	}
	return v
}

// --- Mock SkillQuerier (for Stage2) ---

type mockQuerier struct {
	sim float64
}

func (m *mockQuerier) QueryNearest(_ context.Context, _ []float32, _ string) (*extraction.SkillQueryResult, error) {
	return &extraction.SkillQueryResult{CosineSim: m.sim}, nil
}

// --- Mock HumanReviewWriter ---

type mockReviewWriter struct {
	samples []reviewSample
}

type reviewSample struct {
	sessionID string
	data      []byte
}

func (m *mockReviewWriter) InsertReviewSample(_ context.Context, sessionID string, data []byte) error {
	m.samples = append(m.samples, reviewSample{sessionID, data})
	return nil
}

// --- Test Fixtures ---

func goodSession(id, libraryID string) extraction.SessionRecord {
	now := time.Now()
	return extraction.SessionRecord{
		ID:                id,
		StartedAt:         now.Add(-15 * time.Minute),
		EndedAt:           now,
		DurationMinutes:   15,
		ToolCallCount:     8,
		ErrorCount:        1,
		MessageCount:      20,
		ErrorRate:         0.125,
		HasSuccessfulExec: true,
		TokensUsed:        5000,
		AgentModel:        "claude-3",
		UserID:            "user-1",
		LibraryID:         libraryID,
	}
}

func shortSession(id, libraryID string) extraction.SessionRecord {
	s := goodSession(id, libraryID)
	s.DurationMinutes = 0.5
	s.ToolCallCount = 1
	return s
}

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

func extractVerdictJSON() string {
	return `{
		"verdict": "extract",
		"reasoning": "Clear problem-solution pattern with verified results.",
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
		"reasoning": "Too trivial for extraction.",
		"refined_scores": {
			"problem_specificity": 2,
			"solution_completeness": 2,
			"context_portability": 1,
			"reasoning_transparency": 2,
			"technical_accuracy": 2,
			"verification_evidence": 1,
			"innovation_level": 1
		},
		"confidence": 0.9,
		"reusable_pattern": false,
		"explicit_reasoning": false,
		"contradicts_best_practices": false
	}`
}

func classifyJSON(intent, domain string, patterns []string) string {
	pats := "["
	for i, p := range patterns {
		if i > 0 {
			pats += ","
		}
		pats += fmt.Sprintf("%q", p)
	}
	pats += "]"
	return fmt.Sprintf(`{"intent":%q,"domain":%q,"patterns":%s}`, intent, domain, pats)
}

// --- Test Infrastructure ---

func newTestStore(t *testing.T) *storage.SQLiteStore {
	t.Helper()
	s, err := storage.NewSQLiteStore(":memory:", "test-embed-model")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func defaultExtractionConfig() config.ExtractionConfig {
	c := config.DefaultConfig()
	return c.Extraction
}

// assertApprox checks that two floats are within epsilon.
func assertApprox(t *testing.T, got, want, eps float64, msg string) {
	t.Helper()
	if math.Abs(got-want) > eps {
		t.Errorf("%s: got %.4f, want %.4f (±%.4f)", msg, got, want, eps)
	}
}
