# Kinoko 🍄

**Every problem solved once is solved for everyone.**

Kinoko is knowledge-sharing infrastructure for AI agents. When an agent solves a problem, Kinoko extracts the knowledge, stores it as a versioned skill in git, and injects it into future sessions — automatically. No human curation. No copy-paste. Knowledge flows like mycelium.

```
Agent solves problem → Extracted to SKILL.md → Pushed to git → Injected into future sessions
```

## Design Principles

- **Git is truth** — skills live in git repos. Everything else (SQLite, embeddings, scores) is derived cache that can be rebuilt from git at any time.
- **Repo-per-skill** — each skill is its own git repository. Independent versioning, granular access, composable layers.
- **Two processes, clean split** — `kinoko serve` (shared infrastructure) + `kinoko run` (local daemon). They communicate over HTTP and SSH. Solo or team, same architecture.
- **Zero friction** — extraction and injection are fully automatic. Agents don't know Kinoko exists.
- **Security ships on day one** — credential scanning in the pipeline and in pre-receive hooks. Secrets never reach git.

## Project Status

Active development. Core infrastructure is complete and reviewed.

117 Go files · 456 tests · ~30K lines · 16 internal packages · 73.6% test coverage

## Quick Start

**Requirements:** Go 1.24+, Git, SSH

```bash
# Install
git clone https://github.com/kinoko-dev/kinoko.git
cd kinoko
go install ./cmd/kinoko

# Initialize workspace (~/.kinoko/)
kinoko init

# Terminal 1 — Start the server (git server + discovery API + indexing)
kinoko serve

# Terminal 2 — Start the local daemon (workers + scheduler + injection)
export OPENAI_API_KEY=sk-...
kinoko run --server localhost:23231
```

That's it. `serve` manages git repos, the discovery/ingest API, and the skill index. `run` extracts knowledge from sessions, pushes skills to git, and injects them into future sessions.

**Solo use:** `kinoko serve` + `kinoko run` on the same machine.
**Team use:** One shared `kinoko serve`, each machine runs `kinoko init --connect <server>` + `kinoko run`.

## Architecture

Kinoko runs as two cooperating processes:

- **`kinoko serve`** — the shared infrastructure server. Runs the Soft Serve git server (SSH), the discovery/ingest HTTP API, credential-scanning hooks, and the SQLite index DB (skills, embeddings, injection events).
- **`kinoko run`** — the local agent daemon. Runs the worker pool, extraction pipeline, scheduler (decay, stale sweep), and injection. Communicates with `serve` over HTTP (`serverclient` package) for indexing, search, embedding, and decay operations.

```
┌─────────────────────────────────────────────────────────┐
│  kinoko run (local daemon)                              │
│                                                         │
│  Queue DB (SQLite) ← session logs                       │
│  Worker Pool → Extraction Pipeline (Stage 1→2→3)        │
│  Scheduler (decay cron, stale sweep, stats)             │
│  Injection → classify prompt → query skills → inject    │
│       │                                                 │
│       │ HTTP (serverclient)                              │
│       ▼                                                 │
│  ┌──────────────────────────────────────────────────┐   │
│  │  kinoko serve (shared infrastructure)            │   │
│  │                                                  │   │
│  │  Soft Serve (git) ← pre-receive (cred scan)     │   │
│  │                   ← post-receive (indexing)      │   │
│  │  Discovery API (/api/v1/discover, /health,       │   │
│  │                  /ingest, /search, /embed, ...)  │   │
│  │  Index DB (SQLite) — skills, embeddings, events  │   │
│  └──────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

### The Client/Server Boundary

`kinoko run` never touches the index DB directly. All reads and writes go through `kinoko serve`'s HTTP API via the `serverclient` package. This means:

- Multiple `run` instances can share one `serve`
- The index DB is only opened by one process (no SQLite lock contention)
- `run` maintains its own **queue DB** for the local job queue

For the full architecture reference, see [internal-docs/docs/architecture.md](internal-docs/docs/architecture.md).

### Extraction Pipeline

Sessions pass through a 3-stage filter. Each stage is more expensive; most sessions are rejected early.

1. **Stage 1 — Metadata pre-filters.** Duration, tool calls, error rate, successful execution. No I/O.
2. **Stage 2 — Embedding novelty + rubric scoring.** Embedding distance from existing skills, then 7 quality dimensions via LLM.
3. **Stage 3 — LLM critic.** Independent extract/reject verdict with retry, circuit breaker, and contradiction detection.

Extracted skills are written as `SKILL.md` files with YAML front matter and pushed to git. Post-receive hooks index them into the index DB with embeddings.

### Injection

When a session starts, Kinoko classifies the prompt, queries the skill index, and injects top-ranked skills. Ranking combines pattern overlap, embedding similarity, and historical success rate. Works in degraded mode (pattern-only) without an LLM key.

### Credential Scanning

14 patterns (AWS keys, GitHub tokens, JWTs, private keys, etc.) with context-aware matching. Runs in the extraction pipeline before git push and again in the pre-receive hook as a safety net. `kinoko scan` for manual checks.

### Background Workers

SQLite-backed job queue (queue DB, local to `kinoko run`) with atomic claim, backpressure, and retry scheduling. Configurable worker pool with graceful shutdown. Scheduler runs decay cycles and sweeps stale claims.

### Decay

Category-specific half-lives (foundational: 365d, tactical: 90d, contextual: 180d). Recently-used skills are rescued. Below-threshold skills are retired.

## CLI Reference

| Command | Description |
|---|---|
| `kinoko init` | Initialize workspace (`~/.kinoko/`), generate SSH key |
| `kinoko init --connect <url>` | Connect to a remote Kinoko server |
| `kinoko serve` | Start git server + discovery API + hooks + index DB |
| `kinoko run` | Start local daemon (workers, scheduler, injection, queue DB) |
| `kinoko run --server host:port` | Connect daemon to a specific server |
| `kinoko extract <file>` | Manually extract from a session log |
| `kinoko pull <repo>` | Clone/sync a skill from the server |
| `kinoko pull --all` | Sync all cached skills |
| `kinoko scan <file>` | Scan for credentials |
| `kinoko scan --dir <path>` | Scan a directory recursively |
| `kinoko index <repo-path>` | Re-index a git repo into the index DB |
| `kinoko decay` | Run a decay cycle |
| `kinoko stats` | View pipeline metrics and A/B results |
| `kinoko queue` | Inspect the local job queue |
| `kinoko import <path>` | Batch-import session logs into the queue |

## Documentation

- **Docs site:** [kinoko.tech](https://kinoko.tech) (WIP) — source in [`site/`](site/), built with Astro Starlight.
- **Architecture:** [internal-docs/docs/architecture.md](internal-docs/docs/architecture.md) — full technical reference.
- **Concepts:** [internal-docs/docs/concepts.md](internal-docs/docs/concepts.md) — mental models for contributors.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

TBD
