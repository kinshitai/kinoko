package extraction

import (
	"strings"
	"testing"

	"github.com/mycelium-dev/mycelium/internal/model"
)

// Tests for kebab/titleCase/skillNameFromClassification — R7 area.
// Must exist BEFORE extracting to skillmd.go.

func TestKebab(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"DatabaseConnection", "database-connection"},
		{"FIX", "fix"},
		{"CIPipeline", "ci-pipeline"},
		{"APIDesign", "api-design"},
		{"camelCase", "camel-case"},
		{"simple", "simple"},
		{"A", "a"},
		{"", ""},
		{"Already-kebab", "already-kebab"},
		{"With Spaces", "with-spaces"},
		{"With_Underscores", "with-underscores"},
		{"MixedCASEStuff", "mixed-case-stuff"},
		{"123Number", "123-number"},
		{"Number123", "number123"},
		{"ABC", "abc"},
		{"ABCDef", "abc-def"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := kebab(tt.input)
			if got != tt.want {
				t.Errorf("kebab(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTitleCase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello world", "Hello World"},
		{"fix database", "Fix Database"},
		{"", ""},
		{"single", "Single"},
		{"already Titled", "Already Titled"},
		{"  extra   spaces  ", "Extra Spaces"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := titleCase(tt.input)
			if got != tt.want {
				t.Errorf("titleCase(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSkillNameFromClassification_LongPattern(t *testing.T) {
	longPattern := "VERY_LONG/Category/SubcategoryThatIsExtremelyLongAndWouldExceedTheLimit"
	name := skillNameFromClassification([]string{longPattern}, model.CategoryTactical)
	if len(name) > 50 {
		t.Errorf("name too long: %d chars, max 50", len(name))
	}
	if strings.HasSuffix(name, "-") {
		t.Error("name should not end with dash after truncation")
	}
}

func TestSkillNameFromClassification_MultiplePatterns(t *testing.T) {
	patterns := []string{"BUILD/Frontend/ComponentDesign", "FIX/Backend/AuthFlow"}
	name := skillNameFromClassification(patterns, model.CategoryTactical)
	if name != "build-frontend-component-design" {
		t.Errorf("got %q, want build-frontend-component-design", name)
	}
}

func TestBuildSkillMD_EmptyContent(t *testing.T) {
	skill := &model.SkillRecord{
		ID:       "test",
		Name:     "empty-content",
		Version:  1,
		Category: model.CategoryTactical,
	}
	body := buildSkillMD(skill, &model.Stage3Result{CriticReasoning: "good"}, nil)
	if len(body) == 0 {
		t.Error("expected non-empty body even with nil content")
	}
	s := string(body)
	if !strings.Contains(s, "## When to Use") {
		t.Error("missing When to Use section")
	}
}

func TestBuildSkillMD_ContradictionWarning(t *testing.T) {
	skill := &model.SkillRecord{Name: "test", Version: 1, Category: model.CategoryTactical}

	body := string(buildSkillMD(skill, &model.Stage3Result{ContradictsBestPractices: true}, []byte("x")))
	if !strings.Contains(body, "contradict") {
		t.Error("missing contradiction warning")
	}

	body = string(buildSkillMD(skill, &model.Stage3Result{ContradictsBestPractices: false}, []byte("x")))
	if strings.Contains(body, "contradict") {
		t.Error("unexpected contradiction warning")
	}
}
