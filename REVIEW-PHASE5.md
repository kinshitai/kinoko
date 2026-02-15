# Phase 5 Review — Extraction Pipeline

**Reviewer:** Jazz  
**Date:** 2026-02-15  
**Files:** `pipeline.go`, `pipeline_test.go`  
**Grade: C+**

Decent wiring job. The happy path works. But there are spec deviations, a broken name generation heuristic, a non-compliant SKILL.md format, and the sampling ignores the spec's stratification requirement. Not shipping this without fixes.

---

## Critical Issues

### P5-01: Skill ID is not UUIDv7

**Spec §1.1:** `ID: UUIDv7, sortable by creation time`  
**Actual:** `fmt.Sprintf("skill-%s-%d", skillName, start.UnixMilli())`

This is a concatenated string, not a UUIDv7. It'll break any code that expects UUID parsing, and the ID format leaks the skill name (which changes the threat model for enumeration). Use a proper UUIDv7 library.

**File:** `pipeline.go:119`

### P5-02: Human review sampling is not stratified

**Spec §3.4:** "50% from extracted sessions (did we extract something good?) / 50% from rejected sessions (did we miss something?)"  
**Actual:** Uniform random sampling regardless of outcome.

The whole point of stratified sampling is to avoid the base-rate problem — if 90% of sessions are rejected, uniform 1% sampling gives you 90% rejected samples and barely any extracted ones to validate. The spec is explicit about this.

**File:** `pipeline.go:155–175`

### P5-03: SKILL.md format is wrong

`buildSkillMD` generates a front matter with `name`, `category`, `patterns`, `source_session` and then dumps truncated raw content. The spec references `pkg/skill/skill.go` v1 format. Missing from front matter:
- `version`
- `extracted_by`
- `quality` scores (at minimum composite)
- `id`

The body is just the first 2000 bytes of session content with no structure. A SKILL.md should contain the *extracted knowledge*, not a raw session dump. This is placeholder code pretending to be implementation.

**File:** `pipeline.go:194–215`

### P5-04: `generateSkillName` is unreliable for session content

The function takes the first "meaningful" line of content and kebab-cases it. Session logs don't start with titles — they start with timestamps, system messages, or user prompts like "hey can you fix the database". You'll get skill names like `hey-can-you-fix-the-database` or `2026-02-15t14-32-00z-user`.

The name should come from Stage 2/3 classification (patterns + category), not from raw content heuristics. At minimum, the LLM rubric response should include a suggested skill name.

**File:** `pipeline.go:179–215`

---

## Moderate Issues

### P5-05: Pipeline doesn't persist session state

The spec's `SessionRecord` has `ExtractionStatus`, `RejectedAtStage`, `RejectionReason`, `ExtractedSkillID` fields. The pipeline returns an `ExtractionResult` but never updates the session record in storage. The caller is presumably responsible, but there's no `SessionWriter` interface or any indication of how session state gets persisted. This is a gap in the wiring.

### P5-06: `CreatedAt` / `UpdatedAt` not set on SkillRecord

Pipeline constructs a `SkillRecord` but leaves `CreatedAt` and `UpdatedAt` as zero values. Either the pipeline should set them to `time.Now()` or `SkillWriter.Put` should — but that contract isn't documented anywhere.

**File:** `pipeline.go:121–133`

### P5-07: No `Extractor` interface declared

Spec §2.1 defines `Extractor` interface. It's not declared in `types.go` or anywhere in the package. `Pipeline` satisfies it structurally but there's no compile-time verification. Add `var _ Extractor = (*Pipeline)(nil)` or declare the interface.

### P5-08: `allScoresAre` is dead code

`stage3.go` defines `allScoresAre()` but it's never called. The contradiction detection uses `allScoresAbove()` and `averageScore()`. Remove it.

**File:** `stage3.go`

---

## Minor Issues

### P5-09: Error path returns `nil` error always

`Extract` never returns a non-nil `error`. All errors go into `result.Error` + `result.Status = StatusError`. The `Extractor` interface signature (`(*ExtractionResult, error)`) implies callers should check both. Either document this contract explicitly or actually return errors for truly fatal conditions (e.g., nil logger panic, nil stage1).

### P5-10: No nil-guard on stage dependencies

If `Stage1`, `Stage2`, `Stage3`, or `Writer` is nil in `PipelineConfig`, `Extract` will panic on first call. `NewPipeline` should validate required deps and return an error (or the constructor should return `(*Pipeline, error)`).

### P5-11: Redundant logging between pipeline and stages

Pipeline logs `"stage1 entry"`, `"stage1 pass"`. Stage1 itself logs `"stage1 pass"` and `"stage1 reject"`. Every stage transition is logged twice. Pick one location.

### P5-12: `nonAlphaNum` regex operates on already-lowercased ASCII but compiled for `[^a-z0-9]+`

The `strings.Map` call above it already converts non-letter/digit to spaces, so the regex is replacing spaces with hyphens. This works but it's an obfuscated two-step when a single `strings.Fields` + `strings.Join` would be clearer.

---

## Test Coverage Assessment

Tests are solid for what they cover:
- ✅ All rejection paths (stage 1, 2, 3)
- ✅ All error paths (stage 2, 3, store)
- ✅ Sampling probability math with deterministic rand
- ✅ Skill field propagation
- ✅ Timing non-negative
- ✅ `generateSkillName` edge cases
- ✅ Never-returns-error contract

Missing:
- ❌ No test for nil `Reviewer` (sampling disabled path — it's tested implicitly but not explicitly)
- ❌ No test for context cancellation mid-pipeline
- ❌ No test for `buildSkillMD` output format (front matter correctness)
- ❌ No test verifying `generateSkillName` with realistic session log content (timestamps, multi-turn)
- ❌ No test for concurrent `Extract` calls (pipeline should be safe for concurrent use if stages are)

---

## Consistency with Wired Components

| Component | Consistent? | Notes |
|-----------|------------|-------|
| `stage1.go` (Stage1Filter) | ✅ | Interface matches, pipeline calls `Filter(session)` correctly |
| `stage2.go` (Stage2Scorer) | ✅ | Interface matches, `Score(ctx, session, content)` correct |
| `stage3.go` (Stage3Critic) | ✅ | Interface matches, `Evaluate(ctx, session, content, s2)` correct |
| `types.go` | ⚠️ | Types used correctly but `Extractor` interface missing |
| `SkillWriter` | ✅ | Properly segregated from full `SkillStore` |
| `HumanReviewWriter` | ✅ | Properly segregated, clean interface |

Interface segregation is done well. `SkillWriter` and `HumanReviewWriter` are minimal, testable, and don't pull in the full storage interface. Good.

---

## Summary

The pipeline structure is correct — stages are wired in order, errors and rejections are handled differently (errors → `StatusError`, rejections → `StatusRejected`), logging is present at every decision point, and sampling is hooked in at every exit. The interface segregation is clean.

But the output quality is wrong: skill IDs aren't UUIDv7, skill names are derived from a bad heuristic, SKILL.md is a raw content dump, and human review sampling ignores stratification. These aren't edge cases — they're core requirements from the spec that got skipped.

Fix P5-01 through P5-04 before merging.
