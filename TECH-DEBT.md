# Kinoko Tech Debt Audit

**Reviewer:** Jazz (30 years in, seen it all, impressed by nothing)
**Date:** 2026-02-15
**Codebase:** ~7,300 LOC across 32 Go source files
**Verdict:** Surprisingly not terrible for what it is. But oh boy, do I have notes.

---

## Severity Key

| Rating | Meaning |
|--------|---------|
| **P0** | Fix now — bug, security issue, or data loss risk |
| **P1** | Fix soon — correctness concern or maintainability blocker |
| **P2** | Fix eventually — design smell, will bite you later |
| **P3** | Nice to have — style, idiom, polish |

---

## A. Files That Are Too Big

### 1. `internal/storage/store.go` — 765 lines (P2)

This file is a classic "everything and the kitchen sink" storage layer. It contains:
- Two interface definitions (`SessionStore`, `SkillStore`)
- The `SQLiteStore` struct + constructor with inline migrations
- Full CRUD for skills (Put, Get, GetLatestByName, Query)
- Full CRUD for sessions (GetSession, InsertSession, UpdateSessionResult)
- Injection event operations (WriteInjectionEvent, UpdateInjectionOutcome)
- Human review sample operations (InsertReviewSample)
- Decay operations (UpdateDecay, ListByDecay)
- Helper types (ScoredSkill, SkillQuery, InjectionEventRecord)
- Binary encoding helpers (float32sToBytes, bytesToFloat32s, cosineSimilarity)
- SQL scan helpers

**Suggested split:**
- `store.go` — constructor, Close, DB, schema migration (~120 lines)
- `skill_store.go` — Put, Get, GetLatestByName, Query, UpdateUsage, UpdateDecay, ListByDecay, loadPatterns*, loadEmbedding* (~350 lines)
- `session_store.go` — GetSession, InsertSession, UpdateSessionResult (~80 lines)
- `injection_store.go` — WriteInjectionEvent, UpdateInjectionOutcome, InsertReviewSample, InjectionEventRecord (~60 lines)
- `helpers.go` — scanSkillFrom, nullString, nullTime, float32sToBytes, bytesToFloat32s, cosineSimilarity (~100 lines)

### 2. `internal/extraction/pipeline.go` — 553 lines (P2)

Contains the Pipeline struct, Extract method, sampling logic, AND the entire `buildSkillMD` template generator plus `kebab`/`titleCase` string utilities.

**Suggested split:**
- `pipeline.go` — Pipeline struct, NewPipeline, Extract, updateSessionStatus (~250 lines)
- `sampling.go` — maybeSample, stratified sampling logic (~60 lines)
- `skillmd.go` — buildSkillMD, skillNameFromClassification, kebab, titleCase (~120 lines)

### 3. `internal/extraction/stage3.go` — 540 lines (P2)

The LLM critic + circuit breaker + retry logic + prompt builder + JSON parser all in one file.

**Suggested split:**
- `stage3.go` — stage3Critic struct, Evaluate, parseAndValidate (~200 lines)
- `stage3_retry.go` — callWithRetry, callLLM, isRetryable, isRateLimit, isTimeout (~100 lines)
- `stage3_circuit.go` — circuit breaker methods (~50 lines)
- `stage3_prompt.go` — buildCriticPrompt, parseCriticResponse, truncateContent, sanitizeDelimiters (~120 lines)

### 4. `internal/config/config.go` — 404 lines (P3)

Honestly, for a config file this isn't that bad. The `expandPath` function is overly complex for what it does (40 lines to expand `~`), but splitting this would create more confusion than it solves. **Leave it.** Grudgingly.

### 5. `internal/embedding/embedding.go` — 399 lines (P3)

Embedding client + circuit breaker + retry logic. Similar to stage3 — the circuit breaker is duplicated (see Section C). Could extract the circuit breaker into a shared package but at 399 lines it's borderline. **Leave it** but see the duplication note below.

### 6. `cmd/kinoko/serve.go` — 349 lines (P3)

Contains `buildSessionHooks`, `buildPipeline`, `startWorkerSystem`, `runServe`, and `waitForShutdown`. It's a composition root — these are supposed to be big. The `waitForShutdown` function is cleanly separated. **Acceptable.**

### 7. `cmd/kinoko/extract.go` — 314 lines (P3)

Half of this is `parseSessionFromLog` which is a heuristic log parser with regex. Could be its own package if other commands need it. `openAILLMClient` and `openAIComplete` are here AND used by `serve.go` — they're in `helpers.go` effectively but defined here. See duplication note.

---

## B. Structs That Do Too Much

### 1. `SQLiteStore` — God Object (P1)

**Methods:** Put, Get, GetLatestByName, Query, UpdateUsage, WriteInjectionEvent, UpdateInjectionOutcome, GetSession, InsertSession, UpdateSessionResult, InsertReviewSample, UpdateDecay, ListByDecay, loadPatterns, loadPatternsMulti, loadEmbedding, loadEmbeddingsMulti, Close, DB

That's **19 methods** spanning 4 different domain concerns (skills, sessions, injection events, review samples). This struct implements `SkillStore`, `SessionStore`, `SkillWriter`, `InjectionEventWriter`, AND `HumanReviewWriter`. It's the Swiss Army knife nobody asked for.

**Recommendation:** Even if the underlying `*sql.DB` is shared, wrap it in focused structs:
```go
type SkillRepository struct { db *sql.DB }
type SessionRepository struct { db *sql.DB }
type InjectionRepository struct { db *sql.DB }
```
Each implements only its interface. The constructor can return all three from the same DB handle.

### 2. `stage3Critic` — Mixed Concerns (P2)

This struct owns: LLM calling, retry logic with backoff, circuit breaker state, prompt construction, response parsing, and verdict logic. That's at least 3 responsibilities jammed into one type.

### 3. `Pipeline` — Borderline (P3)

The Pipeline struct has 11 fields including two sampling counters (`extractedSamples`, `rejectedSamples`) that are **not thread-safe**. If `Extract` is called concurrently, those counters race. See Section D.

---

## C. Refactoring Opportunities

### 1. **Duplicated Circuit Breaker** (P1)

`internal/embedding/embedding.go` and `internal/extraction/stage3.go` both implement their own circuit breakers from scratch. Same pattern: consecutive failure counting, open/half-open/closed states, exponential backoff on re-open.

**Fix:** Extract a `internal/circuitbreaker` package. Both can use it. This is ~80 lines of shared code you're maintaining twice.

### 2. **Duplicated JSON-from-LLM Parsing** (P1)

`parseCriticResponse` (stage3.go:267-295) and `parseRubricResponse` (stage2.go:204-230) are **nearly identical**. Both try: raw parse → ```json block → ``` block → first-{-to-last-}. Same 4-strategy cascade, copy-pasted.

`parseClassificationResponse` (injector.go:217-228) is a simplified version of the same.

**Fix:** One generic `extractJSON[T any](resp string) (T, error)` function in a shared `internal/llmutil` package.

### 3. **`openAILLMClient` Defined in cmd/** (P2)

The `openAILLMClient` struct and `openAIComplete` function live in `cmd/kinoko/extract.go` but are used by `serve.go` too. This is application-level code that should be in `internal/llm/` or `internal/openai/`.

The `storeQuerier` adapter also lives in `cmd/` — same problem.

### 4. **Inconsistent Error Wrapping** (P2)

Most of the codebase uses `fmt.Errorf("context: %w", err)` consistently — **grudging credit**. But there are exceptions:

- `store.go:115` — migration errors use `strings.Contains(err.Error(), "duplicate column")` instead of checking a proper error type. Fragile string matching.
- `store.go:167` — duplicate detection via `strings.Contains(err.Error(), "UNIQUE constraint")`. Same problem.

SQLite error handling by string matching is a ticking time bomb when you upgrade the driver.

### 5. **Inconsistent Context Usage** (P2)

- `metrics/collector.go` — `Collect()` takes no context. Every single DB query inside uses the implicit background context. If the DB is slow or the caller wants cancellation, tough luck.
- `decay/runner.go:RunCycle` — properly uses context, good.
- `gitserver/server.go:runSSHCommand` — no context at all. `exec.Command` without `CommandContext` means you can't cancel SSH operations.

### 6. **Dead/Unused Exports** (P3)

- `extraction.ValidateDomain` is exported but only called from `injection/injector.go`. Could be unexported or the validation map moved.
- `extraction.Taxonomy` is exported for injection to share. Fine, but the comment should say so. (It does. OK.)
- `extraction.ValidPattern` — exported, used by injection. Fine.
- `pkg/skill/Skill.Version` validation enforces `Version == 1` always (skill.go:188). But `extraction/pipeline.go:259` hardcodes `Version: 1` and `types.go` has Version as `int`. If version is always 1, why is it an int? If it's meant to increment, why does validation reject non-1?

### 7. **Missing Interfaces Where Needed** (P2)

- `gitserver.Server` has no interface. Testing anything that depends on the git server requires a real Soft Serve binary. Even `SetSessionHooks` operates on the concrete struct.
- `metrics.Collector` has no interface. Can't mock metrics collection.
- `worker.SQLiteQueue` — good, `SessionQueue` interface exists. ✓

### 8. **Package Dependency Tangle** (P3)

`extraction` package defines types used by everyone (`SessionRecord`, `SkillRecord`, `InjectionRequest`, `InjectionResponse`, etc.). This makes `extraction` a de facto `types` package. Meanwhile, `storage` imports `extraction` for `SkillRecord` and `SessionRecord`, and `injection` imports both `extraction` and `storage`. 

The dependency graph:
```
cmd/kinoko → config, storage, extraction, embedding, injection, worker, decay, gitserver, metrics
injection → extraction, storage, embedding
worker → extraction, storage, decay
storage → extraction
decay → extraction
metrics → (just sql)
```

`extraction` is the implicit types package. Either accept it and rename it `model` or `domain`, or extract the shared types into a `internal/model` package. Currently the package is doing double duty: types AND pipeline logic.

---

## D. Bugs / Latent Issues

### 1. **Race Condition in Pipeline Sampling Counters** (P0)

`pipeline.go:52-53`:
```go
extractedSamples int
rejectedSamples  int
```

These counters are read and written in `maybeSample()` without any synchronization. If `Pipeline.Extract` is called concurrently (which the worker pool does — multiple goroutines can call the same pipeline), these counters race.

**Fix:** Use `atomic.Int64` or protect with a mutex.

### 2. **File Write After DB Commit — Inconsistency Window** (P1)

`store.go:186-193`:
```go
// Write SKILL.md body to disk AFTER commit to avoid orphaned files on rollback.
// If this fails, the skill exists in DB without a file — degraded but detectable.
```

The comment acknowledges this! But there's no detection mechanism. No health check, no startup scan, no reconciliation. The skill will be in the DB, queries will return it, injection will try to use it, and there's no file. The comment says "detectable" but nobody's detecting.

**Fix:** Either make the file write part of the transaction (write to temp, commit, rename — atomic on most filesystems), or add a startup consistency check, or at minimum log a P0 error when this happens.

### 3. **Unbounded Query in `loadPatternsMulti` / `loadEmbeddingsMulti`** (P1)

`store.go:559-571`:
```go
func (s *SQLiteStore) loadPatternsMulti(ctx context.Context, skillIDs []string) (map[string][]string, error) {
    placeholders := make([]string, len(skillIDs))
    args := make([]any, len(skillIDs))
    for i, id := range skillIDs {
        placeholders[i] = "?"
        args[i] = id
    }
    rows, err := s.db.QueryContext(ctx,
        `SELECT skill_id, pattern FROM skill_patterns WHERE skill_id IN (`+strings.Join(placeholders, ",")+`)`, args...)
```

If `Query()` returns 10,000 candidate skills (the `Limit` field is optional and could be 0), this builds an IN clause with 10,000 placeholders. SQLite has a default `SQLITE_MAX_VARIABLE_NUMBER` of 999. This will blow up.

**Fix:** Batch the IN clause in groups of ~500, or use a temp table.

### 4. **No Timeout on Pipeline.Extract** (P1)

The `Extract` method has no overall timeout. Stage 2 calls an embedding API + LLM API. Stage 3 calls an LLM API with retry (up to 5 retries with 30s timeouts = 150s worst case). Total worst case: potentially minutes with no deadline. The worker pool uses a 60s detached context (`context.WithTimeout(context.Background(), 60*time.Second)`) which could expire mid-extraction.

`pool.go:148`:
```go
dbCtx, dbCancel := context.WithTimeout(context.Background(), 60*time.Second)
```

This 60s timeout covers file read + session fetch + full extraction pipeline. But stage3 alone can retry for 150s. The context will cancel mid-extraction, leaving partial state.

**Fix:** Either extend the timeout or make the pipeline respect the context deadline properly.

### 5. **Error Swallowing in Session Insert** (P2)

`pipeline.go:126-130`:
```go
if err := p.sessions.InsertSession(ctx, &session); err != nil {
    p.log.Error("failed to insert session", "session_id", session.ID, "error", err)
    // Non-fatal: continue extraction even if session persistence fails.
}
```

If the session insert fails, extraction continues, but `updateSessionStatus` later will try to UPDATE a row that was never INSERTed. That UPDATE silently affects 0 rows. The session's extraction result is lost.

### 6. **`Queue.Enqueue` Inserts Duplicate Sessions** (P2)

`queue.go:69-93` — `Enqueue` does an INSERT INTO sessions but doesn't check if a session with that ID already exists. If the same session log is imported twice (via `kinoko import`), the second insert will fail with a UNIQUE constraint violation — but the log file was already written to disk and won't be cleaned up in the error path (it is cleaned up for some errors but not this one, because the error happens at `tx.ExecContext` and the `os.Remove` is only called in certain branches).

Actually wait — looking again, `os.Remove(logPath)` IS called on exec error. But the error message will be confusing ("insert session: UNIQUE constraint failed") with no indication it's a duplicate.

### 7. **`noopDecayWriter` Fakes Success in Dry Run** (P3)

`decay.go:109-112` — The dry-run mode uses a no-op writer, which means `RunCycle` thinks it successfully persisted changes. The counts are correct (it still counts demoted/deprecated), but if `RunCycle` ever adds post-write verification, dry-run will silently lie.

### 8. **Missing `created_at` Column in Queue INSERT** (P2)

`queue.go:86` — The INSERT into sessions doesn't include a `created_at` column, but the `Claim` query (line 128) orders by `created_at ASC`. If the schema defaults `created_at` to NULL, ordering is undefined for queued sessions. If it defaults to CURRENT_TIMESTAMP, it's fine — but I don't see the schema file to verify.

### 9. **`waitForReady` Shells Out SSH in a Loop** (P2)

`gitserver/server.go:141-161` — The readiness check spawns a new `ssh` subprocess every second for up to 30 seconds. Each one does key exchange, auth, and command execution. This is expensive, noisy (generates host key warnings), and racey with the server startup. A simple TCP dial would suffice for "is the port open?"

---

## E. Go Idiom Violations

### 1. **`reflect` Usage for Nil Check** (P2)

`cmd/kinoko/worker.go:78-87`:
```go
func ignoreNil[T any](v T, fn func(T) error) error {
    rv := reflect.ValueOf(&v).Elem()
    switch rv.Kind() {
    case reflect.Ptr, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
        if rv.IsNil() {
            return nil
        }
    }
    return fn(v)
}
```

Using `reflect` to check if an interface is nil is a code smell. Just check `if sched != nil` and `if pool != nil` at the call site. This is Go, not Java. We don't need generic null-safety wrappers.

### 2. **`panic` in `cryptoRandIntn`** (P2)

`pipeline.go:108`:
```go
panic(fmt.Sprintf("crypto/rand failed: %v", err))
```

The comment says "Entropy exhaustion is catastrophic." True, but panicking in library code is a Go anti-pattern. Return an error or use `log.Fatal`. The caller (`maybeSample`) is a non-critical sampling function — crashing the entire process because random sampling failed is disproportionate.

### 3. **Package-Level `var` for HTTP Client** (P3)

`cmd/kinoko/helpers.go:10`:
```go
var defaultHTTPClient = &http.Client{Timeout: 60 * time.Second}
```

This is shared mutable global state. It works because `http.Client` is concurrent-safe, but it means tests can't inject a mock transport without races. Pass the client through constructors instead.

### 4. **Interface Definitions in Consumer Packages** (P3 — Actually Good)

The codebase defines small interfaces where they're consumed (`SkillWriter` in extraction, `SkillReader`/`SkillWriter` in decay, `SessionQueue` in worker). This is idiomatic Go. **Grudging acknowledgment: this is done correctly.**

### 5. **`init()` Function in `stage2.go`** (P3)

```go
func init() {
    validPatterns = make(map[string]bool, len(Taxonomy))
    for _, p := range Taxonomy {
        validPatterns[p] = true
    }
}
```

`init()` is fine here but the computed map should just be built inline at package level. Not worth changing, but it adds hidden initialization order dependency.

### 6. **`db` Tag on Struct Fields** (P3)

`types.go` uses `db:"..."` struct tags on `SkillRecord` and `SessionRecord`, but nobody uses a struct scanner like `sqlx`. The actual scanning is manual (`scanSkillFrom`). The tags are documentation theater — they do nothing.

**Fix:** Remove them or actually use `sqlx`.

### 7. **No `errors.New` Sentinel Errors** (P3)

Most error creation uses `fmt.Errorf` which is fine, but the codebase is inconsistent:
- `storage` has proper sentinels (`ErrNotFound`, `ErrDuplicate`) ✓
- `worker` has `ErrBackpressure` using `fmt.Errorf` instead of `errors.New` — it works but `fmt.Errorf` allocates more
- `embedding` has `ErrCircuitOpen` ✓
- `extraction/stage3` has `ErrCircuitOpen` — wait, there are TWO `ErrCircuitOpen` sentinels in different packages. They're not the same error. `errors.Is` will behave differently depending on which package you import.

---

## F. Additional Observations

### Things That Are Actually Good (I Can't Believe I'm Writing This)

1. **Consistent `defer rows.Close()`** — Every query that returns rows properly closes them. I checked. Every. Single. One.
2. **WAL mode + busy_timeout** — SQLite is configured correctly for concurrent access.
3. **Bulk loading to fix N+1** — `loadPatternsMulti` and `loadEmbeddingsMulti` show someone thought about query performance.
4. **Transaction usage** — `Put` uses a transaction correctly, with deferred rollback.
5. **Graceful shutdown ordering** — scheduler → pool → server → store. Correct.
6. **Circuit breaker with exponential backoff on re-open** — Both implementations (stage3 and embedding) handle the half-open → re-open escalation correctly.
7. **Content truncation respects UTF-8 boundaries** — `truncateContent` in stage3.go backs off incomplete runes. Someone's been bitten before.
8. **Prompt injection mitigation** — `sanitizeDelimiters` + nonce-based delimiters in `buildCriticPrompt`. Not perfect, but shows awareness.

### Summary Priority Counts

| Priority | Count | Key Items |
|----------|-------|-----------|
| **P0** | 1 | Race condition in sampling counters |
| **P1** | 6 | SQLiteStore god object, duplicated circuit breaker, duplicated JSON parsing, unbounded IN clause, no pipeline timeout, file-after-commit inconsistency |
| **P2** | 12 | File splits, mixed concerns in structs, inconsistent error handling, missing contexts, missing interfaces, error swallowing, reflect abuse, panic in library code |
| **P3** | 10 | Style, unused tags, minor idiom issues |

### Top 5 Actions

1. **Fix the sampling counter race** (P0) — add `sync.Mutex` or `atomic`
2. **Extract shared circuit breaker** (P1) — deduplicate ~160 lines
3. **Extract shared LLM JSON parser** (P1) — deduplicate ~90 lines  
4. **Split `SQLiteStore`** into focused repositories (P1)
5. **Add timeout to pipeline execution** and fix the 60s worker context (P1)

---

*This audit covers production code only. Test coverage analysis would be a separate exercise — and based on what I see here, I'm not optimistic.*

*— Jazz, Feb 2026*
