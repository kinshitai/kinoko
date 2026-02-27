# Architecture

> Canonical architecture document. Every PR, spec, and design decision must align with this.

## Overview

Kinoko has two binaries and one communication channel:

| Component | Binary | Role |
|-----------|--------|------|
| **Server** | `kinoko serve` | Git hosting, search index, embeddings |
| **Client** | `kinoko run` | Extraction, injection, local ranking |
| **Git** | — | The *only* communication path between them |

## Server (`kinoko serve`)

The server is pure infrastructure: git repos, a search index, and an embedding engine. It has no concept of sessions, clients, or behavioral data.

**Does:**
- Host git repos (Soft Serve)
- Index SKILL.md on push (post-receive hook)
- Return raw search signals on discover (pattern overlap, cosine similarity, quality scores, git stats)
- Compute embeddings on request
- Serve a health endpoint

**Does not:**
- Compute scores, decay, or rankings
- Store sessions, injection events, or per-client behavior
- Expose mutation endpoints (no PATCH, no PUT)
- Know anything about individual clients

## Client (`kinoko run`)

The client runs alongside your agent. It extracts skills from sessions, commits SKILL.md, pushes to the server via git. At injection time it queries the server for raw signals, combines them with local usage data, and ranks locally.

**Does:**
- Extract skills from session logs (3-stage pipeline)
- Commit SKILL.md to git and push to server
- Query server for raw signals, rank locally
- Track personal usage in `.kinoko/local.json` (gitignored)
- Compute decay from git metadata (commit dates, activity)

**Does not:**
- Write to server except via `git push`
- Share session data with the server
- Push `.kinoko/` files

## Git as the Communication Path

Git is the only write path between client and server. Client pushes skills; server indexes on receive. No HTTP mutations. No side channels.

## The Boundary

Server never sees sessions or per-client behavior. Client never writes to server except via `git push`. Shared knowledge flows through git. Personal experience stays local.

**Principles:**
- No HTTP mutation endpoints on server. Ever.
- No client-side SQLite for skill metadata — gitignored files in cloned repos are the personal layer.
- Server returns raw data. Client decides.
- Per-client behavioral data (injection count, success, preferences) stays in gitignored `.kinoko/` files.
- Global signals (git activity, clone counts, contributors) come from the server's git stats.
- Decay is client-computed from git metadata — not a server concept.
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
