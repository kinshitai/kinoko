# Kinoko 🍄

**Every problem solved once is solved for everyone.**

Kinoko is knowledge-sharing infrastructure for AI agents. When an agent solves a problem, Kinoko extracts the knowledge, stores it as a versioned skill in git, and injects it into future sessions — automatically. No human curation. No copy-paste. Knowledge flows like mycelium.

```
Agent solves problem → Extracted to SKILL.md → Pushed to git → Injected into future sessions
```

## Design Principles

- **Git is truth** — skills live in git repos. Everything else (SQLite, embeddings, scores) is derived cache that can be rebuilt from git at any time.
- **Repo-per-skill** — each skill is its own git repository. Independent versioning, granular access, composable layers.
- **Three commands** — `init` (client setup), `serve` (shared infrastructure), `run` (local daemon). Solo or team, same architecture.
- **Zero friction** — extraction and injection are fully automatic. Agents don't know Kinoko exists.
- **Security ships on day one** — credential scanning in the pipeline and in pre-receive hooks. Secrets never reach git.

## Project Status

Active development. Core infrastructure is complete and reviewed.

## Quick Start

**Requirements:** Go 1.24+, Git, SSH

```bash
# Install
git clone https://github.com/kinshitai/kinoko.git
cd kinoko
go install ./cmd/kinoko

# Initialize workspace (~/.kinoko/)
kinoko init

# Start the server (git server + discovery API + hooks)
kinoko serve

# In another terminal — start the local daemon (workers + scheduler + injection)
export OPENAI_API_KEY=sk-...
kinoko run
```

That's it. `serve` manages git repos and the discovery API. `run` extracts knowledge from sessions, pushes skills to git, and injects them into future sessions.

**Solo use:** `kinoko serve` + `kinoko run` on the same machine.
**Team use:** One shared `kinoko serve`, each machine runs `kinoko init --connect <server>` + `kinoko run`.

## Architecture

```
┌─────────────────────────────────────────────────────┐
│  kinoko run (local daemon)                          │
│                                                     │
│  Agent sessions → Worker Pool → Extraction Pipeline │
│                                  ↓                  │
│                            Git Push (SKILL.md)      │
│                                  ↓                  │
│  ┌──────────────────────────────────────────────┐   │
│  │  kinoko serve (shared infrastructure)        │   │
│  │                                              │   │
│  │  Soft Serve (git) ← pre-receive (cred scan) │   │
│  │                   ← post-receive (indexing)  │   │
│  │  HTTP API (/discover, /embed, /ingest,       │   │
│  │           /health, /skills/decay)            │   │
│  └──────────────────────────────────────────────┘   │
│                                  ↓                  │
│  Injection → classify prompt → query skills → inject│
└─────────────────────────────────────────────────────┘
```

### Extraction Pipeline

Sessions pass through a 3-stage filter. Each stage is more expensive; most sessions are rejected early.

1. **Stage 1 — Metadata pre-filters.** Duration, tool calls, error rate, successful execution. No I/O.
2. **Stage 2 — Embedding novelty + rubric scoring.** Embedding distance from existing skills, 7-dimension quality scoring via LLM, pattern taxonomy classification.
3. **Stage 3 — LLM critic.** Independent extract/reject verdict with substitution test, hard-reject triggers, SKILL.md generation, retry with circuit breaker.

Extracted skills are written as `SKILL.md` files with YAML front matter and pushed to git. Post-receive hooks index them into SQLite with embeddings.

### Injection

When a session starts, Kinoko classifies the prompt, queries the skill index via `POST /api/v1/discover`, and injects top-ranked skills. Ranking combines pattern overlap, embedding similarity, and historical success rate. Degrades to pattern-only ranking without embeddings.

### Ports

| Port | Service |
|------|---------|
| 23231 | Soft Serve SSH (git) |
| 23232 | Soft Serve HTTP |
| 23233 | Kinoko HTTP API |

### Credential Scanning

14 patterns (AWS keys, GitHub tokens, JWTs, private keys, etc.) with context-aware matching. Runs in the extraction pipeline before git push and again in the pre-receive hook as a safety net. `kinoko scan` for manual checks.

### Background Workers

SQLite-backed job queue with atomic claim, backpressure, and retry scheduling. Configurable worker pool with graceful shutdown. Scheduler runs decay cycles and sweeps stale claims.

### Decay

Category-specific half-lives (foundational: 365d, tactical: 90d, contextual: 180d). Recently-used skills with positive outcomes are rescued. Below-threshold skills are retired.

## CLI Reference

| Command | Description |
|---|---|
| `kinoko init` | Initialize workspace (`~/.kinoko/`), generate SSH key |
| `kinoko init --connect <url>` | Connect to a remote Kinoko server |
| `kinoko serve` | Start git server + HTTP API + hooks |
| `kinoko run` | Start local daemon (workers, scheduler, injection) |
| `kinoko run --server host:port` | Connect daemon to a specific server |
| `kinoko extract <file>` | Run extraction pipeline on a session log |
| `kinoko ingest <file.md>` | Import markdown as a skill through the LLM critic |
| `kinoko import <file...>` | Enqueue session logs for extraction |
| `kinoko match <query>` | Find skills matching a text query |
| `kinoko pull <repo>` | Clone/sync a skill from the server |
| `kinoko pull --all` | Sync all cached skills |
| `kinoko index` | Index a skill repo into SQLite (hook use) |
| `kinoko rebuild` | Rebuild SQLite cache from all git repos |
| `kinoko scan <file>` | Scan for credentials |
| `kinoko scan --dir <path>` | Scan a directory recursively |
| `kinoko decay` | Run a decay cycle |
| `kinoko stats` | View pipeline metrics and A/B results |
| `kinoko queue stats\|list\|retry\|flush` | Manage the extraction queue |

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/health` | Health check with skill count |
| `POST` | `/api/v1/discover` | Unified skill discovery (prompt/embedding/patterns) |
| `POST` | `/api/v1/embed` | Text → embedding vector |
| `POST` | `/api/v1/ingest` | Submit session log for extraction |
| `GET` | `/api/v1/skills/decay` | List skills by decay score |
| `PATCH` | `/api/v1/skills/{id}/decay` | Update skill decay score |

## Documentation

Full docs at [kinoko.tech](https://kinoko.tech) (WIP).

- [Architecture](docs/architecture.md) — detailed package map and data flow
- Source in [`site/`](site/) — built with Astro Starlight

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

TBD
