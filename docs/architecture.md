# Kinoko Architecture

Kinoko is knowledge-sharing infrastructure for AI agents. Agents extract reusable skills from coding sessions and inject relevant skills into future sessions. This document describes the system as implemented.

## System Overview

Kinoko runs as two cooperating processes:

- **`kinoko serve`** — the shared infrastructure server: Soft Serve git server (SSH), HTTP API, SQLite index, post-receive hooks
- **`kinoko run`** — the local agent daemon: worker pool, scheduler, extraction pipeline, injection

A third mode, **standalone CLI commands**, lets operators inspect and manage the system without running either daemon.

## Binary and Entry Point

A single binary (`cmd/kinoko/main.go`) uses Cobra to register all subcommands in `cmd/kinoko/root.go`:

| Command | Description |
|---------|-------------|
| `serve` | Start the infrastructure server (git + API + hooks) |
| `run` | Start the local agent daemon (workers + scheduler + injection) |
| `init` | Initialize `~/.kinoko/` workspace; `--connect <url>` for client mode |
| `extract` | Run extraction pipeline on a single session log file |
| `ingest` | Import a markdown file as a skill through the Stage 3 LLM critic |
| `import` | Parse session log files and enqueue them for extraction |
| `match` | Query the server for skills matching a text prompt |
| `pull` | Clone or update skill repos from the server |
| `index` | Index a skill repo into SQLite (called by post-receive hook) |
| `rebuild` | Rebuild the SQLite cache from all git repos |
| `decay` | Run one decay cycle over skills in a library |
| `stats` | Print pipeline metrics (stage pass rates, yield, injection, A/B) |
| `queue` | Queue inspection: `stats`, `list`, `retry <id>`, `flush` |
| `scan` | Scan files for credentials and secrets |

## Package Map

### `cmd/kinoko/`

CLI entry point. Each file maps to a subcommand. Notable wiring:

- `serve.go` — bootstraps data dir, starts Soft Serve subprocess, installs hooks, opens SQLite, starts API server, optionally starts embedded scheduler
- `run.go` — loads config, connects to serve via HTTP, opens local queue DB, builds extraction pipeline, starts worker pool and scheduler
- `workers_run.go` — `buildClientPipeline()` wires Stage 1→2→3 with serverclient adapters for embedding and querying
- `serve_embedding.go` / `serve_no_embedding.go` — build-tag-gated ONNX engine setup
- `serve_scheduler.go` — embedded decay scheduler for serve mode

### `internal/model/`

Domain types and interfaces. No business logic. Key types:

- `SkillRecord` — database row for a skill with 7-dimension `QualityScores`, decay score, injection stats
- `SessionRecord` — metadata for an agent session (duration, tool calls, error rate, extraction status)
- `ExtractionResult` — pipeline output with `Stage1Result`, `Stage2Result`, `Stage3Result`
- `SkillCategory` — `foundational`, `tactical`, `contextual` (determines decay half-life)

Key interfaces:
- `Extractor` — `Extract(ctx, session, content) → ExtractionResult`
- `Embedder` — `Embed(ctx, text) → []float32`
- `SkillStore` — `Put`, `Get`, `Query`, `UpdateUsage`, `UpdateDecay`, `ListByDecay`
- `SkillIndexer` — `IndexSkill(ctx, skill, embedding)`
- `SkillQuerier` — `QueryNearest(ctx, embedding, libraryID)`
- `SkillCommitter` — `CommitSkill(ctx, libraryID, skill, body) → commitHash`

### `internal/extraction/`

The 3-stage extraction pipeline:

- **Stage 1** (`stage1.go`) — Metadata pre-filter. Checks duration bounds, minimum tool calls, max error rate. Pure function, no network calls.
- **Stage 2** (`stage2.go`) — Embedding novelty + LLM rubric scoring. Embeds the session, checks cosine distance against existing skills (novelty window), classifies patterns from a fixed taxonomy (20 patterns), scores on 7 quality dimensions via LLM.
- **Stage 3** (`stage3.go`) — LLM critic with circuit breaker. Runs a substitution test ("would a competent developer find this useful?"), checks hard-reject triggers, generates `SKILL.md`. Retries with exponential backoff. Protected by `circuitbreaker.Breaker`.
- **Pipeline** (`pipeline.go`) — Wires stages together. Implements `model.Extractor`. Handles credential scanning via `sanitize.Scanner`, human review sampling (stratified ~50/50 extracted/rejected), debug tracing.
- **`skillmd.go`** — Parses YAML front matter from LLM-generated `SKILL.md` files.
- **`logparser.go`** — Extracts session metadata (duration, tool calls, errors) from raw log content.
- **`sampling.go`** — Stratified sampling for human review with crypto/rand.
- **`stage3_prompt.go`** — Prompt template for the Stage 3 critic.

### `internal/storage/`

SQLite persistence layer. `SQLiteStore` is the central type:

- `skill_store.go` — CRUD for skills: `Put`, `Get`, `GetLatestByName`, `Query` (pattern + embedding + composite scoring), `UpdateUsage`, `UpdateDecay`, `ListByDecay`, `CountSkills`
- `session_store.go` — `InsertSession`, `UpdateSessionResult`, `GetSession`
- `injection_store.go` — `WriteInjectionEvent`, `UpdateInjectionOutcome`, `InsertReviewSample`
- `indexer.go` — `SQLiteIndexer` implements `model.SkillIndexer` for upsert-on-push
- `querier.go` — `NewSkillQuerier` wraps store as `model.SkillQuerier`
- `novelty.go` — `FindSimilar` does brute-force cosine similarity scan over embeddings
- `helpers.go` — byte⟷float32 conversion, cosine similarity math
- `store.go` — schema DDL (embedded via `//go:embed schema.sql`), constructor, migrations

Skill queries combine pattern overlap, cosine similarity, and historical success rate into a composite score.

### `internal/api/`

HTTP API server. Endpoints registered in `server.go`:

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/health` | Health check with skill count |
| `POST` | `/api/v1/discover` | Skill discovery (prompt, embedding, patterns, library_ids, top_k) |
| `POST` | `/api/v1/embed` | Text → embedding vector (requires Engine) |
| `POST` | `/api/v1/ingest` | Submit session log for extraction (enqueues) |
| `GET` | `/api/v1/skills/decay` | List skills ordered by decay score |
| `PATCH` | `/api/v1/skills/{id}/decay` | Update a skill's decay score |

Discover accepts prompt text (auto-embeds if no embedding provided), raw embeddings, or pattern lists. Bounded by a semaphore (max 10 concurrent). Request body limit: 10 MB for ingest.

### `internal/gitserver/`

Manages Soft Serve as a subprocess:

- `server.go` — `Server` type wraps Soft Serve lifecycle: start, stop, create repos, register session hooks
- `committer.go` — `GitCommitter` implements `model.SkillCommitter`. Creates repos via `ssh soft serve repo create`, writes `SKILL.md` to a temp workdir, pushes via SSH. Per-skill mutex prevents concurrent workdir conflicts.
- `hooks.go` — `InstallHooks` writes `pre-receive` (credential scan) and `post-receive` (index skill) shell scripts. Path-validated against shell injection.
- `keys.go` — Admin ed25519 keypair generation for Soft Serve access.
- `binary.go` — Locates the `soft` binary.

### `internal/embedding/`

Two embedding backends:

- **HTTP client** (`embedding.go`) — `Client` calls OpenAI-compatible `/v1/embeddings` API with retry + circuit breaker. Implements `Embedder` interface.
- **ONNX engine** (`onnx.go`, build tag `embedding`) — `ONNXEngine` runs BGE-small-en-v1.5 locally (384 dimensions). Implements `Engine` interface.
- `engine.go` — `Engine` interface (always available).
- `download.go` — Downloads ONNX model files.
- `mock.go` — Deterministic mock for testing.

### `internal/injection/`

Skill injection into agent sessions:

- `injector.go` — `Injector` interface and `DefaultInjector`. Classifies prompt via LLM, queries store by patterns + embedding, ranks by composite score, respects decay threshold.
- `ab.go` — `ABInjector` wraps `Injector` for A/B testing (treatment vs. control group). Control group: injection runs but skills not delivered.
- `client.go` — HTTP client for the discover API.
- `prompt.go` — `BuildInjectionPrompt` formats matched skills as markdown. Max 32 KB.

### `internal/decay/`

- `runner.go` — `Runner` applies half-life decay per category: foundational (365d), tactical (90d), contextual (180d). Skills below `DeprecationThreshold` (default 0.05) are deprecated. Recently-injected skills with positive outcomes get a rescue boost.

### `internal/worker/`

- `pool.go` — `Pool` manages goroutines that claim sessions from the queue, run extraction, track stats (processed/extracted/rejected/errors/failed).
- `scheduler.go` — `Scheduler` runs periodic tasks: decay cycles, stale session sweep, queue depth logging. Configurable intervals.
- `config.go` — `Config` and `SchedulerConfig` for pool/scheduler tuning.
- `interfaces.go` — `SessionQueue` interface (claim, complete, fail, enqueue, stats).

### `internal/queue/`

- `queue.go` — `Queue` implements `worker.SessionQueue`. Writes log files to disk, inserts queue entries in SQLite.
- `store.go` — `Store` wraps a dedicated SQLite database for queue state (separate from the main skill DB).
- `session.go` — Session metadata table operations.

### `internal/config/`

- `config.go` — YAML config loading with defaults. `~/.kinoko/config.yaml`. Sections: `server`, `storage`, `client`, `libraries`, `extraction`, `decay`, `embedding`, `llm`, `hooks`, `defaults`, `debug`. Tilde expansion, validation. Default ports: SSH 23231, API 23233.

### `internal/client/`

- `client.go` — End-user client library: `Discover`, `CloneSkill`, `SyncSkills`, `ReadSkill`. Used by `kinoko pull` and `kinoko match`.
- `config.go` — Local client config (`~/.kinoko/client.yaml`): server URL, SSH URL, cache dir.

### `internal/serverclient/`

HTTP client for `kinoko run` → `kinoko serve` communication:

- `client.go` — Base HTTP client with timeout and response size limits.
- `embed.go` — `HTTPEmbedder` implements `model.Embedder` via POST /api/v1/embed.
- `querier.go` — `HTTPQuerier` implements `model.SkillQuerier` via POST /api/v1/discover.
- `commit.go` — `HTTPCommitter` implements `model.SkillCommitter` via git push.
- `decay.go` — Decay operations via API.
- `discover.go` — Discovery client.
- `assertions.go` — Test helpers.

### `internal/llm/`

- `client.go` — `LLMClient` and `LLMClientV2` interfaces.
- `openai.go` — OpenAI-compatible implementation.
- `anthropic.go` — Anthropic Claude implementation.
- `factory.go` — `NewClient(provider, apiKey, model, baseURL)` factory.
- `errors.go` — Typed errors for rate limiting, auth, etc.

### `internal/llmutil/`

- `json.go` — `ExtractJSON[T]` generic function with 4-strategy cascade for parsing LLM responses (direct, ```json block, ``` block, substring extraction).

### `internal/circuitbreaker/`

- `breaker.go` — Thread-safe circuit breaker with exponential backoff. Three states: closed, open, half-open.

### `internal/sanitize/`

- `scanner.go` — Credential scanner with regex patterns, context-aware matching, confidence scoring. Used in pre-receive hooks and extraction pipeline.

### `internal/debug/`

- `trace.go` — Pipeline debug tracing. Creates per-run trace directories with stage results, raw data, gzipped logs.

### `internal/metrics/`

- `collector.go` — `Collector` queries the database for pipeline health: stage pass rates, extraction yield, injection utilization, A/B test significance, quality distributions, decay buckets.

### `pkg/skill/`

Public package for SKILL.md parsing and validation:

- `skill.go` — `Skill` struct with YAML front matter parsing, body section validation (When to Use, Solution, Why It Works, Pitfalls), quality scores.

## Data Flow

### Extraction Flow

```
Session log → logparser → Stage 1 (metadata filter) → Stage 2 (embed + novelty + rubric)
→ Stage 3 (LLM critic + SKILL.md generation) → credential scan → git commit → post-receive hook → SQLite index
```

### Injection Flow

```
Agent prompt → LLM classification (intent/domain/patterns) → discover API
→ pattern + embedding query → composite scoring → rank → build prompt → inject
```

### Discovery Flow

```
POST /api/v1/discover {prompt, embedding, patterns, library_ids, top_k}
→ auto-embed prompt if needed → SQLiteStore.Query → pattern overlap + cosine sim + historical rate
→ composite score → ranked results with clone URLs
```

## Storage

- **Git repos** (Soft Serve) — source of truth for skill content (`SKILL.md` files)
- **Main SQLite** (`~/.kinoko/kinoko.db`) — derived index: skills, sessions, injection events, review samples, embeddings, patterns
- **Queue SQLite** (`~/.kinoko/queue.db`) — local extraction work queue (separate DB)
- **Disk** — queue log files under `~/.kinoko/data/queue/`

## Configuration

Config file: `~/.kinoko/config.yaml`

Default ports:
- **23231** — Soft Serve SSH (git)
- **23233** — HTTP API (port = SSH port + 2; SSH port + 1 is reserved for Soft Serve HTTP)

Key environment variables:
- `KINOKO_LLM_API_KEY` / `OPENAI_API_KEY` — LLM for extraction and injection
- `KINOKO_EMBEDDING_API_KEY` / `OPENAI_API_KEY` — Embedding API
- `KINOKO_STORAGE_DSN` — SQLite database path override
- `KINOKO_API_URL` — API URL override
- `KINOKO_REPO`, `KINOKO_REV` — Set by post-receive hook for `kinoko index`

## Build Tags

- Default build: no ONNX, embedding via HTTP API only
- `embedding` build tag: enables `ONNXEngine` (requires `libonnxruntime.so` + `libtokenizers.a`)
