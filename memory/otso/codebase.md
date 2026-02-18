# Codebase Knowledge

## 2026-02-15
- Go module: github.com/kinoko-dev/kinoko
- CLI: cobra, entry point cmd/kinoko/main.go
- Config: internal/config/config.go — YAML with Server, Storage, Libraries, Extraction, Hooks, Defaults sections
- Git server: internal/gitserver/ — server.go (subprocess mgmt), keys.go (SSH keygen), binary.go (soft binary detection)
- Skill parser: pkg/skill/skill.go — YAML front matter + markdown body, case-insensitive section matching
- Version: var injection via ldflags, not hardcoded
- Logging: slog throughout
- Storage: internal/storage/ — SQLiteStore implementing SkillStore interface, uses modernc.org/sqlite (pure Go)
- Extraction types: internal/extraction/types.go — SkillRecord, QualityScores, SkillCategory
- Schema: internal/storage/schema.sql — embedded DDL, 6 tables, run on startup with CREATE IF NOT EXISTS
