# Codebase Knowledge

## 2026-02-15
- Go module: github.com/mycelium-dev/mycelium
- CLI: cobra, entry point cmd/mycelium/main.go
- Config: internal/config/config.go — YAML with Server, Storage, Libraries, Extraction, Hooks, Defaults sections
- Git server: internal/gitserver/ — server.go (subprocess mgmt), keys.go (SSH keygen), binary.go (soft binary detection)
- Skill parser: pkg/skill/skill.go — YAML front matter + markdown body, case-insensitive section matching
- Version: var injection via ldflags, not hardcoded
- Logging: slog throughout
