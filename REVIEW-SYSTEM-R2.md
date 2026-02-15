# System Re-Review (R2) — Jazz

**Date:** 2026-02-15  
**Reviewer:** Jazz  
**Scope:** Verification of P0 ship-blocker fixes + regression check  
**Previous Grade:** B- overall / D+ integration  
**Updated Grade: B+**

---

## P0 Fix Verification

### P0-1: Persist sessions to `sessions` table ✅ FIXED

**pipeline.go** now has `SessionWriter` interface with `InsertSession` + `UpdateSessionResult`. The pipeline calls `InsertSession` before extraction begins (line ~120) and `updateSessionStatus` at every exit point — rejections, errors, and successful extractions. The `updateSessionStatus` helper correctly maps rejection stage and reason from the result.

**store.go** implements both methods. `InsertSession` writes all 17 columns. `UpdateSessionResult` updates the four extraction-result columns. SQL looks correct.

**serve.go** passes `Sessions: store` to `PipelineConfig`. Wired.

**Verdict:** Solid fix. Non-fatal error handling on insert is the right call — extraction shouldn't fail because session logging fails.

### P0-2: Compute and store skill embeddings during extraction ✅ FIXED

**pipeline.go** lines 267-273: After Stage 3 passes, before `writer.Put()`, calls `p.embedder.Embed()` on content and sets `skill.Embedding`. Falls back gracefully with a Warn log if embedding fails (skill stored without embedding — degraded but not broken).

**serve.go** passes `Embedder: embedder` to `PipelineConfig`. Wired.

**Verdict:** Exactly what I asked for. One note: it embeds the raw session `content`, not a distilled summary. For `text-embedding-3-small` with 8K token limit, large session logs will get truncated by the API. Not a P0 — works fine for typical sessions.

### P0-3: Fix double injection event writing ✅ FIXED

**ab.go** `NewABInjector` doc now explicitly says: "ensure the inner injector was constructed without an eventWriter to avoid double-writing."

**serve.go** lines 82-90 implement this correctly:
```go
if abCfg.Enabled {
    baseInj := injection.New(embedder, store, llmClient, nil, logger)  // nil eventWriter!
    inj = injection.NewABInjector(baseInj, store, abCfg, logger)
} else {
    inj = injection.New(embedder, store, llmClient, store, logger)
}
```

When A/B is enabled: base injector gets `nil` eventWriter, ABInjector writes events with group info. When A/B is disabled: base injector writes events directly. No double-write in either path.

**Verdict:** Clean fix. The conditional nil is the right pattern.

### P0-4: Connect serve hooks to something ✅ FIXED

**serve.go** no longer has `_ = hooks`. Instead:
```go
server.SetSessionHooks(hooks.OnSessionStart, hooks.OnSessionEnd)
```

**gitserver/server.go** has `SetSessionHooks` that stores the callbacks on the Server struct.

**Verdict:** Hooks are wired. The `SetSessionHooks` signature uses `any` types which is... not great (no compile-time safety), but it works. P2 debt, not a blocker.

### P0-5: Implement `InsertReviewSample` on SQLiteStore ✅ FIXED

**store.go** implements `InsertReviewSample` — generates an ID, inserts into `human_review_samples` with session_id, extraction_result JSON, and timestamp.

**pipeline.go** has the `HumanReviewWriter` interface and `maybeSample` calls it. The stratified sampling logic (50/50 extracted vs rejected) is actually more sophisticated than I expected — uses in-memory counters for balancing. **Note:** counters reset on restart, so balance drifts across restarts. Acceptable for 1% sampling.

**serve.go** passes `Reviewer: store` to `PipelineConfig`. Wired.

**Verdict:** Fixed. The FK constraint on `sessions(id)` that I flagged as "doubly broken" is now satisfied because sessions are persisted first (P0-1). Good.

---

## Bonus Fix: DecayScore Bug

Pavel's E2E test caught that the pipeline wasn't setting `DecayScore = 1.0` on new skills, meaning Go's zero value (0.0) would make newly extracted skills invisible to injection queries.

**pipeline.go** line 261: `DecayScore: 1.0` — fixed with a clear comment.

**However:** Pavel's test (`TestFullExtractionFlow`, line 106) still asserts the OLD buggy behavior:
```go
if dbSkill.DecayScore != 0.0 {
    t.Errorf("initial decay = %f, want 0.0 (BUG: pipeline should set 1.0)", dbSkill.DecayScore)
}
```

This test **currently fails** because the fix was applied but the test wasn't updated. The comment says "BUG: pipeline should set 1.0" — so Pavel documented the bug but wrote the assertion against the broken behavior. **This test needs to be updated to expect 1.0.**

I ran it. It fails:
```
initial decay = 1.000000, want 0.0 (BUG: pipeline should set 1.0)
```

---

## New Issues Introduced by Fixes

### N1: Stale E2E Test (P1)

`TestFullExtractionFlow` fails. The DecayScore assertion and a `duration = 0` assertion both fail. The test was written against pre-fix behavior. **Must fix before merge.**

### N2: `SetSessionHooks` Uses `any` Types (P2)

```go
func (s *Server) SetSessionHooks(onStart, onEnd any) {
```

This accepts literally anything at compile time. A typo in the callback signature won't be caught until runtime. Should use concrete function types. Not blocking, but sloppy.

### N3: Session Insert Is Non-Fatal but Silent (P2)

If `InsertSession` fails (e.g., duplicate session ID), the pipeline logs an error and continues. The subsequent `UpdateSessionResult` will silently fail too (no row to update). Metrics for that session will be missing with no obvious indicator. Consider returning early or at least setting `p.sessions = nil` for the remainder of that extraction to avoid the confusing update-on-nonexistent-row pattern.

### N4: `TestFullExtractionFlow` Doesn't Wire Sessions (P2)

The integration test builds the pipeline without `Sessions` in `PipelineConfig`, so session persistence isn't tested in E2E. Given that session persistence was the #1 P0, it should be tested end-to-end.

---

## What I Didn't Re-Check (Out of Scope)

These P1/P2 items from R1 were not in scope for this re-review:
- P1-6: Decay config from YAML (not checked)
- P1-7: Embedding/LLM config from YAML (not checked)
- P1-8: ExtractionStatus update (actually, this IS fixed — `updateSessionStatus` handles it)
- P1-9: Missing indexes (not checked)
- P2-11 through P2-15: Technical debt items (not checked)

---

## Pavel's E2E Tests: Assessment

15 integration tests covering:
- Full extraction flow (Test 1)
- Full injection flow (Test 2)
- Feedback loop / success correlation (Test 3)
- Decay cycle with category-specific half-lives (Test 4)
- A/B test treatment vs control (Test 5)
- Rejection at each stage (Test 6)
- Embedding failure degradation (Test 7)
- Stats/metrics accuracy (Test 8)
- Decay rescue (Test 9)
- Query ordering (Test 10)
- Version chains (Test 11)
- Dead skill filtering (Test 12)
- Transaction atomicity (Test 13)
- Concurrent extraction safety (Test 14)
- Schema constraints (Test 15)

**This is good work.** The coverage addresses most of the integration seams I flagged in R1 §8.2-8.3. Tests use real SQLite with mock LLM/embedder — correct tradeoff. Test 8 (stats) even manually inserts sessions to verify metrics, which is exactly what was needed.

**Issues:**
- Test 1 fails (stale assertion, see N1 above)
- Test 1 doesn't wire `Sessions` (see N4)
- Test 8 uses a manual `insertSession` helper instead of the pipeline's session persistence — tests the metrics but not the pipeline→session→metrics chain

---

## Updated Grades

| Area | R1 Grade | R2 Grade | Notes |
|---|---|---|---|
| Individual packages | B+ to A- | B+ to A- | Unchanged |
| Integration | D+ | B+ | All 5 P0s fixed correctly |
| Test coverage | C | B+ | Pavel's 15 E2E tests fill the gap |
| Error handling | B+ | B+ | Unchanged |
| Dependency graph | A- | A- | Unchanged |

**Overall: B+** (up from B-)

The integration story went from "will crash in 30 seconds" to "will work correctly for the happy path and degrade gracefully for edge cases." The five ship-blockers are genuinely fixed — not papered over, not hacked around. The code is clean and follows the patterns established in the existing codebase.

Fix the stale test (N1), wire sessions in the E2E test (N4), and this is shippable.

---

## Summary

| P0 | Status | Quality of Fix |
|---|---|---|
| 1. Session persistence | ✅ Fixed | Clean — insert before, update after, non-fatal errors |
| 2. Skill embeddings | ✅ Fixed | Clean — graceful degradation on failure |
| 3. Double event write | ✅ Fixed | Clean — conditional nil eventWriter |
| 4. Serve hooks wired | ✅ Fixed | Adequate — `any` types are sloppy but functional |
| 5. InsertReviewSample | ✅ Fixed | Clean — with stratified sampling bonus |
| Bonus: DecayScore | ✅ Fixed | Clean — but test is stale |

**Remaining before ship:** Fix `TestFullExtractionFlow` assertions (30 seconds of work).

— Jazz

*P.S. I raised this from B- to B+, not to A-, because the P1/P2 debt from R1 is still there. Config is still half-wired, indexes are still missing, `session_outcome` is still never written. But the system won't crash on contact with reality anymore, and that's what matters for a first ship.*
