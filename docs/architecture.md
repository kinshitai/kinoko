# Kinoko Architecture

Kinoko is knowledge-sharing infrastructure for AI agents. Agents extract reusable skills from coding sessions, store them in git repositories, and inject relevant skills into future sessions.

## High-Level Data Flow

```
Session log → extract → git commit → index → SQLite
                                                ↓
Agent prompt ← inject ← discover ← query ← SQLite
```

The full pipeline:

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

Kinoko has two operational modes that share configuration but run independently:

### `internal/serve/` — Server

The shared infrastructure server, started with `kinoko serve`. It runs:

- **Soft Serve git server** (`gitserver/`) — SSH-accessible git hosting for skill repos. Includes post-receive hooks that trigger indexing and credential scanning.
- **HTTP API** (`api/`) — Discovery, ingestion, embedding, health, and decay endpoints.
- **SQLite store** (`storage/`) — Indexed skill metadata, embeddings, and quality scores. Derived from git — the git repos are the source of truth.
- **Embedding engine** (`embedding/`) — OpenAI API or local ONNX models for computing vector embeddings.

### `internal/run/` — Client Daemon

The local agent daemon, started with `kinoko run`. It runs:

- **Worker pool** (`worker/`) — Processes session logs from the local queue.
- **Scheduler** (`worker/`) — Periodic decay cycles, stale sweep, stats.
- **Extraction pipeline** (`extraction/`) — 3-stage skill extraction (see below).
- **Injection** (`injection/`) — Classifies prompts, queries the server, and builds injection prompts.
- **Job queue** (`queue/`) — Local SQLite queue for pending session log files.
- **API client** (`apiclient/`) — HTTP client for pushing skills and querying the server.

### `internal/shared/` — Common

Packages imported by both run and serve:

- **config** — YAML config loading with sensible defaults. Covers server, storage, client, extraction, decay, embedding, LLM, hooks, and debug settings.
- **circuitbreaker** — Protects external calls (LLM, embedding APIs) with failure counting and automatic recovery.
- **decay** — Half-life decay logic. Skills degrade over time by category; recent usage rescues them.

## Public Packages

### `pkg/model`

Domain types and interfaces shared across the entire codebase:

- **Domain types** — `SessionRecord`, `SkillRecord`, `ExtractionResult`, `Stage1Result`, `Stage2Result`, `Stage3Result`, `InjectionRequest`, `InjectionResponse`, `QualityScores`.
- **Interfaces** — `Extractor`, `Embedder`, `Committer`, `Indexer`, `Querier`, `SkillStore`.
- **Categories** — Tactical, Architectural, Debugging, etc.
- **Domains** — Frontend, Backend, DevOps, Data, Security, Performance.

### `pkg/skill`

Parser and validator for SKILL.md files:

- Parses YAML front matter (name, version, tags, quality scores, confidence).
- Validates required fields and body sections (When to Use, Solution, Why It Works, Pitfalls).
- Used by the indexer and the `ingest` command.

## Extraction Pipeline

The extraction pipeline (`internal/run/extraction/`) processes session logs through three stages:

### Stage 1 — Metadata Filter
Fast, synchronous, no I/O. Filters sessions by:
- Duration (min/max minutes)
- Tool call count (minimum threshold)
- Error rate (maximum threshold)
- Presence of successful exec calls

### Stage 2 — Classification & Scoring
Uses embeddings and LLM to:
- Classify the session into taxonomy patterns (e.g. `FIX/Backend/DatabaseConnection`)
- Score quality on a rubric (problem specificity, solution completeness, context portability, etc.)
- Determine category (Tactical, Architectural, Debugging)

The taxonomy is a fixed list of patterns covering BUILD, FIX, OPTIMIZE, LEARN, and DEBUG domains.

### Stage 3 — LLM Critic
Sends the session content to an LLM for final evaluation:
- Extract/reject verdict with confidence score
- Generates the SKILL.md content (structured front matter + body)
- Applies substitution test: "Would this help someone who has never seen this project?"
- Hard reject triggers for project-specific documentation masquerading as skills
- Circuit breaker protection with configurable retry (up to 3 retries, 5 for rate limits)

After extraction, skills go through:
- **Novelty check** — Queries the discover endpoint to avoid duplicating existing skills.
- **Credential scan** — Scans output for secrets before committing.
- **Git commit** — Commits SKILL.md to the appropriate library repo via Soft Serve.
- **Human review sampling** — Stratified sampling writes a fraction of results (extracted and rejected) to local files for quality auditing.

## CLI Commands

| Command | Description |
|---------|-------------|
| `kinoko serve` | Start the infrastructure server (git + API + hooks + SQLite) |
| `kinoko run` | Start the local agent daemon (workers + scheduler + injection) |
| `kinoko init` | Initialize workspace; `--connect <url>` links to a server |
| `kinoko extract <file>` | Run the extraction pipeline on a single session log |
| `kinoko ingest <file.md>` | Import a markdown file through the quality critic |
| `kinoko index` | Index a skill repo into SQLite (used by post-receive hooks) |
| `kinoko pull [repo]` | Clone or update skill repos; `--all` syncs everything |
| `kinoko match <query>` | Find skills matching a text query |
| `kinoko scan [file]` | Scan files for credentials and secrets |
| `kinoko decay` | Run one decay cycle over a library |
| `kinoko stats` | Print client pipeline metrics |
| `kinoko import` | Bulk import skills |
| `kinoko queue` | Manage the local job queue |
| `kinoko rebuild` | Rebuild the SQLite index from git |

## API Endpoints

The HTTP API server listens on port `server.port + 2` by default (23233 when server port is 23231).

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/health` | Health check; returns `{"status":"ok","skills":<count>}` |
| `POST` | `/api/v1/discover` | Skill discovery — accepts prompt, embedding, patterns, library_ids, min_quality, top_k |
| `POST` | `/api/v1/embed` | Compute embedding for text |
| `POST` | `/api/v1/ingest` | Trigger indexing for a repo (async); used by post-receive hooks |
| `GET` | `/api/v1/skills/decay` | List skills ordered by decay score |
| `PATCH` | `/api/v1/skills/{id}/decay` | Update a skill's decay score |

### Discovery Request

```json
{
  "prompt": "fix database timeout in Go",
  "embedding": [],
  "patterns": ["FIX/Backend/DatabaseConnection"],
  "library_ids": ["local"],
  "min_quality": 0.5,
  "top_k": 10
}
```

At least one of `prompt`, `embedding`, or `patterns` must be provided. If `prompt` is given without `embedding`, the server embeds it automatically. Concurrent discover requests are limited to 10 via semaphore.

## Configuration

Configuration lives in `~/.kinoko/config.yaml` (YAML). The `config` package provides defaults for all settings:

- **Server** — host, port, data directory, API port
- **Storage** — SQLite driver and DSN
- **Client** — API URL, SSH URL, cache directory
- **Libraries** — Named skill libraries with paths
- **Extraction** — Duration limits, tool call thresholds, error rate caps
- **Decay** — Half-life periods per category, rescue thresholds
- **Embedding** — Provider (OpenAI/ONNX), model name, API key
- **LLM** — Provider, model, API key, temperature
- **Hooks** — Credential scanning, auto-indexing
- **Debug** — Trace directory for pipeline debugging
