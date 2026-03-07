package extraction

import (
	"strings"
	"testing"

	"github.com/kinoko-dev/kinoko/pkg/model"
)

func TestBuildCombinedPrompt_ConvertMode(t *testing.T) {
	content := []byte("some document about database migrations")
	stage2 := &model.Stage2Result{
		Passed:             true,
		ClassifiedCategory: model.CategoryTactical,
		ClassifiedPatterns: []string{"FIX/Backend/DatabaseConnection"},
		RubricScores: model.QualityScores{
			ProblemSpecificity:    4,
			SolutionCompleteness:  4,
			ContextPortability:    3,
			ReasoningTransparency: 4,
			TechnicalAccuracy:     4,
			VerificationEvidence:  3,
			InnovationLevel:       3,
		},
	}

	t.Run("convert mode uses document delimiters", func(t *testing.T) {
		prompt := buildCombinedPrompt(content, stage2, "convert")

		if !strings.Contains(prompt, "---BEGIN DOCUMENT") {
			t.Error("convert prompt should use DOCUMENT delimiter")
		}
		if !strings.Contains(prompt, "---END DOCUMENT") {
			t.Error("convert prompt should use DOCUMENT end delimiter")
		}
		if strings.Contains(prompt, "---BEGIN SESSION") {
			t.Error("convert prompt should NOT use SESSION delimiter")
		}
	})

	t.Run("convert mode has genre preamble", func(t *testing.T) {
		prompt := buildCombinedPrompt(content, stage2, "convert")

		if !strings.Contains(prompt, "converted from existing documentation") {
			t.Error("convert prompt should contain genre-aware preamble")
		}
		if !strings.Contains(prompt, "evaluating a document for") {
			t.Error("convert prompt should use 'document' terminology")
		}
	})

	t.Run("session mode uses session delimiters", func(t *testing.T) {
		prompt := buildCombinedPrompt(content, stage2, "session")

		if !strings.Contains(prompt, "---BEGIN SESSION") {
			t.Error("session prompt should use SESSION delimiter")
		}
		if !strings.Contains(prompt, "---END SESSION") {
			t.Error("session prompt should use SESSION end delimiter")
		}
		if strings.Contains(prompt, "---BEGIN DOCUMENT") {
			t.Error("session prompt should NOT use DOCUMENT delimiter")
		}
	})

	t.Run("session mode has no convert preamble", func(t *testing.T) {
		prompt := buildCombinedPrompt(content, stage2, "session")

		if strings.Contains(prompt, "converted from existing documentation") {
			t.Error("session prompt should NOT contain convert preamble")
		}
		if !strings.Contains(prompt, "evaluating a session for") {
			t.Error("session prompt should use 'session' terminology")
		}
	})

	t.Run("empty sourceType defaults to session behavior", func(t *testing.T) {
		prompt := buildCombinedPrompt(content, stage2, "")

		if strings.Contains(prompt, "converted from existing documentation") {
			t.Error("empty sourceType should NOT contain convert preamble")
		}
		if !strings.Contains(prompt, "---BEGIN SESSION") {
			t.Error("empty sourceType should use SESSION delimiter")
		}
	})

	t.Run("content is included in both modes", func(t *testing.T) {
		for _, st := range []string{"session", "convert"} {
			prompt := buildCombinedPrompt(content, stage2, st)
			if !strings.Contains(prompt, string(content)) {
				t.Errorf("sourceType=%q: content not included in prompt", st)
			}
		}
	})
}
