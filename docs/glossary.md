# Glossary

Terms used throughout the Mycelium project with specific meanings.

---

**ABInjector**
A decorator that wraps an `Injector` with A/B test logic. Randomly assigns sessions to treatment (skills delivered) or control (skills withheld). Logs injection events with group info. Located in `internal/injection/ab.go`.

**Category**
A classification of a skill's persistence characteristics. One of three values:
- **Foundational** — Core patterns that rarely change. Half-life: 365 days.
- **Tactical** — Solutions tied to specific tool versions or APIs. Half-life: 90 days.
- **Contextual** — Environment-specific knowledge. Half-life: 180 days.

Defined as `SkillCategory` constants in `internal/extraction/types.go`.

**Circuit Breaker**
A resilience pattern in Stage 3 (`stage3Critic`). Opens after 5 consecutive LLM failures, blocking further calls for 5 minutes (doubling on re-failure during half-open probes). Returns `ErrCircuitOpen` while open.

**Composite Score**
A weighted sum of all 7 quality dimensions used for ranking. Weights: Problem Specificity (0.15), Solution Completeness (0.20), Context Portability (0.15), Reasoning Transparency (0.10), Technical Accuracy (0.20), Verification Evidence (0.10), Innovation Level (0.10). Computed by `compositeScore()` in Stage 2.

**Critic**
The LLM evaluator that runs at Stage 3 of the extraction pipeline. Implemented by `Stage3Critic` interface / `stage3Critic` struct. Returns extract/reject verdicts with refined scores and confidence. Features retry, circuit breaker, and contradiction detection.

**Decay**
The process by which skills lose ranking over time. Implemented by `decay.Runner` using half-life degradation: `newDecay = oldDecay × 0.5^(days/halfLife)`. Skills below the deprecation threshold (0.05) are set to 0.0. Decayed skills can be rescued by recent successful usage.

**Degraded Mode**
Injection operating without embeddings (embedder nil or failed). Re-ranks skills as `0.7 × PatternOverlap + 0.3 × HistoricalRate` instead of using the full composite score.

**Dimensional Scoring**
The evaluation method for skill quality. Seven independent dimensions (Problem Specificity, Solution Completeness, Context Portability, Reasoning Transparency, Technical Accuracy, Verification Evidence, Innovation Level), each scored 1–5. Used in Stage 2 (rubric) and Stage 3 (refined scores).

**Embedding**
A vector representation of content, used for similarity-based retrieval. Computed by the `embedding.Embedder` interface. Stored as binary blobs in the `skill_embeddings` table. Default model: `text-embedding-3-small`.

**Extraction**
The process of identifying reusable knowledge in an agent session and distilling it into a skill. A three-stage pipeline: metadata pre-filters → embedding novelty + rubric scoring → LLM critic. Implemented by `extraction.Pipeline`.

**ExtractionResult**
The output of the full pipeline for a single session. Contains stage results, final status (`pending`/`extracted`/`rejected`/`error`), optional `SkillRecord`, timing, and error info.

**ExtractionStatus**
The pipeline state of a session: `pending`, `extracted`, `rejected`, or `error`. Stored in the `sessions` table.

**Human Review Sampling**
Stratified sampling of pipeline results for manual calibration. Maintains ~50/50 balance between extracted and rejected samples. Configured via `sample_rate`. Stored in `human_review_samples` table.

**Injection**
The process of delivering relevant skills as context to an agent at the start of a session. Implemented by `injection.Injector`. Classifies prompts, queries the skill store, ranks candidates, and logs events for the feedback loop.

**InjectionEventRecord**
A database row in `injection_events` tracking a single skill's injection into a single session. Records rank position, match scores, A/B group, delivery status, and eventual session outcome.

**Knowledge Library**
A collection of skills scoped by `library_id`. Libraries are layered by priority: local → team → public. Higher priority wins during injection.

**LLMClient**
A lightweight interface for LLM calls: `Complete(ctx, prompt) → (string, error)`. Used by Stage 2 (rubric scoring) and Stage 3 (critic), plus injection (prompt classification). Extended by `LLMClientV2` for token tracking and timeout control.

**Pattern (Problem Pattern)**
A tag from the fixed `Taxonomy` (20 entries) that classifies the type of problem a skill addresses. Three tiers: Intent/Domain/Specific (e.g., `FIX/Backend/DatabaseConnection`). Validated at extraction and injection. See `extraction.Taxonomy`.

**Pipeline**
The `extraction.Pipeline` struct that implements `Extractor`. Orchestrates Stage 1 → Stage 2 → Stage 3 → skill persistence, with session tracking and human review sampling.

**QualityScores**
A struct holding 7 dimensional scores (int, 1–5 each), a composite score (float64), and critic confidence (float64). Methods: `MinimumViable()`, `HighValue()`, `InjectionPriority()`.

**Rescue**
A decay mechanic that boosts skills recently used with positive outcomes. If a skill was injected within `RescueWindowDays` (default: 30) and has positive `SuccessCorrelation`, its decay score increases by `RescueBoost` (default: 0.3), capped at 1.0.

**Session**
A single interaction between a user and an AI agent. Represented by `extraction.SessionRecord` with metadata: duration, tool calls, error rate, tokens, etc. The raw input to the extraction pipeline.

**Skill**
The atomic unit of knowledge in Mycelium. Represented by `extraction.SkillRecord` in the database and a `SKILL.md` file on disk with YAML front matter. Versioned. Contains quality scores, patterns, category, decay score, and usage statistics.

**SkillQuerier**
An interface used by Stage 2 to find the nearest-neighbor skill by embedding: `QueryNearest(ctx, embedding, libraryID) → *SkillQueryResult`. Returns cosine similarity of the closest match.

**SkillStore**
The main storage interface (`storage.SkillStore`): `Put`, `Get`, `GetLatestByName`, `Query`, `UpdateUsage`, `UpdateDecay`, `ListByDecay`. Implemented by `SQLiteStore`.

**SkillWriter**
An interface for persisting extracted skills: `Put(ctx, skill, body) → error`. Implemented by `SQLiteStore`. Used by the pipeline to store results.

**Success Correlation**
A metric tracking how often a skill's injection correlates with positive session outcomes. Computed from `injection_events` as `(success_count − failure_count) / total_count`. Ranges from -1.0 to 1.0. Used in injection ranking and decay rescue.

**Taxonomy**
The canonical list of 20 problem patterns defined in `extraction.Taxonomy`. Used for classification at both extraction (Stage 2) and injection (prompt classification). Validated via `ValidPattern()`.

**Usage Tracking**
The system that records injection events in `injection_events`. Feeds injection ranking (via `HistoricalRate`) and decay (via `SuccessCorrelation` and `LastInjectedAt`).
