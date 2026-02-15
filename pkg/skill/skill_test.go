package skill

import (
	"strings"
	"testing"
	"time"
)

func TestParseValidSkill(t *testing.T) {
	validSkill := `---
name: test-skill
version: 1
tags: [debugging, golang]
author: test-author
confidence: 0.85
created: 2026-02-14
updated: 2026-02-14
dependencies: [other-skill]
---

# Test Skill

## When to Use
This is a test skill for unit testing.

## Solution
Follow these steps:
1. Write test
2. Run test
3. Debug if needed

## Gotchas
- Be careful with edge cases
- Always validate input

## See Also
- [[related-skill]]
`

	skill, err := Parse(strings.NewReader(validSkill))
	if err != nil {
		t.Fatalf("Failed to parse valid skill: %v", err)
	}

	// Verify front matter
	if skill.Name != "test-skill" {
		t.Errorf("Expected name 'test-skill', got '%s'", skill.Name)
	}
	if skill.Version != 1 {
		t.Errorf("Expected version 1, got %d", skill.Version)
	}
	if skill.Author != "test-author" {
		t.Errorf("Expected author 'test-author', got '%s'", skill.Author)
	}
	if skill.Confidence != 0.85 {
		t.Errorf("Expected confidence 0.85, got %f", skill.Confidence)
	}
	if len(skill.Tags) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(skill.Tags))
	}
	if len(skill.Dependencies) != 1 {
		t.Errorf("Expected 1 dependency, got %d", len(skill.Dependencies))
	}

	// Verify dates
	expectedDate := time.Date(2026, 2, 14, 0, 0, 0, 0, time.UTC)
	if !skill.Created.Equal(expectedDate) {
		t.Errorf("Expected created date %v, got %v", expectedDate, skill.Created)
	}

	// Verify body content
	if !strings.Contains(skill.Body, "# Test Skill") {
		t.Error("Body should contain title section")
	}
	if !strings.Contains(skill.Body, "## When to Use") {
		t.Error("Body should contain 'When to Use' section")
	}
	if !strings.Contains(skill.Body, "## Solution") {
		t.Error("Body should contain 'Solution' section")
	}
}

func TestParseMinimalSkill(t *testing.T) {
	minimalSkill := `---
name: minimal-skill
version: 1
author: test-author
confidence: 0.7
created: 2026-02-14
---

# Minimal Skill

## When to Use
When testing minimal requirements.

## Solution
This is the minimal solution.
`

	skill, err := Parse(strings.NewReader(minimalSkill))
	if err != nil {
		t.Fatalf("Failed to parse minimal skill: %v", err)
	}

	// Verify required fields are set
	if skill.Name != "minimal-skill" {
		t.Errorf("Expected name 'minimal-skill', got '%s'", skill.Name)
	}
	if len(skill.Tags) != 0 {
		t.Errorf("Expected empty tags, got %v", skill.Tags)
	}
	if len(skill.Dependencies) != 0 {
		t.Errorf("Expected empty dependencies, got %v", skill.Dependencies)
	}
	if !skill.Updated.IsZero() {
		t.Error("Updated should be zero when not specified")
	}
}

func TestParseMissingRequiredFields(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		errorMsg string
	}{
		{
			name: "missing name",
			content: `---
version: 1
author: test-author
confidence: 0.7
created: 2026-02-14
---

# Test
## When to Use
Test
## Solution
Test`,
			errorMsg: "name is required",
		},
		{
			name: "missing author",
			content: `---
name: test-skill
version: 1
confidence: 0.7
created: 2026-02-14
---

# Test
## When to Use
Test
## Solution
Test`,
			errorMsg: "author is required",
		},
		{
			name: "missing created",
			content: `---
name: test-skill
version: 1
author: test-author
confidence: 0.7
---

# Test
## When to Use
Test
## Solution
Test`,
			errorMsg: "created date is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tt.content))
			if err == nil {
				t.Errorf("Expected error for %s, got none", tt.name)
				return
			}
			if !strings.Contains(err.Error(), tt.errorMsg) {
				t.Errorf("Expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
			}
		})
	}
}

func TestParseInvalidValues(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		errorMsg string
	}{
		{
			name: "invalid name format",
			content: `---
name: InvalidName
version: 1
author: test-author
confidence: 0.7
created: 2026-02-14
---

# Test
## When to Use
Test
## Solution
Test`,
			errorMsg: "name must be kebab-case",
		},
		{
			name: "invalid version",
			content: `---
name: test-skill
version: 2
author: test-author
confidence: 0.7
created: 2026-02-14
---

# Test
## When to Use
Test
## Solution
Test`,
			errorMsg: "version must be 1",
		},
		{
			name: "confidence too high",
			content: `---
name: test-skill
version: 1
author: test-author
confidence: 1.5
created: 2026-02-14
---

# Test
## When to Use
Test
## Solution
Test`,
			errorMsg: "confidence must be between 0.0 and 1.0",
		},
		{
			name: "confidence too low",
			content: `---
name: test-skill
version: 1
author: test-author
confidence: -0.1
created: 2026-02-14
---

# Test
## When to Use
Test
## Solution
Test`,
			errorMsg: "confidence must be between 0.0 and 1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tt.content))
			if err == nil {
				t.Errorf("Expected error for %s, got none", tt.name)
				return
			}
			if !strings.Contains(err.Error(), tt.errorMsg) {
				t.Errorf("Expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
			}
		})
	}
}

func TestParseEmptyBody(t *testing.T) {
	emptyBodySkill := `---
name: empty-body
version: 1
author: test-author
confidence: 0.7
created: 2026-02-14
---

`

	_, err := Parse(strings.NewReader(emptyBodySkill))
	if err == nil {
		t.Error("Expected error for empty body, got none")
	}
	if !strings.Contains(err.Error(), "body cannot be empty") {
		t.Errorf("Expected 'body cannot be empty' error, got '%s'", err.Error())
	}
}

func TestParseMissingRequiredSections(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		errorMsg string
	}{
		{
			name: "missing title",
			content: `---
name: test-skill
version: 1
author: test-author
confidence: 0.7
created: 2026-02-14
---

## When to Use
Test
## Solution
Test`,
			errorMsg: "body must contain a title section",
		},
		{
			name: "missing when to use",
			content: `---
name: test-skill
version: 1
author: test-author
confidence: 0.7
created: 2026-02-14
---

# Test

## Solution
Test`,
			errorMsg: "body must contain '## When to Use' section (case-insensitive)",
		},
		{
			name: "missing solution",
			content: `---
name: test-skill
version: 1
author: test-author
confidence: 0.7
created: 2026-02-14
---

# Test

## When to Use
Test`,
			errorMsg: "body must contain '## Solution' section (case-insensitive)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tt.content))
			if err == nil {
				t.Errorf("Expected error for %s, got none", tt.name)
				return
			}
			if !strings.Contains(err.Error(), tt.errorMsg) {
				t.Errorf("Expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
			}
		})
	}
}

func TestCaseInsensitiveSections(t *testing.T) {
	// Test that section names are case-insensitive
	tests := []struct {
		name        string
		content     string
		shouldPass  bool
	}{
		{
			name: "standard case",
			content: `---
name: test-skill
version: 1
author: test-author
confidence: 0.7
created: 2026-02-14
---

# Test
## When to Use
Test
## Solution
Test`,
			shouldPass: true,
		},
		{
			name: "different case - when to use",
			content: `---
name: test-skill
version: 1
author: test-author
confidence: 0.7
created: 2026-02-14
---

# Test
## When To Use
Test
## Solution
Test`,
			shouldPass: true,
		},
		{
			name: "lowercase sections",
			content: `---
name: test-skill
version: 1
author: test-author
confidence: 0.7
created: 2026-02-14
---

# Test
## when to use
Test
## solution
Test`,
			shouldPass: true,
		},
		{
			name: "mixed case sections",
			content: `---
name: test-skill
version: 1
author: test-author
confidence: 0.7
created: 2026-02-14
---

# Test
## WHEN TO USE
Test
## SoLuTiOn
Test`,
			shouldPass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tt.content))
			if tt.shouldPass && err != nil {
				t.Errorf("Expected parsing to succeed for %s, got error: %v", tt.name, err)
			}
			if !tt.shouldPass && err == nil {
				t.Errorf("Expected parsing to fail for %s, got no error", tt.name)
			}
		})
	}
}

func TestParseMalformedFrontMatter(t *testing.T) {
	malformedSkill := `---
name: test-skill
version: 1
author: test-author
confidence: 0.7
created: 2026-02-14
invalid yaml: [unclosed
---

# Test
## When to Use
Test
## Solution
Test`

	_, err := Parse(strings.NewReader(malformedSkill))
	if err == nil {
		t.Error("Expected error for malformed front matter, got none")
	}
	if !strings.Contains(err.Error(), "failed to parse front matter") {
		t.Errorf("Expected front matter parse error, got '%s'", err.Error())
	}
}

func TestRoundTrip(t *testing.T) {
	originalSkill := `---
name: round-trip-test
version: 1
tags: [test, golang]
author: test-author
confidence: 0.85
created: 2026-02-14
updated: 2026-02-15
dependencies: [dep1, dep2]
---

# Round Trip Test

## When to Use
When testing round-trip parsing and rendering.

## Solution
Parse the skill, render it back, then parse again.
The result should be identical.

## Gotchas
- Watch out for date formatting
- YAML field ordering might change
- Whitespace handling matters

## See Also
- [[other-skill]]
`

	// First parse
	skill1, err := Parse(strings.NewReader(originalSkill))
	if err != nil {
		t.Fatalf("Failed first parse: %v", err)
	}

	// Render back
	rendered, err := Render(skill1)
	if err != nil {
		t.Fatalf("Failed to render: %v", err)
	}

	// Second parse
	skill2, err := Parse(strings.NewReader(string(rendered)))
	if err != nil {
		t.Fatalf("Failed second parse: %v", err)
	}

	// Compare key fields
	if skill1.Name != skill2.Name {
		t.Errorf("Names don't match: %s vs %s", skill1.Name, skill2.Name)
	}
	if skill1.Version != skill2.Version {
		t.Errorf("Versions don't match: %d vs %d", skill1.Version, skill2.Version)
	}
	if skill1.Author != skill2.Author {
		t.Errorf("Authors don't match: %s vs %s", skill1.Author, skill2.Author)
	}
	if skill1.Confidence != skill2.Confidence {
		t.Errorf("Confidence doesn't match: %f vs %f", skill1.Confidence, skill2.Confidence)
	}
	if !skill1.Created.Equal(skill2.Created) {
		t.Errorf("Created dates don't match: %v vs %v", skill1.Created, skill2.Created)
	}
	if !skill1.Updated.Equal(skill2.Updated) {
		t.Errorf("Updated dates don't match: %v vs %v", skill1.Updated, skill2.Updated)
	}

	// Compare slices
	if len(skill1.Tags) != len(skill2.Tags) {
		t.Errorf("Tags length doesn't match: %d vs %d", len(skill1.Tags), len(skill2.Tags))
	}
	if len(skill1.Dependencies) != len(skill2.Dependencies) {
		t.Errorf("Dependencies length doesn't match: %d vs %d", len(skill1.Dependencies), len(skill2.Dependencies))
	}

	// Body should be functionally the same (allowing for whitespace differences)
	body1 := strings.TrimSpace(skill1.Body)
	body2 := strings.TrimSpace(skill2.Body)
	if body1 != body2 {
		t.Errorf("Bodies don't match:\n%s\n---\n%s", body1, body2)
	}
}

func TestRenderValidation(t *testing.T) {
	// Create an invalid skill
	invalidSkill := &Skill{
		Name:       "InvalidName", // Should be kebab-case
		Version:    1,
		Author:     "test-author",
		Confidence: 1.5, // Invalid confidence
		Created:    time.Now(),
		Body:       "# Test\n## When to Use\nTest\n## Solution\nTest",
	}

	_, err := Render(invalidSkill)
	if err == nil {
		t.Error("Expected error when rendering invalid skill, got none")
	}
	if !strings.Contains(err.Error(), "cannot render invalid skill") {
		t.Errorf("Expected validation error, got '%s'", err.Error())
	}
}

func TestLargeSkillParsing(t *testing.T) {
	// Create a skill with a large body (> 64KB) to test buffer limits
	// This tests that the buffer size fix works correctly
	
	// Create a large content block (approximately 100KB)
	largeContentBlock := strings.Repeat("This is a very long line of example code or documentation that repeats many times to create a large skill body that exceeds the default 64KB buffer limit of bufio.Scanner. ", 500)
	
	largeSkill := `---
name: large-skill
version: 1
author: test-author
confidence: 0.8
created: 2026-02-15
---

# Large Skill for Buffer Testing

## When to Use
When you need to test that large skills can be parsed without hitting buffer limits.

## Solution
This skill contains a very large amount of content to ensure the parser can handle files larger than the default 64KB scanner buffer.

### Large Code Example
` + largeContentBlock + `

### More Content
` + largeContentBlock + `

### Even More Content  
` + largeContentBlock + `

### Final Large Section
` + largeContentBlock + `

## Gotchas
- Large files should not be truncated
- Parser should handle files larger than 64KB
- Buffer limits should not silently corrupt content

## See Also
- [[buffer-management]]
`

	// Parse the large skill
	skill, err := Parse(strings.NewReader(largeSkill))
	if err != nil {
		t.Fatalf("Failed to parse large skill: %v", err)
	}

	// Verify basic parsing worked
	if skill.Name != "large-skill" {
		t.Errorf("Expected name 'large-skill', got '%s'", skill.Name)
	}
	if skill.Author != "test-author" {
		t.Errorf("Expected author 'test-author', got '%s'", skill.Author)
	}
	if skill.Confidence != 0.8 {
		t.Errorf("Expected confidence 0.8, got %f", skill.Confidence)
	}

	// Verify the body contains the large content blocks
	if !strings.Contains(skill.Body, "Large Skill for Buffer Testing") {
		t.Error("Body should contain title content")
	}
	
	// Verify that all the large content blocks are present (not truncated)
	occurrences := strings.Count(skill.Body, "This is a very long line of example code")
	expectedOccurrences := 4 * 500 // 4 blocks × 500 repetitions each
	if occurrences != expectedOccurrences {
		t.Errorf("Expected %d occurrences of repeated content, got %d (content may be truncated)", 
			expectedOccurrences, occurrences)
	}

	// Verify the body is actually large (> 64KB)
	if len(skill.Body) <= 64*1024 {
		t.Errorf("Body should be larger than 64KB, got %d bytes", len(skill.Body))
	}
}

func TestDateParsingEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		createdDate string
		updatedDate string
		expectError bool
		errorMsg    string
	}{
		// Leap year tests
		{
			name:        "valid leap year date",
			createdDate: "2024-02-29",
			expectError: false,
		},
		{
			name:        "invalid leap year date",
			createdDate: "2023-02-29",
			expectError: true,
			errorMsg:    "invalid created date format",
		},
		{
			name:        "century leap year (divisible by 400)",
			createdDate: "2000-02-29",
			expectError: false,
		},
		{
			name:        "century non-leap year (divisible by 100 but not 400)", 
			createdDate: "1900-02-29",
			expectError: true,
			errorMsg:    "invalid created date format",
		},
		
		// Boundary date tests
		{
			name:        "invalid month - too high",
			createdDate: "2024-13-01", 
			expectError: true,
			errorMsg:    "invalid created date format",
		},
		{
			name:        "invalid day for month",
			createdDate: "2024-02-30",
			expectError: true, 
			errorMsg:    "invalid created date format",
		},
		{
			name:        "invalid month - zero",
			createdDate: "2024-00-15",
			expectError: true,
			errorMsg:    "invalid created date format",
		},
		{
			name:        "invalid day - zero",
			createdDate: "2024-01-00",
			expectError: true,
			errorMsg:    "invalid created date format",
		},
		{
			name:        "valid boundary - december 31",
			createdDate: "2024-12-31",
			expectError: false,
		},
		{
			name:        "valid boundary - january 1",
			createdDate: "2024-01-01",
			expectError: false,
		},
		
		// Updated date validation
		{
			name:        "updated before created",
			createdDate: "2024-02-15",
			updatedDate: "2024-02-14", 
			expectError: true,
			errorMsg:    "updated date cannot be before created date",
		},
		{
			name:        "updated same as created",
			createdDate: "2024-02-15",
			updatedDate: "2024-02-15",
			expectError: false,
		},
		
		// Format validation
		{
			name:        "wrong date format - with time",
			createdDate: "2024-02-15T10:30:00Z",
			expectError: true,
			errorMsg:    "invalid created date format",
		},
		{
			name:        "wrong date format - american style",
			createdDate: "02/15/2024",
			expectError: true,
			errorMsg:    "invalid created date format",
		},
		{
			name:        "wrong date format - missing leading zero",
			createdDate: "2024-2-5",
			expectError: true,
			errorMsg:    "invalid created date format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build skill content
			skillContent := `---
name: date-test
version: 1
author: test-author
confidence: 0.7
created: ` + tt.createdDate

			if tt.updatedDate != "" {
				skillContent += "\nupdated: " + tt.updatedDate
			}

			skillContent += `
---

# Date Test

## When to Use
When testing date parsing edge cases.

## Solution
Test various date formats and boundary conditions.
`

			_, err := Parse(strings.NewReader(skillContent))
			
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for %s, got none", tt.name)
					return
				}
				if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for %s, got: %v", tt.name, err)
				}
			}
		})
	}
}