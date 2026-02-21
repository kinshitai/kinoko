package injection

import (
	"strings"
	"testing"
)

func TestBuildInjectionPrompt_Basic(t *testing.T) {
	skills := []MatchedSkill{
		{Name: "fix-db-timeout", Score: 0.87, Content: "Restart the connection pool."},
		{Name: "go-subprocess", Score: 0.72, Content: "Use exec.CommandContext."},
	}

	got := BuildInjectionPrompt(skills)

	if !strings.Contains(got, "## Relevant Knowledge (auto-injected by Kinoko)") {
		t.Error("missing header")
	}
	if !strings.Contains(got, "### fix-db-timeout (relevance: 0.87)") {
		t.Error("missing first skill header")
	}
	if !strings.Contains(got, "Restart the connection pool.") {
		t.Error("missing first skill content")
	}
	if !strings.Contains(got, "### go-subprocess (relevance: 0.72)") {
		t.Error("missing second skill header")
	}
}

func TestBuildInjectionPrompt_Empty(t *testing.T) {
	got := BuildInjectionPrompt(nil)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}

	got = BuildInjectionPrompt([]MatchedSkill{})
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestBuildInjectionPrompt_Truncation(t *testing.T) {
	// Create a skill with content that exceeds 32KB.
	bigContent := strings.Repeat("A", 40*1024)
	skills := []MatchedSkill{
		{Name: "big-skill", Score: 0.95, Content: bigContent},
	}

	got := BuildInjectionPrompt(skills)

	if len(got) > 32*1024 {
		t.Errorf("prompt exceeds 32KB: %d bytes", len(got))
	}
	if !strings.Contains(got, "## Relevant Knowledge") {
		t.Error("missing header after truncation")
	}
}

func TestBuildInjectionPrompt_MultipleSkillsTruncation(t *testing.T) {
	// First skill fits, second gets truncated.
	skills := []MatchedSkill{
		{Name: "small", Score: 0.9, Content: "Short content."},
		{Name: "huge", Score: 0.8, Content: strings.Repeat("B", 40*1024)},
	}

	got := BuildInjectionPrompt(skills)

	if len(got) > 32*1024 {
		t.Errorf("prompt exceeds 32KB: %d bytes", len(got))
	}
	if !strings.Contains(got, "### small (relevance: 0.90)") {
		t.Error("missing first skill")
	}
}
