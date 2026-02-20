package extraction

import (
	"strings"
	"testing"

	"github.com/kinoko-dev/kinoko/internal/model"
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

func TestParseGeneratedSkillMD_Valid(t *testing.T) {
	raw := `---
name: fix-database-timeout
description: How to fix database connection timeouts under load
version: 2
category: FIX
tags:
  - databases/connection-pooling
  - go/sql
---

# Fix Database Timeout

## Problem
Connection timeouts under load.
`
	name, version, category, tags, description, err := ParseGeneratedSkillMD(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "fix-database-timeout" {
		t.Errorf("name = %q, want fix-database-timeout", name)
	}
	if description != "How to fix database connection timeouts under load" {
		t.Errorf("description = %q", description)
	}
	if version != 2 {
		t.Errorf("version = %d, want 2", version)
	}
	if category != "FIX" {
		t.Errorf("category = %q, want FIX", category)
	}
	if len(tags) != 2 || tags[0] != "databases/connection-pooling" || tags[1] != "go/sql" {
		t.Errorf("tags = %v, want [databases/connection-pooling go/sql]", tags)
	}
}

func TestParseGeneratedSkillMD_DefaultVersion(t *testing.T) {
	raw := `---
name: simple-skill
description: A simple skill for testing
category: BUILD
---

# Simple
`
	_, version, _, _, _, err := ParseGeneratedSkillMD(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version != 1 {
		t.Errorf("version = %d, want 1 (default)", version)
	}
}

func TestParseGeneratedSkillMD_MissingFrontMatter(t *testing.T) {
	_, _, _, _, _, err := ParseGeneratedSkillMD("# Just a heading\nNo front matter.")
	if err == nil {
		t.Error("expected error for missing front matter")
	}
}

func TestParseGeneratedSkillMD_MissingName(t *testing.T) {
	raw := `---
description: Some description
category: FIX
version: 1
---

# No name field
`
	_, _, _, _, _, err := ParseGeneratedSkillMD(raw)
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestParseGeneratedSkillMD_MissingClosingDelimiter(t *testing.T) {
	raw := `---
name: broken
category: FIX
`
	_, _, _, _, _, err := ParseGeneratedSkillMD(raw)
	if err == nil {
		t.Error("expected error for missing closing delimiter")
	}
}

func TestParseGeneratedSkillMD_CRLFLineEndings(t *testing.T) {
	raw := "---\r\nname: crlf-skill\r\ndescription: CRLF test skill\r\nversion: 1\r\ncategory: BUILD\r\ntags:\r\n  - go/testing\r\n---\r\n\r\n# CRLF Skill\r\n"
	name, version, category, tags, _, err := ParseGeneratedSkillMD(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "crlf-skill" {
		t.Errorf("name = %q, want crlf-skill", name)
	}
	if version != 1 {
		t.Errorf("version = %d, want 1", version)
	}
	if category != "BUILD" {
		t.Errorf("category = %q, want BUILD", category)
	}
	if len(tags) != 1 || tags[0] != "go/testing" {
		t.Errorf("tags = %v, want [go/testing]", tags)
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

func TestParseGeneratedSkillMD_MissingDescription(t *testing.T) {
	raw := `---
name: no-desc-skill
version: 1
category: BUILD
---

# No Description
`
	_, _, _, _, _, err := ParseGeneratedSkillMD(raw)
	if err == nil {
		t.Error("expected error for missing description")
	}
	if !strings.Contains(err.Error(), "missing description") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseGeneratedSkillMD_DescriptionTooLong(t *testing.T) {
	longDesc := strings.Repeat("a", 201)
	raw := "---\nname: long-desc\ndescription: " + longDesc + "\nversion: 1\ncategory: BUILD\n---\n\n# Long\n"
	_, _, _, _, _, err := ParseGeneratedSkillMD(raw)
	if err == nil {
		t.Error("expected error for description >200 chars")
	}
	if !strings.Contains(err.Error(), "exceeds 200") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseGeneratedSkillMD_DescriptionExactly200(t *testing.T) {
	desc := strings.Repeat("b", 200)
	raw := "---\nname: exact-desc\ndescription: " + desc + "\nversion: 1\ncategory: BUILD\n---\n\n# Exact\n"
	_, _, _, _, description, err := ParseGeneratedSkillMD(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if description != desc {
		t.Errorf("description length = %d, want 200", len(description))
	}
}

func TestValidateSkillMD_Valid(t *testing.T) {
	raw := "---\nname: valid\ndescription: A valid skill\nversion: 1\ncategory: BUILD\n---\n\n# Valid\n"
	errs := ValidateSkillMD(raw)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidateSkillMD_MissingFields(t *testing.T) {
	raw := "---\nversion: 1\ncategory: BUILD\n---\n\n# Missing\n"
	errs := ValidateSkillMD(raw)
	if len(errs) < 2 {
		t.Errorf("expected at least 2 errors (name+description), got %d: %v", len(errs), errs)
	}
}

func TestValidateSkillMD_InvalidCategory(t *testing.T) {
	raw := "---\nname: cat-test\ndescription: Test\ncategory: INVALID\n---\n\n# Cat\n"
	errs := ValidateSkillMD(raw)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "invalid category") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected invalid category error, got %v", errs)
	}
}

func TestValidateSkillMD_NoFrontMatter(t *testing.T) {
	errs := ValidateSkillMD("# Just markdown")
	if len(errs) == 0 {
		t.Error("expected error for missing front matter")
	}
}

func TestBuildSkillMD_IncludesDescription(t *testing.T) {
	skill := &model.SkillRecord{
		Name:        "desc-test",
		Description: "A test skill description",
		Version:     1,
		Category:    model.CategoryTactical,
	}
	body := string(buildSkillMD(skill, nil, nil))
	if !strings.Contains(body, "description: A test skill description") {
		t.Error("buildSkillMD output missing description field")
	}
}
