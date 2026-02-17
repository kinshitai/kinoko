# Kinoko Architecture

> Last updated: 2026-02-17 — reflects the client/server split (T1–T8 merged on `main`).

---

## 1. System Overview

Kinoko is a knowledge extraction and injection system for AI coding agents. It watches agent sessions, extracts reusable **skills** from successful work, persists them in a git-backed library, and injects relevant skills into future sessions.

The system is split into two processes:

```
kinoko serve (infrastructure server)        kinoko run (local agent daemon)
──────────────────────────────────          ──────────────────────────────
Soft Serve SSH git server                   Worker pool + scheduler
HTTP API (search, embed, sessions, etc.)    Extraction pipeline (3-stage LLM)
Post-receive hook → skill indexing          Injection pipeline
Decay scheduler                             Local queue DB (SQLite)
Index DB (SQLite: skills, embeddings,       Talks to serve via HTTP + git push
  sessions, injection events, reviews)      Never imports internal/storage
```

**The fundamental rule:** `kinoko run` never imports `internal/storage`. All communication between client and server goes through HTTP endpoints (`internal/serverclient`) and git push (SSH to Soft Serve). This holds even when both run on localhost.

### Data Flow (High Level)

```
                          HTTP + git push
  ┌──────────────┐       ────────────────►       ┌──────────────┐
  │  kinoko run   │                               │ kinoko serve │
  │               │  POST /api/v1/embed           │              │
  │  Extraction ──┼──POST /api/v1/sessions───────►│  Index DB    │
  │  Pipeline     │  POST /api/v1/review-samples  │  (SQLite)    │
  │               │  git push (skill files)       │              │
  │  Injection ───┼──POST /api/v1/search─────────►│  Search API  │
  │               │  POST /api/v1/embed           │  Embed API   │
  │               │                               │              │
  │  Queue DB ────┤  (local SQLite)               │  Decay ──────┤
  │  (queue.db)   │                               │  Scheduler   │
  └──────────────┘                               └──────────────┘
```

---

## 2. Package Map

Every package in `internal/` classified by which side uses it.

### Server-only

| Package | Path | Responsibility |
|---------|------|----------------|
| `storage` | `internal/storage/` | SQLite index DB: skills, embeddings, sessions, injection events, review samples. `SQLiteStore` implements `model.SkillStore`, `decay.SkillReader`, `decay.SkillWriter`. |
| `api` | `internal/api/` | HTTP API server. Endpoints: `/health`, `/discover`, `/match`, `/novelty`, `/embed`, `/search`, `/sessions`, `/review-samples`, `/injection-events`, `/skills/{id}/decay`, `/skills/{id}/usage`. |
| `gitserver` | `internal/gitserver/` | Soft Serve SSH git server wrapper. Post-receive hooks trigger `kinoko index` for skill indexing. |
| `embedding` | `internal/embedding/` | Embedding engines (OpenAI API + optional ONNX). Satisfies `model.Embedder`. |

### Client-only

| Package | Path | Responsibility |
|---------|------|----------------|
| `queue` | `internal/queue/` | Client-side SQLite queue DB (`queue_entries` + `session_metadata` tables). Own schema, own DB file. `queue.Queue` implements `worker.SessionQueue`. |
| `serverclient` | `internal/serverclient/` | HTTP client adapters that implement server-side interfaces over HTTP. See §6 for full file map. |
| `llm` | `internal/llm/` | LLM client (OpenAI-compatible). Used by extraction pipeline. |
| `llmutil` | `internal/llmutil/` | JSON parsing helpers for LLM responses. |
| `sanitize` | `internal/sanitize/` | Credential scanner for session content. |
| `debug` | `internal/debug/` | Pipeline debug tracing (writes trace files to disk). |
| `client` | `internal/client/` | Config client utility (not the server API client). |

### Shared (used by both sides)

| Package | Path | Responsibility |
|---------|------|----------------|
| `model` | `internal/model/` | Pure domain types and interfaces: `SkillStore`, `SkillQuery`, `ScoredSkill`, `Embedder`, `Extractor`, `SessionRecord`, `InjectionEventRecord`, `ErrNotFound`, `ErrDuplicate`. Zero dependencies. |
| `config` | `internal/config/` | YAML config loading. Shared struct with `Server`, `Client`, `Storage`, `Extraction`, `Decay`, `Embedding`, `LLM`, `Libraries`, `Debug` sections. |
| `extraction` | `internal/extraction/` | 3-stage extraction pipeline. Depends on `model` interfaces, not `storage`. |
| `injection` | `internal/injection/` | Injection pipeline: prompt classification → skill ranking → delivery. Depends on `model` interfaces. |
| `decay` | `internal/decay/` | Decay runner logic. Defines `SkillReader`/`SkillWriter` interfaces satisfied by `storage.SQLiteStore` (server) or `serverclient` adapters (client, if needed). |
| `worker` | `internal/worker/` | Worker pool + scheduler. ⚠️ Has residual `queue.go` importing `storage` (dead code — `run` uses `internal/queue/` instead). |
| `metrics` | `internal/metrics/` | In-memory pipeline health metrics collector. |
| `circuitbreaker` | `internal/circuitbreaker/` | Generic circuit breaker utility. |

---

## 3. `cmd/kinoko/` File Map

### Server-side (import `storage`, run inside `kinoko serve`)

| File | Purpose |
|------|---------|
| `serve.go` | `kinoko serve` command. Opens `storage.SQLiteStore`, starts git server + API + hooks. Registers noop session hooks (extraction lives on client). |
| `serve_scheduler.go` | `decayScheduler` — runs `decay.Runner.RunCycle()` on a cron (default 6h). Imports `storage` for direct DB access. |
| `serve_embedding.go` | ONNX embedding engine init (build tag `embedding`). |
| `serve_no_embedding.go` | Stub when built without ONNX. |
| `index.go` | `kinoko index` — post-receive hook entry point. Parses pushed skill files, indexes into `storage`. |
| `rebuild.go` | `kinoko rebuild` — rebuilds the entire index from git repos. Server admin command. |
| `importcmd.go` | `kinoko import` — imports skills from external sources into index DB. |
| `stats.go` | `kinoko stats` — reads aggregate stats from index DB. |
| `decay.go` | `kinoko decay` — manual decay trigger. Also defines `decayConfigFromYAML()` used by `serve_scheduler.go`. |

### Client-side (no `storage` import, run inside `kinoko run`)

| File | Purpose |
|------|---------|
| `run.go` | `kinoko run` command. Opens `queue.Store`, creates `serverclient.Client`, calls `startClientWorkerSystem()`. Imports: `config`, `queue`, `serverclient`. |
| `workers_run.go` | `buildClientPipeline()` — wires extraction stages with `serverclient` adapters. `startClientWorkerSystem()` — creates queue, pool, scheduler (decay=nil). Also defines `libraryIDs()`. |
| `extract.go` | `kinoko extract` — one-shot extraction via `queue` + `serverclient`. |
| `queuecmd.go` | `kinoko queue` — inspect/manage local queue DB. |
| `match.go` | `kinoko match` — skill search via HTTP. |
| `pull.go` | `kinoko pull` — pull skills from server. |
| `scan.go` | `kinoko scan` — scan session logs. |
| `ingest.go` | `kinoko ingest` — enqueue sessions into local queue. |

### Shared (CLI scaffolding)

| File | Purpose |
|------|---------|
| `main.go` | Entry point. |
| `root.go` | Root cobra command, global flags. |
| `init.go` | `kinoko init` — library initialization. |

---

## 4. Component Deep-Dives

### 4.1 Extraction Pipeline

**Package:** `internal/extraction/`  
**Entry:** `Pipeline` struct implementing `model.Extractor`  
**Constructor:** `NewPipeline(PipelineConfig)`

Three-stage LLM pipeline that evaluates whether an agent session contains reusable knowledge:

```
SessionRecord + content
       │
       ▼
   Stage1Filter.Filter(session) → Stage1Result
       │ (metadata pre-filter: duration, tool calls, error rate, successful exec)
       ▼
   Stage2Scorer.Score(ctx, session, content) → Stage2Result
       │ (embedding novelty + LLM rubric scoring across 7 dimensions)
       ▼
   Stage3Critic.Evaluate(ctx, session, content, stage2) → Stage3Result
       │ (LLM critic: final verdict, circuit breaker, contradiction detection)
       ▼
   Committer.Commit(skill)  → git push to Soft Serve
   Sessions.WriteSession()  → POST /api/v1/sessions
   Reviewer.Submit()        → POST /api/v1/review-samples (stratified sampling)
```

**Stage 1** (`NewStage1Filter`): Pure metadata checks. No I/O, no LLM. Rejects sessions below thresholds (min 2 min, max 180 min, ≥3 tool calls, ≤70% error rate, ≥1 successful exec).

**Stage 2** (`NewStage2Scorer`): Two classifiers — embedding novelty (cosine distance from nearest existing skill) and structured rubric scoring (7 dimensions, 1–5 each via LLM). Pass requires Problem Specificity ≥ 3, Solution Completeness ≥ 3, Technical Accuracy ≥ 3.

**Stage 3** (`NewStage3Critic`): Most expensive. Features: content truncation at 100KB, random-nonce delimiters (anti-injection), retry with exponential backoff, circuit breaker (5 consecutive failures → 5 min cooldown), contradiction detection.

**In client mode**, all external dependencies are `serverclient` adapters:
- `serverclient.NewHTTPEmbedder` → `POST /api/v1/embed`
- `serverclient.NewHTTPQuerier` → `POST /api/v1/novelty`
- `serverclient.NewHTTPSessionWriter` → `POST /api/v1/sessions`
- `serverclient.NewHTTPReviewer` → `POST /api/v1/review-samples`
- `serverclient.NewGitPushCommitter` → `git push` via SSH

### 4.2 Storage / Index DB

**Package:** `internal/storage/`  
**Type:** `SQLiteStore` (WAL mode, 5s busy timeout, foreign keys)

Server-only. The canonical data store for skills, embeddings, sessions, injection events, and review samples.

**Schema (embedded `schema.sql`):**

| Table | Purpose |
|-------|---------|
| `skills` | Skill metadata, quality scores (7 dimensions), decay score, usage stats |
| `skill_patterns` | Many-to-many: skill ↔ taxonomy pattern tags |
| `skill_embeddings` | Binary embedding blobs with model name |
| `sessions` | Session metadata, extraction status, rejection reason |
| `injection_events` | Per-skill-per-session injection records (A/B group, delivered flag) |
| `human_review_samples` | Stratified extraction samples for human review |

**Query ranking formula:**
```
CompositeScore = 0.5 × PatternOverlap + 0.3 × CosineSim + 0.2 × HistoricalRate
```

### 4.3 Queue (Client-Side)

**Package:** `internal/queue/`  
**Files:** `store.go` (constructor, schema DDL), `queue.go` (queue operations), `session.go` (session metadata), `schema.sql`

Separate SQLite DB file (default `~/.kinoko/queue.db`). Two tables:
- `queue_entries` — jobs to be processed by the worker pool
- `session_metadata` — local copy of session info needed for extraction

`queue.Queue` wraps `queue.Store` and implements `worker.SessionQueue`. No dependency on `internal/storage`.

### 4.4 Server Client (`serverclient`)

**Package:** `internal/serverclient/`

HTTP client adapters that let `kinoko run` talk to `kinoko serve`. Each file implements a server-side interface over HTTP:

| File | Type | Implements | Server Endpoint |
|------|------|-----------|-----------------|
| `client.go` | `Client` | Base HTTP client (shared helpers, base URL) | — |
| `embed.go` | `HTTPEmbedder` | `model.Embedder` | `POST /api/v1/embed` |
| `session.go` | `HTTPSessionWriter` | `extraction.SessionWriter` | `POST /api/v1/sessions`, `PUT /api/v1/sessions/{id}` |
| `review.go` | `HTTPReviewer` | `extraction.HumanReviewWriter` | `POST /api/v1/review-samples` |
| `querier.go` | `HTTPQuerier` | `model.SkillQuerier` | `POST /api/v1/novelty` |
| `search.go` | `HTTPSearch` | `model.SkillStore` (read-only) | `POST /api/v1/search` |
| `commit.go` | `GitPushCommitter` | `model.SkillCommitter` | `git push` via SSH to Soft Serve |
| `injection.go` | `HTTPInjectionWriter` | injection event writer | `POST /api/v1/injection-events`, `PUT /api/v1/injection-events/{session_id}/outcome` |
| `decay.go` | `HTTPDecay` | `decay.SkillReader` + `decay.SkillWriter` | `GET /api/v1/skills/decay`, `PATCH /api/v1/skills/{id}/decay` |

### 4.5 Injection Pipeline

**Package:** `internal/injection/`  
**Entry:** `injector` via `New(embedder, store, llm, eventWriter, logger)`

Steps:
1. **Prompt classification** — LLM classifies into intent/domain/patterns using `extraction.Taxonomy`
2. **Embedding** — compute prompt embedding (falls back to degraded pattern-only mode)
3. **Skill query** — `model.SkillStore.Query()` with patterns + embedding, min decay 0.05, limit 50
4. **Re-ranking** — composite score or degraded mode (`0.7 × PatternOverlap + 0.3 × HistoricalRate`)
5. **Limit** — cap to `MaxSkills` (default 3)
6. **Event logging** — write `InjectionEventRecord` for feedback loop

**A/B testing:** `ABInjector` wraps any `Injector`. Randomly assigns sessions to treatment/control (default 10% control). Control group: pipeline runs but skills withheld; events logged with `delivered=false`.

### 4.6 Decay System

**Package:** `internal/decay/`  
**Runner:** `decay.Runner` via `NewRunner(reader, writer, cfg, logger)`

Runs server-side on a cron (default every 6 hours) in `serve_scheduler.go`.

**Half-life formula:**
```
newDecay = oldDecay × 0.5^(daysSince / halfLifeDays)
```

| Category | Half-Life |
|----------|-----------|
| Foundational | 365 days |
| Tactical | 90 days |
| Contextual | 180 days |

**Rescue:** Skills injected within 30 days with positive `SuccessCorrelation` get +0.3 boost (capped at 1.0).  
**Deprecation:** Skills below 0.05 decay → set to 0.0 (dead), filtered from injection.

### 4.7 Git Server

**Package:** `internal/gitserver/`

Soft Serve SSH git server. Skills are stored as `SKILL.md` files in git repos (one repo per library). Post-receive hooks call `kinoko index` to parse pushed files and update the index DB.

`serve.go` calls `gitserver.NewServer(cfg)`, `gitserver.InstallHooks()`, and `server.Start()`.

### 4.8 HTTP API

**Package:** `internal/api/`  
**Type:** `api.New(Config)` → `*Server`

Endpoints (all under `/api/v1/`):

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/health` | Health check |
| POST | `/embed` | Compute embedding vector |
| POST | `/discover` | Discover skills by embedding similarity |
| POST | `/match` | Match skills by pattern |
| POST | `/novelty` | Check embedding novelty (distance from nearest skill) |
| POST | `/search` | Unified search (wraps `store.Query`) |
| POST | `/sessions` | Record session + extraction result |
| PUT | `/sessions/{id}` | Update session extraction result |
| POST | `/review-samples` | Submit human review sample |
| POST | `/injection-events` | Record injection event |
| PUT | `/injection-events/{session_id}/outcome` | Update injection outcome |
| GET | `/skills/decay` | List skills by decay score |
| PATCH | `/skills/{id}/decay` | Update skill decay score |
| POST | `/skills/{id}/usage` | Update usage count |
| POST | `/ingest` | Enqueue session for processing (returns error — extraction lives on client) |

---

## 5. Data Flow Diagrams

### Extraction Flow (client → server)

```
Agent session ends
       │
       ▼
kinoko ingest (or hook)
       │
       ▼
queue.Store.Enqueue()           ← local queue.db
       │
       ▼
worker.Pool claims job
       │
       ▼
queue.GetSessionMetadata()      ← local queue.db
       │
       ▼
extraction.Pipeline.Extract()
  ├── Stage 1: metadata filter   (local, no I/O)
  ├── Stage 2: embed + score     → POST /api/v1/embed, POST /api/v1/novelty
  └── Stage 3: LLM critic        (local LLM call)
       │
       ▼ (if extracted)
serverclient.GitPushCommitter   → git push SKILL.md via SSH
serverclient.HTTPSessionWriter  → POST /api/v1/sessions
serverclient.HTTPReviewer       → POST /api/v1/review-samples (sampled)
       │
       ▼ (on server)
post-receive hook → kinoko index → storage.SQLiteStore.Put()
```

### Injection Flow (server → client)

```
Agent session starts
       │
       ▼
injection.Injector.Inject(prompt)
  ├── LLM prompt classification   (local LLM call)
  ├── Embed prompt                → POST /api/v1/embed
  ├── Query skills                → POST /api/v1/search
  ├── Re-rank + limit (≤3)
  └── Log events                  → POST /api/v1/injection-events
       │
       ▼
Skills delivered to agent session context
```

---

## 6. Client ↔ Server Communication

### HTTP Endpoints

All HTTP communication goes through `serverclient.Client` (base URL from `cfg.ServerURL()`). See §4.4 for the full adapter map.

### Git Push

`serverclient.GitPushCommitter` (in `commit.go`) handles skill persistence:
1. `git clone` the library repo from Soft Serve via SSH
2. Write `SKILL.md` file with YAML front matter + structured body
3. `git add` + `git commit` + `git push` via SSH

The server's post-receive hook (`gitserver.InstallHooks`) then calls `kinoko index` to parse the pushed file and update the index DB.

### Authentication

- HTTP: currently unauthenticated (localhost assumption)
- Git SSH: SSH key registered with Soft Serve. Generated by `kinoko serve` bootstrap (`kinoko_admin_ed25519`).

---

## 7. Configuration

Config loaded from `~/.kinoko/config.yaml` via `internal/config/`.

### Server-relevant sections

```yaml
server:
  host: "localhost"
  port: 23231          # SSH (Soft Serve)
  data_dir: "~/.kinoko/data"
  api_port: 23232      # HTTP API

storage:
  driver: "sqlite"
  dsn: "~/.kinoko/data/index.db"

decay:
  interval_hours: 6
  # half-life overrides per category

embedding:
  model: "text-embedding-3-small"
  # ONNX settings for server-side engine
```

### Client-relevant sections

```yaml
client:
  queue_dsn: "~/.kinoko/queue.db"   # local queue DB

llm:
  provider: "openai"
  model: "gpt-4o-mini"
  base_url: ""                      # optional override

extraction:
  min_duration_minutes: 2
  max_duration_minutes: 180
  min_tool_calls: 3
  max_error_rate: 0.70
  sample_rate: 0.01

debug:
  enabled: false
  dir: "~/.kinoko/debug"
```

### Shared sections

```yaml
libraries:
  - name: "default"
    path: "~/.kinoko/libraries/default"
```

---

## 8. Dimensional Scoring

Skills are evaluated on seven dimensions (1–5 each):

| Dimension | Weight |
|-----------|--------|
| Problem Specificity | 0.15 |
| Solution Completeness | 0.20 |
| Context Portability | 0.15 |
| Reasoning Transparency | 0.10 |
| Technical Accuracy | 0.20 |
| Verification Evidence | 0.10 |
| Innovation Level | 0.10 |

**Minimum viable:** Problem Specificity ≥ 3, Solution Completeness ≥ 3, Technical Accuracy ≥ 3.

---

## 9. Problem Pattern Taxonomy

Three-tier classification used at extraction (Stage 2) and injection (prompt classification):

**Tier 1 — Intent:** `BUILD` · `FIX` · `OPTIMIZE` · `INTEGRATE` · `CONFIGURE` · `LEARN`  
**Tier 2 — Domain:** `Frontend` · `Backend` · `DevOps` · `Data` · `Security` · `Performance`  
**Tier 3 — 20 specific patterns** defined in `extraction.Taxonomy` (e.g., `BUILD/Backend/APIDesign`, `FIX/Performance/MemoryLeak`).

---

## 10. Known Architectural Caveats

1. **`internal/worker` straddles the boundary.** `worker/queue.go` imports `storage` (dead code from pre-split). The client path uses `internal/queue/` instead. Cleanup ticket pending.
2. **Single binary.** Both `serve` and `run` compile into one `kinoko` binary. Compile-time import separation is impossible; the boundary is enforced at runtime (convention + lint rules).
3. **Ingest wiring gap.** No automatic mechanism for session logs to enter the client queue. Currently requires `kinoko ingest` CLI or manual enqueue.
4. **`serverclient` test coverage at 54%.** Critical HTTP client layer needs more coverage, especially error paths.
