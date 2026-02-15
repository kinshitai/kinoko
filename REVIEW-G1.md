# G1 Phase 1 Review — GitCommitter & Indexer

**Reviewer:** Jazz  
**Date:** 2026-02-15  
**Grade:** B-

---

## 1. File-by-File Code Review

### `internal/model/committer.go` — SkillCommitter Interface

Clean, minimal, correct. Nothing to complain about. That's rare.

One nit: the doc comment says "pushes a skill to a git repository" but the return value `(string, error)` doesn't document what the string is. Add `// Returns the commit hash.` or put it in the signature name: `(commitHash string, err error)`.

**Verdict:** ✅ Fine.

---

### `internal/model/indexer.go` — SkillIndexer Interface

Same — clean, minimal. The `embedding []float32` parameter is a bit raw. If the embedding model or dimension ever matters at the interface level you'll regret this, but for now it's acceptable.

**Verdict:** ✅ Fine.

---

### `internal/storage/indexer.go` — SQLiteIndexer

**Issues:**

1. **Mutating the input struct.** `IndexSkill` mutates `skill.CreatedAt` and `skill.UpdatedAt` in place. The caller passes a `*SkillRecord` and gets it silently modified. This is a side-effect land mine. Either accept the struct by value or document it loudly. In 30 years I've seen this cause exactly the kind of bug where someone passes the same skill to two goroutines and gets a race on `UpdatedAt`.

2. **`INSERT OR REPLACE` deletes then re-inserts.** This means any foreign key references to the skills row get cascade-deleted. If you ever add FK constraints pointing at `skills.id`, this will silently nuke related data. Use `INSERT ... ON CONFLICT(id) DO UPDATE SET ...` (UPSERT) instead. It's what SQLite added it for.

3. **`idx.store.embeddingModel`** — where does this come from? The `SQLiteStore` is initialized with `embeddingModel` as a constructor param, but `IndexSkill` just blindly trusts it. If the model changes between index runs (e.g., migration from `text-embedding-ada-002` to `text-embedding-3-small`), you'll have mixed embeddings in the same table with no way to distinguish them. The embedding model should probably be stored per-row or at least validated.

4. **Delete-then-insert for patterns and embeddings** inside a transaction is fine functionally, but generates unnecessary WAL churn. For patterns especially, consider diffing or just using UPSERT.

5. **No input validation.** `skill.ID` empty? `skill.Name` empty? You'll get a row with empty PK. Add guards.

**Verdict:** ⚠️ Works but has design smells. The struct mutation is the worst offender.

---

### `internal/gitserver/committer.go` — GitCommitter

This is the big one. Several issues:

**Bugs / Risks:**

1. **`os.Environ()` append is not safe.** `append(os.Environ(), ...)` may or may not copy the underlying slice. In practice Go's `os.Environ()` returns a fresh slice each call so this is *fine today*, but it's a bad habit. Use `slices.Concat` or explicitly copy.

2. **Shell injection via `sshCmd`.** `sshCmd` is built with `fmt.Sprintf` and includes `sshKey` (a file path). If `adminKeyPath` ever contains spaces or shell metacharacters, the `GIT_SSH_COMMAND` will break or worse. Use an SSH wrapper script or `ssh -F` with a config file instead.

3. **`isAlreadyExists` is fragile.** String-matching on error messages is the cockroach of error handling — it survives everything but never works reliably. If Soft Serve changes its error wording, or localizes it, this breaks silently. At minimum, check for specific exit codes or structured errors.

4. **Empty repo detection** in `ensureWorkdir` checks for "empty" or "warning" in output. Git's clone of an empty repo says `"warning: You appear to have cloned an empty repository."` — but you're matching substrings broadly. The word "warning" could appear in any git output. Match the specific message.

5. **No cleanup on failure.** If `commitAndPush` fails after `ensureWorkdir` created the workdir, the half-baked workdir persists. Next call will find `.git` and try to pull, which may also fail. Add cleanup or at least document the recovery path.

6. **Concurrent access to workdir.** Two goroutines extracting the same skill name simultaneously will stomp on the same workdir. There's no locking. The worker pool presumably serializes this, but `GitCommitter` itself has no protection. Add a per-repo mutex or use temp dirs.

7. **`git pull --ff-only`** will fail if someone force-pushed or if local has divergent commits (e.g., from a previous failed push that committed locally but didn't push). Should handle this — reset to origin, or delete and re-clone.

**Design Smells:**

8. **GitCommitter knows about indexing AND embedding.** The spec says "post-receive hook handles SQLite indexing" but the implementation has `GitCommitter` calling `indexer.IndexSkill()` directly after push. This is the application-level indexing that the spec wants to replace with hooks. More on this in Section 2.

9. **Embedding is computed twice.** The pipeline computes embedding in `Extract()` (stored on `skill.Embedding`), and then `GitCommitter` computes it *again* via `g.embedder.Embed()`. That's two LLM API calls for the same content. Wasteful.

10. **SSH key path used directly.** `g.server.adminKeyPath` is accessed as a raw field. If Server is ever refactored, this breaks. Expose a method.

11. **`git` binary dependency is undocumented.** No check that `git` is installed, no minimum version requirement. The `exec.Command("git", ...)` calls will fail with an unhelpful error.

**Verdict:** ⚠️⚠️ Functional for happy path, fragile for everything else. The concurrent access and double-embedding issues are the most concerning.

---

### `internal/extraction/pipeline.go` — CommitSkill Integration

The integration is clean. Committer is optional (nil-safe), failure is non-fatal, logging is adequate. Good.

**Issues:**

1. **Commit happens after `writer.Put()`.** So the skill is already in SQLite *before* git push. If git push fails, SQLite has a skill that git doesn't. This contradicts the "git is truth, SQLite is cache" principle. In the spec's model, `writer.Put()` should be removed from the pipeline and indexing should only happen via the hook.

2. **No commit hash stored.** The commit hash returned by `CommitSkill` is logged but not stored on the `SkillRecord` or `ExtractionResult`. You'll want this for traceability and the `git status` CLI command.

**Verdict:** ⚠️ Clean integration but architecturally backwards — writes SQLite first, then git.

---

### `internal/extraction/committer_test.go` — Tests

Tests cover the three important cases: happy path, error-is-non-fatal, nil-committer. That's good.

**Gaps:**

1. **No test for double-embedding.** Doesn't verify that CommitSkill receives correct body content.
2. **No test for commit hash propagation.** The hash `"abc123"` is returned but never asserted against `result` (because it's not stored — see above).
3. **No concurrency test.** Given the workdir stomping risk, this matters.
4. **Relies on `pipelineTestSession()` and mock stages from another file.** Fine, but the test file doesn't show those helpers — hope they're solid.

**Verdict:** ✅ Adequate for Phase 1. Not great, not terrible.

---

### `internal/storage/indexer_test.go` — Tests

Good coverage: insert, upsert, nil embedding, pattern replacement, embedding replacement.

**Gaps:**

1. **No test for empty skill ID or name.** (See input validation concern above.)
2. **No test for concurrent IndexSkill calls.** SQLite under concurrent writes is a known foot-gun.
3. **No test verifying the embedding model is stored correctly.**
4. **Doesn't test `CreatedAt` preservation on upsert** — the code sets `CreatedAt` only if zero, but the test always sets it.

**Verdict:** ✅ Solid for a first pass.

---

### `cmd/mycelium/serve.go` — Wiring

**Issues:**

1. **`buildPipeline` creates a *second* embedder.** `buildSessionHooks` already creates one. Two `embedding.New()` instances for the same API key. Not a bug, but wasteful — share the instance.

2. **GitCommitter gets both indexer and embedder.** This is the architectural mismatch — in the hook model, GitCommitter shouldn't need either. The wiring reflects the current (wrong) architecture.

3. **`store` is opened with empty `embeddingModel` string.** `storage.NewSQLiteStore(cfg.Storage.DSN, "")` — that empty string propagates to `skill_embeddings.model`. Every embedding row will have `model = ""`. That's a data quality bug.

4. **Error handling in `buildPipeline`** returns `nil, nil` when there's no API key. The caller checks for nil pipeline, but a more explicit pattern (e.g., sentinel error) would be clearer.

**Verdict:** ⚠️ Works but has wiring issues that will bite during the hook migration.

---

## 2. Hook Architecture Evaluation

### What the spec wants:

- **Global `post-receive` hook**: fires on every push → parses SKILL.md → computes embedding → writes SQLite
- **Global `pre-receive` hook**: credential scanning (G2)
- **Pipeline**: only does `git push`, never writes SQLite directly
- **SQLite**: populated exclusively by hooks, recoverable from git via `mycelium git rebuild`

### What Otso built:

- `GitCommitter` does git push AND calls `indexer.IndexSkill()` AND calls `embedder.Embed()`
- Pipeline still calls `writer.Put()` (SQLite) before `committer.CommitSkill()` (git)
- No hooks installed. No hook infrastructure.

### Reusability Assessment

| Component | Reusable? | Notes |
|---|---|---|
| `SkillCommitter` interface | ✅ Yes | Perfect. Keep as-is. |
| `SkillIndexer` interface | ✅ Yes | This is exactly what the hook needs to call. |
| `SQLiteIndexer` implementation | ✅ Yes | The hook's indexing logic will call this. Core is solid. |
| `GitCommitter.CommitSkill` (git ops) | ✅ Mostly | The clone/commit/push logic is reusable. Strip out the indexer/embedder calls. |
| `GitCommitter` indexer/embedder fields | ❌ Remove | Hook handles this, not the committer. |
| `ensureWorkdir` / `commitAndPush` | ✅ Yes | Pure git plumbing. Keep. |
| Pipeline's `writer.Put()` call | ❌ Remove | In the hook model, pipeline should NOT write SQLite. |
| Pipeline's `committer.CommitSkill()` call | ✅ Yes | This becomes the ONLY write. |
| `buildPipeline` wiring | ⚠️ Refactor | GitCommitter should be simpler (no indexer/embedder). |

**Bottom line: ~70% reusable.** The git plumbing is good. The indexer is good. They just need to be decoupled — committer does git only, indexer runs in the hook.

### Should GitCommitter still call the indexer directly?

**No.** The spec is clear: hook-triggered indexing. Direct indexing creates two problems:

1. **Dual writes.** SQLite gets written by both the pipeline (`writer.Put`) and the committer (`indexer.IndexSkill`). If either fails, they're out of sync.
2. **Bypasses the hook contract.** External pushes (future contributors, `mycelium git sync`) won't trigger application-level indexing — only the hook does. If the committer also indexes, you have two code paths doing the same thing.

**However**, there's a pragmatic argument for Phase 1: hooks aren't implemented yet. Keeping direct indexing as a stopgap until hooks land is fine, **if** it's explicitly marked as temporary with a `// TODO: remove when post-receive hook is implemented` comment.

### Is SQLiteIndexer the right abstraction?

**Yes.** It's exactly what the `post-receive` hook needs to call. The interface is clean (`IndexSkill(ctx, skill, embedding)`), and the implementation is solid. When the hook fires:

```
post-receive hook
  → parse SKILL.md from repo
  → call embedder
  → call SQLiteIndexer.IndexSkill()
```

The indexer doesn't need to change. What changes is *who calls it*.

---

## 3. Recommended Changes

### Immediate (before merge)

1. **Add `// TODO` on direct indexing.** Mark `GitCommitter`'s indexer/embedder usage and pipeline's `writer.Put()` as temporary until hooks land.

2. **Fix double embedding.** Pipeline computes embedding, then GitCommitter computes it again. Pass the already-computed embedding through, or remove it from GitCommitter entirely.

3. **Fix empty `embeddingModel` in serve.go.** Pass the actual model name to `NewSQLiteStore`.

4. **Fix struct mutation in `SQLiteIndexer.IndexSkill`.** Don't mutate the caller's `*SkillRecord`. Copy the timestamps locally or accept by value.

5. **Store commit hash on SkillRecord/ExtractionResult.** You'll need it.

### Short-term (G1 Phase 2: hooks)

6. **Implement `InstallHooks(dataDir)`** — write global `post-receive` shell script that calls `mycelium index`.

7. **Implement `mycelium index` CLI command** — parses SKILL.md from repo, computes embedding, calls `SQLiteIndexer.IndexSkill()`.

8. **Strip indexer/embedder from GitCommitter.** It becomes pure git: create repo, clone, write, commit, push. That's it.

9. **Remove `writer.Put()` from pipeline.** Git push is the only write. Hook populates SQLite.

10. **Add per-repo mutex or temp workdirs** to prevent concurrent workdir stomping.

### Medium-term (G2+)

11. **Implement `pre-receive` hook** for credential scanning.
12. **Implement `mycelium git rebuild`** — iterates all repos, re-indexes.
13. **Replace `isAlreadyExists` string matching** with structured error handling from Soft Serve SSH commands.
14. **Add integration test** — real Soft Serve, push, verify hook fires, verify SQLite populated.

---

## Summary

Otso built a solid first pass. The git plumbing works, the interfaces are clean, and the indexer is well-tested. The main problem is architectural: he wired indexing *inside* the committer instead of leaving it to hooks, and the pipeline still writes SQLite directly. This is understandable — hooks weren't confirmed when he started — but it needs to be refactored before G1 is "done."

The good news: most of the code survives the refactor. It's a reshuffling, not a rewrite.

**Grade: B-** — Good bones, wrong wiring, needs another pass.
