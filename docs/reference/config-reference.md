# Configuration Reference

Kinoko configuration lives at `~/.kinoko/config.yaml`. Created by `kinoko init` with sensible defaults.

## Full Configuration

```yaml
server:
  host: "127.0.0.1"
  port: 23231
  dataDir: ~/.kinoko/data

storage:
  driver: sqlite
  dsn: ~/.kinoko/kinoko.db

libraries:
  - name: local
    path: ~/.kinoko/skills
    priority: 100
    description: "Local skills on this machine"

extraction:
  auto_extract: true
  min_confidence: 0.5
  require_validation: true
  min_duration_minutes: 2
  max_duration_minutes: 180
  min_tool_calls: 3
  max_error_rate: 0.7
  novelty_min_distance: 0.15
  novelty_max_distance: 0.95
  version_similarity_threshold: 0.85
  sample_rate: 0.01
  ab_test:
    enabled: false
    control_ratio: 0.1
    min_sample_size: 100

decay:
  foundational_half_life_days: 365
  tactical_half_life_days: 90
  contextual_half_life_days: 180
  deprecation_threshold: 0.05
  rescue_boost: 0.3
  rescue_window_days: 30

embedding:
  provider: openai
  model: text-embedding-3-small
  base_url: https://api.openai.com

llm:
  model: gpt-4o-mini

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

Controls the git server started by `kinoko serve`.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `host` | string | `"127.0.0.1"` | IP to bind. Use `"0.0.0.0"` for all interfaces. |
| `port` | int | `23231` | SSH port. Must be 1–65535. |
| `dataDir` | string | `~/.kinoko/data` | Directory for server data (git repos, metadata). Auto-created. |

### `storage`

Database backend for skills, sessions, and injection events.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `driver` | string | `"sqlite"` | `"sqlite"` or `"postgres"` |
| `dsn` | string | `~/.kinoko/kinoko.db` | Connection string. File path for SQLite, URL for Postgres. |

SQLite uses WAL mode, 5s busy timeout, and foreign keys. Schema is auto-migrated on startup.

### `libraries`

Skill libraries in priority order. Higher priority wins when skills share a name.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Unique identifier (used as `library_id` in storage) |
| `path` | string | one of path/url | Local filesystem path |
| `url` | string | one of path/url | Remote URL (`ssh://host:port`) |
| `priority` | int | yes | Higher = higher priority. Non-negative. |
| `description` | string | no | Human-readable description |

`path` and `url` are mutually exclusive.

### `extraction`

Controls the 3-stage extraction pipeline.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `auto_extract` | bool | `true` | Enable automatic extraction on session end |
| `min_confidence` | float | `0.5` | Minimum confidence threshold (0.0–1.0) |
| `require_validation` | bool | `true` | Require validation before storing |
| `min_duration_minutes` | float | `2` | Stage 1: minimum session duration |
| `max_duration_minutes` | float | `180` | Stage 1: maximum session duration |
| `min_tool_calls` | int | `3` | Stage 1: minimum tool call count |
| `max_error_rate` | float | `0.7` | Stage 1: maximum error rate (0.0–1.0) |
| `novelty_min_distance` | float | `0.15` | Stage 2: minimum embedding distance for novelty |
| `novelty_max_distance` | float | `0.95` | Stage 2: maximum embedding distance for novelty |
| `version_similarity_threshold` | float | `0.85` | Threshold for skill versioning (future use) |
| `sample_rate` | float | `0.01` | Human review sample rate (0.0–1.0). 0.01 = 1% |

#### `extraction.ab_test`

Controls A/B testing of the injection pipeline.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable A/B testing |
| `control_ratio` | float | `0.1` | Fraction of sessions in control group (0.0–1.0) |
| `min_sample_size` | int | `100` | Minimum sessions per group before computing z-test |

When enabled, a fraction of sessions receive no injected skills (control group). Both groups have injection events logged for statistical comparison.

### `decay`

Controls skill decay behavior. All fields have defaults via `decay.DefaultConfig()`.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `foundational_half_life_days` | int | `365` | Half-life for foundational skills |
| `tactical_half_life_days` | int | `90` | Half-life for tactical skills |
| `contextual_half_life_days` | int | `180` | Half-life for contextual skills |
| `deprecation_threshold` | float | `0.05` | Decay score below which skills are deprecated (set to 0.0) |
| `rescue_boost` | float | `0.3` | Decay score boost for recently-used successful skills (0.0–1.0) |
| `rescue_window_days` | int | `30` | Window for rescue eligibility (days since last injection) |

### `embedding`

Configures the embedding provider used for skill and prompt embeddings.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `provider` | string | `"openai"` | Embedding provider |
| `model` | string | `"text-embedding-3-small"` | Embedding model name |
| `base_url` | string | `"https://api.openai.com"` | Base URL for API calls |

API key is set via `KINOKO_EMBEDDING_API_KEY` or `OPENAI_API_KEY` environment variable (not in config file).

### `llm`

Configures the LLM used for extraction scoring, critic evaluation, and prompt classification.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `model` | string | `"gpt-4o-mini"` | LLM model name |

API key is set via `KINOKO_LLM_API_KEY` or `OPENAI_API_KEY` environment variable (not in config file).

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

## Environment Variables

| Variable | Purpose |
|---|---|
| `OPENAI_API_KEY` | Fallback API key for both embeddings and LLM |
| `KINOKO_EMBEDDING_API_KEY` | API key for embedding calls (overrides `OPENAI_API_KEY`) |
| `KINOKO_LLM_API_KEY` | API key for LLM calls (overrides `OPENAI_API_KEY`) |

## Path Expansion

All path fields support `~` expansion:
- `~` or `~/path` → current user's home directory
- `~user/path` → that user's home directory (Unix only)

## Config Loading Order

1. Explicit path via `--config` flag
2. `~/.kinoko/config.yaml`
3. Built-in defaults (if no file exists)

## Validation

Kinoko validates config on startup. You'll get clear error messages for:
- Port out of range
- Empty required fields
- Invalid confidence values
- Libraries with both `path` and `url`
- Libraries with neither `path` nor `url`
- Negative library priority
- Decay half-life ≤ 0
- Rescue boost outside [0, 1]
