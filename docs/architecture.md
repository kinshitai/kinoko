# Internal Architecture

This document describes Mycelium's technical architecture: how knowledge flows from agent sessions into a shared library and back out into future sessions. Every claim here matches the implemented code.

---

## System Overview

Mycelium is a pipeline with four subsystems:

```
Agent Session → EXTRACTION → STORAGE → INJECTION → Agent Session
                                ↑                       |
                                └── DECAY ──────────────┘
                                    (ongoing)
```

A session produces work. Extraction decides whether that work contains reusable knowledge and, if so, distills it into a **skill**. Storage persists skills in a SQLite database and as `SKILL.md` files on disk. Injection matches relevant skills to new sessions and delivers them as context. Decay continuously demotes or removes skills that are no longer useful.

---

## Package Map

| Package | Path | Responsibility |
|---|---|---|
| `extraction` | `internal/extraction/` | 3-stage pipeline: types, pipeline orchestration, Stage 1/2/3 |
| `storage` | `internal/storage/` | SQLite-backed persistence for skills, sessions, embeddings, injection events, review samples |
| `injection` | `internal/injection/` | Prompt classification, skill ranking, A/B test decorator |
| `decay` | `internal/decay/` | Half-life degradation, rescue mechanics |
| `metrics` | `internal/metrics/` | Pipeline health: stage pass rates, yield, A/B z-test, decay distribution |
| `embedding` | `internal/embedding/` | Embedding computation (OpenAI API) |
| `config` | `internal/config/` | YAML config loading and validation |
| `gitserver` | `internal/gitserver/` | Soft Serve SSH git server |

CLI entry points live in `cmd/mycelium/`.

---

## Components

### 1. Extraction Pipeline

**Package:** `internal/extraction/`

The pipeline is implemented by the `Pipeline` struct, which satisfies the `Extractor` interface:

```go
type Extractor interface {
    Extract(ctx context.Context, session SessionRecord, content []byte) (*ExtractionResult, error)
}
```

`Pipeline` is constructed via `NewPipeline(PipelineConfig)` and wires three stages plus persistence:

```
SessionRecord + content
       │
       ▼
   Stage1Filter.Filter(session) → Stage1Result
       │ (pass/reject)
       ▼
   Stage2Scorer.Score(ctx, session, content) → Stage2Result
       │ (pass/reject)
       ▼
   Stage3Critic.Evaluate(ctx, session, content, stage2) → Stage3Result
       │ (pass/reject)
       ▼
   SkillWriter.Put(ctx, skill, body)  → persisted SkillRecord + SKILL.md
```

Sessions rejected at any stage return `StatusRejected`. Errors return `StatusError`. The pipeline is fail-safe: session persistence failures are non-fatal.

**Pipeline dependencies** (all injected via `PipelineConfig`):

| Interface | Purpose |
|---|---|
| `Stage1Filter` | Metadata pre-filtering (sync, no I/O) |
| `Stage2Scorer` | Embedding novelty + LLM rubric scoring |
| `Stage3Critic` | LLM critic for final verdict |
| `SkillWriter` | Persists skill record + SKILL.md body |
| `SessionWriter` | Persists/updates session records |
| `SkillEmbedder` | Computes embedding for extracted skills |
| `HumanReviewWriter` | Stratified sampling for human review |

---

#### Stage 1: Metadata Pre-Filters

**Interface:** `Stage1Filter` · **Implementation:** `stage1Filter` via `NewStage1Filter(cfg, logger)`

Operates on `SessionRecord` metadata only — no content analysis, no I/O. Rejects sessions that fail any threshold:

| Filter | Config Field | Default | Rationale |
|---|---|---|---|
| Duration range | `min_duration_minutes` / `max_duration_minutes` | 2–180 min | Too brief = no depth; too long = exploratory |
| Tool call count | `min_tool_calls` | ≥ 3 | Minimal interaction unlikely to produce knowledge |
| Error rate | `max_error_rate` | ≤ 0.70 | Mostly-failed sessions rarely contain solutions |
| Successful execution | — | ≥ 1 | No successful execution = no verified work |

All four must pass. `Stage1Result` includes per-criterion booleans and a human-readable `Reason` on rejection.

---

#### Stage 2: Embedding Novelty + Rubric Scoring

**Interface:** `Stage2Scorer` · **Implementation:** `stage2Scorer` via `NewStage2Scorer(embedder, querier, llm, cfg, logger)`

Runs two classifiers:

1. **Embedding novelty** — Embeds the session content, queries the skill store for the nearest neighbor (`SkillQuerier.QueryNearest`), computes distance = 1 − cosine similarity. Distance must fall within `[novelty_min_distance, novelty_max_distance]` (defaults: 0.15–0.95). A triangle function normalizes the score to [0, 1] with a 0.05 floor at boundaries.

2. **Structured rubric scoring** — Sends session content to an LLM (`LLMClient.Complete`) with a prompt requesting JSON scores across 7 dimensions (1–5 each). The response is parsed with multi-strategy JSON extraction (raw → fenced → brace-matching). Scores are validated to [1,5]. Category is validated against `{foundational, tactical, contextual}` (default: `tactical`). Patterns are validated against the 20-entry `Taxonomy` (Appendix B).

**Pass criteria:** `QualityScores.MinimumViable()` — Problem Specificity ≥ 3, Solution Completeness ≥ 3, Technical Accuracy ≥ 3.

**Output:** `Stage2Result` with novelty score, rubric scores, classified category, and validated patterns.

**Key interfaces consumed:**

| Interface | Package | Purpose |
|---|---|---|
| `embedding.Embedder` | `internal/embedding` | `Embed(ctx, text) → []float32` |
| `SkillQuerier` | `internal/extraction` | `QueryNearest(ctx, embedding, libraryID) → *SkillQueryResult` |
| `LLMClient` | `internal/extraction` | `Complete(ctx, prompt) → string` |

---

#### Stage 3: LLM Critic

**Interface:** `Stage3Critic` · **Implementation:** `stage3Critic` via `NewStage3Critic(llm, cfg, logger)`

The most expensive stage. Features:

- **Content truncation** at 100 KB with UTF-8-safe boundary handling
- **Delimiter injection** using random nonces to prevent prompt injection
- **Retry with exponential backoff** (1s, 2s, 4s; max 3 retries, extended to 5 for rate limits)
- **Timeout escalation** (30s default → 60s on retry after timeout)
- **Circuit breaker** — opens after 5 consecutive failures (5 min cooldown, doubling on re-failure)
- **Contradiction detection** — overrides verdict when scores and verdict conflict (e.g., "extract" with avg score < 1.5 → reject)
- **Parse error tolerance** — unparseable LLM responses become rejections, not errors

Supports `LLMClientV2` for token usage tracking and explicit timeout control; falls back to basic `LLMClient` with context deadlines.

**Output:** `Stage3Result` with verdict (extract/reject), refined quality scores, critic confidence, and flags for reusable pattern, explicit reasoning, and best-practice contradiction.

---

#### Human Review Sampling

The pipeline implements stratified sampling per §3.4. After every pipeline run (regardless of outcome), `maybeSample` probabilistically selects results for human review:

- **Stratified 50/50 balance** between extracted and rejected pools
- Underrepresented pool: always sampled
- Overrepresented pool: skipped until balance is restored
- Equal counts: sampled at the configured `sample_rate` (default 1%)
- Uses crypto/rand for unbiased sampling

---

#### Skill Generation

Extracted skills receive:
- A UUID v7 ID
- A kebab-case name derived from classified patterns (e.g., `FIX/Backend/DatabaseConnection` → `fix-backend-database-connection`)
- Version 1
- `DecayScore` initialized to 1.0 (fully active)
- An embedding vector (if `SkillEmbedder` is provided)
- A `SKILL.md` file with YAML front matter and structured body (When to Use, Solution, Why It Works, Pitfalls)

---

### 2. Skill Storage

**Package:** `internal/storage/`

`SQLiteStore` implements both `SkillStore` and `SessionStore`. Uses WAL mode, 5s busy timeout, and foreign keys.

**Database schema** (managed via embedded `schema.sql`):

| Table | Purpose |
|---|---|
| `skills` | Skill metadata, quality scores, decay score, usage stats |
| `skill_patterns` | Many-to-many: skill ↔ pattern tags |
| `skill_embeddings` | Binary embedding blobs with model name |
| `sessions` | Session metadata, extraction status, rejection info |
| `injection_events` | Per-skill-per-session injection records with A/B group |
| `human_review_samples` | Sampled extraction results for human review |

**Key operations:**

| Method | Description |
|---|---|
| `Put(ctx, skill, body)` | Insert skill + patterns + embedding in a transaction; write SKILL.md to disk post-commit |
| `Get(ctx, id)` | Load skill with patterns and embedding |
| `Query(ctx, SkillQuery)` | Multi-signal ranked search (pattern overlap + cosine similarity + historical rate) |
| `UpdateUsage(ctx, id, outcome)` | Increment injection count, recompute success correlation |
| `UpdateDecay(ctx, id, decayScore)` | Set decay score |
| `ListByDecay(ctx, libraryID, limit)` | Skills ordered by decay score ascending |

**Query ranking formula:**

```
CompositeScore = 0.5 × PatternOverlap + 0.3 × CosineSim + 0.2 × HistoricalRate
```

Bulk-loads patterns and embeddings via multi-ID queries (no N+1).

**Skill data model** (as stored):

```go
type SkillRecord struct {
    ID, Name, ParentID, LibraryID string
    Version                        int
    Category                       SkillCategory  // "foundational" | "tactical" | "contextual"
    Patterns                       []string
    Quality                        QualityScores  // 7 dimensions + composite + confidence
    Embedding                      []float32
    InjectionCount                 int
    LastInjectedAt                 time.Time
    SuccessCorrelation             float64
    DecayScore                     float64
    SourceSessionID                string
    ExtractedBy                    string
    FilePath                       string
    CreatedAt, UpdatedAt           time.Time
}
```

---

### 3. Injection Pipeline

**Package:** `internal/injection/`

**Interface:** `Injector` · **Implementation:** `injector` via `New(embedder, store, llm, eventWriter, logger)`

Steps:

1. **Prompt classification** — LLM classifies the prompt into intent (BUILD/FIX/OPTIMIZE/INTEGRATE/CONFIGURE/LEARN), domain (validated via `extraction.ValidateDomain`), and 1–3 patterns (validated against `extraction.Taxonomy`).

2. **Embedding** — Computes prompt embedding. Falls back to **degraded mode** (pattern-only) if embedder is nil or fails.

3. **Skill query** — Queries `SkillStore.Query` with patterns, embedding, library IDs, min decay of 0.05, candidate limit of 50.

4. **Re-ranking** — In degraded mode, re-ranks as `0.7 × PatternOverlap + 0.3 × HistoricalRate`. In normal mode, uses the store's composite score.

5. **Limit** — Caps to `MaxSkills` (default: 3).

6. **Event logging** — Writes `InjectionEventRecord` per skill for the feedback loop.

**Output:** `InjectionResponse` with `[]InjectedSkill` and `PromptClassification`.

#### A/B Testing

**`ABInjector`** wraps any `Injector` with A/B test logic:

- Randomly assigns sessions to **treatment** (skills delivered) or **control** (skills withheld) based on `ControlRatio` (default: 10%)
- Always runs the real injection pipeline to get candidates
- Control group: events logged with `delivered=false`, response returns empty skills
- Treatment group: events logged with `delivered=true`, skills passed through

The `ABInjector` writes events itself — the inner injector should be constructed without an `eventWriter` to avoid double-writing.

---

### 4. Decay System

**Package:** `internal/decay/`

**`Runner`** performs decay cycles via `RunCycle(ctx, libraryID)`.

**Half-life formula:**

```
newDecay = oldDecay × 0.5^(daysSince / halfLifeDays)
```

Where `daysSince` is measured from `LastInjectedAt` (or `UpdatedAt` if never injected).

**Half-life defaults by category:**

| Category | Half-Life |
|---|---|
| Foundational | 365 days |
| Tactical | 90 days |
| Contextual | 180 days |

**Rescue mechanic:** Skills injected within `RescueWindowDays` (default: 30) with positive `SuccessCorrelation` receive a `RescueBoost` (default: 0.3) added to their decay score, capped at 1.0.

**Deprecation:** Skills that decay below `DeprecationThreshold` (default: 0.05) are set to 0.0 (dead). Dead skills are filtered from injection queries by the `MinDecay` floor.

**Output:** `DecayCycleResult` with counts: Processed, Demoted, Deprecated, Rescued.

---

### 5. Metrics

**Package:** `internal/metrics/`

`Collector` computes pipeline health metrics from the database:

| Metric Group | Details |
|---|---|
| Sessions | Total, extracted, rejected, errored, extraction yield |
| Stage pass rates | Per-stage total/passed/rate |
| Human review | Total reviewed, agree count, extraction precision |
| Injection | Events, sessions with injection, injection rate, skill utilization |
| Quality | Average composite score, average confidence |
| Decay | Bucketed distribution (dead / low / medium / high / fresh) |
| A/B test | Per-group success rates, two-proportion z-test, p-value, significance |

A/B significance requires `MinSampleSize` (default: 100) sessions per group.

---

## Dimensional Scoring

Skills are evaluated on seven dimensions, each scored 1–5:

| Dimension | Question |
|---|---|
| Problem Specificity | Does this solve a clearly defined problem? |
| Solution Completeness | Can someone follow this to solve the problem? |
| Context Portability | How broadly applicable is this beyond its original context? |
| Reasoning Transparency | Does this explain *why*, not just *what*? |
| Technical Accuracy | Are the technical details correct and current? |
| Verification Evidence | Is there proof this solution works? |
| Innovation Level | How novel is the approach? |

**Minimum viable skill:** `MinimumViable()` — Problem Specificity ≥ 3, Solution Completeness ≥ 3, Technical Accuracy ≥ 3.

**High-value skill:** `HighValue()` — average across all 7 dimensions ≥ 4.0.

**Injection priority:** `InjectionPriority()` = `ContextPortability × 0.6 + VerificationEvidence × 0.4`.

**Composite score** (weighted sum for ranking):

```
0.15 × ProblemSpecificity + 0.20 × SolutionCompleteness + 0.15 × ContextPortability
+ 0.10 × ReasoningTransparency + 0.20 × TechnicalAccuracy + 0.10 × VerificationEvidence
+ 0.10 × InnovationLevel
```

---

## Problem Pattern Taxonomy

Skills and prompts are classified into a three-tier taxonomy. The canonical list is defined in `extraction.Taxonomy` (20 patterns):

**Tier 1 — Intent:**
`BUILD` · `FIX` · `OPTIMIZE` · `INTEGRATE` · `CONFIGURE` · `LEARN`

**Tier 2 — Domain:**
`Frontend` · `Backend` · `DevOps` · `Data` · `Security` · `Performance`

**Tier 3 — Specific patterns:**

```
BUILD/Frontend/ComponentDesign     FIX/Frontend/RenderingBug
BUILD/Frontend/StateManagement     FIX/Backend/DatabaseConnection
BUILD/Backend/APIDesign            FIX/Backend/AuthFlow
BUILD/Backend/DataModeling         FIX/DevOps/DeploymentFailure
BUILD/DevOps/CIPipeline            FIX/Performance/MemoryLeak
BUILD/DevOps/ContainerSetup        FIX/Performance/SlowQuery
OPTIMIZE/Performance/Caching       INTEGRATE/Backend/ThirdPartyAPI
OPTIMIZE/Performance/BundleSize    INTEGRATE/DevOps/CloudService
OPTIMIZE/Backend/QueryOptimization CONFIGURE/DevOps/InfraAsCode
CONFIGURE/Security/AccessControl   LEARN/Data/DataPipeline
```

The taxonomy is a fixed, manually curated list validated at both extraction (Stage 2) and injection (prompt classification).

---

## Component Boundaries

```
┌──────────────────┐     ┌──────────────────┐     ┌──────────────────┐
│    Extraction     │────▶│     Storage      │◀────│      Decay       │
│    Pipeline       │     │  (SQLite + disk) │     │     Runner       │
│ (extraction pkg)  │     │  (storage pkg)   │     │   (decay pkg)    │
└──────────────────┘     └────────┬─────────┘     └──────────────────┘
                                  │
                         ┌────────▼─────────┐
                         │    Injection      │
                         │    Pipeline       │
                         │ (injection pkg)   │
                         └──────────────────┘
                                  │
                         ┌────────▼─────────┐
                         │     Metrics       │
                         │   Collector       │
                         │  (metrics pkg)    │
                         └──────────────────┘
```

**Extraction → Storage:** Pipeline calls `SkillWriter.Put()` to persist skills and `SessionWriter` to track session state.

**Storage → Injection:** Injection calls `SkillStore.Query()` for ranked candidates.

**Decay → Storage:** Decay reads via `SkillReader.ListByDecay()` and writes via `SkillWriter.UpdateDecay()`.

**Injection → Storage (feedback):** Injection writes `InjectionEventRecord` rows. Storage recomputes `SuccessCorrelation` on usage updates.

**Metrics → Storage:** Collector reads directly from the `*sql.DB` with aggregate queries.

Each component communicates through defined Go interfaces. Extraction does not know about injection. Injection does not trigger extraction. Decay operates independently on a schedule or via CLI.

---

## Server Integration

`cmd/mycelium/serve.go` wires both pipelines into session lifecycle hooks:

- **`OnSessionStart`** — runs the injection pipeline (with optional A/B testing)
- **`OnSessionEnd`** — runs the extraction pipeline on the session log

Hooks are registered on the git server. If API keys are missing, hooks degrade gracefully (injection returns empty, extraction returns rejected).
