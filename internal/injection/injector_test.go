package injection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
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
func (m *mockStore) Get(_ context.Context, _ string) (*extraction.SkillRecord, error) {
	return nil, nil
}
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

type mockEventWriter struct {
	events []storage.InjectionEventRecord
	err    error
}

func (m *mockEventWriter) WriteInjectionEvent(_ context.Context, ev storage.InjectionEventRecord) error {
	if m.err != nil {
		return m.err
	}
	m.events = append(m.events, ev)
	return nil
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
		CompositeScore: 0.5*0.5 + 0.3*0.8 + 0.2*0.7, // store-computed
	}
}

func newTestInjector(emb *mockEmbedder, store *mockStore, llm *mockLLM, ew InjectionEventWriter) Injector {
	return New(emb, store, llm, ew, slog.Default())
}

func approxEqual(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// --- tests ---

func TestFullFlow(t *testing.T) {
	llm := &mockLLM{response: classifyJSON("BUILD", "Backend", []string{"BUILD/Backend/APIDesign"})}
	emb := &mockEmbedder{result: []float32{0.1, 0.2, 0.3}}
	ew := &mockEventWriter{}
	store := &mockStore{results: []storage.ScoredSkill{
		makeSkill("s1", []string{"BUILD/Backend/APIDesign"}, 0.5),
		makeSkill("s2", []string{"BUILD/Backend/DataModeling"}, 0.3),
	}}

	inj := newTestInjector(emb, store, llm, ew)
	resp, err := inj.Inject(context.Background(), extraction.InjectionRequest{
		Prompt:     "build a REST API",
		LibraryIDs: []string{"lib1"},
		MaxSkills:  5,
		SessionID:  "sess-1",
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
	// In normal mode, store's composite is used as-is.
	if resp.Skills[0].CompositeScore != makeSkill("", nil, 0).CompositeScore {
		t.Errorf("composite should match store-computed value")
	}
	// Verify query uses MinDecay and Limit.
	if store.lastQ.MinDecay != defaultMinDecay {
		t.Errorf("MinDecay = %f, want %f", store.lastQ.MinDecay, defaultMinDecay)
	}
	if store.lastQ.Limit != defaultCandidateLimit {
		t.Errorf("Limit = %d, want %d", store.lastQ.Limit, defaultCandidateLimit)
	}
}

func TestInjectionEventWriting(t *testing.T) {
	llm := &mockLLM{response: classifyJSON("BUILD", "Backend", []string{"BUILD/Backend/APIDesign"})}
	emb := &mockEmbedder{result: []float32{0.1}}
	ew := &mockEventWriter{}
	store := &mockStore{results: []storage.ScoredSkill{
		makeSkill("s1", nil, 0.5),
		makeSkill("s2", nil, 0.3),
	}}

	inj := newTestInjector(emb, store, llm, ew)
	_, err := inj.Inject(context.Background(), extraction.InjectionRequest{
		Prompt:    "build something",
		SessionID: "sess-42",
		MaxSkills: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ew.events) != 2 {
		t.Fatalf("expected 2 injection events, got %d", len(ew.events))
	}
	if ew.events[0].SessionID != "sess-42" {
		t.Errorf("event session = %q, want sess-42", ew.events[0].SessionID)
	}
	if ew.events[0].RankPosition != 1 {
		t.Errorf("rank = %d, want 1", ew.events[0].RankPosition)
	}
	if ew.events[1].RankPosition != 2 {
		t.Errorf("rank = %d, want 2", ew.events[1].RankPosition)
	}
}

func TestInjectionEventNotWrittenWithoutSessionID(t *testing.T) {
	llm := &mockLLM{response: classifyJSON("BUILD", "Backend", []string{})}
	emb := &mockEmbedder{result: []float32{0.1}}
	ew := &mockEventWriter{}
	store := &mockStore{results: []storage.ScoredSkill{makeSkill("s1", nil, 0.5)}}

	inj := newTestInjector(emb, store, llm, ew)
	_, err := inj.Inject(context.Background(), extraction.InjectionRequest{
		Prompt:    "build",
		SessionID: "", // no session ID
		MaxSkills: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ew.events) != 0 {
		t.Errorf("expected no events without SessionID, got %d", len(ew.events))
	}
}

func TestInjectionEventWriteError(t *testing.T) {
	llm := &mockLLM{response: classifyJSON("BUILD", "Backend", []string{})}
	emb := &mockEmbedder{result: []float32{0.1}}
	ew := &mockEventWriter{err: errors.New("db write failed")}
	store := &mockStore{results: []storage.ScoredSkill{makeSkill("s1", nil, 0.5)}}

	inj := newTestInjector(emb, store, llm, ew)
	// Event write failure should not break injection.
	resp, err := inj.Inject(context.Background(), extraction.InjectionRequest{
		Prompt:    "build",
		SessionID: "sess-1",
		MaxSkills: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Skills) != 1 {
		t.Errorf("got %d skills, want 1", len(resp.Skills))
	}
}

func TestEmbeddingFallback(t *testing.T) {
	llm := &mockLLM{response: classifyJSON("FIX", "Backend", []string{"FIX/Backend/DatabaseConnection"})}
	emb := &mockEmbedder{err: errors.New("circuit open")}
	store := &mockStore{results: []storage.ScoredSkill{
		makeSkill("s1", []string{"FIX/Backend/DatabaseConnection"}, 0.8),
	}}

	inj := newTestInjector(emb, store, llm, nil)
	resp, err := inj.Inject(context.Background(), extraction.InjectionRequest{
		Prompt:    "fix db connection",
		MaxSkills: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Degraded weights: 0.7*pattern + 0.3*historical.
	s := resp.Skills[0]
	want := 0.7*0.5 + 0.3*0.7 // from makeSkill defaults
	if !approxEqual(s.CompositeScore, want) {
		t.Errorf("composite = %f, want %f (degraded)", s.CompositeScore, want)
	}
}

func TestNilEmbedder(t *testing.T) {
	llm := &mockLLM{response: classifyJSON("BUILD", "Backend", []string{})}
	store := &mockStore{results: []storage.ScoredSkill{makeSkill("s1", nil, 0.5)}}

	inj := New(nil, store, llm, nil, slog.Default())
	resp, err := inj.Inject(context.Background(), extraction.InjectionRequest{
		Prompt:    "build something",
		MaxSkills: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	s := resp.Skills[0]
	want := 0.7*0.5 + 0.3*0.7
	if !approxEqual(s.CompositeScore, want) {
		t.Errorf("composite = %f, want %f (nil embedder)", s.CompositeScore, want)
	}
}

func TestEmptyLibrary(t *testing.T) {
	llm := &mockLLM{response: classifyJSON("BUILD", "Backend", []string{"BUILD/Backend/APIDesign"})}
	emb := &mockEmbedder{result: []float32{0.1}}
	store := &mockStore{results: nil}

	inj := newTestInjector(emb, store, llm, nil)
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

	inj := newTestInjector(emb, store, llm, nil)
	resp, err := inj.Inject(context.Background(), extraction.InjectionRequest{
		Prompt:    "help",
		MaxSkills: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Classification.Intent != "" {
		t.Errorf("expected empty intent on failure, got %q", resp.Classification.Intent)
	}
}

func TestMaxSkillsLimiting(t *testing.T) {
	llm := &mockLLM{response: classifyJSON("BUILD", "Backend", []string{})}
	emb := &mockEmbedder{result: []float32{0.1}}
	skills := make([]storage.ScoredSkill, 10)
	for i := range skills {
		skills[i] = makeSkill(fmt.Sprintf("s%d", i), nil, float64(i)*0.1)
		skills[i].PatternOverlap = float64(10-i) * 0.1
		skills[i].CompositeScore = float64(10-i) * 0.1
	}
	store := &mockStore{results: skills}

	inj := newTestInjector(emb, store, llm, nil)
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
	llm := &mockLLM{response: classifyJSON("BUILD", "Backend", []string{"BUILD/Backend/APIDesign"})}
	emb := &mockEmbedder{result: []float32{0.1}}

	s1 := makeSkill("low", nil, -0.5)
	s1.CompositeScore = 0.1

	s2 := makeSkill("high", nil, 0.9)
	s2.CompositeScore = 0.9

	// Store returns sorted — high first.
	store := &mockStore{results: []storage.ScoredSkill{s2, s1}}

	inj := newTestInjector(emb, store, llm, nil)
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
	if resp.Skills[0].SkillID != "high" {
		t.Errorf("expected 'high' first, got %q", resp.Skills[0].SkillID)
	}
	if resp.Skills[0].CompositeScore <= resp.Skills[1].CompositeScore {
		t.Error("scores not in descending order")
	}
}

func TestDefaultMaxSkills(t *testing.T) {
	llm := &mockLLM{response: classifyJSON("BUILD", "Backend", []string{})}
	emb := &mockEmbedder{result: []float32{0.1}}
	skills := make([]storage.ScoredSkill, 10)
	for i := range skills {
		skills[i] = makeSkill(fmt.Sprintf("s%d", i), nil, 0.5)
	}
	store := &mockStore{results: skills}

	inj := newTestInjector(emb, store, llm, nil)
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

func TestStoreQueryError(t *testing.T) {
	llm := &mockLLM{response: classifyJSON("BUILD", "Backend", []string{})}
	emb := &mockEmbedder{result: []float32{0.1}}
	store := &mockStore{err: errors.New("database locked")}

	inj := newTestInjector(emb, store, llm, nil)
	_, err := inj.Inject(context.Background(), extraction.InjectionRequest{
		Prompt:    "build",
		MaxSkills: 3,
	})
	if err == nil {
		t.Fatal("expected error from store query")
	}
	if !errors.Is(err, store.err) && err.Error() != "query skill store: database locked" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEmptyPrompt(t *testing.T) {
	llm := &mockLLM{response: classifyJSON("BUILD", "Backend", []string{})}
	emb := &mockEmbedder{result: []float32{0.1}}
	store := &mockStore{results: []storage.ScoredSkill{makeSkill("s1", nil, 0.5)}}

	inj := newTestInjector(emb, store, llm, nil)
	resp, err := inj.Inject(context.Background(), extraction.InjectionRequest{
		Prompt:    "",
		MaxSkills: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Empty prompt → classification fails → empty classification, but injection proceeds.
	if resp.Classification.Intent != "" {
		t.Errorf("expected empty intent for empty prompt, got %q", resp.Classification.Intent)
	}
}

func TestDomainValidation(t *testing.T) {
	llm := &mockLLM{response: classifyJSON("BUILD", "Go backend stuff", []string{"BUILD/Backend/APIDesign"})}
	emb := &mockEmbedder{result: []float32{0.1}}
	store := &mockStore{results: nil}

	inj := newTestInjector(emb, store, llm, nil)
	resp, err := inj.Inject(context.Background(), extraction.InjectionRequest{
		Prompt:    "build an API in Go",
		MaxSkills: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Unknown domain should default to "Backend".
	if resp.Classification.Domain != "Backend" {
		t.Errorf("domain = %q, want Backend (default)", resp.Classification.Domain)
	}
}

func TestInvalidIntentDefaultsToBuild(t *testing.T) {
	llm := &mockLLM{response: classifyJSON("YOLO", "Backend", []string{})}
	emb := &mockEmbedder{result: []float32{0.1}}
	store := &mockStore{results: nil}

	inj := newTestInjector(emb, store, llm, nil)
	resp, err := inj.Inject(context.Background(), extraction.InjectionRequest{
		Prompt:    "do something",
		MaxSkills: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Classification.Intent != "BUILD" {
		t.Errorf("intent = %q, want BUILD (default)", resp.Classification.Intent)
	}
}

func TestMarkdownFencedJSON(t *testing.T) {
	fenced := "```json\n" + classifyJSON("FIX", "Frontend", []string{"FIX/Frontend/RenderingBug"}) + "\n```"
	llm := &mockLLM{response: fenced}
	emb := &mockEmbedder{result: []float32{0.1}}
	store := &mockStore{results: nil}

	inj := newTestInjector(emb, store, llm, nil)
	resp, err := inj.Inject(context.Background(), extraction.InjectionRequest{
		Prompt:    "fix rendering",
		MaxSkills: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Classification.Intent != "FIX" {
		t.Errorf("intent = %q, want FIX", resp.Classification.Intent)
	}
	if len(resp.Classification.Patterns) != 1 || resp.Classification.Patterns[0] != "FIX/Frontend/RenderingBug" {
		t.Errorf("patterns = %v, want [FIX/Frontend/RenderingBug]", resp.Classification.Patterns)
	}
}

func TestPatternValidationFiltering(t *testing.T) {
	llm := &mockLLM{response: classifyJSON("BUILD", "Backend", []string{
		"BUILD/Backend/APIDesign",
		"INVALID/Pattern/Here",
		"BUILD/Backend/DataModeling",
	})}
	emb := &mockEmbedder{result: []float32{0.1}}
	store := &mockStore{results: nil}

	inj := newTestInjector(emb, store, llm, nil)
	resp, err := inj.Inject(context.Background(), extraction.InjectionRequest{
		Prompt:    "build API",
		MaxSkills: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Classification.Patterns) != 2 {
		t.Errorf("patterns = %v, want 2 valid patterns", resp.Classification.Patterns)
	}
	for _, p := range resp.Classification.Patterns {
		if p == "INVALID/Pattern/Here" {
			t.Error("invalid pattern should have been filtered")
		}
	}
}
