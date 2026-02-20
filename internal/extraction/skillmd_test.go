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
	// Missing name — caught by ParseSkillMDFrontMatter.
	raw := "---\ndescription: Some desc\nversion: 1\ncategory: BUILD\n---\n\n# Missing\n"
	errs := ValidateSkillMD(raw)
	if len(errs) < 1 {
		t.Errorf("expected at least 1 error (name), got %d: %v", len(errs), errs)
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "name") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected name-related error, got %v", errs)
	}
}

func TestValidateSkillMD_MissingDescription(t *testing.T) {
	raw := "---\nname: no-desc\nversion: 1\ncategory: BUILD\n---\n\n# No Desc\n"
	errs := ValidateSkillMD(raw)
	if len(errs) < 1 {
		t.Errorf("expected error for missing description, got none")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "description") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected description-related error, got %v", errs)
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

func TestExportSkillMD_StripsInternalFields(t *testing.T) {
	raw := `---
name: fix-database-timeout
description: How to fix database connection timeouts
id: abc-123
version: 2
category: FIX
patterns:
  - databases/connection-pooling
  - go/sql
extracted_by: kinoko-worker
quality: 0.85
confidence: 0.92
source_session: sess-xyz
tags:
  - extra-tag
created: 2026-01-15
---

# Fix Database Timeout

## Problem
Connection timeouts under load.
`
	got, err := ExportSkillMD(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should keep
	if !strings.Contains(got, "name: fix-database-timeout") {
		t.Error("missing name")
	}
	if !strings.Contains(got, "description: How to fix database connection timeouts") {
		t.Error("missing description")
	}
	if !strings.Contains(got, "version: 2") {
		t.Error("missing version")
	}
	if !strings.Contains(got, "category: FIX") {
		t.Error("missing category")
	}

	// patterns + tags should merge into tags
	if !strings.Contains(got, "  - databases/connection-pooling") {
		t.Error("missing pattern->tag: databases/connection-pooling")
	}
	if !strings.Contains(got, "  - extra-tag") {
		t.Error("missing tag: extra-tag")
	}

	// Should strip
	for _, field := range []string{"id:", "extracted_by:", "quality:", "confidence:", "source_session:", "created:"} {
		if strings.Contains(got, field) {
			t.Errorf("internal field %q not stripped", field)
		}
	}

	// Body preserved
	if !strings.Contains(got, "# Fix Database Timeout") {
		t.Error("body not preserved")
	}
	if !strings.Contains(got, "Connection timeouts under load.") {
		t.Error("body content not preserved")
	}
}

func TestExportSkillMD_PreservesBodyUnchanged(t *testing.T) {
	body := "\n# My Skill\n\nSome **markdown** content.\n\n```go\nfmt.Println(\"hello\")\n```\n"
	raw := "---\nname: test\ndescription: Test skill\nversion: 1\ncategory: BUILD\n---\n" + body
	got, err := ExportSkillMD(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Find body after closing ---
	idx := strings.Index(got[4:], "---\n") // skip opening ---
	if idx < 0 {
		t.Fatal("no closing delimiter")
	}
	gotBody := got[4+idx+4:]
	if gotBody != body {
		t.Errorf("body changed:\ngot:  %q\nwant: %q", gotBody, body)
	}
}

func TestExportSkillMD_HandlesMissingFields(t *testing.T) {
	raw := "---\nname: minimal\ndescription: Minimal skill\n---\n\n# Minimal\n"
	got, err := ExportSkillMD(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "name: minimal") {
		t.Error("missing name")
	}
	// No version/category/tags — should still be valid
	if strings.Contains(got, "version:") {
		t.Error("should not have version when not in input")
	}
	if strings.Contains(got, "tags:") {
		t.Error("should not have tags when none present")
	}
}

func TestExportSkillMD_RoundTrip(t *testing.T) {
	// An already-clean SKILL.md should round-trip unchanged.
	clean := "---\nname: clean-skill\ndescription: A clean skill\nversion: 1\ncategory: BUILD\ntags:\n  - go/testing\n---\n\n# Clean Skill\n"
	got, err := ExportSkillMD(clean)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != clean {
		t.Errorf("round-trip mismatch:\ngot:\n%s\nwant:\n%s", got, clean)
	}
}

func TestExportSkillMD_NoFrontMatter(t *testing.T) {
	_, err := ExportSkillMD("# Just markdown")
	if err == nil {
		t.Error("expected error for missing front matter")
	}
}

func TestExportSkillMD_PatternsBecomeTags(t *testing.T) {
	raw := "---\nname: pat\ndescription: Test\npatterns:\n  - foo/bar\n  - baz/qux\n---\n\n# Pat\n"
	got, err := ExportSkillMD(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "tags:\n  - foo/bar\n  - baz/qux") {
		t.Errorf("patterns not converted to tags: %s", got)
	}
	if strings.Contains(got, "patterns:") {
		t.Error("patterns field should not appear in export")
	}
}

func TestExportSkillMD_DuplicateTagsFromPatternsAndTags(t *testing.T) {
	// patterns: [a, b] and tags: [b, c] — should tags be deduped?
	raw := "---\nname: dup\ndescription: test\npatterns:\n  - a\n  - b\ntags:\n  - b\n  - c\n---\n\n# Dup\n"
	got, err := ExportSkillMD(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Document current behaviour: duplicates ARE present (b appears twice).
	// Count occurrences of "  - b"
	count := strings.Count(got, "  - b\n")
	t.Logf("output:\n%s", got)
	t.Logf("'  - b' appears %d time(s)", count)
	if count > 1 {
		t.Errorf("BUG: duplicate tag 'b' appears %d times — patterns+tags merge should deduplicate", count)
	}
}

func TestExportSkillMD_YAMLFlowSyntaxTags(t *testing.T) {
	// YAML flow syntax: tags: [foo, bar] on one line
	raw := "---\nname: flow\ndescription: test\ntags: [foo, bar]\n---\n\n# Flow\n"
	got, err := ExportSkillMD(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Logf("output:\n%s", got)
	if !strings.Contains(got, "foo") || !strings.Contains(got, "bar") {
		t.Errorf("BUG: YAML flow syntax tags: [foo, bar] were lost in export")
	}
}

func TestExportSkillMD_DescriptionWithColon(t *testing.T) {
	raw := "---\nname: colon\ndescription: \"foo: bar baz\"\nversion: 1\n---\n\n# Colon\n"
	got, err := ExportSkillMD(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Logf("output:\n%s", got)
	if !strings.Contains(got, "description: \"foo: bar baz\"") {
		t.Errorf("description with colon not preserved: got %s", got)
	}
}

func TestExportSkillMD_EmptyBody(t *testing.T) {
	// Frontmatter only, no content after closing ---
	raw := "---\nname: empty\ndescription: no body\n---\n"
	got, err := ExportSkillMD(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Logf("output:\n%s", got)
	// Should produce valid output with just frontmatter
	if !strings.Contains(got, "name: empty") {
		t.Error("missing name")
	}
	// Body should be empty
	idx := strings.Index(got[4:], "---\n")
	if idx < 0 {
		t.Fatal("no closing delimiter")
	}
	body := got[4+idx+4:]
	if body != "" {
		t.Errorf("expected empty body, got: %q", body)
	}
}

func TestParseSkillMDFrontMatter_RejectsMissingDescription(t *testing.T) {
	raw := `---
name: legacy-skill
version: 1
category: BUILD
---

# Legacy skill without description
`
	_, _, _, _, _, err := ParseSkillMDFrontMatter(raw)
	if err == nil {
		t.Fatal("ParseSkillMDFrontMatter should reject missing description")
	}
	if !strings.Contains(err.Error(), "missing description") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseSkillMDFrontMatter_WithDescription(t *testing.T) {
	raw := `---
name: new-skill
description: A skill with a proper description
version: 2
category: FIX
---

# New skill
`
	name, _, _, _, description, err := ParseSkillMDFrontMatter(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "new-skill" {
		t.Errorf("name = %q, want new-skill", name)
	}
	if description != "A skill with a proper description" {
		t.Errorf("description = %q", description)
	}
}
