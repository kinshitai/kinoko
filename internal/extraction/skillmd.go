package extraction

import (
	"fmt"
	"strings"
	"time"

	"github.com/kinoko-dev/kinoko/internal/model"
)

// skillNameFromClassification derives a kebab-case skill name from classified
// patterns and category. E.g. "FIX/Backend/DatabaseConnection" → "fix-backend-database-connection".
func skillNameFromClassification(patterns []string, category model.SkillCategory) string {
	if len(patterns) > 0 {
		// Use first pattern: split on "/" and kebab-case.
		parts := strings.Split(patterns[0], "/")
		var segments []string
		for _, p := range parts {
			seg := kebab(p)
			if seg != "" {
				segments = append(segments, seg)
			}
		}
		if name := strings.Join(segments, "-"); name != "" {
			if len(name) > 50 {
				name = name[:50]
				name = strings.TrimRight(name, "-")
			}
			return name
		}
	}

	// Fallback to category.
	if category != "" {
		return string(category) + "-skill"
	}
	return "unnamed-skill"
}

// kebab converts a CamelCase or mixed string to kebab-case.
// Handles all-caps segments: "FIX" → "fix", "DatabaseConnection" → "database-connection".
func kebab(s string) string {
	var out strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		switch {
		case r >= 'A' && r <= 'Z':
			// Insert dash before uppercase if preceded by lowercase or
			// if this is the start of a new word (upper followed by lower, after uppers).
			if i > 0 {
				prev := runes[i-1]
				prevIsLower := prev >= 'a' && prev <= 'z'
				prevIsDigit := prev >= '0' && prev <= '9'
				nextIsLower := i+1 < len(runes) && runes[i+1] >= 'a' && runes[i+1] <= 'z'
				if prevIsLower || prevIsDigit || (prev >= 'A' && prev <= 'Z' && nextIsLower) {
					out.WriteByte('-')
				}
			}
			out.WriteRune(r + ('a' - 'A'))
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			out.WriteRune(r)
		case r == '-' || r == '_' || r == ' ':
			if out.Len() > 0 {
				out.WriteByte('-')
			}
		}
	}
	return strings.Trim(out.String(), "-")
}

// titleCase converts a space-separated string to title case without using deprecated strings.Title.
func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// buildSkillMD generates a proper SKILL.md with YAML front matter and structured body.
// It populates sections from the Stage 3 critic reasoning and session content.
func buildSkillMD(skill *model.SkillRecord, stage3 *model.Stage3Result, content []byte) []byte {
	var b strings.Builder

	// YAML front matter
	fmt.Fprintf(&b, "---\n")
	fmt.Fprintf(&b, "name: %s\n", skill.Name)
	fmt.Fprintf(&b, "id: %s\n", skill.ID)
	fmt.Fprintf(&b, "version: %d\n", skill.Version)
	fmt.Fprintf(&b, "category: %s\n", skill.Category)
	if len(skill.Patterns) > 0 {
		fmt.Fprintf(&b, "patterns:\n")
		for _, p := range skill.Patterns {
			fmt.Fprintf(&b, "  - %s\n", p)
		}
	}
	fmt.Fprintf(&b, "extracted_by: %s\n", skill.ExtractedBy)
	fmt.Fprintf(&b, "quality: %.2f\n", skill.Quality.CompositeScore)
	fmt.Fprintf(&b, "confidence: %.2f\n", skill.Quality.CriticConfidence)
	fmt.Fprintf(&b, "source_session: %s\n", skill.SourceSessionID)
	fmt.Fprintf(&b, "created: %s\n", skill.CreatedAt.Format(time.DateOnly))
	fmt.Fprintf(&b, "---\n\n")

	// Title from name
	title := strings.ReplaceAll(skill.Name, "-", " ")
	title = titleCase(title)
	fmt.Fprintf(&b, "# %s\n\n", title)

	// Populate body from Stage 3 critic analysis and session content.
	reasoning := ""
	if stage3 != nil {
		reasoning = stage3.CriticReasoning
	}

	// When to Use — derived from patterns and category
	fmt.Fprintf(&b, "## When to Use\n\n")
	if len(skill.Patterns) > 0 {
		fmt.Fprintf(&b, "Applicable when encountering: %s\n\n", strings.Join(skill.Patterns, ", "))
	}
	fmt.Fprintf(&b, "Category: %s\n\n", skill.Category)

	// Solution — session content summary (truncated for readability)
	fmt.Fprintf(&b, "## Solution\n\n")
	if len(content) > 0 {
		excerpt := content
		if len(excerpt) > 4096 {
			excerpt = excerpt[:4096]
		}
		fmt.Fprintf(&b, "```\n%s\n```\n\n", string(excerpt))
	}

	// Why It Works — from critic reasoning
	fmt.Fprintf(&b, "## Why It Works\n\n")
	if reasoning != "" {
		fmt.Fprintf(&b, "%s\n\n", reasoning)
	}

	// Pitfalls — note if critic flagged contradictions
	fmt.Fprintf(&b, "## Pitfalls\n\n")
	if stage3 != nil && stage3.ContradictsBestPractices {
		fmt.Fprintf(&b, "**Warning:** This skill may contradict established best practices. Review carefully before applying.\n\n")
	}

	return []byte(b.String())
}
