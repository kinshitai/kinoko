# Phase 6 Review — Round 2

**Reviewer:** Jazz  
**Date:** 2026-02-15  
**Files:** `internal/injection/injector.go`, `internal/injection/injector_test.go`  
**Previous Grade:** C+  
**Grade: B+**

---

## Critical Issues — Status

### C1. `extraction.ValidPattern` doesn't exist → ✅ FIXED
Implemented in `extraction/stage2.go:303` with a `validPatterns` map built from `Taxonomy` via `init()`. O(1) lookups. Clean.

### C2. `extraction.LLMClient` interface undefined → ✅ FIXED
Defined in `extraction/stage2.go:50-52`. `Complete(ctx, string) (string, error)` — matches usage.

### C3. No injection event logging → ✅ FIXED
`InjectionEventWriter` interface added. `storage.InjectionEventRecord` defined with all relevant fields. Events written per-skill with session ID gating (`req.SessionID != ""`). Write failures are non-fatal — logged and swallowed. Correct design. `WriteInjectionEvent` implemented on `SQLiteStore` with proper INSERT.

---

## Major Issues — Status

### M1. Duplicate `ScoredSkill` types → ✅ FIXED
Only `storage.ScoredSkill` exists now. `InjectedSkill` in extraction is a separate, purpose-built response struct with rank position and scores — not a duplicate. Good.

### M2. Store recomputes composite unnecessarily → ✅ FIXED
Normal mode now uses the store's pre-computed composite as-is (line comment: "In normal mode the store already sorted by composite — no recomputation needed"). Degraded mode recomputes with `0.7/0.3` weights. Correct.

### M3. Dead skills are candidates → ✅ FIXED
`MinDecay: defaultMinDecay` (0.05) passed to store query. Matches §5.5 deprecation threshold.

### M4. Unbounded candidate loading → ✅ FIXED
`Limit: defaultCandidateLimit` (50). Reasonable bound.

### M5. Domain field not validated → ✅ FIXED
`extraction.ValidateDomain()` called. Test confirms unknown domain defaults to "Backend".

---

## Minor Issues — Status

### m1. Hand-rolled insertion sort → ✅ FIXED
Uses `slices.SortFunc` now. Consistent with the rest of the codebase.

### m2. Taxonomy hardcoded in prompt → ✅ FIXED
`buildClassificationPrompt` uses `extraction.Taxonomy` slice joined at runtime. Single source of truth.

### m3. Magic number: pattern cap of 3 → ✅ FIXED
Extracted to `const maxPatterns = 3` with a comment. Still not configurable, but at least it's named and visible. Acceptable.

---

## Test Coverage — Status

| Gap from R1 | Status |
|---|---|
| Markdown-fenced JSON parsing | ✅ `TestMarkdownFencedJSON` |
| Invalid intent defaults to BUILD | ✅ `TestInvalidIntentDefaultsToBuild` |
| Pattern validation filtering | ✅ `TestPatternValidationFiltering` |
| Store query error propagation | ✅ `TestStoreQueryError` |
| Empty prompt | ✅ `TestEmptyPrompt` |
| SessionID usage / event writing | ✅ `TestInjectionEventWriting` |
| No events without SessionID | ✅ `TestInjectionEventNotWrittenWithoutSessionID` |
| Event write error non-fatal | ✅ `TestInjectionEventWriteError` |
| Domain validation | ✅ `TestDomainValidation` |

Thorough. Every gap I flagged has a test now.

---

## Remaining Nits

### n1. Event ID generation is fragile
`fmt.Sprintf("%s-%s-%d", req.SessionID, c.Skill.ID, i)` — if the same session injects the same skill twice (e.g., across retries), the ID collides. A UUID or timestamp suffix would be safer. Not a blocker — the INSERT will fail and the error is swallowed gracefully — but it means you silently lose events on retry.

### n2. `InjectionRequest`/`InjectionResponse` still in extraction package
m4 from R1. Still lives in `extraction` rather than `injection` per spec §2.1. Probably intentional to avoid circular imports, but no comment explaining it. Minor.

### n3. No concurrent injection test
Flagged in R1. The injector is stateless so this is low risk, but if you ever add caching or rate limiting you'll wish you had the test. Not blocking.

---

## Summary

Every critical and major issue from R1 is resolved. The injection event pipeline is wired end-to-end. Test coverage went from spotty to comprehensive — 16 tests covering happy path, degraded mode, error paths, validation, and edge cases. The code reads clean: named constants, structured logging, clear separation between normal and degraded paths.

The remaining nits are minor and none are blockers. This is shippable.

**Verdict: Ship it.**
