package storage

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kinoko-dev/kinoko/pkg/model"
)

func testStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := NewSQLiteStore(":memory:", "test-model")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func testSkill(id, name, lib string) *model.SkillRecord {
	return &model.SkillRecord{
		ID:        id,
		Name:      name,
		Version:   1,
		LibraryID: lib,
		Category:  model.CategoryTactical,
		Quality: model.QualityScores{
			ProblemSpecificity:    4,
			SolutionCompleteness:  4,
			ContextPortability:    3,
			ReasoningTransparency: 3,
			TechnicalAccuracy:     4,
			VerificationEvidence:  3,
			InnovationLevel:       3,
			CompositeScore:        3.5,
			CriticConfidence:      0.8,
		},
		DecayScore:  1.0,
		ExtractedBy: "test-v1",
		FilePath:    "skills/" + name + "/SKILL.md",
		Patterns:    []string{"FIX/Backend/DatabaseConnection"},
		Embedding:   []float32{0.1, 0.2, 0.3},
	}
}

func TestPutAndGet(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sk := testSkill("id-1", "fix-db-conn", "default")
	if err := s.Put(ctx, sk, []byte("body")); err != nil {
		t.Fatalf("put: %v", err)
	}

	got, err := s.Get(ctx, "id-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "fix-db-conn" {
		t.Errorf("name = %q, want fix-db-conn", got.Name)
	}
	if got.Quality.ProblemSpecificity != 4 {
		t.Errorf("problem_specificity = %d, want 4", got.Quality.ProblemSpecificity)
	}
	if len(got.Patterns) != 1 || got.Patterns[0] != "FIX/Backend/DatabaseConnection" {
		t.Errorf("patterns = %v, want [FIX/Backend/DatabaseConnection]", got.Patterns)
	}
	if len(got.Embedding) != 3 {
		t.Errorf("embedding len = %d, want 3", len(got.Embedding))
	}
	if got.DecayScore != 1.0 {
		t.Errorf("decay = %f, want 1.0", got.DecayScore)
	}
}

func TestGetNotFound(t *testing.T) {
	s := testStore(t)
	_, err := s.Get(context.Background(), "nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetLatestByName(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	// Git-first: one row per name+library. Put upserts to latest version.
	v1 := testSkill("id-v1", "fix-db-conn", "default")
	v1.Version = 1
	if err := s.Put(ctx, v1, nil); err != nil {
		t.Fatalf("put v1: %v", err)
	}

	// Upsert same name+library with newer version.
	v2 := testSkill("id-v2", "fix-db-conn", "default")
	v2.Version = 2
	if err := s.Put(ctx, v2, nil); err != nil {
		t.Fatalf("put v2 (upsert): %v", err)
	}

	got, err := s.GetLatestByName(ctx, "fix-db-conn", "default")
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	if got.Version != 2 {
		t.Errorf("version = %d, want 2", got.Version)
	}
}

func TestGetLatestByNameNotFound(t *testing.T) {
	s := testStore(t)
	_, err := s.GetLatestByName(context.Background(), "nope", "default")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestQueryPatternOverlap(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sk := testSkill("id-1", "fix-db-conn", "default")
	sk.Patterns = []string{"FIX/Backend/DatabaseConnection", "FIX/Backend/AuthFlow"}
	if err := s.Put(ctx, sk, nil); err != nil {
		t.Fatalf("put: %v", err)
	}

	results, err := s.Query(ctx, SkillQuery{
		Patterns:   []string{"FIX/Backend/DatabaseConnection", "FIX/Backend/AuthFlow", "BUILD/Frontend/ComponentDesign"},
		LibraryIDs: []string{"default"},
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}

	// 2 out of 3 query patterns matched
	wantOverlap := 2.0 / 3.0
	if math.Abs(results[0].PatternOverlap-wantOverlap) > 0.001 {
		t.Errorf("pattern overlap = %f, want %f", results[0].PatternOverlap, wantOverlap)
	}
}

func TestQueryCosineSimilarity(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sk := testSkill("id-1", "fix-db-conn", "default")
	sk.Embedding = []float32{1, 0, 0}
	if err := s.Put(ctx, sk, nil); err != nil {
		t.Fatalf("put: %v", err)
	}

	results, err := s.Query(ctx, SkillQuery{
		Embedding:  []float32{1, 0, 0},
		LibraryIDs: []string{"default"},
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if math.Abs(results[0].CosineSim-1.0) > 0.001 {
		t.Errorf("cosine sim = %f, want 1.0", results[0].CosineSim)
	}
}

func TestQueryMinQuality(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sk := testSkill("id-1", "fix-db-conn", "default")
	sk.Quality.CompositeScore = 2.0
	if err := s.Put(ctx, sk, nil); err != nil {
		t.Fatalf("put: %v", err)
	}

	results, err := s.Query(ctx, SkillQuery{
		LibraryIDs: []string{"default"},
		MinQuality: 3.0,
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for min quality filter, got %d", len(results))
	}
}

func TestQueryMinDecay(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sk := testSkill("id-1", "fix-db-conn", "default")
	sk.DecayScore = 0.01
	if err := s.Put(ctx, sk, nil); err != nil {
		t.Fatalf("put: %v", err)
	}

	results, err := s.Query(ctx, SkillQuery{
		LibraryIDs: []string{"default"},
		MinDecay:   0.1,
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for min decay filter, got %d", len(results))
	}
}

func TestQueryCompositeScoreOrdering(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	a := testSkill("id-a", "skill-a", "default")
	a.Patterns = []string{"FIX/Backend/DatabaseConnection"}
	a.Embedding = nil
	a.SuccessCorrelation = 0.5
	if err := s.Put(ctx, a, nil); err != nil {
		t.Fatalf("put a: %v", err)
	}

	b := testSkill("id-b", "skill-b", "default")
	b.Patterns = []string{"BUILD/Frontend/ComponentDesign"}
	b.Embedding = []float32{1, 0, 0}
	b.SuccessCorrelation = -0.5
	if err := s.Put(ctx, b, nil); err != nil {
		t.Fatalf("put b: %v", err)
	}

	results, err := s.Query(ctx, SkillQuery{
		Patterns:   []string{"FIX/Backend/DatabaseConnection"},
		Embedding:  []float32{1, 0, 0},
		LibraryIDs: []string{"default"},
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	if results[0].CompositeScore < results[1].CompositeScore {
		t.Error("results not sorted by composite score descending")
	}
}

func TestQueryLimit(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		sk := testSkill(fmt.Sprintf("id-%d", i), fmt.Sprintf("skill-%d", i), "default")
		if err := s.Put(ctx, sk, nil); err != nil {
			t.Fatalf("put %d: %v", i, err)
		}
	}

	results, err := s.Query(ctx, SkillQuery{LibraryIDs: []string{"default"}, Limit: 2})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("results = %d, want 2", len(results))
	}
}

func TestUpdateUsage(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sk := testSkill("id-1", "fix-db-conn", "default")
	if err := s.Put(ctx, sk, nil); err != nil {
		t.Fatalf("put: %v", err)
	}

	if err := s.UpdateUsage(ctx, "id-1", ""); err != nil {
		t.Fatalf("update usage: %v", err)
	}

	got, err := s.Get(ctx, "id-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.InjectionCount != 1 {
		t.Errorf("injection_count = %d, want 1", got.InjectionCount)
	}
	if got.LastInjectedAt.IsZero() {
		t.Error("last_injected_at should be set")
	}
}

func TestUpdateDecay(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sk := testSkill("id-1", "fix-db-conn", "default")
	if err := s.Put(ctx, sk, nil); err != nil {
		t.Fatalf("put: %v", err)
	}

	if err := s.UpdateDecay(ctx, "id-1", 0.42); err != nil {
		t.Fatalf("update decay: %v", err)
	}

	got, err := s.Get(ctx, "id-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if math.Abs(got.DecayScore-0.42) > 0.001 {
		t.Errorf("decay = %f, want 0.42", got.DecayScore)
	}
}

func TestListByDecay(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	for _, d := range []struct {
		id    string
		name  string
		decay float64
	}{
		{"id-a", "skill-a", 0.9},
		{"id-b", "skill-b", 0.1},
		{"id-c", "skill-c", 0.5},
	} {
		sk := testSkill(d.id, d.name, "default")
		sk.DecayScore = d.decay
		if err := s.Put(ctx, sk, nil); err != nil {
			t.Fatalf("put %s: %v", d.id, err)
		}
	}

	results, err := s.ListByDecay(ctx, "default", 10)
	if err != nil {
		t.Fatalf("list by decay: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	if results[0].DecayScore > results[1].DecayScore || results[1].DecayScore > results[2].DecayScore {
		t.Errorf("not sorted ascending: %f, %f, %f", results[0].DecayScore, results[1].DecayScore, results[2].DecayScore)
	}
}

func TestListByDecayLimit(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		sk := testSkill(fmt.Sprintf("id-%d", i), fmt.Sprintf("skill-%d", i), "default")
		sk.DecayScore = float64(i) * 0.2
		if err := s.Put(ctx, sk, nil); err != nil {
			t.Fatalf("put %d: %v", i, err)
		}
	}

	results, err := s.ListByDecay(ctx, "default", 2)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("results = %d, want 2", len(results))
	}
}

func TestListByDecayZeroLimitReturnsAll(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		sk := testSkill(fmt.Sprintf("id-nolim-%d", i), fmt.Sprintf("skill-nolim-%d", i), "default")
		sk.DecayScore = float64(i) * 0.2
		if err := s.Put(ctx, sk, nil); err != nil {
			t.Fatalf("put %d: %v", i, err)
		}
	}

	// limit=0 should return all rows, not zero rows.
	results, err := s.ListByDecay(ctx, "default", 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(results) != 5 {
		t.Errorf("limit=0 returned %d rows, want 5 (all)", len(results))
	}
}

func TestListByDecayLibraryFilter(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sk1 := testSkill("id-1", "skill-a", "lib-a")
	sk2 := testSkill("id-2", "skill-b", "lib-b")
	s.Put(ctx, sk1, nil)
	s.Put(ctx, sk2, nil)

	results, err := s.ListByDecay(ctx, "lib-a", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("results = %d, want 1", len(results))
	}
}

func TestCosineSimilarity(t *testing.T) {
	if v := cosineSimilarity([]float32{1, 0, 0}, []float32{1, 0, 0}); math.Abs(v-1.0) > 0.001 {
		t.Errorf("identical = %f", v)
	}
	if v := cosineSimilarity([]float32{1, 0, 0}, []float32{0, 1, 0}); math.Abs(v) > 0.001 {
		t.Errorf("orthogonal = %f", v)
	}
	if v := cosineSimilarity(nil, nil); v != 0 {
		t.Errorf("empty = %f", v)
	}
}

func TestPutTimestamps(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	before := time.Now().UTC().Add(-time.Second)
	sk := testSkill("id-1", "fix-db-conn", "default")
	if err := s.Put(ctx, sk, nil); err != nil {
		t.Fatalf("put: %v", err)
	}

	got, err := s.Get(ctx, "id-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.CreatedAt.Before(before) {
		t.Error("created_at too old")
	}
	if got.UpdatedAt.Before(before) {
		t.Error("updated_at too old")
	}
}

func TestPutNoEmbedding(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sk := testSkill("id-1", "fix-db-conn", "default")
	sk.Embedding = nil
	if err := s.Put(ctx, sk, nil); err != nil {
		t.Fatalf("put: %v", err)
	}

	got, err := s.Get(ctx, "id-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Embedding != nil {
		t.Errorf("expected nil embedding, got %v", got.Embedding)
	}
}

func TestUpsertSameNameLibrary(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	sk := testSkill("id-1", "fix-db-conn", "default")
	sk.Version = 1
	if err := s.Put(ctx, sk, nil); err != nil {
		t.Fatalf("put: %v", err)
	}

	// Upsert with same name+library should succeed and update.
	sk2 := testSkill("id-2", "fix-db-conn", "default")
	sk2.Version = 2
	if err := s.Put(ctx, sk2, nil); err != nil {
		t.Fatalf("upsert should succeed: %v", err)
	}

	got, err := s.GetLatestByName(ctx, "fix-db-conn", "default")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Version != 2 {
		t.Errorf("version = %d, want 2", got.Version)
	}
}

func TestPutWritesBodyToDisk(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	dir := t.TempDir()
	sk := testSkill("id-1", "fix-db-conn", "default")
	sk.FilePath = filepath.Join(dir, "skills", "fix-db-conn", "SKILL.md")
	body := []byte("# Fix DB Connection\n\nRestart the pool.")

	if err := s.Put(ctx, sk, body); err != nil {
		t.Fatalf("put: %v", err)
	}

	got, err := os.ReadFile(sk.FilePath)
	if err != nil {
		t.Fatalf("read body file: %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("body = %q, want %q", got, body)
	}
}

func TestEmbeddingModelConfigurable(t *testing.T) {
	s, err := NewSQLiteStore(":memory:", "custom-embed-v2")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer s.Close()

	if s.embeddingModel != "custom-embed-v2" {
		t.Errorf("embedding model = %q, want custom-embed-v2", s.embeddingModel)
	}
}

func TestEmbeddingModelDefault(t *testing.T) {
	s, err := NewSQLiteStore(":memory:", "")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer s.Close()

	if s.embeddingModel != "text-embedding-3-small" {
		t.Errorf("embedding model = %q, want text-embedding-3-small", s.embeddingModel)
	}
}

func TestSentinelErrors(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	// ErrNotFound from Get
	_, err := s.Get(ctx, "nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Get: expected ErrNotFound, got %v", err)
	}

	// ErrNotFound from GetLatestByName
	_, err = s.GetLatestByName(ctx, "nope", "default")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetLatestByName: expected ErrNotFound, got %v", err)
	}

	// Upsert same name+library should succeed (git-first: one row per name+library).
	sk := testSkill("id-1", "fix-db-conn", "default")
	if err := s.Put(ctx, sk, nil); err != nil {
		t.Fatalf("put: %v", err)
	}
	sk2 := testSkill("id-2", "fix-db-conn", "default")
	sk2.Version = 2
	if err = s.Put(ctx, sk2, nil); err != nil {
		t.Errorf("Put upsert: expected success, got %v", err)
	}
}
