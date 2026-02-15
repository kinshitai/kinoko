# Contributing to Mycelium

Mycelium is early-stage and built in the open. Contributions welcome.

## Getting Started

```bash
git clone https://github.com/mycelium-dev/mycelium.git
cd mycelium
go build ./cmd/mycelium
go test ./...
```

**Requirements:** Go 1.24+, Git

## Project Structure

```
cmd/mycelium/       CLI entry point (cobra commands)
internal/config/    Configuration loading and validation
internal/gitserver/ Soft Serve git server wrapper
pkg/skill/          SKILL.md parsing and validation
docs/               Documentation
rfcs/               Architecture decisions
```

## Development Workflow

1. Fork and clone the repo
2. Create a branch: `git checkout -b my-change`
3. Make your changes
4. Run tests: `go test ./...`
5. Commit with a clear message
6. Open a pull request

## Code Style

- Standard Go formatting (`gofmt`)
- Structured logging via `log/slog`
- Error wrapping with `fmt.Errorf("context: %w", err)`
- Tests live next to the code they test (`_test.go`)

## Documentation

Docs live in `docs/` as plain Markdown. If you change CLI behavior, config options, or the skill format, update the corresponding doc.

Key docs:
- `docs/reference/cli-reference.md` — CLI commands
- `docs/reference/config-reference.md` — Configuration
- `docs/reference/skill-format.md` — SKILL.md spec

## Architecture Decisions

Major design decisions are documented as RFCs in `rfcs/`. If you're proposing a significant change, write an RFC first.

## Reporting Issues

Include:
- What you did
- What you expected
- What happened
- Output of `mycelium --version`, `go version`, and `uname -a`

## Questions?

Open an issue. We're a small team and happy to help.
