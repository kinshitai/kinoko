# Project State

## 2026-02-15 — End of Day Zero

### What Exists
- Go project at /home/claw/.openclaw/workspace/mycelium/
- CLI: `mycelium init` (workspace setup), `mycelium serve` (Soft Serve subprocess)
- SKILL.md parser: pkg/skill/ — YAML front matter + markdown, case-insensitive sections, 1MB buffer
- Git server: internal/gitserver/ — Soft Serve as managed subprocess, SSH admin key, repo CRUD
- Config: internal/config/ — YAML, tilde expansion, storage abstraction ready
- E2e tests: tests/e2e/ — build tag `integration`, fixtures in tests/fixtures/
- Docs: README, quickstart, installation, config ref, CLI ref, troubleshooting, llms.txt

### What's Next (Phase 1 remaining)
- Extraction pipeline: Stop hook → sanitize → extract → critic → commit
- Injection: UserPromptSubmit hook → embedding search → inject top 3
- Feedback signal: helpful/not helpful per injected skill
- Multi-remote config (layered libraries actually working)
- Actually use it ourselves (Hal on server + Egor on 2 Macbooks)

### What's NOT Built Yet
- Metadata server (SQLite + embedding search) — delayed intentionally, simple grep/search first
- Trust scoring, weighted voting, PRs
- Background workers (extraction, maintenance)
- Framework-agnostic hooks (only OpenClaw/Claude Code initially)
