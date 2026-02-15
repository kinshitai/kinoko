# SKILL.md Format

Skills are structured knowledge stored as Markdown files with YAML front matter. Each skill lives in a subdirectory of a skills repository (e.g., `~/.mycelium/skills/my-skill/SKILL.md`).

## Structure

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
[When this skill applies]

## Solution
[The actual knowledge — steps, patterns, code]

## Gotchas
[Edge cases and pitfalls]

## See Also
- [[related-skill-name]]
```

## Front Matter

### Required Fields

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Kebab-case identifier. Must match `^[a-z0-9]+(-[a-z0-9]+)*$` |
| `version` | int | Schema version. Must be `1`. |
| `author` | string | Contributor ID (email, username, or `agent:name`) |
| `confidence` | float | Reliability score, 0.0–1.0 |
| `created` | string | Creation date, `YYYY-MM-DD` format |

### Optional Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `tags` | string[] | `[]` | Categorization labels |
| `updated` | string | — | Last modified date, `YYYY-MM-DD`. Must be ≥ `created`. |
| `dependencies` | string[] | `[]` | Names of skills this one depends on (kebab-case) |

## Body Sections

### Required

- **`# Title`** — at least one H1 heading
- **`## When to Use`** — when this skill applies (case-insensitive match)
- **`## Solution`** — the actual knowledge (case-insensitive match)

### Optional

- **`## Gotchas`** — edge cases, pitfalls
- **`## See Also`** — links to related skills

## Validation Rules

1. Front matter must be wrapped in `---` delimiters
2. `name` must be kebab-case
3. `version` must be `1`
4. `confidence` must be 0.0–1.0
5. `created` must be valid `YYYY-MM-DD`
6. `updated` (if present) must be ≥ `created`
7. Dependency names must be kebab-case
8. Body must contain title, "When to Use", and "Solution" sections
9. Body cannot be empty

## Confidence Guidelines

| Range | Meaning |
|-------|---------|
| 0.9–1.0 | Well-tested, high reliability |
| 0.7–0.9 | Good solution, some edge cases |
| 0.5–0.7 | Works but needs validation |
| < 0.5 | Experimental |

## Examples

### Minimal

```markdown
---
name: simple-tip
version: 1
author: alice@example.com
confidence: 0.8
created: 2026-02-14
---

# Simple Debugging Tip

## When to Use
When you need to inspect variable values in Go.

## Solution
Use `fmt.Printf("DEBUG: %+v\n", variable)` to see the full structure.
```

### Complete

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
When you suspect race conditions in concurrent Go code.

## Solution
1. Build with race detector: `go build -race`
2. Run tests: `go test -race ./...`
3. For thorough testing:
   ```bash
   go test -race -count=100 ./...
   ```

## Gotchas
- Race detector adds 2–20x overhead
- Light testing may miss some races
- Some races only appear under heavy load

## See Also
- [[golang-mutex-patterns]]
- [[concurrent-testing-strategies]]
```

## File Layout

Skills are stored as `SKILL.md` inside named subdirectories:

```
~/.mycelium/skills/
├── simple-tip/
│   └── SKILL.md
├── golang-race-condition-debugging/
│   └── SKILL.md
└── ...
```

The directory name should match the skill's `name` field.
