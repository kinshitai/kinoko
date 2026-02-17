# T6: Rewire `kinoko run` — Summary

**Author:** Otso  
**Branch:** `feat/t6-rewire-run`  
**Date:** 2026-02-17  

## What Changed

### `cmd/kinoko/run.go`
- Replaced `storage.NewSQLiteStore()` with `queue.New()` + `serverclient.New()`
- No longer imports `internal/storage`

### `cmd/kinoko/workers_serve.go` (renamed from `workers.go`)
- Unchanged — server-side wiring for `kinoko serve` and `kinoko worker`

### `cmd/kinoko/workers_run.go` (new)
- `buildClientPipeline()`: wires extraction using serverclient types:
  - `serverclient.NewHTTPEmbedder` for embeddings
  - `serverclient.NewHTTPQuerier` for skill search
  - `serverclient.NewGitPushCommitter` for git commits via SSH
  - `serverclient.NewHTTPSessionWriter` for session CRUD
  - `serverclient.NewHTTPReviewer` for review samples
- `startClientWorkerSystem()`: uses `queue.NewQueue()` for local work queue, passes `nil` for decay (moves to serve in T7)

### `cmd/kinoko/queuecmd.go`
- Uses `queue.New()` instead of `storage.NewSQLiteStore()`
- Queries `queue_entries` table instead of `sessions`

### `cmd/kinoko/extract.go`
- Uses `serverclient.New()` for all server communication
- Removed local `httpEmbedder` type (replaced by `serverclient.HTTPEmbedder`)
- No longer imports `internal/storage`

### `internal/config/config.go`
- Added `ClientConfig` with `QueueDSN` field
- Added `GetQueueDSN()` (default `~/.kinoko/queue.db`)
- Added `ServerURL()` method

### Test updates
- `extract_test.go`: updated interface check to use `serverclient.NewHTTPQuerier`
- `http_embedder_test.go`: rewritten to use `serverclient.HTTPEmbedder`
- `cli_e2e_test.go`: updated `httpEmbedder` test to use `serverclient.HTTPEmbedder`

## Verification
- `grep -r "internal/storage" cmd/kinoko/run.go` → no matches ✅
- `go build ./...` → clean ✅
- `go test -race ./...` → all pass ✅
- `go vet ./...` → clean ✅

## Files NOT touched (server-side, as specified)
- `serve.go`, `index.go`, `rebuild.go`, `importcmd.go`, `decay.go`
