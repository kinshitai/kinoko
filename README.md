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

192 Go files · 707 tests · ~37K lines · 19 internal packages · 82% avg test coverage

## Quick Start

**Requirements:** Go 1.22+, Git, SSH

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
kinoko run --server localhost:23231
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
│  │  API (/health, /discover, /embed, /ingest,   │
│  │       /skills/decay)                         │   │
│  └──────────────────────────────────────────────┘   │
│                                  ↓                  │
│  Injection → classify prompt → query skills → inject│
└─────────────────────────────────────────────────────┘
```

### Extraction Pipeline

Sessions pass through a 3-stage filter. Each stage is more expensive; most sessions are rejected early.

1. **Stage 1 — Metadata pre-filters.** Duration, tool calls, error rate, successful execution. No I/O.
2. **Stage 2 — Embedding novelty + rubric scoring.** Embedding distance from existing skills, then 7 quality dimensions via LLM.
3. **Stage 3 — LLM critic.** Independent extract/reject verdict with retry, circuit breaker, and contradiction detection.

Extracted skills are written as `SKILL.md` files with YAML front matter and pushed to git. Post-receive hooks index them into SQLite with embeddings.

### Injection

When a session starts, Kinoko classifies the prompt, queries the skill index, and injects top-ranked skills. Ranking combines pattern overlap, embedding similarity, and historical success rate. Works in degraded mode (pattern-only) without an LLM key.

### Credential Scanning

14 patterns (AWS keys, GitHub tokens, JWTs, private keys, etc.) with context-aware matching. Runs in the extraction pipeline before git push and again in the pre-receive hook as a safety net. `kinoko scan` for manual checks.

### Background Workers

SQLite-backed job queue with atomic claim, backpressure, and retry scheduling. Configurable worker pool with graceful shutdown. Scheduler runs decay cycles and sweeps stale claims.

### Decay

Category-specific half-lives (foundational: 365d, tactical: 90d, contextual: 180d). Recently-used skills are rescued. Below-threshold skills are retired.

## CLI Reference

| Command | Description |
|---|---|
| `kinoko init` | Initialize workspace (`~/.kinoko/`), generate SSH key |
| `kinoko init --connect <url>` | Connect to a remote Kinoko server |
| `kinoko serve` | Start git server + discovery API + hooks |
| `kinoko run` | Start local daemon (workers, scheduler, injection) |
| `kinoko run --server host:port` | Connect daemon to a specific server |
| `kinoko extract <file>` | Run extraction pipeline on a session log |
| `kinoko import <files>` | Import session logs into the extraction queue |
| `kinoko ingest <file>` | Import a markdown file as a skill through the quality critic |
| `kinoko match <query>` | Find skills matching a query |
| `kinoko pull <repo>` | Clone/sync a skill from the server |
| `kinoko pull --all` | Sync all cached skills |
| `kinoko queue` | Queue inspection and management |
| `kinoko rebuild` | Rebuild SQLite cache from git repos |
| `kinoko scan <file>` | Scan for credentials |
| `kinoko scan --dir <path>` | Scan a directory recursively |
| `kinoko index <repo-path>` | Re-index a git repo into SQLite |
| `kinoko decay` | Run a decay cycle |
| `kinoko stats` | Print pipeline metrics |

## API Endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/health` | Server health check |
| `POST` | `/api/v1/discover` | Find skills matching a prompt |
| `POST` | `/api/v1/embed` | Generate embeddings for text |
| `POST` | `/api/v1/ingest` | Submit a markdown skill through the quality critic |
| `GET` | `/api/v1/skills/decay` | List skills ordered by decay score |
| `PATCH` | `/api/v1/skills/{id}/decay` | Update a skill's decay metadata |

## Documentation

- [Architecture overview](docs/architecture.md) — system design, data flow, and package map
- [kinoko.tech](https://kinoko.tech) (WIP) — full docs site, source in [`site/`](site/)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

TBD
