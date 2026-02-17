# Contributing to Mycelium

Welcome to the Mycelium project! This document covers our development workflow, testing practices, architecture patterns, and contribution guidelines.

## Quick Start

```bash
# Clone and setup
git clone <repository-url>
cd mycelium
make install-hooks  # Installs pre-commit hook with full test suite

# Run all tests
go test -race ./...                                    # Unit tests
go test -tags integration -race ./tests/integration/   # Integration tests

# Development workflow
git checkout -b feat/your-feature-name
# Make changes...
git commit -m "feat: add new feature"
git push origin feat/your-feature-name
# Open PR → CI green → review → squash merge to main
```

## Development Workflow

### Git Flow

We use a **branch-based workflow** with squash merging:

1. **Branch** from `main` using our naming convention
2. **Implement** your changes with tests
3. **Push** to your branch — pre-commit hook runs full test suite
4. **Open PR** — CI must be green before review
5. **Review** by team member
6. **Squash merge** to main — preserves clean history

### Branch Naming

Use these prefixes to categorize your work:

```
feat/feature-name        # New features or capabilities  
fix/issue-description    # Bug fixes
docs/documentation-area  # Documentation updates
refactor/component-name  # Code restructuring without behavior changes
test/test-description    # Test additions or improvements
chore/maintenance-task   # Dependency updates, tooling, etc.
```

Examples:
- `feat/unified-discover-endpoint`
- `fix/race-condition-in-queue`
- `docs/api-reference-update`
- `refactor/extraction-pipeline`
- `test/integration-coverage`
- `chore/update-go-modules`

### Pre-commit Hook

The pre-commit hook runs our **full quality suite**:

```bash
make install-hooks  # Run this once after cloning
```

The hook executes:
- **Build**: `go build ./cmd/kinoko` — ensures code compiles
- **Vet**: `go vet ./...` — static analysis for common errors
- **Lint**: `golangci-lint run` — code style enforcement
- **Unit tests**: `go test -race ./...` — all unit tests with race detection
- **Integration tests**: `go test -tags integration -race ./tests/integration/` — full system tests

**⚠️ Commits are blocked if any step fails.** Fix issues before committing.

## Testing

### Running Tests

```bash
# Unit tests (fast, isolated)
go test -race ./...

# Integration tests (slower, full system)
go test -tags integration -race ./tests/integration/

# Specific package
go test -race ./internal/api/

# Verbose output
go test -race -v ./internal/storage/

# Coverage
go test -race -cover ./...
```

### Test Organization

- **Unit tests**: `*_test.go` files alongside source code
- **Integration tests**: `tests/integration/` directory with `// +build integration` tag
- **CLI E2E tests**: `cmd/kinoko/cli_e2e_test.go`

### Writing Tests

- Use **table-driven tests** for multiple scenarios
- Include **race detection** (`go test -race`) for concurrency
- Mock external dependencies (databases, HTTP calls) in unit tests
- Test error paths and edge cases
- Use descriptive test names: `TestExtraction_LargeCfile_ProducesValidSkill`

## Architecture Overview

Mycelium is a **client-server system** where:

- **Client** (`kinoko run`) extracts knowledge from local sessions
- **Server** (`kinoko serve`) provides discovery and indexing services
- **Git** serves as the source of truth for all extracted knowledge
- **SQLite** databases cache derived data on both sides

### Client/Server File Separation

Understanding which code runs where is crucial for development:

#### Client-Side Files
**Primary commands:**
- `cmd/kinoko/run.go` — Main extraction orchestrator
- `cmd/kinoko/workers_run.go` — Worker pool management
- `cmd/kinoko/extract.go` — Knowledge extraction logic
- `cmd/kinoko/importcmd.go` — Import external knowledge
- `cmd/kinoko/queuecmd.go` — Queue management commands

**Client packages:**
- `internal/queue/` — Local task queue and SQLite database
- `internal/extraction/` — 3-stage knowledge extraction pipeline
- `internal/injection/` — Context injection and retrieval
- `internal/serverclient/` — HTTP client for server APIs

#### Server-Side Files
**Primary commands:**
- `cmd/kinoko/serve.go` — HTTP server and routing
- `cmd/kinoko/serve_scheduler.go` — Background job scheduling
- `cmd/kinoko/serve_embedding.go` — Embedding service integration

**Server packages:**
- `internal/api/` — HTTP handlers and API endpoints
- `internal/storage/` — SQLite indexing and search
- `internal/gitserver/` — Git repository management
- `internal/decay/` — Knowledge quality scoring

#### Shared Components
**Both client and server:**
- `internal/model/` — Data structures and types
- `internal/config/` — Configuration management
- `internal/embedding/` — Embedding generation utilities

**Development Tip:** If you're adding client-side functionality (extraction, queue management), work in client files/packages. For server features (APIs, indexing), work in server components. Never mix concerns — client logic shouldn't appear in server files and vice versa.

### API Surface

The server exposes exactly **6 HTTP endpoints**:

```
GET   /api/v1/health                    # Health check
POST  /api/v1/discover                  # Unified knowledge discovery
POST  /api/v1/embed                     # Embedding generation
POST  /api/v1/ingest                    # Post-receive hook trigger
GET   /api/v1/skills/decay              # List skills by decay score
PATCH /api/v1/skills/{id}/decay         # Update decay scores
```

See `internal/api/server.go` for handler implementations.

## Adding New Features

### Adding a New API Endpoint

1. **Add the handler** to `internal/api/server.go`:
   ```go
   func (s *Server) handleNewFeature(w http.ResponseWriter, r *http.Request) {
       // Implementation
   }
   ```

2. **Register the route** in `registerRoutes()`:
   ```go
   mux.HandleFunc("POST /api/v1/feature", s.handleNewFeature)
   ```

3. **Add client methods** to `internal/serverclient/`:
   ```go
   func (c *Client) NewFeature(ctx context.Context, req *NewFeatureRequest) (*NewFeatureResponse, error) {
       // HTTP client implementation
   }
   ```

4. **Write tests** for both handler and client
5. **Update API documentation** in `docs/`

### Adding a New Injection Strategy

Injection strategies control how extracted knowledge gets integrated into new contexts.

1. **Create the strategy** in `internal/injection/strategies/`:
   ```go
   type NewStrategy struct {
       // Configuration
   }
   
   func (s *NewStrategy) Inject(ctx context.Context, query string) (*InjectionResult, error) {
       // Implementation
   }
   ```

2. **Register the strategy** in `internal/injection/registry.go`
3. **Add configuration options** to `internal/config/`
4. **Create integration tests** verifying end-to-end behavior
5. **Document the strategy** in `docs/injection-strategies.md`

### Adding a New Extraction Stage

The extraction pipeline has 3 stages: metadata filtering, content pattern matching, and LLM critic evaluation.

1. **Identify the stage** where your logic fits:
   - **Stage 1**: Metadata filtering (`internal/extraction/stage1/`)
   - **Stage 2**: Pattern matching (`internal/extraction/stage2/`)  
   - **Stage 3**: LLM evaluation (`internal/extraction/stage3/`)

2. **Implement the processor**:
   ```go
   type NewProcessor struct {
       // Dependencies
   }
   
   func (p *NewProcessor) Process(ctx context.Context, input *StageInput) (*StageOutput, error) {
       // Logic
   }
   ```

3. **Add to the pipeline** in `internal/extraction/pipeline.go`
4. **Write unit tests** with various input scenarios
5. **Test with real session data** to validate effectiveness

## Code Style

### Go Conventions

We follow standard Go practices:

- **Package names**: lowercase, short, descriptive (`queue` not `queuemanager`)
- **Function names**: CamelCase, verb-first (`ExtractKnowledge` not `KnowledgeExtract`)
- **Variable names**: camelCase for local, PascalCase for exported
- **Interface names**: `-er` suffix when possible (`Extractor`, `Injector`)
- **Error handling**: explicit error returns, wrap with context using `fmt.Errorf("context: %w", err)`

### Code Formatting

**Required tools:**
- `goimports` — import organization and basic formatting
- `golangci-lint` — comprehensive linting with our custom rules

Both run automatically in the pre-commit hook. To run manually:

```bash
goimports -w .
golangci-lint run
```

### Documentation Standards

- **Package documentation**: Each package has a doc.go file explaining its purpose
- **Function documentation**: Public functions include examples in comments
- **Complex logic**: Inline comments explaining non-obvious business rules
- **API changes**: Update relevant documentation in `docs/`

### Testing Standards

- **Coverage target**: Maintain >80% coverage for new code
- **Test naming**: `TestFunction_Scenario_ExpectedResult`
- **Mock dependencies**: Use interfaces and dependency injection
- **No flaky tests**: Tests must pass consistently
- **Performance tests**: Include benchmarks for critical paths

## Development Environment

### Prerequisites

- **Go 1.22+**
- **SQLite 3.35+** (for database functionality)
- **Git 2.30+** (for repository operations)
- **golangci-lint 1.55+** (for linting)

### IDE Configuration

**VS Code**: Recommended extensions:
- Go (official Google extension)
- golangci-lint
- Test Explorer for Go

**GoLand/IntelliJ**: Built-in Go support works well with our project structure.

### Local Development

```bash
# Start the server locally
go run ./cmd/kinoko serve --port 8080

# Run extraction in another terminal
go run ./cmd/kinoko run --server http://localhost:8080

# Watch logs
tail -f ~/.kinoko/logs/kinoko.log
```

## Getting Help

- **Questions about architecture**: Check `docs/architecture.md`
- **API usage**: See `docs/api-reference.md`
- **Test failures**: Review the error output and ensure you're running the latest main branch
- **Design decisions**: Read `docs/design-rationale.md` for context on architectural choices

## License

By contributing, you agree that your contributions will be licensed under the same license as the project.