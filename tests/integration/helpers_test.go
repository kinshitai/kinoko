//go:build integration

package integration

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kinoko-dev/kinoko/internal/config"
	"github.com/kinoko-dev/kinoko/internal/model"
	"github.com/kinoko-dev/kinoko/internal/storage"
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
	return strings.Contains(s, sub)
}

// --- Mock Embedder ---

// predictableEmbedder returns deterministic embeddings based on text hash.
type predictableEmbedder struct {
	dims      int
	failAfter int // -1 = always fail, 0 = never fail, N = fail after N successes
	callCount atomic.Int64
}

func newPredictableEmbedder(dims int) *predictableEmbedder {
	return &predictableEmbedder{dims: dims}
}

func (e *predictableEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	count := e.callCount.Add(1)
	if e.failAfter == -1 {
		return nil, fmt.Errorf("simulated embedding failure (always)")
	}
	if e.failAfter > 0 && int(count) > e.failAfter {
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

func (m *mockQuerier) QueryNearest(_ context.Context, _ []float32, _ string) (*model.SkillQueryResult, error) {
	return &model.SkillQueryResult{CosineSim: m.sim}, nil
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

func goodSession(id, libraryID string) model.SessionRecord {
	now := time.Now()
	return model.SessionRecord{
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

func shortSession(id, libraryID string) model.SessionRecord {
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

// indexingCommitter simulates the post-receive hook: on CommitSkill it indexes
// the skill into SQLite, mimicking what the real hook pipeline does.
type indexingCommitter struct {
	indexer  model.SkillIndexer
	embedder embeddingEmbedder
}

// embeddingEmbedder is a local interface for embedding in tests.
type embeddingEmbedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

func (c *indexingCommitter) CommitSkill(ctx context.Context, _ string, skill *model.SkillRecord, body []byte) (string, error) {
	var emb []float32
	if c.embedder != nil {
		var err error
		emb, err = c.embedder.Embed(ctx, string(body))
		if err != nil {
			return "", err
		}
	}
	if err := c.indexer.IndexSkill(ctx, skill, emb); err != nil {
		return "", err
	}
	return "deadbeef", nil
}

// noopCommitter is a no-op SkillCommitter for tests that don't need git persistence.
type noopCommitter struct{}

func (noopCommitter) CommitSkill(_ context.Context, _ string, _ *model.SkillRecord, _ []byte) (string, error) {
	return "noop000", nil
}

// insertSession inserts a session record into the sessions table for metrics.
func insertSession(t *testing.T, db *sql.DB, sess model.SessionRecord, result *model.ExtractionResult) {
	t.Helper()
	status := string(result.Status)
	rejStage := 0
	rejReason := ""
	skillID := ""

	if result.Status == model.StatusRejected {
		switch {
		case result.Stage1 != nil && !result.Stage1.Passed:
			rejStage = 1
			rejReason = result.Stage1.Reason
		case result.Stage2 != nil && !result.Stage2.Passed:
			rejStage = 2
			rejReason = result.Stage2.Reason
		case result.Stage3 != nil && !result.Stage3.Passed:
			rejStage = 3
			rejReason = result.Stage3.CriticReasoning
		}
	}
	if result.Skill != nil {
		skillID = result.Skill.ID
	}

	_, err := db.Exec(`INSERT INTO sessions (id, started_at, ended_at, duration_minutes, tool_call_count,
		error_count, message_count, error_rate, has_successful_exec, tokens_used, agent_model, user_id,
		library_id, extraction_status, rejected_at_stage, rejection_reason, extracted_skill_id)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		sess.ID, sess.StartedAt, sess.EndedAt, sess.DurationMinutes, sess.ToolCallCount,
		sess.ErrorCount, sess.MessageCount, sess.ErrorRate, sess.HasSuccessfulExec,
		sess.TokensUsed, sess.AgentModel, sess.UserID,
		sess.LibraryID, status, rejStage, rejReason,
		sql.NullString{String: skillID, Valid: skillID != ""})
	if err != nil {
		t.Fatalf("insert session %s: %v", sess.ID, err)
	}
}
