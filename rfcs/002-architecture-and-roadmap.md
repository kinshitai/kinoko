# RFC-002: Architecture & Roadmap

| Field   | Value       |
|---------|-------------|
| Status  | Living      |
| Date    | 2026-02-14  |

This is a guideline, not a contract. Revise as we learn.

---

## Architectural Decisions

### Repo-Per-Skill

Each skill is its own git repo on the server. Not a monorepo.

Why: independent versioning, granular permissions, natural mapping to fork/override layering, per-skill trust scores and feedback. Clone only what you need. A company can fork one skill without pulling the whole library.

### Three Components

```
┌──────────────────┐
│   BG Worker      │  extraction, validation, index maintenance
└────────┬─────────┘
         │
┌────────┴─────────┐
│ Metadata Server  │  embeddings, discovery, trust, feedback
└────────┬─────────┘
         │
┌────────┴─────────┐
│ Git Server       │  Soft Serve, repo-per-skill, SSH access
│                  │  the source of truth
└──────────────────┘
```

### Soft Serve as Git Server

Why not Forgejo/Gitea: we need custom collaboration workflows (weighted voting, agent-driven PRs, pre-commit quality gates). A full forge gives us PRs we'd replace anyway. Soft Serve gives us git hosting and stays out of our way.

Why not bare git: Soft Serve adds SSH access, repo management, and a TUI. Just enough to be useful without being opinionated about collaboration.

### Pre-Commit Hooks Run on Contributor's Machine

Quality gates split between local (contributor's resources) and server (global knowledge):

**Pre-commit (local):**
- Credential scanning
- Format validation (SKILL.md schema)
- Prompt injection detection
- LLM critic review

**Server-side (on push):**
- Deduplication (needs the full embedding index)
- Cross-skill conflict detection
- Trust score update

This distributes compute. At scale, extraction cost is borne by contributors, not the server.

### Layered Libraries Like Docker Images

```yaml
libraries:
  - name: local
    path: ~/.kinoko/skills
    priority: 100

  - name: home
    url: ssh://hal@home:23231
    priority: 50

  # Adding cloud = adding one line
  # - name: cloud
  #   url: https://cloud.kinoko.dev
  #   priority: 10
```

Resolution: local > company > public. Adding a layer = adding a URL. Removing a layer = removing a URL. Skills shadow lower layers by name.

### SQLite Behind an Abstraction

Storage interface from day one. SQLite ships first. Postgres swappable via config for enterprise/cloud.

```yaml
storage:
  driver: sqlite
  dsn: ~/.kinoko/kinoko.db
```

Embeddings: SQLite vec extension for vector search. Swappable to pgvector. One database, one file, vector search included.

### Go

The team knows Go. The ecosystem (Soft Serve, Charm libraries) is Go. Single binary distribution. Fast, boring, reliable.

## Roadmap

### Phase 1: Server + Extraction (Weeks 1-4)

**Goal:** A running Kinoko server that we use daily. Skills extracted from our sessions, stored as repos.

**Build:**
- Project scaffolding (Go module, CLI skeleton)
- Soft Serve integration — single `kinoko serve` starts the git server
- SKILL.md format spec + parser
- Basic extraction — Stop hook extracts skills from agent sessions
- Skills pushed as repos to Soft Serve
- Pre-commit hooks — credential scan + format validation
- `kinoko init` sets up local config + hooks

**Users:** Hal (OpenClaw on server) + Egor (Claude Code on 2 MacBooks). Three agents, one server.

**What it proves:** Agents can extract skills from real sessions and store them in a self-hosted git server with quality gates.

**Self-hosting experience:**
```bash
kinoko serve                  # starts everything
kinoko init                   # on each client machine
kinoko remote add home ssh://...
```

### Phase 2: Injection + Metadata (Weeks 5-8)

**Goal:** The full loop works. Skills extracted in one session help a different agent in a future session.

**Build:**
- Metadata server (SQLite + SQLite vec)
- Embedding index built from skill repos
- UserPromptSubmit hook — search index, inject relevant skills
- Feedback signal — helpful/not helpful per injected skill
- Multi-remote config (layered libraries)
- Storage abstraction (interface ready for Postgres swap)

**Users:** Same three agents, but now injection works. Knowledge flows.

**What it proves:** The core thesis — a problem solved once helps someone else automatically.

### Phase 3: Trust + Collaboration (Weeks 9-12)

**Goal:** Ready for more people. Quality scales.

**Build:**
- Trust scoring per contributor
- Weighted voting on contributions (PR-like workflow via metadata server)
- Server-side validation pipeline (dedup, conflict detection)
- LLM critic in pre-commit hooks
- Feedback-driven decay and ranking
- Framework-agnostic hook spec (not just OpenClaw/Claude Code)

**Users:** Egor's wife joins with Codex. Four+ agents across different frameworks.

**What it proves:** The system works beyond the founders. Quality holds with more contributors.

### Beyond

Ideas, not commitments:
- Cloud hosted layer (just a URL in config)
- Federation between servers
- Agent identity (DIDs)
- Skill composition
- Web UI for browsing
- AT Protocol / Tangled integration

## Design Principles

Carried from the manifesto into engineering:

1. **Build what you know you need.** No throwaway scaffolding. If we know we need git, pre-commit hooks, and storage abstraction — build them right from the start.

2. **Self-hostable first, cloud-layered.** `kinoko serve` works on a Raspberry Pi. Cloud is one config line away but never required.

3. **Git is the truth.** Everything else — embeddings, trust scores, feedback — is a derived cache. Blow it away and rebuild from git.

4. **Background workers are infrastructure.** Extraction, maintenance, and feedback processing are async from day one.

5. **One config file.** `~/.kinoko/config.yaml` controls everything. Libraries, storage driver, server settings. No scattered config, no env vars for core settings.

---

*This roadmap will change. Phases will shift. Features will move between phases. The principles won't change. Update this document as we build — it should always reflect our current understanding, not our first guess.*
