# T5 Part 1 — HTTP Client Implementations (Core + Simple Clients)

**Author:** Otso  
**Date:** 2026-02-17  
**Branch:** `feat/t5-serverclient`  
**Commit:** `774b164`

## What was done

Created `internal/serverclient/` package with 7 files (557 LOC):

| File | Type | Endpoints |
|------|------|-----------|
| `client.go` | Base `Client` + `doJSON` helper | All (shared) |
| `embed.go` | `HTTPEmbedder` → `model.Embedder` | `POST /api/v1/embed` |
| `session.go` | `HTTPSessionWriter` | `POST /api/v1/sessions`, `PUT /api/v1/sessions/{id}`, `GET /api/v1/sessions/{id}` |
| `review.go` | `HTTPReviewer` | `POST /api/v1/review-samples` |
| `search.go` | `HTTPSkillStore` (read-only) | `POST /api/v1/search` |
| `querier.go` | `HTTPQuerier` → `model.SkillQuerier` | `POST /api/v1/search` (see note) |
| `client_test.go` | Tests for all clients | httptest mocks |

## Design decisions

- **APIError type** — structured error with status code + parsed message for caller inspection
- **HTTPQuerier uses /api/v1/search** not /api/v1/novelty — the novelty endpoint takes text content (embeds server-side), but `model.SkillQuerier` passes pre-computed embeddings. Using `/search` with embedding + limit=1 achieves the same result. TODO added to switch when a dedicated embedding-based novelty endpoint exists.
- **GetSession** — implemented against `GET /api/v1/sessions/{id}` which doesn't exist yet on the server. TODO added for T4 follow-up.
- **EmbedBatch** — sequential calls to Embed. Could be parallelized later.

## Verification

- `go build ./...` ✅
- `go test -race ./internal/serverclient/` ✅
- `go vet ./internal/serverclient/` ✅

## Remaining T5 work (Part 2)

- `commit.go` — GitPushCommitter (git clone + push via SSH)
- `injection.go` — injection event writer
- `decay.go` — decay reader/writer HTTP clients
