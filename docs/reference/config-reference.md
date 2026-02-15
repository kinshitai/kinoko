# Configuration Reference

Mycelium configuration lives at `~/.mycelium/config.yaml`. Created by `mycelium init` with sensible defaults.

## Default Configuration

```yaml
storage:
  driver: sqlite
  dsn: ~/.mycelium/mycelium.db

libraries:
  - name: local
    path: ~/.mycelium/skills
    priority: 100
    description: "Local skills on this machine"

server:
  host: "127.0.0.1"
  port: 23231
  dataDir: ~/.mycelium/data

extraction:
  auto_extract: true
  min_confidence: 0.5
  require_validation: true

hooks:
  credential_scan: true
  format_validation: true
  llm_critic: false

defaults:
  author: ""
  confidence: 0.7
```

## Sections

### `server`

Controls the git server started by `mycelium serve`.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `host` | string | `"127.0.0.1"` | IP to bind. Use `"0.0.0.0"` for all interfaces. |
| `port` | int | `23231` | SSH port. Must be 1–65535. |
| `dataDir` | string | `~/.mycelium/data` | Directory for server data (git repos, metadata). Auto-created. |

### `storage`

Database for metadata and indexes.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `driver` | string | `"sqlite"` | `"sqlite"` or `"postgres"` |
| `dsn` | string | `~/.mycelium/mycelium.db` | Connection string. File path for SQLite, URL for Postgres. |

### `libraries`

Skill libraries in priority order. Higher priority wins when skills share a name.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Unique identifier |
| `path` | string | one of path/url | Local filesystem path |
| `url` | string | one of path/url | Remote URL (`ssh://host:port`) |
| `priority` | int | yes | Higher = higher priority. Non-negative. |
| `description` | string | no | Human-readable description |

`path` and `url` are mutually exclusive.

### `extraction`

Controls automatic skill extraction (Phase 2 feature — not yet active).

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `auto_extract` | bool | `true` | Enable automatic extraction |
| `min_confidence` | float | `0.5` | Minimum confidence threshold (0.0–1.0) |
| `require_validation` | bool | `true` | Require validation before storing |

### `hooks`

Pre-commit validation hooks.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `credential_scan` | bool | `true` | Scan for leaked credentials |
| `format_validation` | bool | `true` | Validate SKILL.md format |
| `llm_critic` | bool | `false` | LLM-based skill review (requires API config) |

### `defaults`

Default values for new skills.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `author` | string | `""` | Default author identifier |
| `confidence` | float | `0.7` | Default confidence score (0.0–1.0) |

## Path Expansion

All path fields support `~` expansion:
- `~` or `~/path` → current user's home directory
- `~user/path` → that user's home directory (Unix only)

## Config Loading Order

1. Explicit path via `--config` flag
2. `~/.mycelium/config.yaml`
3. Built-in defaults (if no file exists)

## Validation

Mycelium validates config on startup. You'll get clear error messages for:
- Port out of range
- Empty required fields
- Invalid confidence values
- Libraries with both `path` and `url`
- Libraries with neither `path` nor `url`
- Negative library priority
