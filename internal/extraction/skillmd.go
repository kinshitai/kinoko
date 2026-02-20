package extraction

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kinoko-dev/kinoko/internal/model"
)

// ParseSkillMDFrontMatter extracts metadata from YAML front matter.
// Returns an error if required fields (name, description) are missing.
func ParseSkillMDFrontMatter(raw string) (name string, version int, category string, tags []string, description string, err error) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "---") {
		return "", 0, "", nil, "", fmt.Errorf("missing YAML front matter delimiter")
	}
	// Find closing delimiter.
	rest := raw[3:] // skip opening "---"
	rest = strings.TrimLeft(rest, " \t")
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
		rest = rest[2:]
	}

	endIdx := strings.Index(rest, "\n---")
	if endIdx < 0 {
		return "", 0, "", nil, "", fmt.Errorf("missing closing YAML front matter delimiter")
	}
	frontMatter := rest[:endIdx]

	version = 1 // default
	var inTags bool

	for _, line := range strings.Split(frontMatter, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Check if this is a tag list item.
		if inTags {
			if strings.HasPrefix(trimmed, "- ") {
				tags = append(tags, strings.TrimSpace(trimmed[2:]))
				continue
			}
			inTags = false
		}
		// Key-value pair.
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "name":
			name = val
		case "description":
			description = val
		case "version":
			if v, e := strconv.Atoi(val); e == nil {
				version = v
			}
		case "category":
			category = val
		case "tags":
			inTags = true
		}
	}

	if name == "" {
		return "", 0, "", nil, "", fmt.Errorf("missing name in front matter")
	}
	if description == "" {
		return "", 0, "", nil, "", fmt.Errorf("missing description in front matter")
	}
	if len(description) > 200 {
		return "", 0, "", nil, "", fmt.Errorf("description exceeds 200 characters (%d)", len(description))
	}
	return name, version, category, tags, description, nil
}

// ParseGeneratedSkillMD extracts name, description, version, category, and tags from the
// YAML front matter of an LLM-generated SKILL.md. Returns an error if the
// front matter is missing, the name field is absent, or description is empty/too long.
func ParseGeneratedSkillMD(raw string) (name string, version int, category string, tags []string, description string, err error) {
	return ParseSkillMDFrontMatter(raw)
}

// ValidateSkillMD checks required fields and constraints on a raw SKILL.md string.
// Returns a list of validation errors (empty if valid).
func ValidateSkillMD(raw string) []error {
	name, _, category, _, _, err := ParseSkillMDFrontMatter(raw)
	if err != nil {
		return []error{err}
	}

	var errs []error

	if name == "" {
		errs = append(errs, fmt.Errorf("required field 'name' is missing or empty"))
	}

	validCategories := map[string]bool{
		"BUILD": true, "FIX": true, "OPTIMIZE": true,
		"DEBUG": true, "DESIGN": true, "LEARN": true,
		"foundational": true, "tactical": true, "contextual": true,
	}
	if category != "" && !validCategories[category] {
		errs = append(errs, fmt.Errorf("invalid category %q", category))
	}

	return errs
}

// ExportSkillMD takes a raw SKILL.md string and returns a cleaned version
// suitable for external consumption (e.g. skills.sh / npx skills add).
// It strips internal metadata fields from frontmatter, keeping only:
// name, description, version, category, tags.
// The body content is preserved unchanged.
func ExportSkillMD(raw string) (string, error) {
	raw = strings.TrimLeft(raw, " \t\r\n")
	if !strings.HasPrefix(raw, "---") {
		return "", fmt.Errorf("missing YAML front matter delimiter")
	}

	// Find the end of the opening delimiter line.
	rest := raw[3:]
	rest = strings.TrimLeft(rest, " \t")
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	} else if len(rest) > 1 && rest[0] == '\r' && rest[1] == '\n' {
		rest = rest[2:]
	}

	endIdx := strings.Index(rest, "\n---")
	if endIdx < 0 {
		return "", fmt.Errorf("missing closing YAML front matter delimiter")
	}
	frontMatter := rest[:endIdx]
	// Body is everything after the closing "---" line.
	afterClose := rest[endIdx+4:] // skip "\n---"
	// Skip the newline after closing delimiter.
	if len(afterClose) > 0 && afterClose[0] == '\n' {
		afterClose = afterClose[1:]
	} else if len(afterClose) > 1 && afterClose[0] == '\r' && afterClose[1] == '\n' {
		afterClose = afterClose[2:]
	}

	// Parse frontmatter, keeping only allowed fields.
	allowedScalar := map[string]bool{
		"name": true, "description": true, "version": true, "category": true,
	}

	var name, description, version, category string
	var tags []string
	var inList bool
	var currentListKey string

	for _, line := range strings.Split(frontMatter, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if inList {
			if strings.HasPrefix(trimmed, "- ") {
				if currentListKey == "tags" {
					tags = append(tags, strings.TrimSpace(trimmed[2:]))
				}
				continue
			}
			inList = false
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		if key == "tags" || key == "patterns" {
			// Both map to tags in export.
			inList = true
			currentListKey = "tags"
			continue
		}
		if allowedScalar[key] {
			switch key {
			case "name":
				name = val
			case "description":
				description = val
			case "version":
				version = val
			case "category":
				category = val
			}
		}
	}

	// Rebuild clean frontmatter.
	var b strings.Builder
	b.WriteString("---\n")
	if name != "" {
		fmt.Fprintf(&b, "name: %s\n", name)
	}
	if description != "" {
		fmt.Fprintf(&b, "description: %s\n", description)
	}
	if version != "" {
		fmt.Fprintf(&b, "version: %s\n", version)
	}
	if category != "" {
		fmt.Fprintf(&b, "category: %s\n", category)
	}
	if len(tags) > 0 {
		b.WriteString("tags:\n")
		for _, tag := range tags {
			fmt.Fprintf(&b, "  - %s\n", tag)
		}
	}
	b.WriteString("---\n")
	if len(afterClose) > 0 {
		b.WriteString(afterClose)
	}

	return b.String(), nil
}

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
	fmt.Fprintf(&b, "description: %s\n", skill.Description)
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
