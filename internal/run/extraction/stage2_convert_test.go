package extraction

import (
	"strings"
	"testing"
)

func TestBuildRubricPrompt_ConvertMode(t *testing.T) {
	content := []byte("some document content about Go testing patterns")

	t.Run("convert mode has genre-aware preamble", func(t *testing.T) {
		prompt := buildRubricPrompt(content, SourceTypeConvert, "")

		if !strings.Contains(prompt, "converted from existing documentation") {
			t.Error("convert prompt should contain genre-aware preamble")
		}
		if !strings.Contains(prompt, "Analyze this document") {
			t.Error("convert prompt should use 'document' instead of 'agent session'")
		}
		if !strings.Contains(prompt, "Document content:") {
			t.Error("convert prompt should use 'Document content:' delimiter")
		}
		if strings.Contains(prompt, "Session content:") {
			t.Error("convert prompt should NOT contain 'Session content:'")
		}
		if strings.Contains(prompt, "Analyze this agent session") {
			t.Error("convert prompt should NOT contain 'agent session'")
		}
	})

	t.Run("session mode has no preamble", func(t *testing.T) {
		prompt := buildRubricPrompt(content, SourceTypeSession, "")

		if strings.Contains(prompt, "converted from existing documentation") {
			t.Error("session prompt should NOT contain convert preamble")
		}
		if !strings.Contains(prompt, "Analyze this agent session") {
			t.Error("session prompt should use 'agent session' terminology")
		}
		if !strings.Contains(prompt, "Session content:") {
			t.Error("session prompt should use 'Session content:' delimiter")
		}
	})

	t.Run("empty sourceType uses session mode", func(t *testing.T) {
		prompt := buildRubricPrompt(content, "", "")

		if strings.Contains(prompt, "converted from existing documentation") {
			t.Error("empty sourceType should NOT contain convert preamble")
		}
		if !strings.Contains(prompt, "Analyze this agent session") {
			t.Error("empty sourceType should use 'agent session' terminology")
		}
	})

	t.Run("prompt structure is valid", func(t *testing.T) {
		for _, st := range []SourceType{SourceTypeSession, SourceTypeConvert, ""} {
			prompt := buildRubricPrompt(content, st, "")
			if !strings.Contains(prompt, "problem_specificity") {
				t.Errorf("sourceType=%q: missing rubric dimension", st)
			}
			if !strings.Contains(prompt, "JSON format:") {
				t.Errorf("sourceType=%q: missing JSON format section", st)
			}
			if !strings.Contains(prompt, string(content)) {
				t.Errorf("sourceType=%q: content not included in prompt", st)
			}
		}
	})
}
