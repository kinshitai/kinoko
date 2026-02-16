//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/model"
	"github.com/kinoko-dev/kinoko/internal/storage"
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
		if results[i].CompositeScore > results[i-1].CompositeScore {
			t.Errorf("results not sorted: [%d].score=%f > [%d].score=%f",
				i, results[i].CompositeScore, i-1, results[i-1].CompositeScore)
		}
	}
	best := results[0]
	worst := results[len(results)-1]
	if best.HistoricalRate <= worst.HistoricalRate {
		t.Errorf("best historical rate %.2f should exceed worst %.2f", best.HistoricalRate, worst.HistoricalRate)
	}
}

func TestVersionChainIntegrity(t *testing.T) {
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

	v2 := &model.SkillRecord{
		ID: "v2-id", Name: "fix-db", Version: 2, ParentID: "v1-id", LibraryID: "test-lib",
		Category: model.CategoryTactical,
		Quality: model.QualityScores{
			ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 3,
			ReasoningTransparency: 3, TechnicalAccuracy: 4, VerificationEvidence: 3,
			InnovationLevel: 3, CompositeScore: 3.5, CriticConfidence: 0.8,
		},
		DecayScore: 1.0, ExtractedBy: "test", FilePath: "skills/fix-db/v2/SKILL.md",
	}
	if err := store.Put(ctx, v2, nil); err != nil {
		t.Fatal(err)
	}

	latest, err := store.GetLatestByName(ctx, "fix-db", "test-lib")
	if err != nil {
		t.Fatal(err)
	}
	if latest.Version != 2 {
		t.Errorf("latest version = %d, want 2", latest.Version)
	}
	if latest.ParentID != "v1-id" {
		t.Errorf("parent = %q, want v1-id", latest.ParentID)
	}

	old, err := store.Get(ctx, "v1-id")
	if err != nil {
		t.Fatal(err)
	}
	if old.Version != 1 {
		t.Errorf("v1 version = %d", old.Version)
	}
}

func TestSkillPutAtomicity(t *testing.T) {
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

	sk2 := &model.SkillRecord{
		ID: "atom-2", Name: "atomic-skill", Version: 1, LibraryID: "test-lib",
		Category: model.CategoryTactical, Patterns: []string{"BUILD/Backend/APIDesign"},
		Quality: model.QualityScores{
			ProblemSpecificity: 3, SolutionCompleteness: 3, ContextPortability: 3,
			ReasoningTransparency: 3, TechnicalAccuracy: 3, VerificationEvidence: 3,
			InnovationLevel: 3, CompositeScore: 3.0, CriticConfidence: 0.7,
		},
		DecayScore: 1.0, ExtractedBy: "test", FilePath: "skills/atomic/SKILL.md",
	}
	err := store.Put(ctx, sk2, nil)
	if err == nil {
		t.Fatal("expected duplicate error")
	}

	var patternCount int
	store.DB().QueryRow("SELECT COUNT(*) FROM skill_patterns WHERE skill_id = ?", "atom-2").Scan(&patternCount)
	if patternCount != 0 {
		t.Errorf("atom-2 patterns = %d, want 0 (should have rolled back)", patternCount)
	}

	got, err := store.Get(ctx, "atom-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "atomic-skill" {
		t.Errorf("original skill corrupted: name = %q", got.Name)
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
