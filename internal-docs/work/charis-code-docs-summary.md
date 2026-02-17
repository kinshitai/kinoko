# Code-Level Docs Audit — Summary

**Branch:** `docs/code-level-docs`  
**Date:** 2026-02-17  
**Author:** Charis 🇨🇦

## What was done

### 1. Package-level doc comments

| Package | Status |
|---------|--------|
| `internal/model/` | **Added** — describes core domain types and interface contracts |
| `internal/worker/` | **Added** — describes worker pool, concurrency, retries |
| `internal/queue/` | ✅ Already accurate ("client-side SQLite-backed work queue") |
| `internal/serverclient/` | ✅ Already accurate ("HTTP client implementations for kinoko run") |
| `internal/storage/` | ✅ Already accurate ("SQLite-backed persistence for skills, sessions...") |
| `internal/api/` | ✅ Already accurate |
| `internal/circuitbreaker/` | ✅ Already accurate |
| `internal/client/` | ✅ Already accurate |
| `internal/config/` | ✅ Already accurate |
| `internal/debug/` | ✅ Already accurate |
| `internal/decay/` | ✅ Already accurate |
| `internal/embedding/` | ✅ Already accurate |
| `internal/extraction/` | ✅ Already accurate |
| `internal/gitserver/` | ✅ Already accurate |
| `internal/injection/` | ✅ Already accurate |
| `internal/llm/` | ✅ Already accurate |
| `internal/llmutil/` | ✅ Already accurate |
| `internal/metrics/` | ✅ Already accurate |
| `internal/sanitize/` | ✅ Already accurate |

### 2. `internal/model/` type docs

All moved types already have doc comments:
- `SkillStore`, `SkillQuery`, `ScoredSkill` — in `store_types.go`
- `InjectionEventRecord`, `SimilarSkill` — in `store_types.go`
- `Embedder` — in `embedder.go`
- `SkillQuerier`, `SkillQueryResult` — in `querier.go`
- `SkillCommitter` — in `committer.go`
- `Extractor` — in `extractor.go`
- `SkillIndexer` — in `indexer.go`
- `SessionRecord`, `ExtractionStatus` — in `session.go`
- `SkillRecord`, `QualityScores`, `SkillCategory` — in `skill.go`
- `ExtractionResult`, `Stage1/2/3Result` — in `result.go`
- Injection types — in `injection.go`

### 3. `cmd/kinoko/` file headers

Added file-level comments to all 10 command files specifying purpose and client/server role:

| File | Role |
|------|------|
| `run.go` | Client daemon |
| `serve.go` | Server daemon |
| `serve_scheduler.go` | Server-side decay scheduler |
| `workers_run.go` | Client-side pipeline wiring |
| `extract.go` | Client-side CLI command |
| `index.go` | Server-side command |
| `rebuild.go` | Server-side command |
| `decay.go` | Server-side command |
| `importcmd.go` | Server-side command |
| `queuecmd.go` | Client-side command |

### 4. API endpoint reference

Created **`docs/api-endpoints.md`** documenting all 17 HTTP endpoints:
- 4 core endpoints (health, discover GET/POST, ingest)
- 5 write-path endpoints (sessions CRUD, review samples, injection events)
- 4 search/query endpoints (search, decay list/update, usage)
- 3 optional endpoints (novelty, embed, match — registered at runtime)

Each entry includes method, path, request/response types, and the client function that calls it.

## Not done

- No PR opened (as requested)
