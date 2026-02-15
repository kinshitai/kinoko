# Spec: Wire Git Into Skill Lifecycle

**Ticket:** G1  
**Author:** Hal (CTO)  
**Priority:** P0  
**Size:** L (full day)  
**Status:** Draft v2

---

## Problem

`SkillStore.Put()` writes to SQLite + local files but never commits to git. The manifesto says "Git is the truth — everything else is derived cache." But in our current code, SQLite is the truth and git is decorative.

## Architecture: Git-First

Git is the **write path**. SQLite is the **read cache**. Soft Serve hooks bridge them.

```
                    ┌─────────────────────────┐
                    │     Soft Serve          │
                    │   (git hosting)          │
                    │                         │
  git push ──────►  │  post-receive hook ──►  │ ──► Metadata Server
                    │                         │     (parse SKILL.md,
                    │  repo: local/fix-nplus1 │      compute embedding,
                    │  repo: local/retry-exp  │      write to SQLite)
                    └─────────────────────────┘
                              │
                    git clone/pull
                              │
                    ┌─────────▼───────────────┐
                    │      Clients            │
                    │  (read SKILL.md locally) │
                    └─────────────────────────┘
```

### Write Path (Extraction)

```
Session ends
  → Worker extracts skill
  → Worker writes SKILL.md to temp
  → Worker does `git push` to Soft Serve repo (creates repo if needed)
  → Soft Serve post-receive hook fires
  → Hook: parse SKILL.md → compute embedding → insert into SQLite
  → Skill is now discoverable + cloneable
```

SQLite is never written directly during extraction. It's populated by the hook.

### Read Path (Injection — server-side)

For server-side injection (current architecture, `kinoko serve`):
```
Session starts
  → Injection pipeline queries SQLite for matching skills (embeddings + patterns)
  → Reads SKILL.md from local clones (cached from git)
  → Injects into prompt
```

### Read Path (Injection — client-side)

For client-side injection (agents on remote machines):
```
Agent session starts
  → Client asks discovery server: "match this prompt"
  → Discovery returns: [{repo: "local/fix-nplus1", score: 0.87}, ...]
  → Client git clone/pull those repos (cached locally)
  → Client reads SKILL.md files
  → Client injects into prompt
```

Client works offline after initial clone. `git pull` periodically for updates.

### Recovery

```bash
# Delete SQLite — it's just a cache
rm ~/.kinoko/kinoko.db

# Rebuild from git (re-triggers hooks on all repos)
kinoko git rebuild
# → Lists all repos in Soft Serve
# → For each: clones, parses SKILL.md, computes embedding, inserts to SQLite
# → Done. Full recovery.
```

This is the "derived cache" promise made real.

## Design

### Repo-per-skill (RFC-002)

Each skill gets its own Soft Serve repo:
```
{library}/{skill-name}
  └── v{version}/
      └── SKILL.md
```

Examples:
```
local/fix-n-plus-one-queries/v1/SKILL.md
local/retry-with-exponential-backoff/v1/SKILL.md
company/circuit-breaker-pattern/v1/SKILL.md
company/circuit-breaker-pattern/v2/SKILL.md
```

### Soft Serve Hooks (native support confirmed)

Soft Serve supports standard git server-side hooks: `pre-receive`, `update`, `post-update`, `post-receive`. Both per-repo hooks (in repo's `hooks/` dir) and **global hooks** (in `{SOFT_SERVE_DATA_PATH}/hooks/`). Standard shell scripts.

Also supports **HTTP webhooks** per-repo for push events via `repo webhook create`.

This means the git-first architecture works natively:

**Global `post-receive` hook** at `{dataDir}/hooks/post-receive`:

```sh
#!/bin/sh
# Called after every successful push to any repo.
# Triggers indexing: parse SKILL.md → compute embedding → write SQLite.
#
# Environment: GIT_DIR is set, stdin has "<old> <new> <ref>" lines.
# The hook calls our indexer binary/endpoint.

while read oldrev newrev refname; do
    # Call kinoko's indexer
    kinoko index --repo "$SOFT_SERVE_REPO_NAME" --ref "$refname" --rev "$newrev"
done
```

**Implementation:** Rather than a shell script calling a CLI, we can write the hook as a small script that POSTs to our local HTTP API (G3), or calls `kinoko index` directly.

Better yet: use Soft Serve's **webhook** feature to POST to our discovery API on push events. This is the cleanest approach — no shell scripts, pure HTTP:

```sh
# Register a webhook for all repos (or per-repo)
ssh -p 23231 localhost repo webhook create <repo> \
    --url http://localhost:23232/api/v1/hooks/post-receive \
    --event push \
    --content-type json
```

**Recommended approach — both hooks for defense in depth:**

1. **Global `pre-receive` hook** — credential scanning (G2). Rejects pushes with secrets. Works for both our pipeline AND external contributors.

2. **Global `post-receive` hook** — calls `kinoko index` to parse SKILL.md, compute embedding, index into SQLite. Fires on every push from any source.

```go
// internal/gitserver/hooks.go

// InstallHooks writes the global pre-receive and post-receive scripts
// to the Soft Serve data directory.
func InstallHooks(dataDir string, kinokoBinary string) error {
    hooksDir := filepath.Join(dataDir, "hooks")
    os.MkdirAll(hooksDir, 0o755)
    
    // pre-receive: credential scanning
    preReceive := fmt.Sprintf(`#!/bin/sh
%s scan --stdin --reject`, kinokoBinary)
    
    // post-receive: indexing
    postReceive := fmt.Sprintf(`#!/bin/sh
while read oldrev newrev refname; do
    %s index --repo "$SOFT_SERVE_REPO_NAME" --rev "$newrev" &
done`, kinokoBinary)
    
    // Write + chmod +x
}

// NewCLI commands needed:
// kinoko index --repo <name> --rev <hash>  → parse SKILL.md, embed, index
// kinoko scan --stdin --reject             → scan stdin for credentials, exit 1 if found
```

### New Interface: `SkillIndexer`

Replace `SkillWriter` (which does too much) with `SkillIndexer` for the hook:

```go
// internal/model/indexer.go
type SkillIndexer interface {
    // IndexSkill upserts skill metadata + embedding into the discovery index.
    // This is the ONLY write path to SQLite for skills.
    IndexSkill(ctx context.Context, skill *SkillRecord, embedding []float32) error
}
```

### Modified Pipeline: `GitCommitter` replaces `SkillWriter`

The extraction pipeline no longer writes to SQLite directly. It commits to git.

```go
// internal/model/committer.go
type SkillCommitter interface {
    // CommitSkill creates a repo (if needed) and pushes SKILL.md.
    // Returns the commit hash.
    // The post-receive hook handles SQLite indexing.
    CommitSkill(ctx context.Context, libraryID string, skill *SkillRecord, body []byte) (string, error)
}
```

Pipeline flow changes:
```
Before: Extract → SkillWriter.Put() [SQLite + file]
After:  Extract → SkillCommitter.CommitSkill() [git push] → hook → SQLite
```

### Implementation: `internal/gitserver/committer.go`

```go
type GitCommitter struct {
    server    *Server
    dataDir   string
    logger    *slog.Logger
}

func (g *GitCommitter) CommitSkill(ctx context.Context, libraryID string, skill *SkillRecord, body []byte) (string, error) {
    repoName := fmt.Sprintf("%s/%s", libraryID, skill.Name)
    
    // 1. Create repo if it doesn't exist
    if err := g.server.CreateRepo(repoName, skill.Name); err != nil {
        // Ignore "already exists" errors
    }
    
    // 2. Clone or pull working copy
    workdir := filepath.Join(g.dataDir, "workdir", libraryID, skill.Name)
    // git clone if not exists, git pull if exists
    
    // 3. Write SKILL.md
    skillDir := filepath.Join(workdir, fmt.Sprintf("v%d", skill.Version))
    os.MkdirAll(skillDir, 0o755)
    os.WriteFile(filepath.Join(skillDir, "SKILL.md"), body, 0o644)
    
    // 4. git add + commit + push
    // Commit message: "v{version}: extracted from session {sessionID}"
    
    // 5. Return commit hash
    // Post-receive hook fires automatically → indexes into SQLite
}
```

### CLI Commands

**`kinoko git rebuild`** — rebuild SQLite from all repos:
```
Lists all repos in Soft Serve → for each:
  clone → parse SKILL.md → compute embedding → index into SQLite
Idempotent. Safe to run anytime.
```

**`kinoko git status`** — show state:
```
Library "local":
  Repos: 47 skills
  Index: 47 indexed, 0 stale
  Last push: 2026-02-15 abc1234 "v1: fix-n-plus-one-queries"
```

**`kinoko git sync`** — for migration from old architecture:
```
Walks skills in SQLite that have file_path but no git repo → commits them.
One-time migration command.
```

## Migration Path

The current `SkillStore.Put()` still works. Migration:

1. Ship G1 with `SkillCommitter` as the new write path in Pipeline
2. Keep `SkillStore.Put()` for backward compat (tests, manual imports)
3. Add `kinoko git sync` to migrate existing skills → git repos
4. Post-receive hook populates SQLite from git
5. Eventually, `SkillStore.Put()` becomes `SkillIndexer.IndexSkill()` (called only by hooks)

## RFC Alignment

| RFC-002 Requirement | This Spec | Status |
|---|---|---|
| Repo-per-skill | ✅ One Soft Serve repo per skill | Aligned |
| Git is the truth, everything else is derived cache | ✅ SQLite populated by git hooks, recoverable from repos | Aligned |
| Pre-commit hooks on contributor's machine | ⚠️ Phase 1: server-side scanning (G2). Phase 3: git pre-receive hooks on Soft Serve for external contributors | Deferred |
| Skills shadow lower layers by name | ❌ Multi-library resolution is a separate feature | Out of scope |
| Blow away DB and rebuild from git | ✅ `kinoko git rebuild` | Aligned |

## Testing

- Unit: mock `SkillCommitter`, verify pipeline calls it
- Unit: post-receive handler parses SKILL.md and indexes correctly
- Integration: real Soft Serve, extract → git push → verify hook fires → verify SQLite populated
- Recovery: create skills via git, delete SQLite, run rebuild, verify index matches
- Concurrent: two workers push simultaneously, no conflicts
- Clone: after push, `git clone ssh://localhost:23231/{lib}/{skill}` works

## Dependencies

- `git` binary on server
- Soft Serve running (already is in `kinoko serve`)
- `kinoko` binary accessible from hook scripts (for `kinoko index` and `kinoko scan`)
