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

func TestQueryRelevanceOrdering(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	a := testSkill("id-a", "skill-a", "default")
	a.Patterns = []string{"FIX/Backend/DatabaseConnection"}
	a.Embedding = nil
	if err := s.Put(ctx, a, nil); err != nil {
		t.Fatalf("put a: %v", err)
	}

	b := testSkill("id-b", "skill-b", "default")
	b.Patterns = []string{"BUILD/Frontend/ComponentDesign"}
	b.Embedding = []float32{1, 0, 0}
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
	// Results should be sorted by relevance (pattern overlap) descending
	if results[0].PatternOverlap < results[1].PatternOverlap {
		t.Error("results not sorted by pattern overlap descending")
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
