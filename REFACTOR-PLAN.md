# Mycelium Structural Refactoring Plan

**Reviewer:** Jazz (still grumpy, now with a blueprint)
**Date:** 2026-02-15
**Companion doc:** `TECH-DEBT.md` (bugs & correctness)
**This doc:** Code smells, structural rot, and how to fix it

---

## 1. File-by-File Smell Report

Only files over 200 lines. If your file is under 200 lines, congratulations, you cleared the lowest possible bar.

---

### `internal/storage/store.go` — 765 lines

**Responsibilities (5):**
1. Schema migration & DB setup (constructor, pragmas, ALTER TABLE loop)
2. Skill CRUD (Put, Get, GetLatestByName, Query, UpdateUsage, UpdateDecay, ListByDecay)
3. Session CRUD (GetSession, InsertSession, UpdateSessionResult)
4. Injection event persistence (WriteInjectionEvent, UpdateInjectionOutcome)
5. Human review samples (InsertReviewSample)

Plus helper types (`ScoredSkill`, `SkillQuery`, `InjectionEventRecord`), binary encoding (`float32sToBytes`, `bytesToFloat32s`, `cosineSimilarity`), and SQL scanning utilities.

**Functions that don't belong here:**
- `float32sToBytes`, `bytesToFloat32s`, `cosineSimilarity` — these are math/encoding utilities, not storage logic. They're only used for embedding comparison during `Query`. Move to a shared `internal/vecutil` or keep in storage but in a `vecutil.go` file.
- `InjectionEventRecord` struct — this is a domain type living in the storage layer. Should be in a model package or at minimum in its own file.

**Longest function:** `Query` at ~75 lines. Borderline but acceptable — it's a single query pipeline (filter → load → score → sort → limit). I'd split it only if scoring logic grows.

**God functions (>80 lines):** `NewSQLiteStore` is ~60 lines which is fine. `Put` is ~55 lines. No true god functions, but the *file* is a god object.

---

### `internal/extraction/pipeline.go` — 553 lines

**Responsibilities (4):**
1. Pipeline struct + constructor + interface definitions (`SkillWriter`, `SessionWriter`, `HumanReviewWriter`, `SkillEmbedder`)
2. `Extract` method — the main pipeline orchestration
3. Sampling logic (`maybeSample` + stratified counters)
4. Skill markdown generation (`buildSkillMD`, `skillNameFromClassification`, `kebab`, `titleCase`)

**Functions that don't belong here:**
- `buildSkillMD`, `skillNameFromClassification`, `kebab`, `titleCase` — these are template/formatting functions. They have zero business being in the same file as the pipeline orchestrator. Extract to `skillmd.go`.
- `maybeSample` and the stratified sampling logic — this is a distinct concern (observability/QA). Extract to `sampling.go`.

**Longest function:** `Extract` at ~130 lines. Yes, it should be split. The repetitive stage-invoke-check-reject pattern repeats 3 times with identical logging structure. Extract a `runStage` helper or at minimum break the skill-building block (lines ~250-310) into `buildAndPersistSkill`.

**God functions:** `Extract` at 130 lines. `buildSkillMD` at ~60 lines (acceptable).

---

### `internal/extraction/stage3.go` — 540 lines

**Responsibilities (5):**
1. LLM type definitions (`LLMError`, `LLMCompleteResult`, `LLMClientV2`)
2. Stage3Critic struct + `Evaluate` method
3. Retry logic with backoff (`callWithRetry`, `callLLM`, `isRetryable`, `isRateLimit`, `isTimeout`)
4. Circuit breaker (`checkCircuit`, `recordFailure`, `recordSuccess`)
5. Prompt construction + response parsing (`buildCriticPrompt`, `parseCriticResponse`, `sanitizeDelimiters`, `truncateContent`, `generateNonce`)

**Functions that don't belong here:**
- Circuit breaker methods — duplicated with `embedding.go`. Extract to `internal/circuitbreaker`.
- LLM type definitions (`LLMError`, `LLMCompleteResult`, `LLMClientV2`) — these are shared contracts, not stage3-specific. Move to `internal/llm/types.go` or a `types.go` in extraction.
- `parseCriticResponse` — nearly identical to `parseRubricResponse` in `stage2.go`. Extract generic JSON-from-LLM parser.
- `isRetryable`, `isRateLimit`, `isTimeout` — these are general LLM error classification functions. Move to `internal/llm/errors.go`.

**Longest function:** `buildCriticPrompt` at ~40 lines (fine). `callWithRetry` at ~40 lines (fine). `Evaluate` at ~55 lines (fine). `parseAndValidate` at ~55 lines (fine). No single god function, but the file as a whole is a god *module*.

**God functions:** None individually, but the file has 540 lines of 5 distinct concerns jammed together.

---

### `internal/extraction/stage2.go` — 353 lines

**Responsibilities (4):**
1. Taxonomy definition + validation (`Taxonomy`, `validPatterns`, `ValidPattern`, `ValidateDomain`, `validatePatterns`, `validateCategory`)
2. Stage2Scorer struct + `Score` method
3. Rubric scoring types + composite score calculation
4. LLM prompt building + response parsing (`buildRubricPrompt`, `parseRubricResponse`)

**Functions that don't belong here:**
- `Taxonomy`, `validPatterns`, `ValidPattern`, `ValidateDomain` — these are domain constants shared with `injection`. They belong in a shared `internal/taxonomy` or `internal/model` package, not buried in stage2.
- `parseRubricResponse` — duplicate of `parseCriticResponse`. See above.
- `compositeScore` — used by both stage2 and stage3. Should be a method on `QualityScores` or in a shared location.

**Longest function:** `Score` at ~70 lines. Acceptable — it's a linear pipeline.

**God functions:** None.

---

### `internal/extraction/types.go` — 222 lines

**Responsibilities (3):**
1. Domain types (`SkillRecord`, `SessionRecord`, `QualityScores`, `SkillCategory`, `ExtractionStatus`)
2. Result types (`ExtractionResult`, `Stage1Result`, `Stage2Result`, `Stage3Result`)
3. Injection types (`InjectionRequest`, `InjectionResponse`, `InjectedSkill`, `PromptClassification`, `ValidDomains`)

**What doesn't belong:** Injection types (`InjectionRequest`, `InjectionResponse`, `InjectedSkill`, `PromptClassification`) live in the extraction package but are consumed by the injection package. This is the core of the dependency tangle — `extraction` is doing double duty as both "extraction pipeline" and "shared types package."

---

### `internal/config/config.go` — 404 lines

**Responsibilities (3):**
1. Type definitions (all the `*Config` structs)
2. Load/Save/Validate
3. Path expansion (`expandPath`, `expandPaths`)

This is a config file. It's supposed to have types + loading. **Leave it.** The `expandPath` function is 40 lines of defensive coding for `~` expansion which is mildly annoying but not a structural problem.

---

### `internal/embedding/embedding.go` — 399 lines

**Responsibilities (4):**
1. `Embedder` interface + `Client` struct
2. Retry logic with backoff
3. Circuit breaker (duplicate of stage3's)
4. OpenAI API HTTP transport

**Functions that don't belong here:**
- Circuit breaker (`cbAllow`, `cbRecordSuccess`, `cbRecordFailure`) — identical pattern to stage3. Extract to shared package.

**Longest function:** `EmbedBatch` at ~40 lines. `doRequest` at ~50 lines. Both acceptable.

**God functions:** None individually. File is borderline at 399 lines but has legitimate coupling between retry/circuit-breaker/HTTP.

---

### `internal/gitserver/server.go` — 320 lines

**Responsibilities (3):**
1. Server lifecycle (Start, Stop, waitForReady)
2. SSH command execution (runSSHCommand)
3. Repository CRUD (CreateRepo, ListRepos, DeleteRepo, GetCloneURL, GetConnectionInfo)

**Functions that don't belong here:** `waitForReady` shells out SSH in a loop — but that's inherent to how Soft Serve works. The real smell is that `SessionStartHook` and `SessionEndHook` types are defined here, tying the git server to the extraction domain. These hook types should be defined elsewhere.

**Longest function:** `Start` at ~40 lines. Fine.

**God functions:** None.

---

### `internal/metrics/collector.go` — 335 lines

**Responsibilities (2):**
1. Metrics collection (lots of SQL queries)
2. Statistical functions (z-test, normalCDF)

**Functions that don't belong here:**
- `TwoProportionZTest` and `normalCDF` — pure math functions. Could live in `internal/stats` but honestly at 2 functions it's not worth a package. **Leave it.**

**Longest function:** `Collect` at ~130 lines. This IS a god function. It's 130 lines of sequential SQL queries with no branching, no complexity, just sheer volume. Should be split into `collectSessionMetrics`, `collectStageMetrics`, `collectInjectionMetrics`, `collectQualityMetrics`, `collectDecayMetrics`.

**God functions:** `Collect` (130 lines).

---

### `internal/worker/queue.go` — 269 lines

**Responsibilities (2):**
1. Queue operations (Enqueue, Claim, Complete, Fail, FailPermanent, Depth, RequeueStale)
2. Type definitions (QueueEntry, SessionQueue interface)

Clean file. Operations are cohesive around the queue concept. **Leave it.**

---

### `internal/worker/pool.go` — 220 lines

**Responsibilities (2):**
1. Pool lifecycle (Start, Stop, Stats)
2. Worker loop (run, process, sleep)

Clean. **Leave it.**

---

### `internal/worker/scheduler.go` — 224 lines

**Responsibilities (3):**
1. Scheduler lifecycle (Start, Stop)
2. Periodic tasks (runDecay, runStaleSweep, runStatsLogger)
3. Cron parsing (parseDailyCron, nextDailyDelay)

**Functions that don't belong here:** `parseDailyCron` and `nextDailyDelay` are general-purpose time utilities. But at 30 lines total, extracting them would be over-engineering. **Leave it.**

---

### `cmd/mycelium/serve.go` — 349 lines

**Responsibilities (4):**
1. `runServe` command handler
2. `buildSessionHooks` — wiring injection/extraction into hooks
3. `buildPipeline` — constructing the extraction pipeline
4. `startWorkerSystem` — creating queue + pool + scheduler
5. `waitForShutdown` — graceful shutdown orchestration

This is a composition root. It's supposed to wire things together. The problem isn't the file size — it's that `buildSessionHooks` and `buildPipeline` duplicate LLM/embedder client construction. They both do the `MYCELIUM_LLM_API_KEY` / `OPENAI_API_KEY` dance independently.

**Longest function:** `runServe` at ~60 lines. `buildSessionHooks` at ~60 lines. Both acceptable for a composition root.

---

### `cmd/mycelium/extract.go` — 314 lines

**Responsibilities (4):**
1. `runExtract` command handler
2. `parseSessionFromLog` — heuristic log parser (~100 lines)
3. `openAILLMClient` + `openAIComplete` — LLM HTTP client
4. `storeQuerier` — adapter from store to `SkillQuerier`

**Functions that don't belong here:**
- `openAILLMClient`, `openAIComplete` — used by both `extract.go` and `serve.go`. This is application infrastructure, not CLI command logic. Move to `internal/llm/openai.go`.
- `storeQuerier` — adapter used by both commands. Move to `internal/storage/querier.go` or `internal/llm/querier.go`.
- `parseSessionFromLog` — used by both `extract.go` and `importcmd.go`. Move to `internal/extraction/logparser.go`.

**Longest function:** `parseSessionFromLog` at ~75 lines. Acceptable but doing regex work that should be tested independently.

**God functions:** None.

---

### `cmd/mycelium/queuecmd.go` — 206 lines

**Responsibilities (4):** Four subcommands (stats, list, retry, flush) + helper. Fine for a CLI file.

---

## 2. Package Structure Review

### Packages doing double duty (types + logic)

**`extraction`** — The worst offender. This package is simultaneously:
- The types package (everyone imports it for `SessionRecord`, `SkillRecord`, `InjectionRequest`, etc.)
- The extraction pipeline implementation (stages 1-3 + pipeline orchestration)
- The LLM interface definition package (`LLMClient`, `LLMClientV2`)
- The taxonomy/validation package (`Taxonomy`, `ValidPattern`, `ValidDomains`)

Everything depends on `extraction`. It's the bottom of the dependency graph by accident, not design.

**`storage`** — Types + implementation, but this is more acceptable since `SQLiteStore` is the only implementation and the types (`SkillQuery`, `ScoredSkill`, `InjectionEventRecord`) are genuinely storage-layer concerns. Still, `InjectionEventRecord` is arguable.

### Dependency graph (current)

```
cmd/mycelium
  ├── config
  ├── storage ──────→ extraction
  ├── extraction
  │   └── config, embedding
  ├── embedding
  ├── injection ────→ extraction, storage, embedding
  ├── worker ───────→ extraction, storage, decay
  ├── decay ────────→ extraction
  ├── gitserver ────→ config, extraction
  └── metrics       (just sql)
```

**The problem:** `extraction` is the implicit god package. 8 of 9 packages import it. If you change a type in `extraction/types.go`, you potentially recompile everything.

### Circular-ish dependencies / awkward import chains

No actual circular imports (Go wouldn't compile), but:
- `injection` imports `extraction` for types AND `storage` for `ScoredSkill`/`SkillQuery`. This means injection knows about both the domain model AND the storage layer. The storage types (`ScoredSkill`) leak into injection's API.
- `gitserver` imports `extraction` just for `InjectionRequest`, `InjectionResponse`, `SessionRecord`, and `ExtractionResult` — all type-only imports. This screams "extract a model package."

### New packages that should be created

| Package | Why | What moves there |
|---------|-----|-----------------|
| `internal/model` | Shared domain types | `SkillRecord`, `SessionRecord`, `QualityScores`, `SkillCategory`, `ExtractionStatus`, `ExtractionResult`, `Stage*Result`, `InjectionRequest`, `InjectionResponse`, `InjectedSkill`, `PromptClassification`, `ValidDomains` from `extraction/types.go` |
| `internal/llm` | LLM client abstraction | `LLMClient`, `LLMClientV2`, `LLMError`, `LLMCompleteResult` from `extraction/stage3.go`; `openAILLMClient`, `openAIComplete` from `cmd/`; `isRetryable`, `isRateLimit`, `isTimeout` |
| `internal/llmutil` | Shared LLM response parsing | Generic `ExtractJSON[T]` function replacing `parseCriticResponse`, `parseRubricResponse`, `parseClassificationResponse` |
| `internal/circuitbreaker` | Shared circuit breaker | Deduplicate from `extraction/stage3.go` and `embedding/embedding.go` |
| `internal/taxonomy` | Problem patterns + validation | `Taxonomy`, `validPatterns`, `ValidPattern`, `ValidateDomain`, `ValidDomains` from `extraction/stage2.go` and `extraction/types.go` |

### Packages that are too thin to justify

- `internal/gitserver/binary.go` (16 lines) and `internal/gitserver/keys.go` (73 lines) — these are fine as separate files within the package. Not suggesting merging packages, just noting they're tiny.
- `internal/worker/config.go` (49 lines) — fine, config types belong in their own file.

---

## 3. Concrete Refactoring Tickets

### Structural changes (package moves, file splits)

---

**R1: Extract `internal/model` package from `extraction/types.go`**

- **What:** Move all shared domain types to a dedicated model package.
- **Why:** `extraction` is an implicit types package. 8 packages import it, most just for types. This couples everything to the extraction pipeline's compilation unit. Changing a stage3 implementation detail forces recompilation of `injection`, `decay`, `gitserver`, etc.
- **How:**
  - Create `internal/model/`.
  - Move from `extraction/types.go`: `SkillRecord`, `SessionRecord`, `QualityScores`, `SkillCategory`, `ExtractionStatus` (+ constants), `ExtractionResult`, `Stage1Result`, `Stage2Result`, `Stage3Result`, `InjectionRequest`, `InjectionResponse`, `InjectedSkill`, `PromptClassification`, `ValidDomains`, `ValidateDomain`, `Extractor` interface.
  - Update all imports (mechanical find-replace: `extraction.SessionRecord` → `model.SessionRecord`).
  - `extraction` package retains pipeline logic, stage implementations, and stage-specific types.
- **Size:** L (half day — lots of import changes, all tests need updating)
- **Dependencies:** None (do this first)
- **Risk:** Medium. Pure mechanical refactor but touches every file. Do in one commit, run tests.

---

**R2: Extract `internal/llm` package**

- **What:** Move LLM client interfaces, error types, and the OpenAI implementation out of `extraction` and `cmd/`.
- **Why:** `LLMClient` is defined in `extraction/stage2.go`. `LLMClientV2`, `LLMError`, `LLMCompleteResult` are in `extraction/stage3.go`. The actual OpenAI implementation lives in `cmd/mycelium/extract.go` (!!). Three different locations for one concern.
- **How:**
  - Create `internal/llm/`.
  - `internal/llm/client.go`: `LLMClient`, `LLMClientV2`, `LLMCompleteResult` interfaces/types.
  - `internal/llm/errors.go`: `LLMError`, `isRetryable`, `isRateLimit`, `isTimeout`.
  - `internal/llm/openai.go`: Move `openAILLMClient` + `openAIComplete` from `cmd/mycelium/extract.go`. Add constructor: `NewOpenAIClient(apiKey, model string) *OpenAIClient`.
  - Remove `defaultHTTPClient` global from `cmd/mycelium/helpers.go`; put HTTP client inside `OpenAIClient` struct.
- **Size:** M (1-2 hours)
- **Dependencies:** R1 (model package should exist first so llm doesn't import extraction)
- **Risk:** Low. Moving code, not changing behavior.

---

**R3: Extract `internal/circuitbreaker` package**

- **What:** Deduplicate the circuit breaker from `extraction/stage3.go` and `embedding/embedding.go`.
- **Why:** Two independent implementations of the same pattern: consecutive failure counting, open/half-open/closed states, exponential backoff on re-open. ~160 lines maintained twice. They'll drift apart.
- **How:**
  - Create `internal/circuitbreaker/breaker.go`.
  - Struct: `Breaker` with `Config` (threshold, base duration, max duration).
  - Methods: `Allow() error`, `RecordSuccess()`, `RecordFailure()`, `State() string`.
  - Injectable clock for testing.
  - Replace `stage3Critic`'s `mu/consecutiveFail/circuitOpenAt/openDuration` with `*circuitbreaker.Breaker`.
  - Replace `embedding.Client`'s `cbState/cbFailures/cbOpenedAt/cbCurrentOpenDur` with `*circuitbreaker.Breaker`.
- **Size:** M (1-2 hours)
- **Dependencies:** None (independent of R1/R2)
- **Risk:** Low. Behavior-preserving extraction. Test both consumers.

---

**R4: Extract `internal/llmutil` — shared JSON-from-LLM parser**

- **What:** Replace 3 copy-pasted JSON extraction functions with one generic implementation.
- **Why:** `parseCriticResponse` (stage3.go:267-295), `parseRubricResponse` (stage2.go:204-230), and `parseClassificationResponse` (injector.go:217-228) all implement the same 4-strategy JSON extraction cascade. When you fix a parsing edge case, you'll fix it in one place and forget the other two.
- **How:**
  - Create `internal/llmutil/json.go`.
  - Function: `ExtractJSON[T any](resp string) (T, error)` using the 4-strategy cascade (direct → ```json → ``` → first-{-to-last-}).
  - Replace all three callers.
  - Delete the three duplicate functions.
- **Size:** S (30 min)
- **Dependencies:** None
- **Risk:** Low. The strategies are identical. Test with the existing test cases from all three callers.

---

**R5: Extract `internal/taxonomy` package**

- **What:** Move `Taxonomy`, `validPatterns`, `ValidPattern`, `ValidateDomain`, `ValidDomains` to a shared package.
- **Why:** These are defined in `extraction/stage2.go` but consumed by `injection/injector.go`. The injection package imports all of `extraction` just to get at a string slice and a validation function. This is a category error — taxonomy is domain knowledge, not extraction logic.
- **How:**
  - Create `internal/taxonomy/taxonomy.go`.
  - Move: `Taxonomy` slice, `validPatterns` map, `init()`, `ValidPattern()`, `ValidDomains` map, `ValidateDomain()`.
  - Update imports in `extraction/stage2.go` and `injection/injector.go`.
- **Size:** S (30 min)
- **Dependencies:** R1 (cleaner if model types exist, but not strictly required)
- **Risk:** Very low.

---

**R6: Split `internal/storage/store.go` into focused files**

- **What:** Split the 765-line store.go into 5 files by domain.
- **Why:** Finding anything in this file requires scrolling past 4 unrelated domains. The `SQLiteStore` struct has 19 methods spanning skills, sessions, injection events, and review samples.
- **How:**
  - `store.go` (120 lines): `SQLiteStore` struct, `NewSQLiteStore`, `Close`, `DB`, schema migration, `skillColumns` const.
  - `skill_store.go` (350 lines): `Put`, `Get`, `GetLatestByName`, `Query`, `UpdateUsage`, `UpdateDecay`, `ListByDecay`, `loadPatterns*`, `loadEmbedding*`, `scanSkillFrom`.
  - `session_store.go` (80 lines): `GetSession`, `InsertSession`, `UpdateSessionResult`.
  - `injection_store.go` (60 lines): `WriteInjectionEvent`, `UpdateInjectionOutcome`, `InsertReviewSample`, `InjectionEventRecord`.
  - `helpers.go` (80 lines): `nullString`, `nullTime`, `float32sToBytes`, `bytesToFloat32s`, `cosineSimilarity`, `scanner` interface.
  - All files stay in `package storage`. Just file splits, no behavioral change.
- **Size:** M (1 hour)
- **Dependencies:** None
- **Risk:** Very low. File splits within same package.

---

**R7: Split `internal/extraction/pipeline.go`**

- **What:** Extract skill markdown generation and sampling logic into separate files.
- **Why:** 553 lines, 4 responsibilities. Markdown template generation has nothing to do with pipeline orchestration.
- **How:**
  - `skillmd.go`: `buildSkillMD`, `skillNameFromClassification`, `kebab`, `titleCase` (~130 lines).
  - `sampling.go`: `maybeSample`, `cryptoRandIntn`, `RandIntn` type (~70 lines).
  - `pipeline.go` shrinks to ~350 lines: struct, constructor, `Extract`, `updateSessionStatus`.
- **Size:** S (30 min)
- **Dependencies:** None
- **Risk:** Very low.

---

**R8: Split `internal/extraction/stage3.go`**

- **What:** Separate prompt construction from LLM calling from retry logic.
- **Why:** 540 lines, 5 responsibilities. After R3 (circuit breaker extraction) and R4 (JSON parser extraction), this file should already shrink by ~120 lines. The remaining split separates prompt building from evaluation logic.
- **How:**
  - After R3 and R4 are done:
  - `stage3_prompt.go`: `buildCriticPrompt`, `sanitizeDelimiters`, `truncateContent`, `generateNonce` (~60 lines).
  - `stage3.go` keeps: `stage3Critic`, `Evaluate`, `parseAndValidate`, `callWithRetry`, `callLLM`, `estimateTokens`, score helpers (~300 lines).
- **Size:** S (30 min)
- **Dependencies:** R3, R4
- **Risk:** Very low.

---

**R9: Move `storeQuerier` and `parseSessionFromLog` out of `cmd/`**

- **What:** Move shared application logic from CLI commands to internal packages.
- **Why:** `storeQuerier` is an adapter used by both `extract.go` and `serve.go`. `parseSessionFromLog` is used by both `extract.go` and `importcmd.go`. Neither belongs in `cmd/`.
- **How:**
  - `internal/storage/querier.go`: Move `storeQuerier` → `storage.NewSkillQuerier(store) extraction.SkillQuerier`. (After R1, this returns `model.SkillQuerier`.)
  - `internal/extraction/logparser.go`: Move `parseSessionFromLog` and its regex patterns. It's extraction-adjacent logic (parsing session metadata).
- **Size:** S (30 min)
- **Dependencies:** R1 (for clean model types), R2 (so storeQuerier doesn't depend on extraction)
- **Risk:** Low.

---

### Deduplication

---

**R10: Deduplicate embedder/LLM client construction in `serve.go`**

- **What:** Extract a `buildClients` helper that creates embedder + LLM client once.
- **Why:** `buildSessionHooks` and `buildPipeline` both independently read env vars, create embedding configs, and construct clients. Same 10 lines of `MYCELIUM_EMBEDDING_API_KEY`/`OPENAI_API_KEY` dance, twice.
- **How:**
  - After R2: Create a factory function in `serve.go` (or `cmd/mycelium/clients.go`): `buildClients(cfg) (embedding.Embedder, llm.Client, error)`. Call once, pass to both `buildSessionHooks` and `buildPipeline`.
- **Size:** S (30 min)
- **Dependencies:** R2
- **Risk:** Very low.

---

**R11: Deduplicate `decayConfigFromYAML` bridging**

- **What:** Eliminate the `config.DecayConfig` → `decay.Config` translation.
- **Why:** `config.DecayConfig` and `decay.Config` are identical structs with identical field names. `decayConfigFromYAML` in `cmd/mycelium/decay.go` manually copies each field with zero-value fallback. This is maintenance theater.
- **How:** Either (a) make `decay.Config` the one config struct and use it directly in `config.Config`, or (b) add a `decay.ConfigFromPartial(partial)` method that handles defaults. Option (a) is cleaner — `config.Config.Decay` becomes `decay.Config` directly.
- **Size:** S (30 min)
- **Dependencies:** None
- **Risk:** Low. YAML tag compatibility needs checking.

---

**R12: Deduplicate session status update logic**

- **What:** The "determine rejected stage + reason" logic appears in both `pipeline.go:updateSessionStatus` and `queue.go:Complete`.
- **Why:** Two places compute `rejectedStage`/`rejectionReason` from the same `ExtractionResult` struct. If you add a Stage 4, you need to update both.
- **How:** Add `ExtractionResult.RejectionInfo() (stage int, reason string)` method in the model package. Both callers use it.
- **Size:** S (20 min)
- **Dependencies:** R1
- **Risk:** Very low.

---

### Polish

---

**R13: Split `metrics/collector.go:Collect` into sub-methods**

- **What:** Break the 130-line `Collect` method into focused helpers.
- **Why:** It's 130 lines of sequential SQL queries. Reading it requires scrolling through 5 unrelated metric domains. Adding a new metric means editing a 130-line function.
- **How:** Extract: `collectSessionCounts(m)`, `collectStagePassRates(m)`, `collectHumanReview(m)`, `collectInjectionMetrics(m)`, `collectQualityAndDecay(m)`. `Collect` becomes a 20-line orchestrator.
- **Size:** S (30 min)
- **Dependencies:** None
- **Risk:** Very low.

---

**R14: Remove unused `db` struct tags from `extraction/types.go`**

- **What:** Remove the `db:"..."` tags that do nothing.
- **Why:** Nobody uses `sqlx` or any struct scanner that reads `db` tags. All scanning is manual via `scanSkillFrom`. These tags are documentation theater that implies a dependency that doesn't exist.
- **How:** Delete every `db:"..."` tag from `SkillRecord`, `SessionRecord`, `QualityScores`.
- **Size:** S (10 min)
- **Dependencies:** R1 (do after types move to model package)
- **Risk:** None.

---

**R15: Remove `reflect`-based `ignoreNil` from `cmd/mycelium/worker.go`**

- **What:** Replace the generic `ignoreNil[T]` with explicit nil checks.
- **Why:** Using `reflect` to check if an interface is nil is a Go anti-pattern. The function exists to handle two call sites. Just write `if sched != nil { sched.Stop(ctx) }`.
- **How:** Inline the nil checks at the two call sites. Delete `ignoreNil`. Remove `reflect` import.
- **Size:** S (10 min)
- **Dependencies:** None
- **Risk:** None.

---

**R16: Add `SessionStartHook`/`SessionEndHook` type aliases to model package**

- **What:** Move hook function types out of `gitserver`.
- **Why:** `gitserver.SessionStartHook` and `gitserver.SessionEndHook` reference `extraction.InjectionRequest`, `extraction.InjectionResponse`, etc. This couples the git server package to the extraction domain. The hook types should live in the model/domain layer.
- **How:** After R1, define hook types in `internal/model/hooks.go`. `gitserver.Server.SetSessionHooks` accepts `model.SessionStartHook` and `model.SessionEndHook`.
- **Size:** S (20 min)
- **Dependencies:** R1
- **Risk:** Low.

---

## 4. Suggested Final Package Layout

After all refactoring is done:

```
internal/
├── circuitbreaker/
│   └── breaker.go              # Shared circuit breaker (from R3)
│
├── config/
│   └── config.go               # Unchanged (404 lines is fine for config)
│
├── decay/
│   └── runner.go               # Unchanged (199 lines, clean)
│
├── embedding/
│   └── embedding.go            # Slimmer: circuit breaker extracted (R3)
│                                 # ~300 lines after extraction
│
├── extraction/
│   ├── pipeline.go             # Pipeline struct + Extract (~350 lines)
│   ├── sampling.go             # maybeSample, stratified logic (R7)
│   ├── skillmd.go              # buildSkillMD, kebab, titleCase (R7)
│   ├── logparser.go            # parseSessionFromLog (R9)
│   ├── stage1.go               # Unchanged (76 lines)
│   ├── stage2.go               # Slimmer: taxonomy extracted (R5), JSON parser extracted (R4)
│   │                             # ~200 lines
│   ├── stage3.go               # Slimmer: circuit breaker (R3), JSON parser (R4),
│   │                             # prompt (R8) extracted. ~300 lines
│   └── stage3_prompt.go        # Prompt construction (R8)
│
├── gitserver/
│   ├── binary.go               # Unchanged
│   ├── keys.go                 # Unchanged
│   └── server.go               # Hook types moved to model (R16)
│
├── injection/
│   ├── injector.go             # Unchanged (uses llmutil for parsing)
│   └── ab.go                   # Unchanged
│
├── llm/
│   ├── client.go               # LLMClient, LLMClientV2 interfaces (R2)
│   ├── errors.go               # LLMError, isRetryable, isRateLimit, isTimeout (R2)
│   └── openai.go               # OpenAI implementation (R2)
│
├── llmutil/
│   └── json.go                 # ExtractJSON[T] generic parser (R4)
│
├── metrics/
│   └── collector.go            # Split Collect into sub-methods (R13)
│
├── model/
│   ├── skill.go                # SkillRecord, QualityScores, SkillCategory (R1)
│   ├── session.go              # SessionRecord, ExtractionStatus (R1)
│   ├── result.go               # ExtractionResult, Stage*Result (R1)
│   ├── injection.go            # InjectionRequest/Response, InjectedSkill (R1)
│   └── hooks.go                # SessionStartHook, SessionEndHook (R16)
│
├── storage/
│   ├── store.go                # Constructor, Close, DB, migrations (~120 lines) (R6)
│   ├── skill_store.go          # Skill CRUD (~350 lines) (R6)
│   ├── session_store.go        # Session CRUD (~80 lines) (R6)
│   ├── injection_store.go      # Injection events + review samples (~60 lines) (R6)
│   ├── querier.go              # SkillQuerier adapter (R9)
│   └── helpers.go              # SQL helpers, vec math (~80 lines) (R6)
│
├── taxonomy/
│   └── taxonomy.go             # Taxonomy list, ValidPattern, ValidDomains (R5)
│
└── worker/
    ├── config.go               # Unchanged
    ├── pool.go                 # Unchanged
    ├── queue.go                # Unchanged
    └── scheduler.go            # Unchanged
```

### Dependency graph (after refactoring)

```
cmd/mycelium
  ├── config
  ├── model (types only)
  ├── llm
  ├── storage ──────→ model
  ├── extraction ───→ model, llm, llmutil, taxonomy, circuitbreaker, embedding
  ├── embedding ────→ circuitbreaker
  ├── injection ────→ model, llm, llmutil, taxonomy, storage, embedding
  ├── worker ───────→ model, extraction, storage, decay
  ├── decay ────────→ model
  ├── gitserver ────→ config, model
  └── metrics       (just sql)
```

Key improvement: `model` replaces `extraction` as the base types package. Now `decay`, `gitserver`, and `metrics` don't import the extraction pipeline at all — they import a thin types package that almost never changes.

---

## Execution Order

```
Phase 1 (structural):  R1 → R2 → R5 → R6
Phase 2 (dedup):       R3 → R4 → R7 → R8
Phase 3 (moves):       R9 → R10 → R16
Phase 4 (polish):      R11 → R12 → R13 → R14 → R15
```

Phase 1 is the big bang — `R1` especially touches everything. Do it on a feature branch with a single commit and run the full test suite. Phases 2-4 are safe to do incrementally, one PR each.

Total estimated effort: ~2 days for someone who knows the codebase. ~3-4 days for someone who doesn't.

---

## Honest Assessment

This codebase doesn't need *surgery*. It needs *tidying*. The architecture is fundamentally sound — clear pipeline stages, proper interfaces at consumption points, clean dependency injection in constructors. The problems are:

1. **One package grew into the de facto types package** (extraction) — fix with R1
2. **Two copy-pasted patterns** (circuit breaker, JSON parser) — fix with R3, R4
3. **Application infrastructure lives in cmd/** (LLM client) — fix with R2
4. **Big files that are big because nobody split them** — fix with R6, R7, R8

None of this is architectural rot. It's just the natural entropy of a codebase that grew fast. The bones are good. I'm grudgingly admitting this.

Don't try to do everything at once. R1 is the linchpin — once the model package exists, everything else falls into place incrementally.

---

*Still not impressed, but at least the patient doesn't need a transplant. Just physical therapy.*

*— Jazz, Feb 2026*
