# Kinoko Server API Endpoints

All endpoints are served by `kinoko serve` on the configured API port (default `:23233`).

## Core Endpoints (always registered)

| Method | Path | Description | Request Body | Response Body | Client Caller |
|--------|------|-------------|-------------|---------------|---------------|
| `GET` | `/api/v1/health` | Health check + skill count | — | `HealthResponse{status, skills}` | — |
| `POST` | `/api/v1/discover` | Semantic skill discovery | `DiscoverRequest{prompt, limit}` | `DiscoverResponse{skills[]}` | `client.Client.Discover()` |
| `GET` | `/api/v1/discover?q=&limit=` | GET variant of discover | query params | `DiscoverResponse{skills[]}` | — |
| `POST` | `/api/v1/ingest` | Queue a session for extraction | `IngestRequest{session_id, log}` | `{queued: true}` | `client.Client.Ingest()` |

## Write-Path Endpoints (T4)

| Method | Path | Description | Request Body | Response Body | Client Caller |
|--------|------|-------------|-------------|---------------|---------------|
| `POST` | `/api/v1/sessions` | Create session record | `CreateSessionRequest{session, extraction_result?}` | `{id}` | `serverclient.HTTPSessionWriter.WriteSession()` |
| `PUT` | `/api/v1/sessions/{id}` | Update extraction result | `UpdateSessionBody{extraction_status, ...}` | `{id}` | `serverclient.HTTPSessionWriter.UpdateSession()` |
| `POST` | `/api/v1/review-samples` | Store human review sample | `CreateReviewSampleRequest{session_id, result_json}` | `{session_id}` | `serverclient.HTTPReviewer.WriteReviewSample()` |
| `POST` | `/api/v1/injection-events` | Record injection event | `InjectionEventRecord` (JSON) | `{id}` | `serverclient.HTTPInjectionEventWriter.WriteEvent()` |
| `PUT` | `/api/v1/injection-events/{session_id}/outcome` | Update injection outcome | `UpdateInjectionOutcomeRequest{outcome}` | `{session_id}` | `serverclient.HTTPInjectionEventWriter.UpdateOutcome()` |

## Search & Query Endpoints

| Method | Path | Description | Request Body | Response Body | Client Caller |
|--------|------|-------------|-------------|---------------|---------------|
| `POST` | `/api/v1/search` | Pattern + embedding skill search | `SearchRequest{patterns, embedding, library_ids, ...}` | `SearchResponse{results[]}` | `serverclient.HTTPQuerier.Query()` |
| `GET` | `/api/v1/skills/decay` | List skills ordered by decay | query: `?library_id=&limit=` | `DecayListResponse{skills[]}` | `serverclient.HTTPDecayClient.ListByDecay()` |
| `PATCH` | `/api/v1/skills/{id}/decay` | Update a skill's decay score | `UpdateDecayRequest{decay_score}` | `{id}` | `serverclient.HTTPDecayClient.UpdateDecay()` |
| `POST` | `/api/v1/skills/{id}/usage` | Record skill usage outcome | `UpdateUsageRequest{outcome}` | `{id}` | `serverclient.HTTPQuerier.UpdateUsage()` |

## Optional Endpoints (registered at runtime)

These are registered via `SetNoveltyChecker()`, `SetEmbedEngine()`, or `SetMatchHandler()` when the embedding engine is available.

| Method | Path | Description | Request Body | Response Body | Client Caller |
|--------|------|-------------|-------------|---------------|---------------|
| `POST` | `/api/v1/novelty` | Check if content is novel vs existing skills | `NoveltyRequest{content, threshold?}` | `NoveltyResponse{novel, score, similar[]}` | `serverclient.HTTPQuerier.QueryNearest()` |
| `POST` | `/api/v1/embed` | Compute embedding vector for text | `EmbedRequest{text}` | `EmbedResponse{vector, model, dims}` | `serverclient.HTTPEmbedder.Embed()` |
| `POST` | `/api/v1/match` | Context-aware skill matching with content | `MatchRequest{context, limit, min_score}` | `MatchResponse{skills[]}` | `client.Client.Match()` |

## Notes

- All endpoints return JSON with `Content-Type: application/json`.
- Error responses use `{"error": "message"}` format.
- The discover endpoint is rate-limited to 10 concurrent requests (429 on overflow).
- The ingest endpoint limits request body to 10 MB.
- Request/response types are defined in `internal/api/` alongside their handlers.
