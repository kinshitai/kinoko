# CLI Reference

## Overview

```
kinoko - Knowledge sharing infrastructure for AI agents

USAGE:
  kinoko [command] [flags]

AVAILABLE COMMANDS:
  init      Initialize Kinoko workspace
  serve     Start the Kinoko git server
  extract   Run extraction pipeline on a session log file
  decay     Run one decay cycle
  stats     Print pipeline metrics
  help      Help about any command

GLOBAL FLAGS:
  -h, --help      Help for kinoko
      --version   Version for kinoko
```

---

## `kinoko init`

Initialize a new Kinoko workspace in `~/.kinoko/`.

```bash
kinoko init
```

**Creates:**
- `~/.kinoko/` — workspace root
- `~/.kinoko/config.yaml` — default configuration
- `~/.kinoko/skills/` — local skills directory (initialized as a git repo)

**Behavior:**
- Safe to run multiple times — won't overwrite existing files
- Skips git init gracefully if git isn't installed
- Creates an initial commit with `.gitignore`

---

## `kinoko serve`

Start the Soft Serve git server for hosting skill repositories. Wires extraction and injection pipelines into session lifecycle hooks.

```bash
kinoko serve [flags]
```

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--config` | string | `~/.kinoko/config.yaml` | Path to config file |

**Behavior:**
- Binds to `server.host`:`server.port` from config (default `127.0.0.1:23231`)
- Opens SQLite database and builds extraction + injection pipelines
- Registers `OnSessionStart` (injection) and `OnSessionEnd` (extraction) hooks
- When A/B testing is enabled (`extraction.ab_test.enabled: true`), wraps injection with `ABInjector`
- Degrades gracefully: no embedding key → injection disabled; no LLM key → extraction disabled
- Handles SIGINT/SIGTERM for graceful shutdown

**Environment variables:**

| Variable | Purpose |
|---|---|
| `OPENAI_API_KEY` | Fallback API key for both embeddings and LLM |
| `KINOKO_EMBEDDING_API_KEY` | API key for embeddings (overrides `OPENAI_API_KEY`) |
| `KINOKO_LLM_API_KEY` | API key for LLM calls (overrides `OPENAI_API_KEY`) |

**Example:**
```bash
export OPENAI_API_KEY=sk-...
kinoko serve
kinoko serve --config ./my-config.yaml
```

---

## `kinoko extract`

Run the 3-stage extraction pipeline on a session log file. For manual testing and debugging.

```bash
kinoko extract <session-log> [flags]
```

**Arguments:**

| Argument | Description |
|---|---|
| `<session-log>` | Path to the session log file |

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--config` | string | `""` | Config file path |
| `--library` | string | first configured | Library ID to use |

**Behavior:**
- Reads the session log file and parses metadata (timestamps, tool calls, errors, model info) via heuristic regex patterns
- Initializes all pipeline dependencies (SQLite store, embedder, LLM client, 3 stages)
- Runs the full extraction pipeline
- Prints the `ExtractionResult` as formatted JSON to stdout
- Logs pipeline progress to stderr

**Environment variables:**

| Variable | Purpose |
|---|---|
| `OPENAI_API_KEY` | Fallback API key for embeddings and LLM |
| `KINOKO_EMBEDDING_API_KEY` | API key for embeddings |
| `KINOKO_LLM_API_KEY` | API key for LLM calls (required) |

**Example:**
```bash
export OPENAI_API_KEY=sk-...
kinoko extract ./session.log
kinoko extract ./session.log --library my-team --config ./config.yaml
```

**Output:** JSON `ExtractionResult` including stage results, skill record (if extracted), timing, and status.

---

## `kinoko decay`

Run one decay cycle over all skills in a library. Applies half-life degradation, rescues recently-used skills, and deprecates stale ones.

```bash
kinoko decay [flags]
```

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--config` | string | `""` | Config file path |
| `--library` | string | first configured | Library ID to process |
| `--dry-run` | bool | `false` | Print what would change without writing |

**Behavior:**
- Loads decay config (with defaults for any unset fields)
- Processes every skill in the library
- Reports counts: processed, demoted, deprecated, rescued
- In `--dry-run` mode, uses a no-op writer — no database changes

**Example:**
```bash
kinoko decay --library my-team
kinoko decay --dry-run
kinoko decay --config ./config.yaml --library local
```

**Output:**
```
Decay cycle complete for library "local"
  Processed:  42
  Demoted:    8
  Deprecated: 3
  Rescued:    2
```

---

## `kinoko stats`

Query the database and print pipeline metrics.

```bash
kinoko stats [flags]
```

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--config` | string | `""` | Config file path |

**Metrics reported:**

| Section | Metrics |
|---|---|
| Sessions | Total, extracted, rejected, errors, extraction yield |
| Stage Pass Rates | Per-stage passed/total/rate |
| Extraction Precision | Human review: reviewed, useful, precision % |
| Injection Metrics | Events, sessions with injection, injection rate, skill utilization |
| A/B Test Results | Treatment/control sessions, success rates, z-score, p-value, significance |
| Skills by Category | Count per category (foundational, tactical, contextual) |
| Quality Scores | Average composite score, average confidence |
| Decay Distribution | Bucketed: dead, low, medium, high, fresh |

**Example:**
```bash
kinoko stats
kinoko stats --config ./config.yaml
```

---

## `kinoko --version`

Print the version string. Development builds show `dev`; release builds show a semantic version set via ldflags.

```bash
kinoko --version
```

---

## `kinoko help`

Show help for any command.

```bash
kinoko help
kinoko help extract
kinoko help decay
```

---

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Error (config issues, startup failures, runtime errors) |
| 2 | Extraction rejected (`kinoko extract` only) |
| 3 | Extraction error (`kinoko extract` only) |
| 130 | Interrupted (Ctrl+C) |
