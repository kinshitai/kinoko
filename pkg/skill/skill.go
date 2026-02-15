package skill

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Skill represents a parsed SKILL.md file
type Skill struct {
	// Front matter fields
	Name         string    `yaml:"name"`
	Version      int       `yaml:"version"`
	Tags         []string  `yaml:"tags,omitempty"`
	Author       string    `yaml:"author"`
	Confidence   float64   `yaml:"confidence"`
	Created      time.Time `yaml:"-"` // Custom parsing for date-only format
	Updated      time.Time `yaml:"-"` // Custom parsing for date-only format
	Dependencies []string  `yaml:"dependencies,omitempty"`

	// Body content
	Body string `yaml:"-"`
}

// skillYAML is used for YAML marshaling/unmarshaling with string dates
type skillYAML struct {
	Name         string   `yaml:"name"`
	Version      int      `yaml:"version"`
	Tags         []string `yaml:"tags,omitempty"`
	Author       string   `yaml:"author"`
	Confidence   float64  `yaml:"confidence"`
	Created      string   `yaml:"created"`
	Updated      string   `yaml:"updated,omitempty"`
	Dependencies []string `yaml:"dependencies,omitempty"`
}

var (
	// namePattern validates kebab-case skill names
	namePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

	// frontMatterDelimiter marks the start/end of YAML front matter
	frontMatterDelimiter = "---"
)

// Parse parses a SKILL.md file from a reader
func Parse(r io.Reader) (*Skill, error) {
	content, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read content: %w", err)
	}

	return parseContent(content)
}

// ParseFile parses a SKILL.md file from a file path
func ParseFile(path string) (*Skill, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", path, err)
	}
	defer file.Close()

	return Parse(file)
}

// parseContent parses SKILL.md content from bytes
func parseContent(content []byte) (*Skill, error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	
	// Set a larger buffer to handle large skill files (1MB should be sufficient)
	// Default bufio.Scanner buffer is only 64KB which can truncate large skills
	buf := make([]byte, 0, 1024*1024) // 1MB buffer
	scanner.Buffer(buf, 1024*1024)

	// Look for opening front matter delimiter
	if !scanner.Scan() || scanner.Text() != frontMatterDelimiter {
		return nil, fmt.Errorf("missing opening front matter delimiter")
	}

	// Read front matter content
	var frontMatter []string
	for scanner.Scan() {
		line := scanner.Text()
		if line == frontMatterDelimiter {
			break
		}
		frontMatter = append(frontMatter, line)
	}

	if len(frontMatter) == 0 {
		return nil, fmt.Errorf("empty front matter")
	}

	// Parse YAML front matter with custom date handling
	var skillYAMLData skillYAML
	yamlContent := strings.Join(frontMatter, "\n")
	if err := yaml.Unmarshal([]byte(yamlContent), &skillYAMLData); err != nil {
		return nil, fmt.Errorf("failed to parse front matter: %w", err)
	}

	// Convert to Skill struct with proper time parsing
	skill := Skill{
		Name:         skillYAMLData.Name,
		Version:      skillYAMLData.Version,
		Tags:         skillYAMLData.Tags,
		Author:       skillYAMLData.Author,
		Confidence:   skillYAMLData.Confidence,
		Dependencies: skillYAMLData.Dependencies,
	}

	// Parse created date
	if skillYAMLData.Created != "" {
		created, err := time.Parse("2006-01-02", skillYAMLData.Created)
		if err != nil {
			return nil, fmt.Errorf("invalid created date format: %w", err)
		}
		skill.Created = created
	}

	// Parse updated date if present
	if skillYAMLData.Updated != "" {
		updated, err := time.Parse("2006-01-02", skillYAMLData.Updated)
		if err != nil {
			return nil, fmt.Errorf("invalid updated date format: %w", err)
		}
		skill.Updated = updated
	}

	// Read remaining content as body
	var bodyLines []string
	for scanner.Scan() {
		bodyLines = append(bodyLines, scanner.Text())
	}

	skill.Body = strings.Join(bodyLines, "\n")

	// Validate the parsed skill
	if err := skill.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return &skill, nil
}

// Validate validates the skill according to the format specification
func (s *Skill) Validate() error {
	// Check required fields
	if s.Name == "" {
		return fmt.Errorf("name is required")
	}
	if s.Author == "" {
		return fmt.Errorf("author is required")
	}
	if s.Created.IsZero() {
		return fmt.Errorf("created date is required")
	}

	// Validate name format (kebab-case)
	if !namePattern.MatchString(s.Name) {
		return fmt.Errorf("name must be kebab-case (lowercase with hyphens): %s", s.Name)
	}

	// Validate version
	if s.Version != 1 {
		return fmt.Errorf("version must be 1, got %d", s.Version)
	}

	// Validate confidence range
	if s.Confidence < 0.0 || s.Confidence > 1.0 {
		return fmt.Errorf("confidence must be between 0.0 and 1.0, got %f", s.Confidence)
	}

	// Validate updated date if present
	if !s.Updated.IsZero() && s.Updated.Before(s.Created) {
		return fmt.Errorf("updated date cannot be before created date")
	}

	// Validate dependency names
	for _, dep := range s.Dependencies {
		if !namePattern.MatchString(dep) {
			return fmt.Errorf("dependency name must be kebab-case: %s", dep)
		}
	}

	// Validate body content
	if err := s.validateBody(); err != nil {
		return fmt.Errorf("body validation failed: %w", err)
	}

	return nil
}

// validateBody validates the structure of the body content
func (s *Skill) validateBody() error {
	body := strings.TrimSpace(s.Body)
	if body == "" {
		return fmt.Errorf("body cannot be empty")
	}

	// Check for required title section (must start with # at beginning of line)
	lines := strings.Split(body, "\n")
	hasTitle := false
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "# ") {
			hasTitle = true
			break
		}
	}
	if !hasTitle {
		return fmt.Errorf("body must contain a title section (# ...)")
	}

	// Check for required sections (case-insensitive)
	bodyLower := strings.ToLower(body)
	hasWhenToUse := strings.Contains(bodyLower, "## when to use")
	hasSolution := strings.Contains(bodyLower, "## solution")

	if !hasWhenToUse {
		return fmt.Errorf("body must contain '## When to Use' section (case-insensitive)")
	}

	if !hasSolution {
		return fmt.Errorf("body must contain '## Solution' section (case-insensitive)")
	}

	return nil
}

// Render renders the skill back to SKILL.md format
func Render(skill *Skill) ([]byte, error) {
	if err := skill.Validate(); err != nil {
		return nil, fmt.Errorf("cannot render invalid skill: %w", err)
	}

	var buf bytes.Buffer

	// Write opening front matter delimiter
	buf.WriteString(frontMatterDelimiter + "\n")

	// Prepare front matter with string dates
	skillYAMLData := skillYAML{
		Name:         skill.Name,
		Version:      skill.Version,
		Tags:         skill.Tags,
		Author:       skill.Author,
		Confidence:   skill.Confidence,
		Created:      skill.Created.Format("2006-01-02"),
		Dependencies: skill.Dependencies,
	}

	// Add updated date if present
	if !skill.Updated.IsZero() {
		skillYAMLData.Updated = skill.Updated.Format("2006-01-02")
	}

	// Marshal front matter to YAML
	yamlData, err := yaml.Marshal(skillYAMLData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal front matter: %w", err)
	}

	// Write front matter
	buf.Write(yamlData)

	// Write closing front matter delimiter
	buf.WriteString(frontMatterDelimiter + "\n")

	// Write body
	body := strings.TrimSpace(skill.Body)
	if body != "" {
		buf.WriteString("\n")
		buf.WriteString(body)
		buf.WriteString("\n")
	}

	return buf.Bytes(), nil
}