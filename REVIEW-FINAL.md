# Final Polish Review — Jazz

**Date:** 2026-02-15  
**Scope:** Every file in the codebase. Code quality, error messages, logging, config, interfaces, tests, docs.  
**Goal:** B+ → A

---

## 1. Stale / Broken Test (MUST FIX)

| # | File | Line | Issue | Fix |
|---|------|------|-------|-----|
| 1.1 | `tests/integration/integration_test.go` | ~106 | `TestFullExtractionFlow` asserts `DecayScore != 1.0` with comment "BUG: pipeline should set 1.0" — but pipeline WAS fixed to set 1.0, so the assertion now fails. The error message text also says `want 0.0` which contradicts the comment. | Change to `if dbSkill.DecayScore != 1.0 { t.Errorf("initial decay = %f, want 1.0", dbSkill.DecayScore) }`. Remove the BUG comment. |
| 1.2 | `tests/integration/integration_test.go` | ~40 | `TestFullExtractionFlow` builds pipeline WITHOUT `Sessions` in `PipelineConfig`. Session persistence (the biggest P0) is not tested end-to-end. | Add `Sessions: store` to the PipelineConfig, then verify the session row exists in DB after extraction. |

---

## 2. Missing Package Doc Comments

| # | File | Issue |
|---|------|-------|
| 2.1 | `internal/extraction/pipeline.go` | No `// Package extraction ...` doc comment. This is the most important package. |
| 2.2 | `internal/extraction/types.go` | Same — extraction package has no package-level doc. Add it in ONE file (types.go is conventional). |
| 2.3 | `internal/storage/store.go` | No `// Package storage ...` doc comment. |
| 2.4 | `internal/injection/injector.go` | No `// Package injection ...` doc comment. |
| 2.5 | `internal/injection/ab.go` | Same package, already covered. |
| 2.6 | `internal/metrics/collector.go` | No `// Package metrics ...` doc comment. |
| 2.7 | `internal/gitserver/server.go` | No `// Package gitserver ...` doc comment. |
| 2.8 | `internal/config/config.go` | No `// Package config ...` doc comment. |
| 2.9 | `pkg/skill/skill.go` | No `// Package skill ...` doc comment. |
| 2.10 | `cmd/mycelium/main.go` | No `// Package main ...` doc. OK for main, but `// Command mycelium ...` would be nice. Low priority. |

Only `internal/decay/runner.go` has a package doc (`// Package decay ...`). Every other package is missing it.

---

## 3. Missing Godoc on Exported Types/Functions

| # | File | Symbol | Issue |
|---|------|--------|-------|
| 3.1 | `extraction/types.go` | `SkillRecord` | Has a short comment but no description of what the struct represents beyond "database representation." Fields lack doc comments (what is `ParentID`? what is `FilePath` relative to?). |
| 3.2 | `extraction/types.go` | `SessionRecord` | No doc on `LogPath` field (what format? who sets it?). |
| 3.3 | `extraction/types.go` | `ExtractionStatus` constants | `StatusStage1`, `StatusStage2`, `StatusStage3` are defined but never used in the pipeline. Dead code — pipeline goes straight from `StatusPending` to `StatusExtracted`/`StatusRejected`/`StatusError`. Remove them or use them. |
| 3.4 | `extraction/types.go` | `InjectionRequest`, `InjectionResponse`, `InjectedSkill`, `PromptClassification` | These belong in the injection package, not extraction. They're here to avoid import cycles, but there's no doc explaining why. Add a comment. |
| 3.5 | `extraction/types.go` | `ValidDomains`, `ValidateDomain` | These are injection concerns living in the extraction package. Same issue as 3.4. |
| 3.6 | `storage/store.go` | `SessionStore`, `SkillStore` | Good interfaces, but `SkillStore` is defined in storage and never used as an interface boundary — `SQLiteStore` is passed directly everywhere. Consider whether these interfaces provide value or are aspirational. Not blocking, but document intent. |
| 3.7 | `storage/store.go` | `ScoredSkill` | No doc on what `CompositeScore` represents (is it the query-time composite or the stored one?). It's the query-time one. Document it. |

---

## 4. Error Message Quality

| # | File | Line | Issue | Fix |
|---|------|------|-------|-----|
| 4.1 | `extraction/pipeline.go` | ~147 | Stage2 error: `result.Error = fmt.Sprintf("stage2: %v", err)` — doesn't include session_id. | Add session_id: `fmt.Sprintf("stage2 [session=%s]: %v", session.ID, err)` |
| 4.2 | `extraction/pipeline.go` | ~174 | Stage3 error: same issue, no session_id in error string. | Same pattern. |
| 4.3 | `extraction/pipeline.go` | ~210 | Store error: same. | Same. |
| 4.4 | `extraction/stage3.go` | `parseAndValidate` | On invalid verdict: `fmt.Errorf("invalid verdict: %q", cr.Verdict)` — doesn't say which session. | This is called from Evaluate which logs the session, so the log has context. The error message is fine for the return value. OK as-is. |
| 4.5 | `storage/store.go` | `InsertSession` | Error: `"insert session: %w"` — good, but doesn't include session ID. | Add: `fmt.Errorf("insert session %s: %w", session.ID, err)` |
| 4.6 | `storage/store.go` | `UpdateSessionResult` | Same: `"update session result: %w"` missing session ID. | Add session.ID. |

---

## 5. Logging Consistency

| # | File | Issue |
|---|------|-------|
| 5.1 | `extraction/pipeline.go` | Uses `"session_id"` as the slog key everywhere — good and consistent. ✓ |
| 5.2 | `extraction/stage1.go` | Uses `"session_id"` — consistent. ✓ |
| 5.3 | `extraction/stage2.go` | Uses `"session_id"` — consistent. ✓ |
| 5.4 | `extraction/stage3.go` | Uses `"session_id"` — consistent. ✓ |
| 5.5 | `injection/injector.go` | Uses `"component", "injector"` via `log.With()` — good. But error log at line for event write uses `"skill_id"` and `"error"` — consistent. ✓ |
| 5.6 | `injection/ab.go` | Uses `"component", "ab_injector"` — consistent. ✓ |
| 5.7 | `decay/runner.go` | Uses `"library"` instead of `"library_id"` used elsewhere. | Change to `"library_id"` for consistency. |
| 5.8 | `cmd/mycelium/serve.go` | Lines ~127,133: uses `"session"` as key instead of `"session_id"`. | Change to `"session_id"`. |
| 5.9 | `storage/store.go` | `NewSQLiteStore` uses bare `slog.Info("sqlite integrity check passed")` — no structured fields, no component tag. | Add `slog.Info("sqlite integrity check passed", "dsn", dsn)` or use a logger parameter. |
| 5.10 | `cmd/mycelium/serve.go` | Lines ~143-149: mixes `slog.Info()` (package-level) with `logger.Info()`. The `runServe` function creates a logger but then uses the global `slog.Info` for some messages. | Use `logger` consistently, or at least don't mix in the same function. |

---

## 6. Config Completeness

| # | Issue | Fix |
|---|-------|-----|
| 6.1 | `embedding.Config` (API key, model, base URL, retry, circuit breaker) is NOT loadable from YAML config. `serve.go` hardcodes `embedding.DefaultConfig()` and reads API key from env var only. | Add `embedding` section to `config.Config` and wire it in `serve.go`. At minimum, model and base_url should be configurable from YAML. |
| 6.2 | LLM model for extraction/injection is hardcoded to `"gpt-4o-mini"` in `serve.go` and `extract.go`. | Add `llm.model` to config. |
| 6.3 | `SampleRate` for human review sampling is not in YAML config. `serve.go` doesn't set it (defaults to 0 — no sampling). `extract.go` hardcodes `0.01`. | Add `extraction.sample_rate` to config. |
| 6.4 | `decay.Config` has its own YAML struct, AND `config.DecayConfig` duplicates it. `decayConfigFromYAML()` manually copies fields with zero-value fallback. | This works but is maintenance debt. Consider having `decay.Config` BE `config.DecayConfig` or embedding it. Low priority. |
| 6.5 | `config.Config` has no `Embedding` field. | See 6.1. |
| 6.6 | `config.Config` has no `LLM` field. | See 6.2. |
| 6.7 | Default config YAML written by `init` command doesn't include `decay`, `ab_test`, or stage threshold sections. Users won't know they exist. | Add commented-out sections showing all available options. |

---

## 7. Interface Hygiene

| # | File | Issue |
|---|------|-------|
| 7.1 | `storage/store.go` | `SkillStore` interface has 7 methods. That's a lot. But all are used by different callers, so it's justified. OK. |
| 7.2 | `storage/store.go` | `SessionStore` interface (2 methods) is defined here but the consuming code is in `extraction/pipeline.go` which defines its OWN `SessionWriter` interface. Both have the same methods. Redundant — pick one. | Remove `SessionStore` from storage (or have pipeline use it). The Go idiom is to define interfaces where they're consumed. Pipeline's `SessionWriter` is correct. |
| 7.3 | `extraction/pipeline.go` | `SkillEmbedder` interface (1 method: `Embed`) is fine — minimal, at consumer. ✓ |
| 7.4 | `extraction/stage2.go` | `SkillQuerier` interface (1 method) — good. ✓ |
| 7.5 | `extraction/stage3.go` | `LLMClientV2` extends `LLMClient` — interface embedding, fine. But `LLMClient` is defined in `stage2.go`, and `LLMClientV2` is in `stage3.go`. Both are in the same package so no issue, but it's confusing to find them. | Move all LLM interfaces to `types.go`. |
| 7.6 | `gitserver/server.go` | `SetSessionHooks(onStart, onEnd any)` — uses `any` type. As noted in R2, this is type-unsafe. | Use concrete function types: `type SessionStartHook func(...)` etc. |

---

## 8. Dead Code / Unnecessary Exports

| # | File | Symbol | Issue |
|---|------|--------|-------|
| 8.1 | `extraction/types.go` | `StatusStage1`, `StatusStage2`, `StatusStage3` | Never used. The pipeline never sets these intermediate statuses. Remove. |
| 8.2 | `extraction/stage2.go` | `Taxonomy` exported var | Used by injection package for prompt building — justified. ✓ |
| 8.3 | `extraction/stage2.go` | `ValidPattern` exported func | Used by injection — justified. ✓ |
| 8.4 | `extraction/types.go` | `ValidDomains`, `ValidateDomain` | Only used by injection. Consider moving. |
| 8.5 | `extraction/stage3.go` | `generateNonce()`, `sanitizeDelimiters()` | Unexported, only used internally. ✓ |
| 8.6 | `gitserver/binary.go` | `InstallSoftBinary()` | Never called anywhere in the codebase. Dead code. | Remove or mark with a TODO explaining future use. |
| 8.7 | `extraction/stage3.go` | `maxRetriesFor(_ error) int` | Takes an error parameter that's always nil and always returns 3. The parameter is unused. | Either use it (inspect error to decide retry count) or remove the parameter: `func maxRetries() int { return 3 }`. |
| 8.8 | `cmd/mycelium/extract.go` | `estimateTokens` function | Duplicates logic in `extraction/stage3.go` `estimateTokens`. | Consolidate or at least note the duplication. |
| 8.9 | `extraction/pipeline.go` | `titleCase` function | Only used in `buildSkillMD`. Fine, but note: `titleCase` doesn't handle all-caps words well (e.g., "FIX" stays "FIX" because first char is already upper). Actually it lowercases via `strings.Fields` then uppercases first char — so "FIX" → "FIX" (first char upper, rest unchanged). This is arguably correct. OK. |

---

## 9. Code Quality Nits

| # | File | Line | Issue | Fix |
|---|------|------|-------|-----|
| 9.1 | `cmd/mycelium/serve.go` | ~69 | `waitForShutdown` creates a new cancellable context from the input, then uses both `sigCh` and `ctx.Done()` in a select. But it also has a goroutine that closes `done` and calls `cancel()`. Then the outer select waits on both `done` and `ctx.Done()` — but `cancel()` was already called in the goroutine. The logic works but is convoluted. | Simplify: just `select { case <-sigCh: case <-ctx.Done(): }` directly. |
| 9.2 | `cmd/mycelium/stats.go` | ~21 | Creates `logger` then immediately `_ = logger`. | Remove the unused logger or use it. |
| 9.3 | `cmd/mycelium/serve.go` | ~56 | `openAILLMClient` is defined in `extract.go` and reused in `serve.go`. That's fine (same package), but `openAIComplete` and `openAILLMClient` are not tested at all. | At minimum, add a doc comment noting this is a thin shim and depends on external API. |
| 9.4 | `internal/extraction/pipeline.go` | `cryptoRandIntn` | `panic` on crypto/rand failure. This is documented and intentional. ✓ |
| 9.5 | `pkg/skill/skill.go` | `Validate()` | `Version != 1` check is extremely restrictive. This means the parser rejects ALL skills with version > 1, making the version field useless for future versioning. | Change to `Version < 1` or at least document why only v1 is allowed. |
| 9.6 | `internal/config/config.go` | `Validate()` | Doesn't validate `DecayConfig` at all. Zero-value `DecayConfig` passes validation. | Either validate decay config here, or document that `decay.ValidateConfig` must be called separately. |
| 9.7 | `internal/storage/store.go` | `Put()` | Writes file to disk (`os.WriteFile`) inside a database transaction. If the file write succeeds but `tx.Commit()` fails, you have an orphan file on disk with no DB record. | Move file write after commit, or document this as acceptable. |
| 9.8 | `tests/integration/helpers_test.go` | `contains` and `searchString` | Hand-rolled string search. Use `strings.Contains`. | Replace with `strings.Contains`. |
| 9.9 | `tests/integration/integration_test.go` | bottom | `var _ = math.Abs` — force import. | Remove: `math` is used in `assertApprox` in helpers. If it's not used in THIS file, the import should be in helpers only. Actually `math` isn't imported in integration_test.go... wait, it IS imported. This unused-import workaround is unnecessary — `math.Abs` IS used in the `assertApprox` helper. Actually no — `assertApprox` is in helpers_test.go, and `math` is imported there. In integration_test.go, `math` is imported but only used through `var _ = math.Abs`. Remove the `math` import from integration_test.go entirely. |
| 9.10 | `internal/extraction/stage3.go` | ~231 | `failedInHalfOpen` field on `stage3Critic` struct — declared but never read or written. Dead field. | Remove. |

---

## 10. Test Quality

| # | File | Issue | Risk |
|---|------|-------|------|
| 10.1 | `embedding/embedding_test.go` | `TestCircuitBreaker_EscalatingOpenDuration` uses `time.Sleep(60ms)` etc. These are timing-dependent and can flake on slow CI. | Inject clock like stage3 tests do, or increase sleep margins. Medium flake risk. |
| 10.2 | `embedding/embedding_test.go` | `TestCircuitBreaker_HalfOpenRecovery` same issue — `time.Sleep(60ms)`. | Same. |
| 10.3 | `extraction/stage3_test.go` | `TestStage3Critic_LatencyTracking` uses `time.Sleep(50ms)` then asserts `>= 50ms`. Can flake. | Low risk since sleep is in the mock LLM, not real time. But still timing-dependent. |
| 10.4 | `tests/integration/integration_test.go` | `TestConcurrentExtractions` — good test but relies on file-based SQLite for concurrent writes. SQLite concurrent write behavior is well-defined (BUSY errors) but results depend on timing. | Acceptable. The test correctly checks for "at least one success" rather than deterministic counts. ✓ |
| 10.5 | `extraction/pipeline_test.go` | `TestPipelineStratifiedSamplingBalance` uses a modular rand function. The assertion `extractedPct >= 20%` is very loose — good for avoiding flakes. ✓ |
| 10.6 | `tests/integration/integration_test.go` | Test 1 doesn't test session persistence (see 1.2 above). | High — this was the #1 P0. |

---

## 11. Schema / Database

| # | File | Issue |
|---|------|-------|
| 11.1 | `schema.sql` | Missing index: `idx_injection_events_outcome` on `(skill_id, session_outcome)` — needed for the `UpdateUsage` correlation subquery. | Add index. |
| 11.2 | `schema.sql` | `session_outcome` on `injection_events` is never written by application code. It's written manually in integration tests via raw SQL (`UPDATE injection_events SET session_outcome = ?`). There's no API to set it. | Add `UpdateInjectionOutcome(ctx, sessionID, outcome)` to store, or document how/when `session_outcome` gets populated. |

---

## 12. Miscellaneous

| # | File | Issue |
|---|------|-------|
| 12.1 | `go.mod` | `golang.org/x/crypto` is a direct dependency but I don't see it imported anywhere in Go files. It might be pulled in transitively. | Run `go mod tidy` to verify. If it's only transitive, move to indirect. |
| 12.2 | `cmd/mycelium/extract.go` | `storeQuerier.QueryNearest` returns `CosineSim: 0` when no results found (instead of nil). Stage2 then computes `distance = 1.0 - 0 = 1.0` which exceeds `maxDist=0.95` → rejection. This means extraction ALWAYS fails on an empty library. | Return `nil` for no results, matching the interface contract in stage2. Currently: `return &extraction.SkillQueryResult{CosineSim: 0}, nil`. Should be: `return nil, nil`. |
| 12.3 | `internal/injection/injector.go` | `InjectionEventWriter` interface is defined here and also conceptually overlaps with what `storage.SQLiteStore` implements. The interface is at the consumer — correct Go idiom. ✓ |
| 12.4 | Root | No `doc.go` files anywhere. While not required, they're conventional for public packages (`pkg/skill`). | Add `doc.go` to `pkg/skill` at minimum. |

---

## Summary: Items for A Grade

### Must Fix (5 items)
1. **1.1** — Fix stale DecayScore assertion in integration test
2. **1.2** — Wire Sessions in integration test pipeline config  
3. **12.2** — Fix `storeQuerier.QueryNearest` returning zero instead of nil for empty library (breaks first extraction)
4. **8.1** — Remove dead `StatusStage1/2/3` constants
5. **9.10** — Remove dead `failedInHalfOpen` field

### Should Fix (15 items)
6. **2.1–2.9** — Add package doc comments to all packages (batch job, 30 min)
7. **4.1–4.3** — Add session_id to pipeline error strings
8. **5.7** — Fix `"library"` → `"library_id"` in decay log
9. **5.8** — Fix `"session"` → `"session_id"` in serve.go log
10. **5.10** — Don't mix `slog.Info` and `logger.Info` in same function
11. **6.1** — Make embedding config loadable from YAML
12. **6.2** — Make LLM model configurable from YAML
13. **6.3** — Make sample rate configurable from YAML
14. **7.6** — Replace `any` in SetSessionHooks with typed callbacks
15. **8.6** — Remove dead `InstallSoftBinary()`
16. **8.7** — Fix unused parameter in `maxRetriesFor`
17. **9.2** — Remove unused logger in stats.go
18. **9.8** — Replace hand-rolled string search with `strings.Contains`
19. **9.9** — Remove unnecessary `var _ = math.Abs`
20. **11.2** — Add store method for setting injection outcome

### Nice to Have (8 items)
21. **3.4** — Document why injection types live in extraction package
22. **5.9** — Add structured fields to store startup logs
23. **6.7** — Add commented-out config sections to init template
24. **7.2** — Remove redundant `SessionStore` interface from storage
25. **7.5** — Move LLM interfaces to types.go
26. **9.5** — Relax `Version != 1` to `Version < 1`
27. **9.7** — Document or fix file-write-inside-transaction
28. **11.1** — Add missing index for outcome correlation query

---

**Estimated effort:** Must-fix items: 1 hour. Should-fix: 3 hours. Nice-to-have: 2 hours. Total: ~6 hours for a clean A.

The must-fix items are genuine bugs (12.2 breaks first-time extraction on empty DB, 1.1/1.2 are failing tests). The should-fix items are the difference between "works" and "polished." The nice-to-haves are perfectionism.

Fix the 5 must-fix items and the 15 should-fix items and this is an A. Ship it.

— Jazz
