# Phase 3 Review — Stage 2 Scorer

**Reviewer:** Jazz  
**Date:** 2026-02-15  
**Grade:** B-  
**Files:** `stage2.go`, `stage2_test.go`, `types.go`, `config.go`

---

## Priority 1 — Must Fix

### P1.1: No score range validation on LLM output
`rubricScoresJSON` accepts any `int`. The spec says scores are 1–5, the DB has `CHECK` constraints enforcing it, but `parseRubricResponse` happily accepts `{"problem_specificity": 47}` or `{"problem_specificity": -3}`. When this hits the DB, it'll blow up at insert time — far from where the bug originated.

**Fix:** Validate all 7 scores are in [1,5] inside `parseRubricResponse` (or a separate validation method). Return an error if any are out of range. Don't trust LLMs with your invariants.

**File:** `stage2.go`, `parseRubricResponse()`

### P1.2: No category validation on LLM output
`rubricResponse.Category` is `SkillCategory` (a string typedef). LLM returns `"category": "strategic"` — congratulations, you've invented a fourth category. No validation anywhere. The DB `CHECK` constraint will catch it eventually, but again, fail early.

**Fix:** Validate `rubric.Category` is one of `foundational`, `tactical`, `contextual` before returning.

**File:** `stage2.go`, `Score()` after `parseRubricResponse`

### P1.3: No pattern validation on LLM output
`rubricResponse.Patterns` is `[]string`. LLM returns `["YOLO/Backend/Everything"]` — you store it. The spec says the taxonomy is manually curated, 20 patterns, "never auto-generated." Yet here you're auto-accepting whatever the LLM hallucinates.

**Fix:** Validate patterns against the known taxonomy. Either hardcode the list or accept it as a config/dependency. Unknown patterns should be dropped or cause rejection.

**File:** `stage2.go`

---

## Priority 2 — Should Fix

### P2.1: `parseRubricResponse` is too optimistic about JSON extraction
The "find first `{` and last `}`" approach works for simple markdown wrapping but breaks badly when the LLM's preamble contains a `{` character. Example:

```
Here's my analysis {as requested}:
{"scores": ...}
```

This extracts `{as requested}: {"scores": ...}` — invalid JSON, error. The fix is straightforward: try unmarshaling the full response first, then try the extraction heuristic.

**File:** `stage2.go`, `parseRubricResponse()`

### P2.2: Novelty score math is a triangle, not a bell
The formula `1.0 - abs(distance - mid) / halfRange` produces a triangle wave peaking at the midpoint. The spec says "novelty score derived from distance" (§1.3) but doesn't prescribe a shape. A triangle is... fine, but the choice should be documented and intentional, not accidental. More importantly:

At the boundaries (`distance == minDist` or `distance == maxDist`), novelty score = 0. But the code returns `Passed` (it doesn't reject) and proceeds to the rubric. So a session can pass Stage 2 with `NoveltyScore = 0.0`. Is that intended? If so, what's the point of the score?

**File:** `stage2.go`, `Score()`

### P2.3: `compositeScore` uses unweighted average
The spec says `CompositeScore` is a "weighted average" (§1.1 QualityScores comment). The implementation is a flat mean of all 7 dimensions. These are different things. If it's intentional to use equal weights for now, add a comment. If it's wrong, fix it.

**File:** `stage2.go`, `compositeScore()`

### P2.4: Missing `VersionSimilarityThreshold` from config
Appendix A specifies `VersionSimilarityThreshold` (default 0.85) in `ExtractionConfig`. It's not in `config.go`. Granted, it's a Phase 5+ concern, but the config struct should match the spec. You'll forget later.

**File:** `config.go`, `ExtractionConfig`

---

## Priority 3 — Nit / Suggestion

### P3.1: `SkillQuerier` interface — acceptable abstraction
I was skeptical, but this is the right call. Stage 2 shouldn't know how skills are stored. The interface is minimal (one method). Fine.

### P3.2: LLM prompt is a hardcoded string
`buildRubricPrompt` embeds the entire prompt as a format string. The spec mentions `CriticPrompt` as a configurable path (Appendix A) — that's Stage 3, but the same principle applies. When you inevitably need to iterate on this prompt, you'll be recompiling. Consider externalizing. Not urgent.

### P3.3: `abs()` function
Go 1.21+ has `math.Abs`. Don't reinvent it. Yes, I know, it's trivial. That's why it's worse — you wrote 4 lines to avoid an import.

**File:** `stage2.go`

### P3.4: Prompt says "no markdown" but tests include markdown-wrapped response
The prompt says "respond with ONLY a JSON object (no markdown, no explanation)" and then you handle markdown code blocks in `parseRubricResponse`. This is defensive and correct — but the inconsistency means you know the instruction doesn't work. Consider adding `You MUST NOT wrap the response in markdown code blocks` to the prompt. Belt and suspenders.

---

## Config Validation (Phase 2 R2 Issue)

**Verdict: Fixed properly.** The `Validate()` method now checks:
- `NoveltyMinDistance` and `NoveltyMaxDistance` are in [0,1]
- `NoveltyMinDistance <= NoveltyMaxDistance`
- All Stage 1 thresholds validated (duration, tool calls, error rate)

This addresses my Phase 2 complaint about validation gaps. The validation is thorough and the error messages are specific. No complaints.

---

## Test Quality

**Coverage:** Good breadth — 12 test cases covering happy path, both novelty rejection directions, rubric failure, error paths for all three dependencies, malformed JSON, markdown wrapping, and boundary conditions.

**Gaps:**
1. No test for out-of-range rubric scores (because there's no validation — see P1.1)
2. No test for invalid category (see P1.2)
3. No test for invalid patterns (see P1.3)
4. No test for the novelty score *value* at specific distances — you test that it's positive, but not that the math is correct. Add a case with a known distance and assert the expected novelty score.
5. The `"full pass with no existing skills"` test correctly asserts rejection (distance 1.0 > 0.95 max), which is a thoughtful edge case. Good.
6. Mock embedder returns zero vectors. That's fine for testing flow, but worth noting that zero vectors will always have cosine similarity of 0 (or undefined) with any real vector. Doesn't affect correctness here since the querier mock overrides the result.

**Mocks:** Realistic enough. The `mockLLM` could capture the prompt for assertion (verify the content was actually interpolated), but that's optional.

---

## Summary

Solid structure. The two-classifier design is clean, the dependency injection is right, tests are above average. But the elephant in the room is **zero validation of LLM output** — scores, categories, and patterns are all accepted verbatim. That's three bugs waiting to happen. In production, LLMs will return 0s, 6s, made-up categories, and hallucinated patterns. You *will* hit these. Fix P1.1–P1.3 before merging.

The novelty math works but has a semantic gap (P2.2) that needs a design decision, and the composite score contradicts the spec (P2.3).

— Jazz
