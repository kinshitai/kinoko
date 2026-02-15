package model

import "testing"

func TestQualityScores_MinimumViable(t *testing.T) {
	tests := []struct {
		name   string
		scores QualityScores
		want   bool
	}{
		{"all zeros", QualityScores{}, false},
		{"just below threshold", QualityScores{ProblemSpecificity: 2, SolutionCompleteness: 3, TechnicalAccuracy: 3}, false},
		{"exactly at threshold", QualityScores{ProblemSpecificity: 3, SolutionCompleteness: 3, TechnicalAccuracy: 3}, true},
		{"above threshold", QualityScores{ProblemSpecificity: 5, SolutionCompleteness: 5, TechnicalAccuracy: 5}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.scores.MinimumViable(); got != tt.want {
				t.Errorf("MinimumViable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestQualityScores_HighValue(t *testing.T) {
	tests := []struct {
		name   string
		scores QualityScores
		want   bool
	}{
		{"all zeros", QualityScores{}, false},
		{"all threes", QualityScores{
			ProblemSpecificity: 3, SolutionCompleteness: 3, ContextPortability: 3,
			ReasoningTransparency: 3, TechnicalAccuracy: 3, VerificationEvidence: 3, InnovationLevel: 3,
		}, false},
		{"all fours", QualityScores{
			ProblemSpecificity: 4, SolutionCompleteness: 4, ContextPortability: 4,
			ReasoningTransparency: 4, TechnicalAccuracy: 4, VerificationEvidence: 4, InnovationLevel: 4,
		}, true},
		{"mixed averaging to 4", QualityScores{
			ProblemSpecificity: 5, SolutionCompleteness: 5, ContextPortability: 5,
			ReasoningTransparency: 5, TechnicalAccuracy: 3, VerificationEvidence: 3, InnovationLevel: 2,
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.scores.HighValue(); got != tt.want {
				t.Errorf("HighValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestQualityScores_InjectionPriority(t *testing.T) {
	q := QualityScores{ContextPortability: 5, VerificationEvidence: 3}
	want := 5*0.6 + 3*0.4 // 4.2
	got := q.InjectionPriority()
	if got != want {
		t.Errorf("InjectionPriority() = %v, want %v", got, want)
	}
}

func TestExtractionStatusConstants(t *testing.T) {
	statuses := map[ExtractionStatus]string{
		StatusQueued:    "queued",
		StatusPending:   "pending",
		StatusExtracted: "extracted",
		StatusRejected:  "rejected",
		StatusError:     "error",
		StatusFailed:    "failed",
	}
	for s, want := range statuses {
		if string(s) != want {
			t.Errorf("status %v = %q, want %q", s, string(s), want)
		}
	}
}

func TestSkillCategoryConstants(t *testing.T) {
	cats := map[SkillCategory]string{
		CategoryFoundational: "foundational",
		CategoryTactical:     "tactical",
		CategoryContextual:   "contextual",
	}
	for c, want := range cats {
		if string(c) != want {
			t.Errorf("category %v = %q, want %q", c, string(c), want)
		}
	}
}

func TestValidateDomain(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Frontend", "Frontend"},
		{"Backend", "Backend"},
		{"DevOps", "DevOps"},
		{"Data", "Data"},
		{"Security", "Security"},
		{"Performance", "Performance"},
		{"Unknown", "Backend"},
		{"", "Backend"},
		{"frontend", "Backend"}, // case-sensitive
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ValidateDomain(tt.input); got != tt.want {
				t.Errorf("ValidateDomain(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidDomains(t *testing.T) {
	expected := []string{"Frontend", "Backend", "DevOps", "Data", "Security", "Performance"}
	for _, d := range expected {
		if !ValidDomains[d] {
			t.Errorf("ValidDomains[%q] should be true", d)
		}
	}
	if ValidDomains["Nonexistent"] {
		t.Error("ValidDomains should not contain Nonexistent")
	}
}
