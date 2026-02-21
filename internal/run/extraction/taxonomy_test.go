package extraction

import (
	"testing"

	"github.com/kinoko-dev/kinoko/pkg/model"
)

// Tests for taxonomy/validation in stage2.go — R5 area.
// Must exist BEFORE extracting to internal/taxonomy package.

func TestValidPattern(t *testing.T) {
	for _, p := range Taxonomy {
		if !ValidPattern(p) {
			t.Errorf("ValidPattern(%q) = false, should be true (in Taxonomy)", p)
		}
	}

	invalids := []string{"", "FAKE/Pattern", "FIX", "FIX/Backend", "fix/backend/databaseconnection"}
	for _, p := range invalids {
		if ValidPattern(p) {
			t.Errorf("ValidPattern(%q) = true, should be false", p)
		}
	}
}

func TestTaxonomy_NotEmpty(t *testing.T) {
	if len(Taxonomy) == 0 {
		t.Fatal("Taxonomy is empty")
	}
	if len(Taxonomy) != 20 {
		t.Errorf("Taxonomy has %d entries, expected 20", len(Taxonomy))
	}
}

func TestValidateCategory(t *testing.T) {
	tests := []struct {
		input model.SkillCategory
		want  model.SkillCategory
	}{
		{model.CategoryFoundational, model.CategoryFoundational},
		{model.CategoryTactical, model.CategoryTactical},
		{model.CategoryContextual, model.CategoryContextual},
		{"", model.CategoryTactical},
		{"invalid", model.CategoryTactical},
		{"strategic", model.CategoryTactical},
		{"TACTICAL", model.CategoryTactical},
	}
	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			got := validateCategory(tt.input)
			if got != tt.want {
				t.Errorf("validateCategory(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidatePatterns(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  int
	}{
		{"all valid", []string{"FIX/Backend/DatabaseConnection", "BUILD/Frontend/ComponentDesign"}, 2},
		{"mixed", []string{"FIX/Backend/DatabaseConnection", "FAKE/Pattern"}, 1},
		{"all invalid", []string{"FAKE/A", "FAKE/B"}, 0},
		{"empty", []string{}, 0},
		{"nil", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validatePatterns(tt.input)
			if len(got) != tt.want {
				t.Errorf("validatePatterns returned %d patterns, want %d", len(got), tt.want)
			}
		})
	}
}

func TestCompositeScore(t *testing.T) {
	all5 := model.QualityScores{
		ProblemSpecificity: 5, SolutionCompleteness: 5, ContextPortability: 5,
		ReasoningTransparency: 5, TechnicalAccuracy: 5, VerificationEvidence: 5, InnovationLevel: 5,
	}
	if cs := compositeScore(all5); cs != 5.0 {
		t.Errorf("compositeScore(all 5s) = %f, want 5.0", cs)
	}

	all1 := model.QualityScores{
		ProblemSpecificity: 1, SolutionCompleteness: 1, ContextPortability: 1,
		ReasoningTransparency: 1, TechnicalAccuracy: 1, VerificationEvidence: 1, InnovationLevel: 1,
	}
	if cs := compositeScore(all1); cs != 1.0 {
		t.Errorf("compositeScore(all 1s) = %f, want 1.0", cs)
	}

	if cs := compositeScore(model.QualityScores{}); cs != 0.0 {
		t.Errorf("compositeScore(zeros) = %f, want 0.0", cs)
	}
}

func TestRubricScoresJSON_Validate(t *testing.T) {
	tests := []struct {
		name    string
		scores  rubricScoresJSON
		wantErr bool
	}{
		{"valid", rubricScoresJSON{4, 4, 3, 3, 4, 3, 3}, false},
		{"all 1s", rubricScoresJSON{1, 1, 1, 1, 1, 1, 1}, false},
		{"all 5s", rubricScoresJSON{5, 5, 5, 5, 5, 5, 5}, false},
		{"zero score", rubricScoresJSON{0, 4, 3, 3, 4, 3, 3}, true},
		{"score 6", rubricScoresJSON{6, 4, 3, 3, 4, 3, 3}, true},
		{"negative", rubricScoresJSON{-1, 4, 3, 3, 4, 3, 3}, true},
		{"all zero", rubricScoresJSON{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.scores.validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("validate() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
