package storage

import (
	"context"
	"testing"
	"time"

	"github.com/kinoko-dev/kinoko/internal/model"
)

func TestSQLiteIndexer_IndexSkill(t *testing.T) {
	store, err := NewSQLiteStore("file::memory:?cache=shared", "test-model")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	idx := NewSQLiteIndexer(store)
	ctx := context.Background()
	now := time.Now().UTC()

	skill := &model.SkillRecord{
		ID:        "idx-001",
		Name:      "test-skill",
		Version:   1,
		LibraryID: "local",
		Category:  model.CategoryFoundational,
		Patterns:  []string{"error-handling", "retry"},
		Quality: model.QualityScores{
			ProblemSpecificity:    4,
			SolutionCompleteness:  4,
			ContextPortability:    3,
			ReasoningTransparency: 3,
			TechnicalAccuracy:     4,
			VerificationEvidence:  3,
			InnovationLevel:       3,
			CompositeScore:        3.6,
			CriticConfidence:      0.85,
		},
		SourceSessionID: "sess-1",
		ExtractedBy:     "test",
		FilePath:        "skills/test-skill/v1/SKILL.md",
		DecayScore:      1.0,
		CreatedAt:       now,
	}
	emb := []float32{0.1, 0.2, 0.3}

	if err := idx.IndexSkill(ctx, skill, emb); err != nil {
		t.Fatalf("IndexSkill: %v", err)
	}

	// Verify skill was inserted.
	got, err := store.Get(ctx, "idx-001")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "test-skill" {
		t.Errorf("name = %q, want test-skill", got.Name)
	}
	if len(got.Patterns) != 2 {
		t.Errorf("patterns = %d, want 2", len(got.Patterns))
	}
	if len(got.Embedding) != 3 {
		t.Errorf("embedding = %d, want 3", len(got.Embedding))
	}

	// Upsert: change version and patterns.
	skill.Version = 2
	skill.Patterns = []string{"error-handling", "retry", "backoff"}
	emb2 := []float32{0.4, 0.5, 0.6}

	if err := idx.IndexSkill(ctx, skill, emb2); err != nil {
		t.Fatalf("IndexSkill upsert: %v", err)
	}

	got, err = store.Get(ctx, "idx-001")
	if err != nil {
		t.Fatalf("Get after upsert: %v", err)
	}
	if got.Version != 2 {
		t.Errorf("version = %d, want 2", got.Version)
	}
	if len(got.Patterns) != 3 {
		t.Errorf("patterns after upsert = %d, want 3", len(got.Patterns))
	}
	if len(got.Embedding) != 3 {
		t.Fatalf("embedding after upsert = %d, want 3", len(got.Embedding))
	}
	if got.Embedding[0] != 0.4 {
		t.Errorf("embedding[0] = %f, want 0.4", got.Embedding[0])
	}
}

func TestSQLiteIndexer_ValidationRejectsEmptyID(t *testing.T) {
	store, err := NewSQLiteStore("file::memory:?cache=shared", "test-model")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	idx := NewSQLiteIndexer(store)
	skill := &model.SkillRecord{ID: "", Name: "test"}
	if err := idx.IndexSkill(context.Background(), skill, nil); err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestSQLiteIndexer_ValidationRejectsEmptyName(t *testing.T) {
	store, err := NewSQLiteStore("file::memory:?cache=shared", "test-model")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	idx := NewSQLiteIndexer(store)
	skill := &model.SkillRecord{ID: "x", Name: ""}
	if err := idx.IndexSkill(context.Background(), skill, nil); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestSQLiteIndexer_NoStructMutation(t *testing.T) {
	store, err := NewSQLiteStore("file::memory:?cache=shared", "test-model")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	idx := NewSQLiteIndexer(store)
	now := time.Now().UTC()
	skill := &model.SkillRecord{
		ID:          "mut-001",
		Name:        "test-mutation",
		Version:     1,
		LibraryID:   "local",
		Category:    model.CategoryFoundational,
		ExtractedBy: "test",
		FilePath:    "skills/test/v1/SKILL.md",
		DecayScore:  1.0,
		CreatedAt:   now,
		UpdatedAt:   now,
		Quality: model.QualityScores{
			ProblemSpecificity: 3, SolutionCompleteness: 3, ContextPortability: 3,
			ReasoningTransparency: 3, TechnicalAccuracy: 3, VerificationEvidence: 3,
			InnovationLevel: 3, CompositeScore: 3.0, CriticConfidence: 0.8,
		},
	}

	originalCreated := skill.CreatedAt
	originalUpdated := skill.UpdatedAt

	if err := idx.IndexSkill(context.Background(), skill, nil); err != nil {
		t.Fatal(err)
	}

	if !skill.CreatedAt.Equal(originalCreated) {
		t.Error("IndexSkill mutated CreatedAt")
	}
	if !skill.UpdatedAt.Equal(originalUpdated) {
		t.Error("IndexSkill mutated UpdatedAt")
	}
}

func TestSQLiteIndexer_NilEmbedding(t *testing.T) {
	store, err := NewSQLiteStore("file::memory:?cache=shared", "test-model")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	idx := NewSQLiteIndexer(store)
	ctx := context.Background()

	skill := &model.SkillRecord{
		ID:        "idx-002",
		Name:      "no-emb",
		Version:   1,
		LibraryID: "local",
		Category:  model.CategoryTactical,
		Quality: model.QualityScores{
			ProblemSpecificity:    3,
			SolutionCompleteness:  3,
			ContextPortability:    3,
			ReasoningTransparency: 3,
			TechnicalAccuracy:     3,
			VerificationEvidence:  3,
			InnovationLevel:       3,
			CompositeScore:        3.0,
			CriticConfidence:      0.8,
		},
	}

	if err := idx.IndexSkill(ctx, skill, nil); err != nil {
		t.Fatalf("IndexSkill without embedding: %v", err)
	}

	got, err := store.Get(ctx, "idx-002")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Embedding) != 0 {
		t.Errorf("embedding = %d, want 0", len(got.Embedding))
	}
}
