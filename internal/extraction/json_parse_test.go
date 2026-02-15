package extraction

import (
	"testing"

	"github.com/kinoko-dev/kinoko/internal/llmutil"
)

func TestParseCriticResponse_AllStrategies(t *testing.T) {
	validJSON := `{"verdict":"extract","reasoning":"good","refined_scores":{"problem_specificity":4,"solution_completeness":4,"context_portability":3,"reasoning_transparency":3,"technical_accuracy":4,"verification_evidence":3,"innovation_level":3},"confidence":0.8,"reusable_pattern":true,"explicit_reasoning":true,"contradicts_best_practices":false}`

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"raw JSON", validJSON, false},
		{"json fence", "```json\n" + validJSON + "\n```", false},
		{"generic fence", "```\n" + validJSON + "\n```", false},
		{"first-brace-to-last", "Here is my analysis:\n" + validJSON + "\nDone.", false},
		{"json fence with preamble", "Sure!\n```json\n" + validJSON + "\n```\nHope this helps!", false},
		{"empty", "", true},
		{"whitespace only", "   \n\t  ", true},
		{"malformed JSON", `{"verdict": "extract"`, true},
		{"no JSON at all", "I think this session is good and should be extracted.", true},
		{"partial JSON no close", `{"verdict": "extract", "reasoning": "good"`, true},
		{"nested braces invalid", `{{{invalid}}}`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := llmutil.ExtractJSON[criticResponse](tt.input)
			if tt.wantErr && err == nil {
				t.Error("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.wantErr && out.Verdict != "extract" {
				t.Errorf("verdict = %q, want extract", out.Verdict)
			}
		})
	}
}

func TestParseRubricResponse_AllStrategies(t *testing.T) {
	validJSON := `{"scores":{"problem_specificity":4,"solution_completeness":4,"context_portability":3,"reasoning_transparency":3,"technical_accuracy":4,"verification_evidence":3,"innovation_level":3},"category":"tactical","patterns":["FIX/Backend/DatabaseConnection"]}`

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"raw JSON", validJSON, false},
		{"json fence", "```json\n" + validJSON + "\n```", false},
		{"generic fence", "```\n" + validJSON + "\n```", false},
		{"first-brace-to-last", "Analysis:\n" + validJSON + "\nEnd.", false},
		{"empty", "", true},
		{"whitespace only", "  ", true},
		{"malformed", `{"scores": {`, true},
		{"no JSON", "This is a tactical pattern.", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := llmutil.ExtractJSON[rubricResponse](tt.input)
			if tt.wantErr && err == nil {
				t.Error("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.wantErr && out.Category != "tactical" {
				t.Errorf("category = %q, want tactical", out.Category)
			}
		})
	}
}
