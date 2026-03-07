# Architecture

> Read this first. Every contributor and every agent should understand these four boundaries before touching the codebase.

**Server (`kinoko serve`):** Git server + search index. Stores skill repos, indexes SKILL.md into SQLite, tracks git stats (last commit, contributors, clones). Two endpoints: `POST /api/v1/discover` (search with raw signals) and `POST /api/v1/embed` (embeddings). Post-receive hook re-indexes on push. No computation, no mutation, no client awareness.

**Client (`kinoko run`):** Runs alongside your agent. Extracts skills from sessions → commits SKILL.md → pushes to server via git. At injection time, asks server for matches, gets raw signals, combines with personal usage data, ranks locally. Computes decay from git metadata (freshness, activity). Personal experience stored in gitignored `.kinoko/` files inside cloned repos — never pushed.

**Git:** Only write path. Only communication channel between client and server. Client pushes skills, server indexes on receive.

**Boundary:** Server never sees sessions or per-client behavior. Client never writes to server except git push. Shared knowledge flows through git. Personal experience stays local.

---

## Data Flow

```
Session log → extract → git commit → index → SQLite
                                                ↓
Agent prompt ← inject ← discover ← query ← SQLite
```

1. **Extract** — A 3-stage pipeline filters and distills session logs into SKILL.md files.
2. **Git** — Extracted skills are committed to Soft Serve git repos (one repo per skill).
3. **Index** — A post-receive hook parses SKILL.md, computes embeddings, and writes to SQLite.
4. **Discover** — The HTTP API accepts a prompt or embedding and returns ranked skill matches.
5. **Inject** — The client queries discover, then formats matching skills into a prompt section for the agent.

## Repository Layout

```
cmd/kinoko/          CLI entry point (cobra commands)
internal/
  run/               Client-side daemon packages
    apiclient/       HTTP client for the server API
    client/          Local client config and SSH key management
    debug/           Pipeline debug tracing
    extraction/      3-stage extraction pipeline
    injection/       Skill injection (classify → discover → prompt)
    llm/             LLM provider abstraction
    llmutil/         JSON extraction and retry helpers
    metrics/         Client pipeline metrics
    queue/           Local SQLite job queue for session processing
    sanitize/        Credential scanning
    worker/          Worker pool and scheduler
  serve/             Server-side packages
    api/             HTTP API server
    embedding/       Embedding providers (OpenAI, ONNX)
    gitserver/       Soft Serve git server wrapper, hooks, committer
    storage/         SQLite skill store (schema, indexer, querier)
  shared/            Packages used by both run and serve
    circuitbreaker/  Circuit breaker for external calls
    config/          YAML config loading and defaults
    decay/           Skill decay (half-life degradation)
pkg/
  model/             Public domain types and interfaces
  skill/             SKILL.md parser and validator
tests/
  architecture/      Boundary tests (import rules)
  e2e/               End-to-end tests
  integration/       Integration tests
  fixtures/          Test data
```

## The run/serve/shared Split

The directory structure mirrors the architectural boundary above.

### `internal/serve/` — Server

Shared infrastructure, started with `kinoko serve`:

- **Soft Serve git server** (`gitserver/`) — SSH-accessible git hosting for skill repos. Post-receive hooks trigger indexing and credential scanning.
- **HTTP API** (`api/`) — Discovery, ingestion, embedding, and health endpoints.
- **SQLite store** (`storage/`) — Indexed skill metadata, embeddings, and quality scores. Derived from git — the git repos are the source of truth.
- **Embedding engine** (`embedding/`) — ONNX Runtime with `bge-small-en-v1.5` for vector embeddings. Runs server-side so clients stay pure Go.

### `internal/run/` — Client Daemon

Local agent daemon, started with `kinoko run`:

- **Worker pool** (`worker/`) — Processes session logs from the local queue.
- **Scheduler** (`worker/`) — Periodic stale sweep, decay cycles, stats.
- **Extraction pipeline** (`extraction/`) — 3-stage skill extraction (see below).
- **Injection** (`injection/`) — Classifies prompts, queries the server, and builds injection prompts.
- **Job queue** (`queue/`) — Local SQLite queue for pending session log files.
- **API client** (`apiclient/`) — HTTP client for pushing skills and querying the server.

### `internal/shared/` — Common

Packages imported by both run and serve:

- **config** — YAML config loading with sensible defaults.
- **circuitbreaker** — Protects external calls (LLM, embedding APIs) with failure counting and automatic recovery.
- **decay** — Half-life decay logic. Skills degrade over time by category; recent usage rescues them.

## Session Parsing

Before extraction begins, session logs must be parsed into structured metadata. The `SessionParser` interface (`internal/run/extraction/parser.go`) handles format detection and dispatch:

1. **Format detection** — the first 4KB of input is peeked to identify the log format.
2. **Parser dispatch** — an ordered list of parsers is tried; the first match wins.
3. **Metadata extraction** — the matched parser reads timestamps, tool calls, error counts, and other metadata into a `SessionRecord`.

Built-in parsers:

| Parser | Format | Detection |
|--------|--------|-----------|
| `ClaudeCodeParser` | Claude Code native JSONL | First line is JSON with a known `type` field (`assistant`, `user`, `system`, etc.) |
| `FallbackParser` | Generic text logs | Catch-all for unrecognized formats |

New agent formats are supported by implementing the `SessionParser` interface — no changes to the pipeline or CLI commands required.

## Extraction Pipeline

The extraction pipeline (`internal/run/extraction/`) processes session logs through three stages:

**Stage 1 — Metadata Filter.** Fast, synchronous, no I/O. Filters sessions by duration, tool call count, error rate, and presence of successful exec calls.

**Stage 2 — Classification & Scoring.** Uses embeddings and LLM to classify into taxonomy patterns (e.g. `FIX/Backend/DatabaseConnection`), score quality on a rubric, and determine category (Tactical, Architectural, Debugging).

**Stage 3 — LLM Critic.** Sends session content to an LLM for extract/reject verdict. Generates the SKILL.md content. Applies substitution test: "Would this help someone who has never seen this project?" Hard-rejects project-specific documentation masquerading as skills.

After extraction: novelty check (avoid duplicates), credential scan, git commit, and optional human-review sampling.

## API Endpoints

The HTTP API listens on port `server.port + 2` (default 23233).

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/health` | Health check; returns `{"status":"ok","skills":<count>}` |
| `POST` | `/api/v1/discover` | Skill discovery — accepts prompt, embedding, patterns, library_ids, min_quality, top_k |
| `POST` | `/api/v1/embed` | Compute embedding for text |
| `POST` | `/api/v1/ingest` | Trigger indexing for a repo (async); used by post-receive hooks |

## CLI Commands

| Command | Description |
|---------|-------------|
| `kinoko serve` | Start the infrastructure server (git + API + hooks + SQLite) |
| `kinoko run` | Start the local agent daemon (workers + scheduler + injection) |
| `kinoko init` | Initialize workspace; `--connect <url>` links to a server |
| `kinoko extract <file>` | Run the extraction pipeline on a single session log |
| `kinoko convert <file>` | Convert a document into SKILL.md format (genre-aware, skips session filtering) |
| `kinoko ingest <file.md>` | Import a markdown file through the quality critic |
| `kinoko index` | Index a skill repo into SQLite (used by post-receive hooks) |
| `kinoko pull [repo]` | Clone or update skill repos; `--all` syncs everything |
| `kinoko match <query>` | Find skills matching a text query |
| `kinoko scan [file]` | Scan files for credentials and secrets |
| `kinoko stats` | Print client pipeline metrics |
| `kinoko import` | Bulk import skills |
| `kinoko queue` | Manage the local job queue |
| `kinoko rebuild` | Rebuild the SQLite index from git |

## Public Packages

**`pkg/model`** — Domain types (`SessionRecord`, `SkillRecord`, `ExtractionResult`, etc.), interfaces (`Extractor`, `Embedder`, `Committer`, `Indexer`, `Querier`, `SkillStore`), categories, and domains.

**`pkg/skill`** — SKILL.md parser and validator. Parses YAML front matter, validates required fields and body sections (When to Use, Solution, Why It Works, Pitfalls).

## Configuration

Config lives in `~/.kinoko/config.yaml`. The `config` package provides defaults for: server, storage, client, libraries, extraction, decay, embedding, LLM, hooks, and debug settings.
