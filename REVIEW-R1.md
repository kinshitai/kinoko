# R1 Review: Extract `internal/model` Package

**Reviewer:** Jazz  
**Date:** 2026-02-15  
**Verdict:** Pass  
**Grade:** B+

---

## Summary

The refactoring is mechanically correct. All shared domain types moved to `internal/model`. Build passes. All 13 test packages pass. No import cycles. No stale references to `extraction.SkillRecord` and friends — I grepped, believe me.

I hate giving passing grades, but facts are facts.

---

## What Was Done Right

- **Clean separation:** `extraction/types.go` was gutted down to a single `Stage1Filter` interface — which is genuinely extraction-specific. Good.
- **File organization in `internal/model/`:** Types are split logically: `skill.go`, `session.go`, `result.go`, `injection.go`, `domain.go`, `extractor.go`. Matches the plan exactly. I can't complain about the file boundaries because they're exactly where I said they should be.
- **Tests exist:** `model_test.go` covers `MinimumViable`, `HighValue`, `InjectionPriority`, status/category constants, `ValidateDomain`, and `ValidDomains`. Table-driven. Idiomatic. *Fine.*
- **Zero behavioral changes:** Pure mechanical move. The type definitions are byte-for-byte identical to what was in `extraction/types.go`.
- **All imports updated:** No package in the repo still references `extraction.SessionRecord`, `extraction.SkillRecord`, etc. Verified via grep.

---

## Issues

### Minor (not blocking)

**1. `db:"..."` struct tags still present** — `SkillRecord`, `SessionRecord`, `QualityScores` all carry `db:"..."` tags. Nobody uses `sqlx` or any tag-based scanner in this codebase — all scanning is manual. These tags are dead weight. But R14 in the plan explicitly defers this to later, so I won't hold it against R1. Just noting it's still there, waiting to confuse someone.

**2. `ValidDomains` and `ValidateDomain` in model package** — The plan said to move these here, and they were moved. But I still think they belong in a `taxonomy` package (R5). Having domain validation logic in a types package is mildly smelly — `model` should be dumb structs, not business rules. The `ValidateDomain` function makes a policy decision ("default to Backend"). That's not a type. But again, R5 will handle this, and the plan explicitly put them here as a staging step. Grudgingly acceptable.

**3. No test for `Extractor` interface** — `extractor.go` defines the `Extractor` interface but there's no compile-time interface satisfaction check anywhere in the test file. This is a nit — Go will catch it at the call site anyway — but a `var _ Extractor = (*extraction.Pipeline)(nil)` somewhere would be nice documentation.

---

## Verification

```
$ go build ./...          # ✅ Clean
$ go test ./... -short    # ✅ All 13 packages pass
$ grep -rn 'extraction\.(SkillRecord|SessionRecord|...)' # ✅ Zero hits
```

Remaining `extraction.` imports in `injection/`, `cmd/`, and `tests/` are all for pipeline-specific symbols (`LLMClient`, `Taxonomy`, `ValidPattern`, `NewPipeline`, etc.) — those are R2/R5 scope, not R1.

---

## Final Word

It's a clean mechanical refactor. The types moved. The imports updated. Nothing broke. The file organization matches the spec. The tests cover the methods that have logic.

I wanted to find something wrong. I really did. The best I've got is struct tags that shouldn't exist and a domain function that arguably belongs one package over. Neither is worth blocking on.

*Grudgingly approved.*

— Jazz
