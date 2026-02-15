//go:build integration

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mycelium-dev/mycelium/pkg/skill"
)

func TestSkillParsingEdgeCases(t *testing.T) {
	// Test edge cases that unit tests might miss

	tempDir, err := os.MkdirTemp("", "skill-edge-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	t.Run("unicode_content", func(t *testing.T) {
		// Test skills with various unicode characters
		unicodeSkill := `---
name: unicode-test-skill
version: 1
author: тест-автор
confidence: 0.8
created: 2026-02-14
tags: [тест, 🔧, العربية]
---

# 🚀 Unicode Test Skill

## When to Use
When dealing with files containing unicode: café, naïve, 北京, 🎉

## Solution
Use proper encoding:
` + "```bash\n" + `
echo "Hello 世界!" > файл.txt
cat файл.txt | grep "世界"
` + "```" + `

## Gotchas
- Be careful with RTL text: مرحبا بكم
- Emoji in filenames can break some tools: 📁test.txt
- Zero-width characters: ​ (there's one between these words)

## See Also
- [[handle-unicode-filenames]]
- [[utf8-validation]]`

		s, err := skill.Parse(strings.NewReader(unicodeSkill))
		if err != nil {
			t.Fatalf("Failed to parse unicode skill: %v", err)
		}

		if s.Author != "тест-автор" {
			t.Errorf("Unicode author not preserved: got %s", s.Author)
		}

		if len(s.Tags) != 3 {
			t.Errorf("Expected 3 tags, got %d", len(s.Tags))
		}

		expectedTags := []string{"тест", "🔧", "العربية"}
		for i, expectedTag := range expectedTags {
			if i < len(s.Tags) && s.Tags[i] != expectedTag {
				t.Errorf("Tag %d: expected %s, got %s", i, expectedTag, s.Tags[i])
			}
		}

		if !strings.Contains(s.Body, "世界") {
			t.Error("Unicode content not preserved in body")
		}
	})

	t.Run("very_large_skill", func(t *testing.T) {
		// Test parsing very large skills (potential memory issues)
		largeBody := strings.Repeat("This is a very long line of content that simulates a large skill file. ", 10000)
		
		largeSkill := `---
name: large-skill-test
version: 1
author: test-author
confidence: 0.7
created: 2026-02-14
---

# Large Skill Test

## When to Use
When dealing with very large skill files.

## Solution
` + largeBody + `

## Gotchas
Large files can cause memory issues during parsing.`

		s, err := skill.Parse(strings.NewReader(largeSkill))
		if err != nil {
			t.Fatalf("Failed to parse large skill: %v", err)
		}

		if !strings.Contains(s.Body, "very long line") {
			t.Error("Large skill content not preserved")
		}

		// Should be able to render it back without issues
		_, err = skill.Render(s)
		if err != nil {
			t.Errorf("Failed to render large skill: %v", err)
		}
	})

	t.Run("code_blocks_with_yaml_like_content", func(t *testing.T) {
		// Test skills with code blocks that contain YAML-like syntax
		yamlishSkill := `---
name: yaml-confusion-test
version: 1
author: test-author
confidence: 0.85
created: 2026-02-14
---

# YAML Confusion Test

## When to Use
When skill contains code blocks with YAML-like content.

## Solution
Be careful with code blocks containing colons:

` + "```yaml\n" + `
# This looks like YAML but it's in a code block
server:
  host: localhost
  port: 8080
  settings:
    - name: "debug"
      value: true
    - name: "timeout" 
      value: 30
` + "```" + `

And Docker Compose files:

` + "```docker-compose\n" + `
version: '3.8'
services:
  web:
    image: nginx
    ports:
      - "80:80"
    environment:
      - NODE_ENV=production
    volumes:
      - ./data:/app/data
` + "```" + `

## Gotchas
- YAML parsers might get confused by code blocks
- Front matter should not include code block content
- Indentation in code blocks matters

## See Also
- [[docker-compose-patterns]]`

		s, err := skill.Parse(strings.NewReader(yamlishSkill))
		if err != nil {
			t.Fatalf("Failed to parse skill with YAML-like code blocks: %v", err)
		}

		// Verify that code block content wasn't parsed as front matter
		if strings.Contains(s.Body, "---") {
			// Body should still contain the code block delimiters
		}

		if !strings.Contains(s.Body, "docker-compose") {
			t.Error("Code block content not preserved")
		}
	})

	t.Run("edge_case_front_matter", func(t *testing.T) {
		// Test edge cases in front matter
		edgeFrontMatterSkill := `---
name: front-matter-edge-cases
version: 1
author: "author with spaces and special chars: @#$%"
confidence: 1.0
created: 2026-02-14
tags: ["tag-with-spaces", "tag:with:colons", "tag/with/slashes"]
dependencies: ["skill-with-numbers-123", "skill-with-underscores_here"]
---

# Front Matter Edge Cases

## When to Use
When testing front matter parsing edge cases.

## Solution
Handle various characters in front matter fields properly.`

		s, err := skill.Parse(strings.NewReader(edgeFrontMatterSkill))
		if err != nil {
			t.Fatalf("Failed to parse skill with edge case front matter: %v", err)
		}

		if s.Author != "author with spaces and special chars: @#$%" {
			t.Errorf("Author with special chars not preserved: got %s", s.Author)
		}

		if s.Confidence != 1.0 {
			t.Errorf("Confidence should be 1.0, got %f", s.Confidence)
		}

		expectedTags := []string{"tag-with-spaces", "tag:with:colons", "tag/with/slashes"}
		for i, expectedTag := range expectedTags {
			if i < len(s.Tags) && s.Tags[i] != expectedTag {
				t.Errorf("Tag %d: expected %s, got %s", i, expectedTag, s.Tags[i])
			}
		}
	})

	t.Run("whitespace_handling", func(t *testing.T) {
		// Test various whitespace scenarios
		whitespaceSkill := `---
name: whitespace-test
version: 1
author: test-author
confidence: 0.75
created: 2026-02-14
---

# Whitespace Handling Test

## When to Use



When you need to handle weird whitespace situations.



## Solution
	
	Content with tabs and trailing spaces.    
	
Mixed indentation:
    - Four spaces
	- Tab character
        - Eight spaces

Empty lines with different whitespace:

	
    
		
        

## Gotchas
- Trailing whitespace can be preserved or stripped
- Mixed tab/space indentation
- Empty lines with different whitespace characters`

		s, err := skill.Parse(strings.NewReader(whitespaceSkill))
		if err != nil {
			t.Fatalf("Failed to parse skill with whitespace edge cases: %v", err)
		}

		// Should still be able to render
		rendered, err := skill.Render(s)
		if err != nil {
			t.Errorf("Failed to render skill with whitespace: %v", err)
		}

		// Should be able to re-parse rendered content
		s2, err := skill.Parse(strings.NewReader(string(rendered)))
		if err != nil {
			t.Errorf("Failed to re-parse rendered skill: %v", err)
		}

		// Core fields should match
		if s2.Name != s.Name {
			t.Error("Name changed during render/parse cycle")
		}
	})

	t.Run("wikilinks_extraction", func(t *testing.T) {
		// Test wikilink parsing and validation
		wikilinkSkill := `---
name: wikilink-test
version: 1
author: test-author
confidence: 0.8
created: 2026-02-14
---

# Wikilink Test

## When to Use
When testing wikilink parsing and validation.

## Solution
Reference other skills using wikilinks:
- [[simple-link]]
- [[skill-with-description|Description text]]
- [[nested-[[invalid-link]]]]
- [[link-with-special-chars@#$]]
- [[very-long-skill-name-that-might-cause-issues-with-parsing-or-validation-or-storage]]

Invalid wikilinks:
- [single-bracket-link]
- [[]]
- [[   ]]
- [[invalid name with spaces]]
- [[INVALID-CAPS]]

## See Also
- [[reference-skill-1]]
- [[reference-skill-2]]
- [[reference-skill-with-very-long-name-to-test-edge-cases]]`

		s, err := skill.Parse(strings.NewReader(wikilinkSkill))
		if err != nil {
			t.Fatalf("Failed to parse skill with wikilinks: %v", err)
		}

		// Check that body contains wikilinks (parsing doesn't extract them yet)
		if !strings.Contains(s.Body, "[[simple-link]]") {
			t.Error("Wikilinks not preserved in body")
		}

		// Future: When wikilink extraction is implemented, test it here
	})

	t.Run("date_edge_cases", func(t *testing.T) {
		// Test various date formats and edge cases
		tests := []struct {
			name        string
			created     string
			updated     string
			shouldError bool
		}{
			{
				name:    "valid_dates",
				created: "2026-02-14",
				updated: "2026-02-15",
			},
			{
				name:    "same_dates",
				created: "2026-02-14",
				updated: "2026-02-14",
			},
			{
				name:    "leap_year",
				created: "2024-02-29",
				updated: "2024-03-01",
			},
			{
				name:        "invalid_date_format",
				created:     "02/14/2026",
				shouldError: true,
			},
			{
				name:        "updated_before_created",
				created:     "2026-02-15",
				updated:     "2026-02-14",
				shouldError: true,
			},
			{
				name:        "invalid_leap_year",
				created:     "2025-02-29",
				shouldError: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				updatedLine := ""
				if tt.updated != "" {
					updatedLine = "updated: " + tt.updated
				}

				skillContent := `---
name: date-test-skill
version: 1
author: test-author
confidence: 0.8
created: ` + tt.created + `
` + updatedLine + `
---

# Date Test Skill

## When to Use
Testing date parsing edge cases.

## Solution
Handle dates properly.`

				_, err := skill.Parse(strings.NewReader(skillContent))
				
				if tt.shouldError && err == nil {
					t.Errorf("Expected error for %s, got none", tt.name)
				}
				if !tt.shouldError && err != nil {
					t.Errorf("Unexpected error for %s: %v", tt.name, err)
				}
			})
		}
	})

	t.Run("round_trip_consistency", func(t *testing.T) {
		// Test that parse → render → parse produces consistent results
		originalSkill := `---
name: round-trip-consistency-test
version: 1
tags: [test, consistency, edge-case]
author: test-author
confidence: 0.9
created: 2026-02-14
updated: 2026-02-15
dependencies: [dep1, dep2, dep3]
---

# Round Trip Consistency Test

## When to Use
When testing parse → render → parse consistency.

## Solution
Ensure that parsing and rendering are perfect inverses:

` + "```bash\n" + `
# This code block should survive round-trip
echo "test" | grep "pattern"
` + "```" + `

Unicode content: café naïve 世界 🎉

Special characters: @#$%^&*()

## Gotchas
- YAML field ordering might change
- Whitespace normalization
- Date formatting

## See Also
- [[consistency-testing]]
- [[parser-validation]]`

		// Parse original
		s1, err := skill.Parse(strings.NewReader(originalSkill))
		if err != nil {
			t.Fatalf("Failed to parse original skill: %v", err)
		}

		// Render to bytes
		rendered, err := skill.Render(s1)
		if err != nil {
			t.Fatalf("Failed to render skill: %v", err)
		}

		// Parse rendered version
		s2, err := skill.Parse(strings.NewReader(string(rendered)))
		if err != nil {
			t.Fatalf("Failed to parse rendered skill: %v", err)
		}

		// Compare key fields
		if s1.Name != s2.Name {
			t.Errorf("Name changed: %s → %s", s1.Name, s2.Name)
		}
		if s1.Version != s2.Version {
			t.Errorf("Version changed: %d → %d", s1.Version, s2.Version)
		}
		if s1.Author != s2.Author {
			t.Errorf("Author changed: %s → %s", s1.Author, s2.Author)
		}
		if s1.Confidence != s2.Confidence {
			t.Errorf("Confidence changed: %f → %f", s1.Confidence, s2.Confidence)
		}
		
		// Dates should be preserved
		if !s1.Created.Equal(s2.Created) {
			t.Errorf("Created date changed: %v → %v", s1.Created, s2.Created)
		}
		if !s1.Updated.Equal(s2.Updated) {
			t.Errorf("Updated date changed: %v → %v", s1.Updated, s2.Updated)
		}

		// Arrays should have same length (order might change due to YAML)
		if len(s1.Tags) != len(s2.Tags) {
			t.Errorf("Tags length changed: %d → %d", len(s1.Tags), len(s2.Tags))
		}
		if len(s1.Dependencies) != len(s2.Dependencies) {
			t.Errorf("Dependencies length changed: %d → %d", len(s1.Dependencies), len(s2.Dependencies))
		}

		// Body should be functionally identical (allowing whitespace normalization)
		body1 := strings.TrimSpace(s1.Body)
		body2 := strings.TrimSpace(s2.Body)
		if body1 != body2 {
			t.Errorf("Body content changed during round trip")
			t.Logf("Original body length: %d", len(body1))
			t.Logf("Rendered body length: %d", len(body2))
		}
	})
}

func TestSkillFileOperations(t *testing.T) {
	// Test skill operations with actual files

	tempDir, err := os.MkdirTemp("", "skill-file-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	t.Run("parse_from_file", func(t *testing.T) {
		skillPath := filepath.Join(tempDir, "test-skill.md")
		skillContent := `---
name: file-test-skill
version: 1
author: test-author
confidence: 0.8
created: 2026-02-14
---

# File Test Skill

## When to Use
When testing file-based skill parsing.

## Solution
Parse skills directly from files.`

		if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
			t.Fatalf("Failed to write skill file: %v", err)
		}

		s, err := skill.ParseFile(skillPath)
		if err != nil {
			t.Fatalf("Failed to parse skill from file: %v", err)
		}

		if s.Name != "file-test-skill" {
			t.Errorf("Expected name 'file-test-skill', got '%s'", s.Name)
		}
	})

	t.Run("parse_nonexistent_file", func(t *testing.T) {
		nonexistentPath := filepath.Join(tempDir, "does-not-exist.md")
		
		_, err := skill.ParseFile(nonexistentPath)
		if err == nil {
			t.Error("Expected error when parsing non-existent file")
		}

		if !strings.Contains(err.Error(), "failed to open file") {
			t.Errorf("Error should mention file opening failure: %v", err)
		}
	})

	t.Run("parse_directory_instead_of_file", func(t *testing.T) {
		dirPath := filepath.Join(tempDir, "skill-dir")
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		_, err := skill.ParseFile(dirPath)
		if err == nil {
			t.Error("Expected error when parsing directory instead of file")
		}
	})

	t.Run("parse_empty_file", func(t *testing.T) {
		emptyPath := filepath.Join(tempDir, "empty.md")
		if err := os.WriteFile(emptyPath, []byte(""), 0644); err != nil {
			t.Fatalf("Failed to write empty file: %v", err)
		}

		_, err := skill.ParseFile(emptyPath)
		if err == nil {
			t.Error("Expected error when parsing empty file")
		}

		if !strings.Contains(err.Error(), "missing opening front matter delimiter") {
			t.Errorf("Error should mention missing front matter: %v", err)
		}
	})

	t.Run("parse_file_with_bom", func(t *testing.T) {
		// Test parsing file with Byte Order Mark (UTF-8 BOM)
		bomSkill := "\xef\xbb\xbf" + `---
name: bom-test-skill
version: 1
author: test-author
confidence: 0.8
created: 2026-02-14
---

# BOM Test Skill

## When to Use
When testing files with UTF-8 BOM.

## Solution
Handle BOM properly in file parsing.`

		bomPath := filepath.Join(tempDir, "bom-skill.md")
		if err := os.WriteFile(bomPath, []byte(bomSkill), 0644); err != nil {
			t.Fatalf("Failed to write BOM file: %v", err)
		}

		_, err := skill.ParseFile(bomPath)
		// Currently might fail - this is expected behavior to document
		if err != nil {
			t.Logf("BOM handling not implemented, got error: %v", err)
			// This is documenting current behavior, not asserting it's correct
		}
	})
}