// Package skill provides parsing, validation, and round-trip serialization
// for Mycelium SKILL.md files. Skills are the atomic unit of knowledge in
// Mycelium — each skill represents a reusable piece of knowledge extracted
// from agent work sessions.
//
// SKILL.md files use Markdown with YAML front matter. This package handles
// parsing both components, validating required sections, and writing skills
// back to disk in canonical format.
package skill
