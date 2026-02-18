# Decisions

## 2026-02-15
- Tool recommendation: Starlight (Astro) for docs site — but Jazz says premature for Phase 1. He's right.
- Phase 1 approach: markdown files in repo, no docs site yet
- 5-minute rule: non-negotiable. README to working example in 5 min.
- AI-native: llms.txt for machine-readable summary, structured front matter on docs
- Jazz's feedback: focus on essentials, don't over-scope
- Docs-as-code: everything in repo, versioned with code

## 2026-02-15 (v2 revision)
- Reconciled Jazz's review: trimmed all speculative/planned content from docs
- Fixed Go version: 1.24+ (was incorrectly 1.21+ in all docs)
- Fixed install instructions: from-source only (go install @latest won't work yet)
- Clarified skill storage: subdirectories of ~/.kinoko/skills/, not separate repos
- Added CONTRIBUTING.md at repo root
- Removed "Coming Soon" sections from CLI reference — document what exists
- Shortened all docs significantly — accuracy over ambition
- README rewritten: tighter, honest about status, working quick start
