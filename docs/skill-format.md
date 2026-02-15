# SKILL.md Format Specification

This document defines the structure and validation rules for SKILL.md files in the Mycelium knowledge sharing system.

## Overview

A SKILL.md file contains practical knowledge extracted from AI agent sessions. Each skill follows a standardized format with YAML front matter and Markdown body.

## File Structure

```markdown
---
name: skill-name-kebab-case
version: 1
tags: [tag1, tag2]
author: contributor-id
confidence: 0.85
created: 2026-02-14
updated: 2026-02-14
dependencies: []
---

# Skill Title

## When to Use
[Description of when this skill applies]

## Solution
[The actual knowledge — concrete steps, patterns, code]

## Gotchas
[Edge cases, pitfalls, things that surprised us]

## See Also
- [[related-skill-name]]
```

## Front Matter Fields

### Required Fields

- **`name`** (string): Unique identifier for the skill in kebab-case format
  - Must match pattern: `^[a-z0-9]+(-[a-z0-9]+)*$`
  - Examples: `debug-golang-race-conditions`, `aws-lambda-cold-start-optimization`

- **`version`** (integer): Schema version for the skill format
  - Currently must be `1`

- **`author`** (string): Identifier for the contributor
  - Can be username, email, or agent identifier
  - Examples: `john.doe`, `agent:claude-3.5`, `hal@mycelium.dev`

- **`confidence`** (float): Confidence score for the skill's reliability
  - Range: `0.0` to `1.0` (inclusive)
  - Higher values indicate more reliable/tested knowledge
  - Examples: `0.95` (well-tested), `0.7` (probably works), `0.4` (experimental)

- **`created`** (string): Creation date in `YYYY-MM-DD` format
  - Must be valid ISO 8601 date
  - Example: `2026-02-14`

### Optional Fields

- **`tags`** (array of strings): Categorization labels
  - Each tag should be lowercase, kebab-case
  - Examples: `["debugging", "golang", "performance"]`, `["aws", "serverless"]`
  - Default: empty array `[]`

- **`updated`** (string): Last modification date in `YYYY-MM-DD` format
  - Must be valid ISO 8601 date
  - Should be >= `created` date
  - If omitted, defaults to `created` value

- **`dependencies`** (array of strings): Other skills this one depends on
  - Each dependency should be a valid skill name
  - Examples: `["setup-golang-project", "install-docker"]`
  - Default: empty array `[]`

## Body Sections

The Markdown body should follow this structure:

### Required Sections

- **`# Skill Title`**: Human-readable title for the skill
- **`## When to Use`**: Clear description of when this skill applies
- **`## Solution`**: The actual knowledge content

### Optional Sections

- **`## Gotchas`**: Edge cases, pitfalls, and surprises
- **`## See Also`**: Links to related skills using `[[skill-name]]` syntax

## Validation Rules

### Front Matter Validation

1. All required fields must be present
2. `name` must match kebab-case pattern
3. `version` must equal `1`
4. `confidence` must be between 0.0 and 1.0 (inclusive)
5. `created` must be valid YYYY-MM-DD format
6. `updated` (if present) must be valid YYYY-MM-DD format and >= `created`
7. All dependency names must match kebab-case pattern

### Body Validation

1. Must contain at least a title section (`# ...`)
2. Must contain "When to Use" and "Solution" sections
3. Content should not be empty (excluding whitespace)

### Security Validation

1. No credential patterns (API keys, passwords, tokens)
2. No prompt injection attempts
3. No executable code that could be harmful

## Examples

### Minimal Valid Skill

```markdown
---
name: simple-debugging-tip
version: 1
author: alice@example.com
confidence: 0.8
created: 2026-02-14
---

# Simple Debugging Tip

## When to Use
When you need to quickly inspect variable values during development.

## Solution
Add `fmt.Printf("DEBUG: %+v\n", variable)` to see the full structure.
```

### Complete Skill

```markdown
---
name: golang-race-condition-debugging
version: 1
tags: [debugging, golang, concurrency]
author: agent:claude-3.5
confidence: 0.9
created: 2026-02-13
updated: 2026-02-14
dependencies: [setup-golang-project]
---

# Debugging Go Race Conditions

## When to Use
When you suspect race conditions in Go concurrent code but can't reproduce them consistently.

## Solution
1. Build with race detector: `go build -race`
2. Run tests with race detection: `go test -race ./...`
3. For production-like load testing:
   ```bash
   go test -race -count=100 ./...
   ```

## Gotchas
- Race detector adds significant overhead (~2-20x slower)
- May not catch all races in light testing
- Some races only appear under heavy load

## See Also
- [[golang-mutex-patterns]]
- [[concurrent-testing-strategies]]
```

## File Naming

- Files must be named exactly `SKILL.md` (uppercase)
- Each skill lives in its own git repository
- Repository name should match the skill's `name` field

## Version Evolution

When the format changes:
1. Increment the version number in this spec
2. Update parsers to handle both old and new versions
3. Provide migration tools for existing skills
4. Maintain backward compatibility for at least one version

---

*This specification is living documentation. Update it as the format evolves.*