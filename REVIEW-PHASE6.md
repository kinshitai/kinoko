# Phase 6 Review — Injection Pipeline

**Reviewer:** Jazz  
**Date:** 2026-02-15  
**Files:** `internal/injection/injector.go`, `internal/injection/injector_test.go`  
**Grade: C+**

Functional skeleton is here. The happy path works. But there are missing functions that won't compile, spec deviations, redundant computation, and test gaps that make this not shippable.

---

## Critical (Won't Compile / Spec Violations)

### C1. `extraction.ValidPattern` doesn't exist
`injector.go:113` calls `extraction.ValidPattern(p)` — this function is not defined anywhere in `internal/extraction/types.go` or any other file reviewed. **This won't compile.** You need to implement it, presumably checking against the Appendix B taxonomy list.

### C2. `extraction.LLMClient` interface is undefined
`injector.go:10` imports and `injector.go:30` uses `extraction.LLMClient`, but `types.go` defines no such interface. The test file mocks it with `Complete(ctx, string) (string, error)` but the real interface doesn't exist. **Won't compile against real types.**

### C3. No injection event logging
Spec §2.1 defines `SessionID` on `InjectionRequest` explicitly "for logging injection events." The `injection_events` table exists in the DDL. The injector completely ignores `req.SessionID` and never writes injection events. This is the data that feeds `success_correlation` recomputation in `UpdateUsage`. Without it, the feedback loop is broken — historical rates will never update.

---

## Major

### M1. Duplicate `ScoredSkill` types
`storage.ScoredSkill` and `extraction.ScoredSkill` are nearly identical structs with the same fields. The injector manually copies field-by-field from one to the other (lines 84-91). Pick one. The spec puts `ScoredSkill` in `storage` (§2.1 SkillStore section). `InjectionResponse` should reference `storage.ScoredSkill`, or the type should live in a shared package. Two copies = two places to forget to update.

### M2. Store already computes composite — injector recomputes
`storage.Query()` already computes `CompositeScore = 0.5*PO + 0.3*CS + 0.2*HR` and returns sorted results. The injector then ignores that score and recomputes. In normal mode, the formula is identical — pure waste. In degraded mode the reweighting is intentional (`0.7/0.3`), but you're still paying for the store's computation. Either:
- Pass a mode/weights to the store query, or
- Don't compute composite in the store when the injector will override it

### M3. Dead skills are candidates
Query passes `MinQuality: 0, MinDecay: 0`. Per spec §5.5, skills at the deprecation threshold (default 0.05) should be "effectively invisible." A skill with `decay_score = 0.0` should never be injected. At minimum pass `MinDecay: deprecationThreshold` from config. Currently there's no config plumbing at all.

### M4. Unbounded candidate loading
`Limit: 0` in the store query means "return everything." For a library with 10,000 skills, you're loading all rows + patterns + embeddings into memory just to re-rank and take 3. Set a reasonable limit (e.g., 50-100 candidates) or at least make it configurable.

### M5. Domain field not validated
`classifyPrompt` validates `Intent` against `validIntents` and defaults to `"BUILD"`. `Domain` is passed through raw from the LLM. Spec §2.1 defines Domain as: `Frontend | Backend | DevOps | Data | Security | Performance`. No validation, no default. Garbage in, garbage stored.

---

## Minor

### m1. Hand-rolled insertion sort
`sortScoredSkills` is a manual insertion sort (O(n²)). The store uses `slices.SortFunc`. Use the same. Consistency matters more than the microseconds you'd save on 3 elements.

### m2. Taxonomy hardcoded in prompt string
`buildClassificationPrompt` hardcodes all 20 patterns from Appendix B as a string literal. If the taxonomy changes, you update it here AND in whatever `ValidPattern` ends up being. Extract to a shared `var Taxonomy []string` in the extraction package.

### m3. Magic number: pattern cap of 3
`validPats[:3]` at line 116 — no comment on why 3, no config option, no spec reference. If this is intentional, document it. If not, make it configurable.

### m4. `InjectionRequest`/`InjectionResponse` package placement
Spec §2.1 defines these under `package injection`. Implementation puts them in `package extraction`. Functional but deviates from spec. If intentional (to avoid circular imports), add a comment.

---

## Test Gaps

| Missing Test | Why It Matters |
|---|---|
| Markdown-fenced JSON parsing | `parseClassificationResponse` has a fallback path for ```` ```json ... ``` ```` — never exercised |
| Invalid intent defaults to BUILD | Code does it, no test proves it |
| Pattern validation filtering | Depends on `ValidPattern` which doesn't exist yet, but no test skeleton either |
| Store query error propagation | `mockStore.err` is set but no test checks that `Inject` returns the error |
| Concurrent injection calls | No test for thread safety — `injector` is stateless so probably fine, but prove it |
| Empty prompt | What happens with `Prompt: ""`? LLM gets a classification prompt with empty input |
| `SessionID` usage | Never tested because never used (see C3) |

---

## What's Actually Good

Not my job to hand out gold stars, but I'll note: the fallback/degraded mode logic is clean, error handling on classification failure is correct (graceful degradation to empty patterns), and the test for `TestNilEmbedder` catches a real edge case. The `parseClassificationResponse` with its fence-stripping fallback is practical defensive coding.

---

## Summary

Two compilation blockers (C1, C2), one broken feedback loop (C3), and architectural messiness (M1, M2) that will cost you later. Fix the criticals, wire up injection event logging, and deduplicate the scored skill type. The rest is cleanup.

**Verdict: Not shippable. Fix C1-C3 and M3-M4, then re-review.**
