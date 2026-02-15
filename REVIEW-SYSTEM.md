# Full System Review — Jazz

**Date:** 2026-02-15  
**Reviewer:** Jazz (30 years in the industry, seen it all twice)  
**Scope:** End-to-end extraction system — all packages as an integrated whole  
**Overall Grade: B-**

---

## Executive Summary

I've reviewed every phase individually and given grades from B- to A-. Now I'm looking at the *system*. Individual packages are surprisingly competent. The integration story is where it falls apart. There's missing glue, type mismatches between what packages promise and what consumers expect, a sessions table nobody writes to, and config that doesn't cover half the knobs the subsystems actually expose. This thing would crash within 30 seconds of hitting real traffic, and it would crash in a way that's *confusing* because each piece looks like it works.

Let me be specific.

---

## 1. Cross-Package Consistency

### 1.1 Interface Mismatch: SkillStore vs Pipeline.SkillWriter

The pipeline declares:
```go
type SkillWriter interface {
    Put(ctx context.Context, skill *SkillRecord, body []byte) error
}
```

The storage package declares:
```go
type SkillStore interface {
    Put(ctx context.Context, skill *extraction.SkillRecord, body []byte) error
    // ... more methods
}
```

These are *compatible* — `SQLiteStore` satisfies both — but only by accident. The pipeline's `SkillWriter` is a subset of `SkillStore`. This works, but it's fragile. If someone adds a method to `SkillWriter` that doesn't match `SkillStore`, you get a compile error in the CLI, not in the package that defines the interface. **Tolerable, grudgingly.**

### 1.2 Interface Mismatch: HumanReviewWriter

`Pipeline.HumanReviewWriter` requires:
```go
InsertReviewSample(ctx context.Context, sessionID string, resultJSON []byte) error
```

**Nobody implements this.** `SQLiteStore` has no `InsertReviewSample` method. The pipeline's `maybeSample` will silently skip if `reviewer` is nil (which it always is because the CLI never passes one), but this means the entire §3.4 human review sampling feature is dead code. The spec explicitly requires 1% sampling. It doesn't exist.

### 1.3 InjectionResponse Type Divergence

The spec says `InjectionResponse.Skills` should be `[]ScoredSkill` (from storage). The implementation uses `[]InjectedSkill` (from extraction types). These are different types with different fields. The `InjectedSkill` type drops the actual `SkillRecord` — it only carries the skill ID and scores. **Any consumer that needs the skill content (patterns, SKILL.md body, category) from injection results can't get it.** This is a design choice, not a bug per se, but it deviates from the spec and limits downstream use.

### 1.4 Naming Inconsistencies

- `extraction.LLMClient` vs `extraction.LLMClientV2` — two interfaces for the same purpose, one extending the other. Fine as a pattern, but the naming is lazy. `LLMClientV2` sounds like it replaced V1. It didn't.
- `decay.SkillReader` and `decay.SkillWriter` are different interfaces than `extraction.SkillWriter` (pipeline) and `storage.SkillStore`. Four different skill-related interfaces across three packages. They all happen to be satisfied by `SQLiteStore`, but discovering that requires reading every interface definition.
- `injection.InjectionEventWriter` — yet another interface, this one with a single method `WriteInjectionEvent`. Also satisfied by `SQLiteStore`. That's five interfaces `SQLiteStore` quietly implements.

**Verdict:** It works because Go's structural typing saves you, but this is an integration test away from disaster. One rename and three packages break silently.

---

## 2. Integration Gaps

### 2.1 Sessions Table: Written by Nobody

The `sessions` table exists in the schema. The `metrics.Collector` queries it extensively. **Nothing in the codebase writes to it.**

The `extract` CLI command creates a `SessionRecord` struct in memory from log parsing, feeds it to the pipeline, and throws it away. The pipeline doesn't persist the session. The `serve` command's hook also doesn't persist sessions. The `stats` command will always show 0 sessions.

This is a **critical gap**. The entire metrics system is querying a table that's always empty. Every metric in `stats` will be zero. The human review sampling references `sessions(id)` with a foreign key — `InsertReviewSample` would fail even if it existed, because there's no session row to reference.

### 2.2 Extraction Status: Never Updated

`SessionRecord` has `ExtractionStatus`, `RejectedAtStage`, `RejectionReason`, and `ExtractedSkillID`. The pipeline computes these values (in `ExtractionResult`) but **never writes them back to the session record or the sessions table**. The stage pass rate queries in metrics rely on `rejected_at_stage` being set. It never is.

### 2.3 Skill Embedding: Never Set During Extraction

The extraction pipeline creates a `SkillRecord` and calls `store.Put()`. But `skill.Embedding` is never populated — nobody calls `embedder.Embed()` on the skill content before storage. The `Put` method checks `if len(skill.Embedding) > 0` before inserting into `skill_embeddings`. It will always be 0.

**Consequence:** Skills are stored without embeddings. Future `Query` calls will have `nil` embeddings for all stored skills. Cosine similarity will always be 0. The injection pipeline's embedding-based matching is useless. You're running on pattern-only mode permanently without knowing it.

### 2.4 Double Injection Event Writing

When A/B testing is enabled, `ABInjector.Inject` calls `inner.Inject` which is the base `injector`. The base `injector.Inject` writes injection events. Then `ABInjector.Inject` *also* writes injection events (with A/B group info). **Every injection event is written twice** — once without A/B data, once with. The IDs are different, so no duplicate key error. Just duplicate rows. Metrics will double-count everything.

### 2.5 serve Command: Hooks Built, Never Connected

`buildSessionHooks` creates a `SessionHooks` struct with `OnSessionStart` and `OnSessionEnd` callbacks. Then:
```go
hooks, err := buildSessionHooks(cfg, store, logger)
_ = hooks // hooks are invoked by the session lifecycle (git push / session API)
```

Those hooks are assigned to `_`. The comment says "invoked by the session lifecycle" but there's no session lifecycle that invokes them. The git server has no concept of sessions. **The entire extraction and injection pipeline is dead in serve mode.**

---

## 3. Missing Glue

### 3.1 What Breaks on `kinoko extract <session.log>`

Running this for real would:

1. ✅ Parse the session log (heuristic, good enough)
2. ✅ Open SQLite store
3. ✅ Create embedding client (needs `OPENAI_API_KEY`)
4. ✅ Create LLM client
5. ✅ Build pipeline
6. ✅ Run Stage 1 (in-proc, works)
7. ⚠️ Run Stage 2 — embedder.Embed() makes real HTTP call, storeQuerier.QueryNearest() works but returns CosineSim=0 on empty DB (so distance=1.0, high novelty — will pass)
8. ⚠️ Run Stage 2 rubric — LLM call works but response parsing is fragile (depends on model following instructions exactly)
9. ⚠️ Run Stage 3 — real LLM call, same fragility
10. ❌ Skill stored **without embedding** (see §2.3)
11. ❌ Session record **not persisted** (see §2.1)
12. ❌ Human review sample **not written** (see §1.2)
13. ✅ SKILL.md written to disk
14. ✅ Result JSON printed to stdout

**It would "work" in the sense that a SKILL.md appears on disk.** But the database is incomplete (no embedding, no session record), metrics are broken, and the skill is invisible to future injection queries.

### 3.2 Missing: Session Persistence

Need a `SessionStore` or extend `SkillStore` with session CRUD. The pipeline should write the session record before extraction and update it after.

### 3.3 Missing: Skill Embedding During Extraction

Between Stage 3 pass and `writer.Put()`, the pipeline should call `embedder.Embed()` on the skill content and set `skill.Embedding`. One line of code, but it's the difference between a searchable skill and a dead one.

### 3.4 Missing: InsertReviewSample Implementation

`SQLiteStore` needs this method. The `human_review_samples` table exists. The method doesn't.

---

## 4. Config Coherence

### 4.1 Missing from Config

| Subsystem Knob | Hardcoded In | Should Be Configurable |
|---|---|---|
| Embedding API key | `os.Getenv("OPENAI_API_KEY")` | `config.EmbeddingConfig.APIKey` |
| Embedding model | `"text-embedding-3-small"` in embedding.DefaultConfig() | In config YAML |
| LLM model | `"gpt-4o-mini"` hardcoded in extract.go | `config.ExtractionConfig.CriticModel` (spec has this!) |
| Circuit breaker thresholds (Stage 3) | `5 failures, 5 min` hardcoded in stage3.go | Config |
| Retry policy (Stage 3) | `3 retries, 1s/2s/4s backoff` hardcoded | `config.ExtractionConfig.Retry` (spec has this!) |
| Decay half-lives | `decay.DefaultConfig()` in decay.go | Config YAML (partially — `decayCfg` ignores config file) |
| Human review sample rate | `pipeline.SampleRate` in PipelineConfig | CLI never sets it |
| Max content bytes (Stage 3) | `100 * 1024` constant | Could be config |
| Injection max skills | `3` default constant | Could be config |
| Injection min decay | `0.05` constant | Could be config |
| A/B test config | Never wired | `config.ExtractionConfig.ABTest` exists but is never read |

**The spec defines `ExtractionConfig` with `Retry`, `CircuitBreaker`, `CriticModel`, `CriticPrompt`, and `HumanReviewSampleRate`. None of these are in the actual `config.ExtractionConfig`.** The config struct implements maybe 60% of what the spec calls for.

### 4.2 Decay Config Ignored

The `decay` CLI command creates `decay.DefaultConfig()` and ignores the YAML config entirely. There's no `config.DecayConfig` struct at all. The spec defines one.

---

## 5. Error Propagation

This is actually **one of the better aspects**. Grudgingly.

- Errors are consistently wrapped with `fmt.Errorf("context: %w", err)` throughout.
- The pipeline converts errors to `StatusError` results rather than returning errors, which is a reasonable design choice for a batch pipeline.
- Storage errors include the operation context (e.g., "insert skill", "update decay").
- The circuit breaker returns `ErrCircuitOpen` which is a sentinel — callers can check with `errors.Is`.

**Gaps:**
- Stage 3's `callWithRetry` swallows intermediate errors — only the last error is returned. If the first attempt failed with a meaningful message and the last with a generic timeout, you lose the useful one.
- The CLI's `exitError` type is nice but only handles extraction rejections. Store failures during `runExtract` return generic errors without exit codes.
- `maybeSample` silently swallows insert errors with a `Warn` log. Correct behavior, but worth noting.

**Grade for error handling: B+.** Better than most code I see.

---

## 6. Dependency Graph

```
cmd/kinoko
  ├── config
  ├── extraction (types, pipeline, stages)
  ├── storage
  ├── embedding
  ├── injection
  ├── decay
  ├── metrics
  └── gitserver (serve only)

extraction → config, embedding (stage2 only)
storage → extraction (types only)
injection → extraction (types), storage, embedding
decay → extraction (types only)
metrics → (database/sql only, no internal deps)
```

**This is clean.** No circular dependencies. The `extraction` package is the type hub — everyone imports its types. `storage` imports `extraction` for `SkillRecord` and friends. `injection` imports `storage` for `ScoredSkill` and `SkillQuery`. The dependency arrows all point inward toward types.

One risk: `extraction` imports `embedding` (for Stage 2's `Embedder` interface). If `embedding` ever needed extraction types, you'd have a cycle. Currently safe.

**Grade for dependency graph: A-.** One of the few things I won't complain about.

---

## 7. Schema Coherence

### 7.1 Schema vs Code Mismatches

| Issue | Details |
|---|---|
| `sessions.total_calls` in spec DDL vs `sessions.message_count` in schema.sql | Spec says `total_calls`, implementation says `message_count`. The Go struct has `MessageCount`. Schema.sql matches the Go struct, not the spec. **Correct behavior, spec is wrong.** |
| `sessions` FK on `session_id` in `injection_events` | `injection_events.session_id` has no FK to `sessions(id)`. Intentional — sessions aren't always persisted. But this means orphan injection events are possible. |
| `human_review_samples.session_id` FK | References `sessions(id)`. Since sessions are never inserted, any attempt to insert a review sample will fail with FK violation. **The human review feature is doubly broken.** |

### 7.2 Queries That Work

- `metrics.Collector` queries `sessions`, `skills`, `injection_events`, `human_review_samples` — all tables exist with correct column names.
- `storage.Query` builds dynamic WHERE clauses — the column names match the schema.
- `UpdateUsage` subquery references `injection_events.session_outcome` — column exists.
- `ListByDecay` orders by `decay_score` — column exists, index exists.

### 7.3 Missing Schema Support

- No index on `skills(name, library_id)` — `GetLatestByName` does a full scan filtered by name and library_id. Needs an index for any non-trivial skill count.
- No index on `injection_events(ab_group)` — the A/B metrics queries filter by `ab_group`. Will table-scan.
- `session_outcome` column in `injection_events` is never updated by any code in the system. The spec says "When a session ends... update `injection_events.session_outcome`". Nobody does this.

---

## 8. Test Coverage Holes

9069 lines of test code. Not bad. But here's what's **not** tested:

### 8.1 No Integration Tests

There is not a single test that wires `Pipeline` → `SQLiteStore` → real database. Every test uses mocks. This means:
- The `SkillWriter`/`SkillStore` interface compatibility is untested at compile time only.
- The actual SQL queries in `store.Put` are tested in `store_test.go` but never through the pipeline.
- The full flow (Stage1 → Stage2 → Stage3 → Store → verify skill in DB) has never been executed in a test.

### 8.2 No End-to-End CLI Test

`commands_test.go` presumably tests CLI wiring but I'd bet money it doesn't test a real extraction flow with a real SQLite database.

### 8.3 Untested Integration Seams

| Seam | Status |
|---|---|
| Pipeline → SQLiteStore.Put | Untested together |
| Injection → SQLiteStore.Query → SQLiteStore.WriteInjectionEvent | Untested together |
| Decay → SQLiteStore.ListByDecay → SQLiteStore.UpdateDecay | Untested together |
| Metrics → real populated DB | Untested |
| ABInjector → inner Injector → event double-write | Untested |
| serve hooks → pipeline → store | Dead code, untested |

### 8.4 Missing Negative Tests

- What happens when SQLite returns a constraint violation mid-transaction in `Put`? (Patterns insert after skill insert — if pattern insert fails, does the skill row get rolled back?)
- What happens when the embedding API returns 0 dimensions?
- What happens when two concurrent extractions produce the same skill name?

---

## Prioritized Punch List (Ship-Blockers First)

### P0 — Will Break in Production

1. **Persist sessions to the `sessions` table.** Without this, metrics are dead, human review FK fails, stage pass rates can't be computed. Add `SessionStore` methods or extend the pipeline to write sessions.

2. **Compute and store skill embeddings during extraction.** After Stage 3 passes, before `writer.Put()`, call `embedder.Embed()` on the content and set `skill.Embedding`. Without this, injection's cosine similarity is permanently zero.

3. **Fix double injection event writing.** Either remove event writing from the base `injector` when wrapped in `ABInjector`, or don't write events in `ABInjector` when the base already did. Currently every injection creates 2x rows.

4. **Connect serve hooks to something.** `_ = hooks` is not a wiring strategy. Either connect them to the git server's push hooks or remove the dead code.

5. **Implement `InsertReviewSample` on SQLiteStore.** The table exists. The method doesn't. Human review sampling is spec-required.

### P1 — Will Degrade Quality

6. **Wire config YAML to decay.** `decayCmd` ignores YAML config and uses `decay.DefaultConfig()`. Should read from `cfg.Extraction` or a new `cfg.Decay` section.

7. **Add embedding/LLM config to YAML.** API keys from env vars are fine, but model names, retry config, and circuit breaker thresholds should come from config, not hardcoded constants.

8. **Update `ExtractionStatus` on the session record after pipeline runs.** The pipeline knows the result but doesn't write it back. Metrics queries depend on this.

9. **Add missing indexes:** `skills(name, library_id)`, `injection_events(ab_group)`.

10. **Write at least one integration test** that goes Pipeline → SQLiteStore → verify skill in DB with embedding and patterns.

### P2 — Technical Debt

11. **Consolidate the five implicit interfaces** that `SQLiteStore` satisfies. At minimum, document which interfaces it implements. Consider explicit compile-time checks.

12. **Remove or implement `session_outcome` update path.** The column exists, the correlation query references it, but nothing writes to it. Success attribution (§3.5) is entirely unimplemented.

13. **Add `InjectionResponse` skill content.** Currently injection returns only skill IDs and scores. The caller can't build a context window without going back to the store.

14. **Config validation for extraction subsystem.** `config.Validate()` checks thresholds but doesn't validate that the system can actually start (e.g., no check for required API keys, no check that DSN is writable).

15. **Consolidate LLM response parsing.** `parseRubricResponse` and `parseCriticResponse` are 90% identical. Extract a generic JSON-from-LLM parser.

---

## What's Actually Good (Grudgingly)

- **Type system is solid.** The data model is well thought out. `QualityScores`, `SkillRecord`, `SessionRecord` are clean, well-documented, and match the spec.
- **Error wrapping is consistent.** Every `fmt.Errorf` wraps with `%w`. Sentinel errors exist where needed.
- **Circuit breakers exist in both embedding and Stage 3.** Most systems at this stage don't have them at all.
- **Dependency graph is clean.** No cycles, clear data flow direction.
- **The storage layer is surprisingly robust.** WAL mode, integrity checks, proper transaction usage, N+1 query fixes. Whoever wrote this has shipped databases before.
- **Structured logging is everywhere.** `slog` with consistent key-value pairs. Good.
- **The test count is respectable** — 9K lines of tests for the codebase size.

---

## Final Verdict

**Grade: B-**

The individual components are B+ to A- quality. The *system integration* is D+. The packages were clearly built in isolation per the phase plan, and nobody went back to wire the seams. The five ship-blockers in P0 are all integration issues — missing writes, dead hooks, double writes, missing embeddings. None of them are architecturally hard to fix. They're just... not done.

This is the classic "works in unit tests, fails in production" codebase. Every mock is green. The real system would produce skills without embeddings, metrics that show all zeros, and injection events counted twice.

Fix the P0 list. Write one honest end-to-end integration test. Then ship it.

— Jazz

*P.S. I've been doing this for 30 years and I've seen this exact pattern at least 15 times: beautiful packages, no glue. The spec is good. The implementation is good. The integration is absent. At least the fix is straightforward.*
