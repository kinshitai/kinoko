# Extraction System Engineering Spec

**Author:** Luka Jensen  
**Reviewed by:** Hal (CTO)  
**Date:** 2026-02-15  
**Status:** Reviewed — ready for implementation  
**Module:** `github.com/kinoko-dev/kinoko`

---

## 1. Data Model

### 1.1 Skill

The existing `pkg/skill/skill.go` defines a v1 Skill as a Markdown file with YAML front matter. The extraction system extends this with quality metadata, pattern classification, usage tracking, and embedding storage. The extended model lives in a SQLite database alongside the SKILL.md files on disk.

#### Extended Skill Record (Go)

```go
package extraction

import "time"

// SkillRecord is the database representation of an extracted skill.
// The SKILL.md file on disk remains the source of truth for content;
// this record holds extraction metadata, quality scores, and usage stats.
type SkillRecord struct {
    // Identity
    ID        string    `db:"id"`         // UUIDv7, sortable by creation time
    Name      string    `db:"name"`       // kebab-case, unique within library
    Version   int       `db:"version"`    // monotonically increasing, append-only
    ParentID  string    `db:"parent_id"`  // ID of previous version, empty for v1
    LibraryID string    `db:"library_id"` // references LibraryConfig.Name

    // Classification
    Category  SkillCategory `db:"category"`  // foundational | tactical | contextual
    Patterns  []string      `db:"-"`         // stored in skill_patterns join table

    // Quality (from extraction pipeline)
    Quality   QualityScores `db:"-"` // stored as individual columns, see below

    // Embedding
    Embedding []float32 `db:"-"` // stored in skill_embeddings table, 1536-dim

    // Usage tracking
    InjectionCount    int       `db:"injection_count"`
    LastInjectedAt    time.Time `db:"last_injected_at"`
    SuccessCorrelation float64  `db:"success_correlation"` // -1.0 to 1.0
    DecayScore        float64   `db:"decay_score"`         // 0.0 (dead) to 1.0 (fresh)

    // Provenance
    SourceSessionID string    `db:"source_session_id"`
    ExtractedBy     string    `db:"extracted_by"` // pipeline version identifier
    CreatedAt       time.Time `db:"created_at"`
    UpdatedAt       time.Time `db:"updated_at"`

    // File reference
    FilePath string `db:"file_path"` // relative path to SKILL.md within library
}

type SkillCategory string

const (
    CategoryFoundational SkillCategory = "foundational"
    CategoryTactical     SkillCategory = "tactical"
    CategoryContextual   SkillCategory = "contextual"
)
```

#### Quality Scores

```go
// QualityScores holds the 7-dimensional evaluation from Stage 2/3.
// Each dimension is scored 1-5. Stored as individual DB columns for queryability.
type QualityScores struct {
    ProblemSpecificity     int     `db:"q_problem_specificity"`     // 1-5
    SolutionCompleteness   int     `db:"q_solution_completeness"`   // 1-5
    ContextPortability     int     `db:"q_context_portability"`     // 1-5
    ReasoningTransparency  int     `db:"q_reasoning_transparency"`  // 1-5
    TechnicalAccuracy      int     `db:"q_technical_accuracy"`      // 1-5
    VerificationEvidence   int     `db:"q_verification_evidence"`   // 1-5
    InnovationLevel        int     `db:"q_innovation_level"`        // 1-5
    CompositeScore         float64 `db:"q_composite_score"`         // weighted average
    CriticConfidence       float64 `db:"q_critic_confidence"`       // 0.0-1.0, LLM self-reported
}

// MinimumViable returns true if the skill meets minimum thresholds.
func (q QualityScores) MinimumViable() bool {
    return q.ProblemSpecificity >= 3 &&
        q.SolutionCompleteness >= 3 &&
        q.TechnicalAccuracy >= 3
}

// HighValue returns true if average across all dimensions >= 4.
func (q QualityScores) HighValue() bool {
    sum := q.ProblemSpecificity + q.SolutionCompleteness + q.ContextPortability +
        q.ReasoningTransparency + q.TechnicalAccuracy + q.VerificationEvidence +
        q.InnovationLevel
    return float64(sum)/7.0 >= 4.0
}

// InjectionPriority returns the injection ranking weight.
// Portability and verification are the strongest signals for reuse —
// a skill that works outside its original context and has evidence it works.
func (q QualityScores) InjectionPriority() float64 {
    return float64(q.ContextPortability)*0.6 + float64(q.VerificationEvidence)*0.4
}
```

### 1.2 Session Record

```go
// SessionRecord captures metadata about an agent session for extraction evaluation.
// This is the input to Stage 1. Content is read from the session log on demand, not stored here.
type SessionRecord struct {
    ID              string        `db:"id"`               // from agent runtime
    StartedAt       time.Time     `db:"started_at"`
    EndedAt         time.Time     `db:"ended_at"`
    DurationMinutes float64       `db:"duration_minutes"`
    ToolCallCount   int           `db:"tool_call_count"`
    ErrorCount      int           `db:"error_count"`        // tool calls with non-zero exit or explicit errors
    MessageCount    int           `db:"message_count"`     // total user+assistant message turns
    ErrorRate       float64       `db:"error_rate"`        // error_count / tool_call_count (0 if no tool calls)
    HasSuccessfulExec bool        `db:"has_successful_exec"` // at least one exit code 0
    TokensUsed      int           `db:"tokens_used"`       // total prompt+completion tokens
    AgentModel      string        `db:"agent_model"`       // model identifier
    UserID          string        `db:"user_id"`           // anonymized user hash
    LibraryID       string        `db:"library_id"`

    // Extraction pipeline state
    ExtractionStatus ExtractionStatus `db:"extraction_status"`
    RejectedAtStage  int              `db:"rejected_at_stage"` // 0=not rejected, 1/2/3
    RejectionReason  string           `db:"rejection_reason"`
    ExtractedSkillID string           `db:"extracted_skill_id"` // set if extraction succeeded

    // Content reference (not stored in DB — resolved at runtime)
    LogPath string `db:"-"`
}

type ExtractionStatus string

const (
    StatusPending   ExtractionStatus = "pending"
    StatusStage1    ExtractionStatus = "stage1"
    StatusStage2    ExtractionStatus = "stage2"
    StatusStage3    ExtractionStatus = "stage3"
    StatusExtracted ExtractionStatus = "extracted"
    StatusRejected  ExtractionStatus = "rejected"
    StatusError     ExtractionStatus = "error"
)
```

### 1.3 Extraction Result

```go
// ExtractionResult is the output of the full pipeline for a single session.
type ExtractionResult struct {
    SessionID    string           `json:"session_id"`
    Status       ExtractionStatus `json:"status"`
    Stage1       *Stage1Result    `json:"stage1,omitempty"`
    Stage2       *Stage2Result    `json:"stage2,omitempty"`
    Stage3       *Stage3Result    `json:"stage3,omitempty"`
    Skill        *SkillRecord     `json:"skill,omitempty"` // nil if rejected
    ProcessedAt  time.Time        `json:"processed_at"`
    DurationMs   int64            `json:"duration_ms"`
    Error        string           `json:"error,omitempty"`
}

type Stage1Result struct {
    Passed          bool    `json:"passed"`
    DurationOK      bool    `json:"duration_ok"`
    ToolCallCountOK bool    `json:"tool_call_count_ok"`
    ErrorRateOK     bool    `json:"error_rate_ok"`
    HasSuccessExec  bool    `json:"has_success_exec"`
    Reason          string  `json:"reason,omitempty"` // why rejected
}

type Stage2Result struct {
    Passed            bool          `json:"passed"`
    EmbeddingDistance  float64       `json:"embedding_distance"`  // to nearest existing skill
    NoveltyScore      float64       `json:"novelty_score"`       // derived from distance
    RubricScores      QualityScores `json:"rubric_scores"`
    ClassifiedCategory SkillCategory `json:"classified_category"`
    ClassifiedPatterns []string      `json:"classified_patterns"`
    Reason            string        `json:"reason,omitempty"`
}

type Stage3Result struct {
    Passed           bool          `json:"passed"`
    CriticVerdict    string        `json:"critic_verdict"`    // "extract" | "reject"
    CriticReasoning  string        `json:"critic_reasoning"`
    RefinedScores    QualityScores `json:"refined_scores"`    // critic may adjust Stage 2 scores
    ReusablePattern  bool          `json:"reusable_pattern"`
    ExplicitReasoning bool         `json:"explicit_reasoning"`
    ContradictsBestPractices bool  `json:"contradicts_best_practices"`
    TokensUsed       int           `json:"tokens_used"`
    LatencyMs        int64         `json:"latency_ms"`
}
```

### 1.4 SQL DDL

```sql
CREATE TABLE skills (
    id                    TEXT PRIMARY KEY,
    name                  TEXT NOT NULL,
    version               INTEGER NOT NULL DEFAULT 1,
    parent_id             TEXT REFERENCES skills(id),
    library_id            TEXT NOT NULL,
    category              TEXT NOT NULL CHECK (category IN ('foundational','tactical','contextual')),
    q_problem_specificity     INTEGER NOT NULL CHECK (q_problem_specificity BETWEEN 1 AND 5),
    q_solution_completeness   INTEGER NOT NULL CHECK (q_solution_completeness BETWEEN 1 AND 5),
    q_context_portability     INTEGER NOT NULL CHECK (q_context_portability BETWEEN 1 AND 5),
    q_reasoning_transparency  INTEGER NOT NULL CHECK (q_reasoning_transparency BETWEEN 1 AND 5),
    q_technical_accuracy      INTEGER NOT NULL CHECK (q_technical_accuracy BETWEEN 1 AND 5),
    q_verification_evidence   INTEGER NOT NULL CHECK (q_verification_evidence BETWEEN 1 AND 5),
    q_innovation_level        INTEGER NOT NULL CHECK (q_innovation_level BETWEEN 1 AND 5),
    q_composite_score         REAL NOT NULL,
    q_critic_confidence       REAL NOT NULL CHECK (q_critic_confidence BETWEEN 0.0 AND 1.0),
    injection_count       INTEGER NOT NULL DEFAULT 0,
    last_injected_at      TIMESTAMP,
    success_correlation   REAL NOT NULL DEFAULT 0.0,
    decay_score           REAL NOT NULL DEFAULT 1.0,
    source_session_id     TEXT,
    extracted_by          TEXT NOT NULL,
    file_path             TEXT NOT NULL,
    created_at            TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at            TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name, version, library_id)
);

CREATE TABLE skill_patterns (
    skill_id  TEXT NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    pattern   TEXT NOT NULL,  -- e.g. 'FIX/Backend/DatabaseConnection'
    PRIMARY KEY (skill_id, pattern)
);

CREATE TABLE skill_embeddings (
    skill_id   TEXT PRIMARY KEY REFERENCES skills(id) ON DELETE CASCADE,
    embedding  BLOB NOT NULL,  -- 1536 float32s, stored as raw bytes (6144 bytes)
    model      TEXT NOT NULL,   -- embedding model identifier, e.g. 'text-embedding-3-small'
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE sessions (
    id                   TEXT PRIMARY KEY,
    started_at           TIMESTAMP NOT NULL,
    ended_at             TIMESTAMP NOT NULL,
    duration_minutes     REAL NOT NULL,
    tool_call_count      INTEGER NOT NULL,
    error_count          INTEGER NOT NULL,
    total_calls          INTEGER NOT NULL,
    error_rate           REAL NOT NULL,
    has_successful_exec  BOOLEAN NOT NULL,
    tokens_used          INTEGER NOT NULL DEFAULT 0,
    agent_model          TEXT NOT NULL DEFAULT '',
    user_id              TEXT NOT NULL DEFAULT '',
    library_id           TEXT NOT NULL,
    extraction_status    TEXT NOT NULL DEFAULT 'pending',
    rejected_at_stage    INTEGER NOT NULL DEFAULT 0,
    rejection_reason     TEXT NOT NULL DEFAULT '',
    extracted_skill_id   TEXT REFERENCES skills(id),
    created_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE injection_events (
    id              TEXT PRIMARY KEY,
    session_id      TEXT NOT NULL,
    skill_id        TEXT NOT NULL REFERENCES skills(id),
    rank_position   INTEGER NOT NULL,    -- 1-based position in injection list
    match_score     REAL NOT NULL,       -- composite score used for ranking
    pattern_overlap REAL NOT NULL,
    cosine_sim      REAL NOT NULL,
    historical_rate REAL NOT NULL,
    injected_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    session_outcome TEXT DEFAULT NULL     -- 'success' | 'failure' | NULL (unknown)
);

CREATE TABLE human_review_samples (
    id              TEXT PRIMARY KEY,
    session_id      TEXT NOT NULL REFERENCES sessions(id),
    extraction_result TEXT NOT NULL,      -- JSON blob of ExtractionResult
    reviewer        TEXT,
    verdict         TEXT,                 -- 'agree' | 'disagree_should_extract' | 'disagree_should_reject'
    notes           TEXT,
    sampled_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    reviewed_at     TIMESTAMP
);

-- Indexes
CREATE INDEX idx_skills_category ON skills(category);
CREATE INDEX idx_skills_decay ON skills(decay_score);
CREATE INDEX idx_skills_library ON skills(library_id);
CREATE INDEX idx_sessions_status ON sessions(extraction_status);
CREATE INDEX idx_sessions_library ON sessions(library_id);
CREATE INDEX idx_injection_events_skill ON injection_events(skill_id);
CREATE INDEX idx_injection_events_session ON injection_events(session_id);
```

### 1.5 Versioning

Skill versions are **append-only**. A new version creates a new `SkillRecord` row with an incremented `version` and `parent_id` pointing to the previous version. The SKILL.md file is written to a new path (e.g., `skills/fix-db-conn/v2/SKILL.md`) and committed to the git repository managed by `gitserver.Server`.

Old versions are never mutated. Decay operates on the latest version only. Injection always selects the latest version. The version chain is a singly-linked list via `parent_id`.

When a new extraction covers the same problem pattern as an existing skill, the system checks embedding similarity. If similarity exceeds the version threshold (configurable, default 0.85), it creates a new version of the existing skill rather than a separate skill. Below the threshold, it's a new skill.

---

## 2. API Boundaries

### 2.1 Core Interfaces

```go
package extraction

import "context"

// Extractor runs the 3-stage extraction pipeline on a session.
// Synchronous — blocks until all stages complete or the session is rejected.
type Extractor interface {
    // Extract processes a single session through all extraction stages.
    // Returns the result regardless of whether a skill was extracted or the session was rejected.
    Extract(ctx context.Context, session SessionRecord, content []byte) (*ExtractionResult, error)
}

// Stage1Filter performs metadata pre-filtering. Synchronous, cheap, no I/O.
type Stage1Filter interface {
    Filter(session SessionRecord) *Stage1Result
}

// Stage2Scorer runs the two classifiers (embedding distance + structured rubric).
// Synchronous. Requires embedding computation (HTTP call) and a lightweight LLM call
// for rubric scoring (structured output, Haiku-class model).
type Stage2Scorer interface {
    Score(ctx context.Context, session SessionRecord, content []byte) (*Stage2Result, error)
}

// Stage3Critic runs the LLM critic on a pre-filtered candidate.
// Synchronous. Expensive — one LLM call per invocation.
type Stage3Critic interface {
    Evaluate(ctx context.Context, session SessionRecord, content []byte, stage2 *Stage2Result) (*Stage3Result, error)
}
```

```go
package storage

import "context"

// SkillStore persists and retrieves skills. All methods are synchronous.
type SkillStore interface {
    // Put stores a new skill. Computes embedding, writes SKILL.md to git, inserts DB record.
    Put(ctx context.Context, skill *extraction.SkillRecord, body []byte) error

    // Get retrieves a skill by ID.
    Get(ctx context.Context, id string) (*extraction.SkillRecord, error)

    // GetLatestByName retrieves the latest version of a skill by name and library.
    GetLatestByName(ctx context.Context, name string, libraryID string) (*extraction.SkillRecord, error)

    // Query finds skills matching pattern tags and/or embedding similarity.
    // Returns results ordered by the composite match score.
    Query(ctx context.Context, q SkillQuery) ([]ScoredSkill, error)

    // UpdateUsage increments injection count and updates success correlation.
    UpdateUsage(ctx context.Context, id string, outcome string) error

    // UpdateDecay sets a new decay score for a skill.
    UpdateDecay(ctx context.Context, id string, decayScore float64) error

    // ListByDecay returns skills ordered by decay score (ascending = most decayed first).
    ListByDecay(ctx context.Context, libraryID string, limit int) ([]extraction.SkillRecord, error)
}

type SkillQuery struct {
    Patterns   []string  // pattern tags to match
    Embedding  []float32 // query embedding for similarity search
    LibraryIDs []string  // library scope (respects priority ordering)
    MinQuality float64   // minimum composite quality score
    MinDecay   float64   // minimum decay score (exclude dead skills)
    Limit      int       // max results
}

type ScoredSkill struct {
    Skill          extraction.SkillRecord
    PatternOverlap float64
    CosineSim      float64
    HistoricalRate float64
    CompositeScore float64 // 0.5*PatternOverlap + 0.3*CosineSim + 0.2*HistoricalRate
}
```

```go
package injection

import "context"

// Injector selects and delivers skills to an agent session.
// Synchronous — called at session start before the agent begins work.
type Injector interface {
    // Inject classifies the prompt, finds matching skills, and returns them ranked.
    // The caller is responsible for inserting the skill content into the agent context.
    Inject(ctx context.Context, req InjectionRequest) (*InjectionResponse, error)
}

type InjectionRequest struct {
    Prompt     string   // user's prompt text
    LibraryIDs []string // library scope, ordered by priority (from config.LibraryConfig)
    MaxSkills  int      // context budget, typically 3-5
    SessionID  string   // for logging injection events
}

type InjectionResponse struct {
    Skills        []ScoredSkill       // ordered by composite score, len <= MaxSkills
    Classification PromptClassification // how the prompt was parsed
}

type PromptClassification struct {
    Intent   string   // BUILD | FIX | OPTIMIZE | INTEGRATE | CONFIGURE | LEARN
    Domain   string   // Frontend | Backend | DevOps | Data | Security | Performance
    Patterns []string // matched pattern tags
}
```

```go
package decay

import "context"

// DecayRunner performs a decay cycle across all skills in a library.
// Runs on a schedule (cron), not triggered by other components.
type DecayRunner interface {
    // RunCycle performs one decay pass. Reads usage stats, computes new decay scores,
    // writes updated scores back. Returns number of skills demoted/deprecated.
    RunCycle(ctx context.Context, libraryID string) (*DecayCycleResult, error)
}

type DecayCycleResult struct {
    Processed   int
    Demoted     int   // decay score reduced
    Deprecated  int   // decay score reached 0, effectively invisible
    Rescued     int   // decay score increased due to recent successful usage
}
```

### 2.2 Embedding Service

```go
package embedding

import "context"

// Embedder computes vector embeddings for text. HTTP call to external API.
type Embedder interface {
    Embed(ctx context.Context, text string) ([]float32, error)
    EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
    Dimensions() int // e.g. 1536 for text-embedding-3-small
}
```

### 2.3 Sync vs Async & Trigger Map

| Operation | Trigger | Sync/Async | Notes |
|---|---|---|---|
| Stage 1 filter | Session ends | Sync | Pure computation, <1ms |
| Stage 2 scoring | Stage 1 pass | Sync | Embedding API call, ~200ms |
| Stage 3 critic | Stage 2 pass | Sync | LLM call, ~2-5s |
| Skill storage | Stage 3 pass | Sync | Git commit + DB insert, ~100ms |
| Embedding compute | Skill storage | Sync (within Put) | Part of SkillStore.Put |
| Injection | Session starts | Sync | Embedding + DB query, ~300ms |
| Decay cycle | Cron (daily) | Async (background) | Bulk DB reads/writes |
| Human review sampling | Post-extraction | Async | 1% random selection, writes to review table |
| Injection outcome logging | Session ends | Async | Updates injection_events.session_outcome |

The full extraction pipeline (`Extractor.Extract`) is synchronous end-to-end because it runs after a session ends — there is no user waiting. Injection is synchronous because the agent needs skills before it starts working. Decay runs on a background schedule independent of everything else.

### 2.4 Pipeline Data Flow

```
SessionRecord + []byte (log content)
        │
        ▼
  ┌─────────────┐
  │ Stage1Filter │──reject──▶ ExtractionResult{Status: rejected, RejectedAtStage: 1}
  │  (in-proc)   │
  └──────┬──────┘
         │ pass
         ▼
  ┌─────────────┐
  │ Stage2Scorer │──reject──▶ ExtractionResult{Status: rejected, RejectedAtStage: 2}
  │ (embed API)  │
  └──────┬──────┘
         │ pass
         ▼
  ┌─────────────┐
  │ Stage3Critic │──reject──▶ ExtractionResult{Status: rejected, RejectedAtStage: 3}
  │  (LLM API)   │
  └──────┬──────┘
         │ pass
         ▼
  ┌─────────────┐
  │ SkillStore   │──────────▶ ExtractionResult{Status: extracted, Skill: &SkillRecord{...}}
  │  .Put()      │
  └─────────────┘
```

---

## 3. Evaluation Framework

### 3.1 Stage-Level Metrics

| Stage | Metric | How Measured | Target Threshold |
|---|---|---|---|
| Stage 1 | Pass rate | `count(passed) / count(total)` | 20-40% of sessions pass (if >50%, filters are too loose) |
| Stage 1 | False negative rate | Human review of rejected sample | <5% of rejected sessions contain extractable knowledge |
| Stage 2 | Pass rate | `count(passed) / count(stage1_passed)` | 30-60% of Stage 1 survivors |
| Stage 2 | Inter-classifier agreement | Both classifiers agree on pass/fail | >80% agreement |
| Stage 2 | Rubric score distribution | Histogram of scores per dimension | Roughly normal, centered at 3 |
| Stage 3 | Pass rate | `count(passed) / count(stage2_passed)` | 40-70% of Stage 2 survivors |
| Stage 3 | Critic consistency | Same session evaluated twice, same verdict | >90% self-agreement |
| Full pipeline | End-to-end yield | `extracted / total_sessions` | 3-10% of all sessions |
| Full pipeline | Extraction precision | Human review: fraction of extracted skills rated useful | >70% |
| Full pipeline | Extraction recall | Human review: fraction of useful sessions in rejected sample | >80% (i.e., <20% missed) |

### 3.2 Injection Metrics

| Metric | How Measured | Target |
|---|---|---|
| Injection rate | Sessions receiving ≥1 skill / total sessions | 30-60% |
| Relevance (human) | Sample injected skills, human rates relevance 1-5 | Mean ≥3.5 |
| Session success delta | Success rate of sessions with injection vs. baseline | Positive delta, measured via A/B |
| Skill utilization | Distinct skills injected / total skills | >20% (avoid dead skill accumulation) |

### 3.3 A/B Testing: Injection vs. No-Injection

**Design:** Randomized controlled trial at session level.

```go
// ABConfig controls the injection A/B test.
type ABConfig struct {
    Enabled       bool    `yaml:"enabled"`
    ControlRatio  float64 `yaml:"control_ratio"`  // fraction of sessions in control group (no injection), e.g. 0.1
    MinSampleSize int     `yaml:"min_sample_size"` // minimum sessions per group before computing results
}
```

- **Treatment group** (90%): Normal injection pipeline.
- **Control group** (10%): Injection pipeline runs but results are logged without delivery. The agent receives no skills.
- **Primary metric:** Session success rate (user-reported or heuristic: task completed, no errors in final state).
- **Secondary metrics:** Session duration, tool call count, error rate.
- **Analysis:** Two-proportion z-test. Require p < 0.05 and >100 sessions per group minimum.
- **Logging:** Every session logs `ab_group: "treatment" | "control"` in the injection event. Control sessions still generate `injection_events` rows with a `delivered: false` flag.

### 3.4 Human Review Sampling

**Sampling rate:** 1% of all sessions processed, stratified:
- 50% from extracted sessions (did we extract something good?)
- 50% from rejected sessions (did we miss something?)

**Cadence:** Samples accumulate continuously. Review batch weekly — a human reviewer processes the `human_review_samples` table, setting `verdict` and `notes`.

**What gets stored:** Full `ExtractionResult` as JSON, plus the session log path for the reviewer to inspect.

**Calibration loop:** Monthly, compute agreement between pipeline decisions and human verdicts. If precision drops below 70% or recall below 80%, adjust Stage 1 thresholds and Stage 2 rubric weights.

### 3.5 Success/Failure Attribution

When a session ends that received injected skills:

1. Determine session outcome (success/failure) via heuristic: (a) explicit user signal if available (thumbs up/down, "thanks" detection), (b) final tool call exit code 0 + no error messages in last 3 turns, (c) task completion signal from agent framework if exposed. Default to `NULL` (unknown) if no signal — don't guess.
2. Update `injection_events.session_outcome` for all skills injected into that session.
3. Periodically (daily, in decay cycle), recompute `skills.success_correlation`:

```
success_correlation = (successful_injections - failed_injections) / total_injections
```

Skills with `success_correlation < -0.2` after ≥10 injections are flagged for human review.

---

## 4. Cost Model

### 4.1 Per-Session Extraction Costs

| Stage | LLM/API Calls | Estimated Tokens | Estimated Cost |
|---|---|---|---|
| Stage 1 | 0 | 0 | $0.00 |
| Stage 2: Embedding | 1 embed call (~2K tokens input) | ~2,000 | ~$0.0002 (text-embedding-3-small @ $0.02/1M tokens) |
| Stage 2: Rubric | 1 lightweight LLM call (~3K input, ~300 output) | ~3,300 | ~$0.005 (Haiku-class, structured output) |
| Stage 3: LLM Critic | 1 LLM call (session content ~4K tokens + prompt ~1K → ~5K input, ~500 output) | ~5,500 | ~$0.03 (Claude Haiku-class @ ~$0.25/1M input + $1.25/1M output) |
| Skill embedding | 1 embed call (~1K tokens) | ~1,000 | ~$0.0001 |
| **Total if extracted** | **2 embed + 2 LLM** | **~11,800** | **~$0.035** |
| **Total if rejected at Stage 1** | **0** | **0** | **$0.00** |
| **Total if rejected at Stage 2** | **1 embed + 1 LLM** | **~5,300** | **~$0.005** |

### 4.2 Per-Session Injection Costs

| Step | Calls | Tokens | Cost |
|---|---|---|---|
| Prompt embedding | 1 embed call (~500 tokens) | ~500 | ~$0.00001 |
| DB query | 0 (local SQLite) | 0 | $0.00 |
| **Total per injection** | **1 embed** | **~500** | **~$0.00001** |

### 4.3 Storage Costs

| Item | Size | Notes |
|---|---|---|
| SKILL.md file | ~2-5 KB | Markdown with front matter |
| DB row (skills table) | ~500 bytes | All columns |
| Embedding vector | 6,144 bytes | 1536 × float32 |
| Patterns (join table) | ~100 bytes/pattern | 2-4 patterns per skill |
| Session record | ~300 bytes | Metadata only |
| Injection event | ~200 bytes | Per injection per session |
| **Per skill total** | ~10 KB | DB + file + embedding |
| **Per 1000 skills** | ~10 MB | Trivial for SQLite |

### 4.4 Cost Per 1000 Sessions (Full Pipeline)

Assumptions: 30% pass Stage 1, 50% of those pass Stage 2, 60% of those pass Stage 3.  
→ 1000 sessions → 300 reach Stage 2 → 150 reach Stage 3 → 90 extracted.

| Component | Count | Unit Cost | Total |
|---|---|---|---|
| Stage 2 embeddings | 300 | $0.0002 | $0.06 |
| Stage 2 rubric LLM | 300 | $0.005 | $1.50 |
| Stage 3 LLM critic | 150 | $0.03 | $4.50 |
| Skill embeddings (extracted) | 90 | $0.0001 | $0.009 |
| Injection embeddings (assuming all 1000 sessions get injection lookup) | 1000 | $0.00001 | $0.01 |
| **Total per 1000 sessions** | | | **~$6.08** |

Stage 3 dominates cost at ~98%. To reduce: use a cheaper model for Stage 3, or tighten Stage 2 to pass fewer candidates.

### 4.5 Decay Cycle Cost

Zero LLM calls. Pure DB reads and writes. Negligible — bounded by number of skills, not sessions.

---

## 5. Error Handling

### 5.1 LLM Critic Failure (Stage 3)

| Failure Mode | Behavior |
|---|---|
| Timeout (>30s) | Retry once with 60s timeout. If second attempt fails, mark session as `StatusError`, log, skip. Session remains in DB for retry in next batch. |
| Rate limit (429) | Exponential backoff: 1s, 2s, 4s, 8s, 16s. Max 5 retries. If exhausted, pause pipeline for 60s, then resume with remaining sessions. |
| Malformed response | Log full response. Attempt JSON repair (trim, re-parse). If unparseable, treat as rejection with `rejection_reason: "critic_parse_error"`. |
| API error (5xx) | Same as timeout: retry once, then skip to next session. |
| **Circuit breaker** | If >5 consecutive Stage 3 failures, open circuit for 5 minutes. During open circuit, Stage 2 survivors are queued (in-memory, bounded at 100). After 5 minutes, half-open: try one. If it succeeds, close circuit and drain queue. If it fails, re-open for 10 minutes. |

**Graceful degradation:** When Stage 3 is down, the system still processes sessions through Stage 1 and Stage 2. Sessions that pass Stage 2 are marked `StatusStage2` and queued for Stage 3 retry. No skills are extracted, but the filtering work is preserved.

### 5.2 Embedding Service Failure

| Failure Mode | Behavior |
|---|---|
| Timeout/5xx | Retry with backoff (same policy as LLM). |
| Circuit breaker | Same pattern: 5 consecutive failures → open for 5 min. |
| **Impact on extraction** | Stage 2 cannot run. Sessions accumulate at `StatusStage1` (passed Stage 1, awaiting Stage 2). Queue is bounded at 1000 sessions. Beyond that, oldest pending sessions are marked `StatusError`. |
| **Impact on injection** | Cannot compute prompt embedding. Fallback: **pattern-only matching**. Skip cosine similarity, use only pattern overlap + historical success rate with reweighted formula: `Score = 0.7 × PatternOverlap + 0.3 × HistoricalRate`. Log that injection ran in degraded mode. |
| **Impact on skill storage** | `SkillStore.Put` fails. Skill is written to git but not searchable via embedding. Marked with `embedding: pending` in DB. Background job retries embedding computation every 5 minutes for pending skills. |

### 5.3 Storage Failure

| Failure Mode | Behavior |
|---|---|
| SQLite locked | Retry with 100ms backoff, max 10 retries. SQLite WAL mode reduces lock contention. |
| Disk full | `SkillStore.Put` returns error. Pipeline halts. Emit alert-level log. Decay cycle continues (read-only operations still work). Injection continues (read-only). |
| DB corruption | On startup, run `PRAGMA integrity_check`. If corrupt, attempt recovery from WAL. If unrecoverable, log critical error. Skills on disk (git) are still intact — DB can be rebuilt from SKILL.md files via a recovery command. |
| Git operation failure | `gitserver.Server` methods return errors. Skill is written to DB but not committed to git. Background reconciliation job detects DB records without corresponding git commits and retries. |

### 5.4 Retry Policy Summary

```go
// RetryConfig is embedded in the extraction, embedding, and storage configs.
type RetryConfig struct {
    MaxRetries     int           `yaml:"max_retries"`      // default: 3
    InitialBackoff time.Duration `yaml:"initial_backoff"`   // default: 1s
    MaxBackoff     time.Duration `yaml:"max_backoff"`       // default: 30s
    BackoffFactor  float64       `yaml:"backoff_factor"`    // default: 2.0
}

// CircuitBreakerConfig controls the circuit breaker for external service calls.
type CircuitBreakerConfig struct {
    FailureThreshold int           `yaml:"failure_threshold"` // default: 5
    OpenDuration     time.Duration `yaml:"open_duration"`     // default: 5m
    HalfOpenMax      int           `yaml:"half_open_max"`     // default: 1
}
```

### 5.5 Degradation Matrix

| Component Down | Extraction | Injection | Decay | Storage |
|---|---|---|---|---|
| LLM API | Stages 1-2 work, Stage 3 queued | Unaffected | Unaffected | Unaffected |
| Embedding API | Stage 1 works, Stages 2-3 queued | Pattern-only matching (degraded) | Unaffected | Skills stored without embedding |
| SQLite | Halted | Read cache if available, else halted | Halted | Halted |
| Git server | Unaffected (DB is primary) | Unaffected | Unaffected | DB writes succeed, git commits queued |
| All external APIs | Stage 1 only | Pattern-only if skill cache warm | Unaffected | Local writes only |

The system is designed so that **injection never blocks on extraction**, and **decay never blocks on anything**. The worst case — all external APIs down — still allows Stage 1 filtering and pattern-based injection from cached data.

---

## 6. Implementation Plan

Build in order. Each phase is a standalone deliverable. Refer to sections above for schemas, interfaces, and behavior details.

### Phase 0: Storage Layer
Create `internal/storage/` with SQLite implementation. Define all tables from §1.4, implement `SkillStore` interface from §2.1 (Put, Get, Query, UpdateUsage, UpdateDecay, ListByDecay). Include migrations and `PRAGMA integrity_check` on startup.

### Phase 1: Embedding Service
Create `internal/embedding/` implementing the `Embedder` interface from §2.2. Support OpenAI's text-embedding-3-small via HTTP. Include retry policy and circuit breaker from §5.4.

### Phase 2: Stage 1 Filter
Create `internal/extraction/stage1.go` implementing `Stage1Filter` from §2.1. Pure in-process metadata checks against config thresholds (duration, tool calls, error rate). No external calls.

### Phase 3: Stage 2 Scorer
Create `internal/extraction/stage2.go` implementing `Stage2Scorer`. Two classifiers: embedding novelty (distance from nearest existing skill) and structured rubric scoring via lightweight LLM call returning `QualityScores`. Classify category and patterns.

### Phase 4: Stage 3 Critic
Create `internal/extraction/stage3.go` implementing `Stage3Critic`. Single LLM call with session content + Stage 2 results, returns verdict + reasoning + refined scores. Include retry/circuit breaker.

### Phase 5: Extraction Pipeline
Create `internal/extraction/pipeline.go` implementing `Extractor` — wires Stage 1→2→3→SkillStore.Put. Structured logging (`slog`) at every decision point. Writes `ExtractionResult` to DB. Includes 1% human review sampling.

### Phase 6: Injection Pipeline
Create `internal/injection/` implementing `Injector` from §2.1. Classify prompt into patterns, query SkillStore with embedding + pattern match, return ranked skills. Fallback to pattern-only when embeddings unavailable (§5.2).

### Phase 7: Decay Runner
Create `internal/decay/` implementing `DecayRunner`. Runs as scheduled job — reads usage stats, applies half-life decay per category, flags skills below deprecation threshold. Pure DB operations.

### Phase 8: CLI Integration
Wire extraction into `kinoko serve` as a post-session hook. Wire injection into session startup. Add `kinoko extract <session-log>` for manual extraction. Add `kinoko decay --dry-run` for manual decay cycles.

### Phase 9: A/B Testing & Metrics
Add injection A/B test (§3.3) with configurable control ratio. Add `injection_events` logging. Add `kinoko stats` command showing pipeline metrics from §3.1-3.2.

---

## Appendix A: Configuration Extensions

These extend the existing `config.Config` in `internal/config/config.go`:

```go
// ExtractionConfig extends the existing config with pipeline-specific settings.
type ExtractionConfig struct {
    AutoExtract       bool    `yaml:"auto_extract"`
    MinConfidence     float64 `yaml:"min_confidence"`
    RequireValidation bool    `yaml:"require_validation"`

    // Stage 1 thresholds
    MinDurationMinutes float64 `yaml:"min_duration_minutes"` // default: 2
    MaxDurationMinutes float64 `yaml:"max_duration_minutes"` // default: 180
    MinToolCalls       int     `yaml:"min_tool_calls"`       // default: 3
    MaxErrorRate       float64 `yaml:"max_error_rate"`       // default: 0.7

    // Stage 2
    NoveltyMinDistance     float64 `yaml:"novelty_min_distance"`      // default: 0.15
    NoveltyMaxDistance     float64 `yaml:"novelty_max_distance"`      // default: 0.95
    VersionSimilarityThreshold float64 `yaml:"version_similarity_threshold"` // default: 0.85

    // Stage 3
    CriticModel  string `yaml:"critic_model"`  // default: "claude-3-haiku"
    CriticPrompt string `yaml:"critic_prompt"` // path to prompt template

    // Retry & circuit breaker
    Retry          RetryConfig          `yaml:"retry"`
    CircuitBreaker CircuitBreakerConfig `yaml:"circuit_breaker"`

    // Evaluation
    HumanReviewSampleRate float64  `yaml:"human_review_sample_rate"` // default: 0.01
    ABTest                ABConfig `yaml:"ab_test"`
}

// DecayConfig configures the decay system.
type DecayConfig struct {
    Enabled         bool          `yaml:"enabled"`
    Schedule        string        `yaml:"schedule"`          // cron expression, default: "0 3 * * *" (daily 3am)
    FoundationalHalfLifeDays int  `yaml:"foundational_half_life_days"` // default: 365
    TacticalHalfLifeDays     int  `yaml:"tactical_half_life_days"`     // default: 90
    ContextualHalfLifeDays   int  `yaml:"contextual_half_life_days"`   // default: 180
    DeprecationThreshold     float64 `yaml:"deprecation_threshold"`    // default: 0.05
}

// EmbeddingConfig configures the embedding service.
type EmbeddingConfig struct {
    Provider   string `yaml:"provider"`    // "openai" | "local"
    Model      string `yaml:"model"`       // default: "text-embedding-3-small"
    Dimensions int    `yaml:"dimensions"`  // default: 1536
    BaseURL    string `yaml:"base_url"`    // for local/custom providers
    APIKey     string `yaml:"api_key"`     // env: KINOKO_EMBEDDING_API_KEY
    Retry      RetryConfig          `yaml:"retry"`
    CircuitBreaker CircuitBreakerConfig `yaml:"circuit_breaker"`
}
```

## Appendix B: Problem Pattern Taxonomy (Initial List)

```
BUILD/Frontend/ComponentDesign
BUILD/Frontend/StateManagement
BUILD/Backend/APIDesign
BUILD/Backend/DataModeling
BUILD/DevOps/CIPipeline
BUILD/DevOps/ContainerSetup
FIX/Frontend/RenderingBug
FIX/Backend/DatabaseConnection
FIX/Backend/AuthFlow
FIX/DevOps/DeploymentFailure
FIX/Performance/MemoryLeak
FIX/Performance/SlowQuery
OPTIMIZE/Performance/Caching
OPTIMIZE/Performance/BundleSize
OPTIMIZE/Backend/QueryOptimization
INTEGRATE/Backend/ThirdPartyAPI
INTEGRATE/DevOps/CloudService
CONFIGURE/DevOps/InfraAsCode
CONFIGURE/Security/AccessControl
LEARN/Data/DataPipeline
```

20 patterns. Manually curated. Extended by team consensus only — never auto-generated.
