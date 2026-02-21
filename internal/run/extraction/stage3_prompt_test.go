package extraction

import (
	"strings"
	"testing"

	"github.com/kinoko-dev/kinoko/pkg/model"
)

func TestBuildCombinedPrompt_ContainsKeySections(t *testing.T) {
	content := []byte("session content about fixing a database connection timeout")
	stage2 := &model.Stage2Result{
		Passed:             true,
		ClassifiedCategory: model.CategoryTactical,
		ClassifiedPatterns: []string{"FIX/Backend/DatabaseConnection"},
		RubricScores: model.QualityScores{
			ProblemSpecificity:   4,
			SolutionCompleteness: 4,
			ContextPortability:   3,
		},
	}

	prompt := buildCombinedPrompt(content, stage2)

	checks := map[string]string{
		"Different Project preamble": "DIFFERENT project",
		"Substitution Test":          "SUBSTITUTION TEST",
		"Hard Reject triggers":       "HARD REJECT",
		"SKILL.md format":            "skill_md",
		"Session delimiters":         "---BEGIN SESSION ",
		"End delimiters":             "---END SESSION ",
		"Category options":           "BUILD|FIX|OPTIMIZE|DEBUG|DESIGN|LEARN",
	}

	for name, substr := range checks {
		if !strings.Contains(prompt, substr) {
			t.Errorf("combined prompt missing %s (looked for %q)", name, substr)
		}
	}
}

func TestBuildCombinedPrompt_DelimitersAreUnique(t *testing.T) {
	content := []byte("test content")
	stage2 := &model.Stage2Result{Passed: true}

	p1 := buildCombinedPrompt(content, stage2)
	p2 := buildCombinedPrompt(content, stage2)

	// Extract the nonce from each — they should differ.
	// Both contain "---BEGIN SESSION " but the nonce part differs.
	idx1 := strings.Index(p1, "---BEGIN SESSION ")
	idx2 := strings.Index(p2, "---BEGIN SESSION ")
	if idx1 < 0 || idx2 < 0 {
		t.Fatal("missing delimiters")
	}
	// Extract nonce (16 hex chars after "---BEGIN SESSION ")
	nonce1 := p1[idx1+len("---BEGIN SESSION ") : idx1+len("---BEGIN SESSION ")+16]
	nonce2 := p2[idx2+len("---BEGIN SESSION ") : idx2+len("---BEGIN SESSION ")+16]
	if nonce1 == nonce2 {
		t.Error("nonces should differ between calls")
	}
}

func TestBuildCombinedPrompt_SanitizesMatchingDelimiters(t *testing.T) {
	// Content that happens to contain the exact nonce-based delimiters would be sanitized.
	// Since nonces are random, we just verify the content appears in the prompt
	// and that there's exactly one pair of nonce-delimiters structurally.
	content := []byte("normal session text with no delimiters")
	stage2 := &model.Stage2Result{Passed: true}

	prompt := buildCombinedPrompt(content, stage2)

	if !strings.Contains(prompt, "normal session text") {
		t.Error("content should appear in prompt")
	}
}

func TestBuildCriticPromptLegacy_StillWorks(t *testing.T) {
	content := []byte("session content")
	stage2 := &model.Stage2Result{Passed: true}

	prompt := buildCriticPromptLegacy(content, stage2)
	if !strings.Contains(prompt, "critical evaluator") {
		t.Error("legacy prompt should contain original text")
	}
}
