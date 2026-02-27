# Architecture

This is the canonical architecture document. Every PR, spec, and design decision must align with it.

## Overview

Kinoko has two binaries and one communication channel:

- **`kinoko serve`** — Server
- **`kinoko run`** — Client
- **Git** — The only communication path between them

## Server (`kinoko serve`)

Git server + search index. Stores skill repos, indexes SKILL.md into SQLite, tracks git stats (last commit, contributors, clones). Two endpoints: `POST /api/v1/discover` (search with raw signals) and `POST /api/v1/embed` (embeddings). Post-receive hook re-indexes on push. No computation, no mutation, no client awareness.

**What the server does:**
- Hosts git repos (Soft Serve)
- Indexes SKILL.md on push (post-receive hook)
- Returns raw search signals on discover (pattern overlap, cosine similarity, quality scores, git stats)
- Computes embeddings on request
- Health endpoint

**What the server does NOT do:**
- Compute scores, decay, or rankings
- Store sessions, injection events, or per-client behavior
- Expose mutation endpoints (no PATCH, no PUT)
- Know anything about individual clients

## Client (`kinoko run`)

Runs alongside your agent. Extracts skills from sessions → commits SKILL.md → pushes to server via git. At injection time, asks server for matches, gets raw signals, combines with personal usage data, ranks locally. Computes decay from git metadata (freshness, activity). Personal experience stored in gitignored `.kinoko/` files inside cloned repos — never pushed.

**What the client does:**
- Extract skills from session logs (3-stage pipeline)
- Commit SKILL.md to git, push to server
- Query server for raw signals, rank locally
- Track personal usage in `.kinoko/local.json` (gitignored)
- Compute decay from git metadata (commit dates, activity)

**What the client does NOT do:**
- Write to server except via git push
- Share session data with the server
- Push `.kinoko/` files

## Git

Only write path. Only communication channel between client and server. Client pushes skills, server indexes on receive.

## The Boundary

Server never sees sessions or per-client behavior. Client never writes to server except git push. Shared knowledge flows through git. Personal experience stays local.

**Key principles:**
- No HTTP mutation endpoints on server. Ever.
- No client-side SQLite for skill metadata — gitignored files in cloned repos are the personal layer.
- Server returns raw data. Client decides.
- Per-client behavioral data (injection count, success, preferences) stays in gitignored `.kinoko/` files.
- Global signals (git activity, clone counts, contributors) come from the server's git stats.
- Decay is client-computed from git metadata, not a server concept.
- Absence of local data ≠ penalty. Personal data is additive boost only.

## Filesystem Layout

```
internal/
  run/          # Client code
  serve/        # Server code
  shared/       # Used by both (config, circuitbreaker, decay)
pkg/
  model/        # Core domain types (shared)
  skill/        # Public SKILL.md parser
```

## API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/v1/health` | Server status |
| `POST` | `/api/v1/discover` | Skill discovery (raw signals) |
| `POST` | `/api/v1/embed` | Compute embeddings |
| `POST` | `/api/v1/ingest` | Trigger re-indexing (post-receive hook) |
