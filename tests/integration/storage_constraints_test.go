//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/serve/storage"
	"github.com/kinoko-dev/kinoko/pkg/model"
)

func TestMultipleSkillsQueryOrdering(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	embedder := newPredictableEmbedder(3)

	for i, tc := range []struct {
		name        string
		composite   float64
		successCorr float64
		patterns    []string
	}{
		{"low-quality", 2.0, -0.5, []string{"FIX/Backend/DatabaseConnection"}},
		{"mid-quality", 3.5, 0.3, []string{"FIX/Backend/DatabaseConnection"}},
		{"high-quality", 4.5, 0.8, []string{"FIX/Backend/DatabaseConnection"}},
	} {
		sk := &model.SkillRecord{
			ID: fmt.Sprintf("skill-order-%d", i), Name: tc.name, Version: 1,
			LibraryID: "test-lib", Category: model.CategoryTactical, Patterns: tc.patterns,
			Quality: model.QualityScores{
				ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
				ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
				InnovationLevel: 3, CompositeScore: tc.composite, CriticConfidence: 0.8,
			},
			Embedding:  embedder.deterministicVector(tc.name),
			DecayScore: 1.0, SuccessCorrelation: tc.successCorr,
			ExtractedBy: "test", FilePath: fmt.Sprintf("skills/%s/SKILL.md", tc.name),
		}
		if err := store.Put(ctx, sk, nil); err != nil {
			t.Fatal(err)
		}
	}

	results, err := store.Query(ctx, storage.SkillQuery{
		Patterns: []string{"FIX/Backend/DatabaseConnection"}, LibraryIDs: []string{"test-lib"}, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	for i := 1; i < len(results); i++ {
		if results[i].PatternOverlap > results[i-1].PatternOverlap {
			t.Errorf("results not sorted: [%d].score=%f > [%d].score=%f",
				i, results[i].PatternOverlap, i-1, results[i-1].PatternOverlap)
		}
	}
}

func TestVersionChainIntegrity(t *testing.T) {
	// With git-first upsert, Put with same name+library replaces the row.
	// Version chain is maintained via the upsert: v2 overwrites v1.
	store := newTestStore(t)
	ctx := context.Background()

	v1 := &model.SkillRecord{
		ID: "v1-id", Name: "fix-db", Version: 1, LibraryID: "test-lib",
		Category: model.CategoryTactical,
		Quality: model.QualityScores{
			ProblemSpecificity: 3, SolutionCompleteness: 3, ContextPortability: 3,
			ReasoningTransparency: 3, TechnicalAccuracy: 3, VerificationEvidence: 3,
			InnovationLevel: 3, CompositeScore: 3.0, CriticConfidence: 0.7,
		},
		DecayScore: 1.0, ExtractedBy: "test", FilePath: "skills/fix-db/v1/SKILL.md",
	}
	if err := store.Put(ctx, v1, nil); err != nil {
		t.Fatal(err)
	}

	// Upsert v2 — same name+library, replaces v1 row in-place.
	v2 := &model.SkillRecord{
		ID: "v2-id", Name: "fix-db", Version: 2, LibraryID: "test-lib",
		Category: model.CategoryTactical,
		Quality: model.QualityScores{
			ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
			ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
			InnovationLevel: 3, CompositeScore: 3.5, CriticConfidence: 0.8,
		},
		DecayScore: 1.0, ExtractedBy: "test", FilePath: "skills/fix-db/v2/SKILL.md",
	}
	if err := store.Put(ctx, v2, nil); err != nil {
		t.Fatal("upsert skill:", err)
	}

	latest, err := store.GetLatestByName(ctx, "fix-db", "test-lib")
	if err != nil {
		t.Fatal(err)
	}
	if latest.Version != 2 {
		t.Errorf("latest version = %d, want 2", latest.Version)
	}
	if latest.ID != "v2-id" {
		t.Errorf("latest ID = %q, want v2-id", latest.ID)
	}

	// v1-id should no longer exist (replaced by upsert).
	_, err = store.Get(ctx, "v1-id")
	if err == nil {
		t.Error("expected v1-id to be gone after upsert, but it still exists")
	}
}

func TestSkillPutUpsert(t *testing.T) {
	// With git-first, Put upserts by name+library. Second Put replaces first.
	store := newTestStore(t)
	ctx := context.Background()

	sk1 := &model.SkillRecord{
		ID: "atom-1", Name: "atomic-skill", Version: 1, LibraryID: "test-lib",
		Category: model.CategoryTactical, Patterns: []string{"FIX/Backend/DatabaseConnection"},
		Quality: model.QualityScores{
			ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
			ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
			InnovationLevel: 3, CompositeScore: 3.5, CriticConfidence: 0.8,
		},
		DecayScore: 1.0, ExtractedBy: "test", FilePath: "skills/atomic/SKILL.md",
	}
	if err := store.Put(ctx, sk1, nil); err != nil {
		t.Fatal(err)
	}

	// Upsert: same name+library, different ID and patterns.
	sk2 := &model.SkillRecord{
		ID: "atom-2", Name: "atomic-skill", Version: 2, LibraryID: "test-lib",
		Category: model.CategoryTactical, Patterns: []string{"BUILD/Backend/APIDesign"},
		Quality: model.QualityScores{
			ProblemSpecificity: 3, SolutionCompleteness: 3, ContextPortability: 3,
			ReasoningTransparency: 3, TechnicalAccuracy: 3, VerificationEvidence: 3,
			InnovationLevel: 3, CompositeScore: 3.0, CriticConfidence: 0.7,
		},
		DecayScore: 1.0, ExtractedBy: "test", FilePath: "skills/atomic/SKILL.md",
	}
	if err := store.Put(ctx, sk2, nil); err != nil {
		t.Fatal("upsert should succeed:", err)
	}

	// Only one row should exist for this name+library.
	var count int
	store.DB().QueryRow("SELECT COUNT(*) FROM skills WHERE name = ? AND library_id = ?",
		"atomic-skill", "test-lib").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 skill row after upsert, got %d", count)
	}

	// The latest values should be from sk2.
	got, err := store.Get(ctx, "atom-2")
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 2 {
		t.Errorf("version = %d, want 2", got.Version)
	}
}

func TestSchemaConstraints(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	t.Run("invalid category rejected", func(t *testing.T) {
		sk := &model.SkillRecord{
			ID: "bad-cat", Name: "bad-category", Version: 1, LibraryID: "test-lib",
			Category: model.SkillCategory("invalid"),
			Quality: model.QualityScores{
				ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
				ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
				InnovationLevel: 3, CompositeScore: 3.5, CriticConfidence: 0.8,
			},
			DecayScore: 1.0, ExtractedBy: "test", FilePath: "skills/bad/SKILL.md",
		}
		if err := store.Put(ctx, sk, nil); err == nil {
			t.Error("expected error for invalid category")
		}
	})

	t.Run("quality score out of range rejected", func(t *testing.T) {
		sk := &model.SkillRecord{
			ID: "bad-score", Name: "bad-score", Version: 1, LibraryID: "test-lib",
			Category: model.CategoryTactical,
			Quality: model.QualityScores{
				ProblemSpecificity: 10, SolutionCompleteness: 4, ContextPortability: 3,
				ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
				InnovationLevel: 3, CompositeScore: 3.5, CriticConfidence: 0.8,
			},
			DecayScore: 1.0, ExtractedBy: "test", FilePath: "skills/bad/SKILL.md",
		}
		if err := store.Put(ctx, sk, nil); err == nil {
			t.Error("expected error for quality score out of range")
		}
	})

	t.Run("confidence out of range rejected", func(t *testing.T) {
		sk := &model.SkillRecord{
			ID: "bad-conf", Name: "bad-conf", Version: 1, LibraryID: "test-lib",
			Category: model.CategoryTactical,
			Quality: model.QualityScores{
				ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
				ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
				InnovationLevel: 3, CompositeScore: 3.5, CriticConfidence: 1.5,
			},
			DecayScore: 1.0, ExtractedBy: "test", FilePath: "skills/bad/SKILL.md",
		}
		if err := store.Put(ctx, sk, nil); err == nil {
			t.Error("expected error for confidence out of range")
		}
	})
}
