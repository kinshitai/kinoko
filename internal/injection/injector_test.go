package injection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/mycelium-dev/mycelium/internal/extraction"
	"github.com/mycelium-dev/mycelium/internal/storage"
)

// --- mocks ---

type mockEmbedder struct {
	result []float32
	err    error
}

func (m *mockEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return m.result, m.err
}
func (m *mockEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	if m.err != nil {
		return nil, m.err
	}
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = m.result
	}
	return out, nil
}
func (m *mockEmbedder) Dimensions() int { return len(m.result) }

type mockLLM struct {
	response string
	err      error
}

func (m *mockLLM) Complete(_ context.Context, _ string) (string, error) {
	return m.response, m.err
}

type mockStore struct {
	results []storage.ScoredSkill
	err     error
	lastQ   storage.SkillQuery
}

func (m *mockStore) Put(_ context.Context, _ *extraction.SkillRecord, _ []byte) error { return nil }
func (m *mockStore) Get(_ context.Context, _ string) (*extraction.SkillRecord, error) { return nil, nil }
func (m *mockStore) GetLatestByName(_ context.Context, _, _ string) (*extraction.SkillRecord, error) {
	return nil, nil
}
func (m *mockStore) Query(_ context.Context, q storage.SkillQuery) ([]storage.ScoredSkill, error) {
	m.lastQ = q
	return m.results, m.err
}
func (m *mockStore) UpdateUsage(_ context.Context, _ string, _ string) error { return nil }
func (m *mockStore) UpdateDecay(_ context.Context, _ string, _ float64) error { return nil }
func (m *mockStore) ListByDecay(_ context.Context, _ string, _ int) ([]extraction.SkillRecord, error) {
	return nil, nil
}

// --- helpers ---

func classifyJSON(intent, domain string, patterns []string) string {
	b, _ := json.Marshal(classificationResponse{Intent: intent, Domain: domain, Patterns: patterns})
	return string(b)
}

func makeSkill(id string, patterns []string, successCorr float64) storage.ScoredSkill {
	return storage.ScoredSkill{
		Skill: extraction.SkillRecord{
			ID:                 id,
			Patterns:           patterns,
			SuccessCorrelation: successCorr,
		},
		PatternOverlap: 0.5,
		CosineSim:      0.8,
		HistoricalRate: 0.7,
	}
}

func newTestInjector(emb *mockEmbedder, store *mockStore, llm *mockLLM) Injector {
	return New(emb, store, llm, slog.Default())
}

// --- tests ---

func TestFullFlow(t *testing.T) {
	llm := &mockLLM{response: classifyJSON("BUILD", "Go backend", []string{"BUILD/Backend/APIDesign"})}
	emb := &mockEmbedder{result: []float32{0.1, 0.2, 0.3}}
	store := &mockStore{results: []storage.ScoredSkill{
		makeSkill("s1", []string{"BUILD/Backend/APIDesign"}, 0.5),
		makeSkill("s2", []string{"BUILD/Backend/DataModeling"}, 0.3),
	}}

	inj := newTestInjector(emb, store, llm)
	resp, err := inj.Inject(context.Background(), extraction.InjectionRequest{
		Prompt:     "build a REST API",
		LibraryIDs: []string{"lib1"},
		MaxSkills:  5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Classification.Intent != "BUILD" {
		t.Errorf("intent = %q, want BUILD", resp.Classification.Intent)
	}
	if len(resp.Skills) != 2 {
		t.Errorf("got %d skills, want 2", len(resp.Skills))
	}
	// Verify composite uses normal weights.
	s := resp.Skills[0]
	want := 0.5*s.PatternOverlap + 0.3*s.CosineSim + 0.2*s.HistoricalRate
	if s.CompositeScore != want {
		t.Errorf("composite = %f, want %f", s.CompositeScore, want)
	}
}

func TestEmbeddingFallback(t *testing.T) {
	llm := &mockLLM{response: classifyJSON("FIX", "database", []string{"FIX/Backend/DatabaseConnection"})}
	emb := &mockEmbedder{err: errors.New("circuit open")}
	store := &mockStore{results: []storage.ScoredSkill{
		makeSkill("s1", []string{"FIX/Backend/DatabaseConnection"}, 0.8),
	}}

	inj := newTestInjector(emb, store, llm)
	resp, err := inj.Inject(context.Background(), extraction.InjectionRequest{
		Prompt:    "fix db connection",
		MaxSkills: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Degraded weights: 0.7*pattern + 0.3*historical.
	s := resp.Skills[0]
	want := 0.7*s.PatternOverlap + 0.3*s.HistoricalRate
	if s.CompositeScore != want {
		t.Errorf("composite = %f, want %f (degraded)", s.CompositeScore, want)
	}
}

func TestNilEmbedder(t *testing.T) {
	llm := &mockLLM{response: classifyJSON("BUILD", "Go", []string{})}
	store := &mockStore{results: []storage.ScoredSkill{makeSkill("s1", nil, 0.5)}}

	inj := New(nil, store, llm, slog.Default())
	resp, err := inj.Inject(context.Background(), extraction.InjectionRequest{
		Prompt:    "build something",
		MaxSkills: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	s := resp.Skills[0]
	want := 0.7*s.PatternOverlap + 0.3*s.HistoricalRate
	if s.CompositeScore != want {
		t.Errorf("composite = %f, want %f (nil embedder)", s.CompositeScore, want)
	}
}

func TestEmptyLibrary(t *testing.T) {
	llm := &mockLLM{response: classifyJSON("BUILD", "Go", []string{"BUILD/Backend/APIDesign"})}
	emb := &mockEmbedder{result: []float32{0.1}}
	store := &mockStore{results: nil}

	inj := newTestInjector(emb, store, llm)
	resp, err := inj.Inject(context.Background(), extraction.InjectionRequest{
		Prompt:    "build an API",
		MaxSkills: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Skills) != 0 {
		t.Errorf("got %d skills, want 0", len(resp.Skills))
	}
	if resp.Classification.Intent != "BUILD" {
		t.Errorf("classification should still work even with empty store")
	}
}

func TestClassificationFailure(t *testing.T) {
	llm := &mockLLM{err: errors.New("llm down")}
	emb := &mockEmbedder{result: []float32{0.1}}
	store := &mockStore{results: []storage.ScoredSkill{makeSkill("s1", nil, 0.5)}}

	inj := newTestInjector(emb, store, llm)
	resp, err := inj.Inject(context.Background(), extraction.InjectionRequest{
		Prompt:    "help",
		MaxSkills: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Classification should be empty, but injection should still work.
	if resp.Classification.Intent != "" {
		t.Errorf("expected empty intent on failure, got %q", resp.Classification.Intent)
	}
	if len(resp.Classification.Patterns) != 0 {
		t.Errorf("expected no patterns on failure")
	}
}

func TestMaxSkillsLimiting(t *testing.T) {
	llm := &mockLLM{response: classifyJSON("BUILD", "Go", []string{})}
	emb := &mockEmbedder{result: []float32{0.1}}
	skills := make([]storage.ScoredSkill, 10)
	for i := range skills {
		skills[i] = makeSkill(fmt.Sprintf("s%d", i), nil, float64(i)*0.1)
		skills[i].PatternOverlap = float64(10-i) * 0.1
	}
	store := &mockStore{results: skills}

	inj := newTestInjector(emb, store, llm)
	resp, err := inj.Inject(context.Background(), extraction.InjectionRequest{
		Prompt:    "build",
		MaxSkills: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Skills) != 3 {
		t.Fatalf("got %d skills, want 3", len(resp.Skills))
	}
}

func TestLibraryPriorityOrdering(t *testing.T) {
	llm := &mockLLM{response: classifyJSON("BUILD", "Go", []string{"BUILD/Backend/APIDesign"})}
	emb := &mockEmbedder{result: []float32{0.1}}

	// Different scores to ensure ordering.
	s1 := makeSkill("low", nil, -0.5)
	s1.PatternOverlap = 0.1
	s1.CosineSim = 0.1
	s1.HistoricalRate = 0.1

	s2 := makeSkill("high", nil, 0.9)
	s2.PatternOverlap = 0.9
	s2.CosineSim = 0.9
	s2.HistoricalRate = 0.9

	store := &mockStore{results: []storage.ScoredSkill{s1, s2}}

	inj := newTestInjector(emb, store, llm)
	resp, err := inj.Inject(context.Background(), extraction.InjectionRequest{
		Prompt:    "build API",
		MaxSkills: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Skills) != 2 {
		t.Fatalf("got %d skills, want 2", len(resp.Skills))
	}
	if resp.Skills[0].Skill.ID != "high" {
		t.Errorf("expected 'high' first, got %q", resp.Skills[0].Skill.ID)
	}
	if resp.Skills[1].Skill.ID != "low" {
		t.Errorf("expected 'low' second, got %q", resp.Skills[1].Skill.ID)
	}
	if resp.Skills[0].CompositeScore <= resp.Skills[1].CompositeScore {
		t.Error("scores not in descending order")
	}
}

func TestDefaultMaxSkills(t *testing.T) {
	llm := &mockLLM{response: classifyJSON("BUILD", "Go", []string{})}
	emb := &mockEmbedder{result: []float32{0.1}}
	skills := make([]storage.ScoredSkill, 10)
	for i := range skills {
		skills[i] = makeSkill(fmt.Sprintf("s%d", i), nil, 0.5)
	}
	store := &mockStore{results: skills}

	inj := newTestInjector(emb, store, llm)
	resp, err := inj.Inject(context.Background(), extraction.InjectionRequest{
		Prompt:    "build",
		MaxSkills: 0, // should default to 3
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Skills) != 3 {
		t.Fatalf("got %d skills, want 3 (default)", len(resp.Skills))
	}
}
