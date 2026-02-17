# Contributing to Kinoko

## Git Flow

**Never push directly to `main`.** All changes go through pull requests.

### Workflow

```
1. Create a branch    →  git checkout -b <type>/<short-name>
2. Do the work        →  commits on branch
3. Push & open PR     →  git push -u origin <branch>
4. CI must pass       →  Lint + Test + Build (all three green)
5. Jazz reviews code  →  must approve before merge
6. Squash merge       →  into main, delete branch
```

### Branch Naming

```
feat/<name>       — new feature or capability
fix/<name>        — bug fix
docs/<name>       — documentation only
refactor/<name>   — code restructuring, no behavior change
test/<name>       — test additions or fixes
chore/<name>      — CI, tooling, cleanup
```

### PR Requirements

Before merging, every PR must have:

- [ ] **CI green** — Lint, Test, Build all pass
- [ ] **Jazz review** — code review approved (grade B+ or higher)
- [ ] **No architectural drift** — changes align with MANIFESTO.md and RFCs
- [ ] **Docs updated** — if behavior changed, docs reflect it (Charis owns this)

### Commit Messages

```
<type>: <short description>

<optional body — what and why, not how>
```

Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`

Keep it English only. No emoji in commit messages.

### Who Does What

| Role | Person | Responsibility |
|---|---|---|
| Architecture & specs | Hal 🔧 | Write specs, delegate, guard architecture |
| Implementation | Otso 🇫🇮 | Build from specs, branch + PR |
| Code review | Jazz 👴 | Review every PR, grade B+ minimum to merge |
| QA | Pavel 🇷🇺 | Test coverage, integration tests, verify on branch |
| Docs | Charis 🇨🇧 | Update docs when behavior changes, audit regularly |
| R&D | Luka 🇩🇰 | Research briefs, specs for new features |

### Review Process

1. Otso (or whoever implements) opens PR with description of what changed and why
2. Pavel runs tests on the branch, reports coverage delta
3. Jazz reviews code — checks correctness, security, alignment with spec
4. Jazz approves or requests changes with specific feedback
5. Once approved + CI green → squash merge to main

### What Hal Does NOT Do

- Write code directly (delegate to Otso)
- Push to main (nobody does)
- Skip review (even "obvious" fixes go through Jazz)

---

## Architecture Overview

Kinoko uses a **client/server split**. Full details: [`internal-docs/docs/architecture.md`](internal-docs/docs/architecture.md).

- **`kinoko serve`** — infrastructure server: Soft Serve git server, HTTP API, SQLite index DB, decay scheduler
- **`kinoko run`** — local agent daemon: worker pool, extraction pipeline, injection, local queue DB
- Communication: HTTP (`internal/serverclient/`) + git push (SSH to Soft Serve)

---

## Client/Server Boundary

**The rule:** `kinoko run` never imports `internal/storage`. All server access goes through `internal/serverclient/` (HTTP) or git push (SSH).

This means:
- Client-side code (`run.go`, `workers_run.go`, `extract.go`, `queuecmd.go`) → uses `queue`, `serverclient`, `model`
- Server-side code (`serve.go`, `serve_scheduler.go`, `index.go`, `rebuild.go`, `stats.go`, `decay.go`, `importcmd.go`) → can use `storage` directly

### How to verify

```bash
# Must return zero hits:
grep -rn "internal/storage" cmd/kinoko/run.go cmd/kinoko/workers_run.go cmd/kinoko/queuecmd.go cmd/kinoko/extract.go

# See which files DO import storage (should only be serve-side):
grep -rn "internal/storage" cmd/kinoko/*.go | grep -v _test.go
```

### Why this matters

The boundary ensures `kinoko run` can operate on a different machine from `kinoko serve`. No shared filesystem, no shared database. The only coupling is the HTTP API contract and the git protocol.

---

## Adding New Features

### Decision framework: which side does it go on?

| Question | If yes → |
|----------|----------|
| Does it read/write the skill index DB? | **Server** (`internal/storage/`, `internal/api/`) |
| Does it process session logs or run LLM calls? | **Client** (`internal/extraction/`, `internal/injection/`) |
| Does it manage the local job queue? | **Client** (`internal/queue/`) |
| Is it a pure domain type or interface? | **Shared** (`internal/model/`) |
| Is it a new API endpoint the client needs? | **Both** — endpoint in `internal/api/`, client adapter in `internal/serverclient/` |

### Adding a new server API endpoint

1. Add handler in `internal/api/` (e.g., `api/my_feature.go`)
2. Add HTTP client adapter in `internal/serverclient/` (e.g., `serverclient/my_feature.go`)
3. Define any new shared types in `internal/model/`
4. Wire the adapter into `workers_run.go` → `buildClientPipeline()` or the relevant client code
5. Verify: `grep -rn "internal/storage" cmd/kinoko/run.go` still returns nothing

### Adding a new extraction stage or modifier

1. Add to `internal/extraction/` — implement the relevant interface
2. Wire in `workers_run.go` → `buildClientPipeline()`
3. If it needs server data, add an API endpoint (see above)

### Adding a CLI command

- **Server admin command** (needs index DB): add to `cmd/kinoko/`, import `storage` freely
- **Client command** (user-facing): add to `cmd/kinoko/`, use `queue` + `serverclient` only
