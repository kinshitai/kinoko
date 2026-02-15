# Configuration Reference

Complete reference for Mycelium's configuration system. Configuration is stored in `~/.mycelium/config.yaml`.

## Configuration Structure

```yaml
# Mycelium Configuration
# This file controls your local Mycelium setup

# Storage configuration  
storage:
  driver: sqlite
  dsn: ~/.mycelium/mycelium.db

# Library layers (resolution order: highest priority first)
libraries:
  - name: local
    path: ~/.mycelium/skills
    priority: 100
    description: "Local skills on this machine"

# Server configuration (for 'mycelium serve')
server:
  host: "127.0.0.1"
  port: 23231
  dataDir: ~/.mycelium/data

# Extraction settings
extraction:
  auto_extract: true
  min_confidence: 0.5
  require_validation: true

# Pre-commit hooks
hooks:
  credential_scan: true
  format_validation: true
  llm_critic: false

# Default skill template values
defaults:
  author: ""
  confidence: 0.7
```

## Configuration Sections

### Storage Configuration

Controls where and how Mycelium stores metadata, embeddings, and indexes.

```yaml
storage:
  driver: "sqlite"                    # Storage backend: "sqlite" or "postgres"
  dsn: "~/.mycelium/mycelium.db"      # Data source name
```

#### Fields

- **`driver`** (string, required)
  - **Options:** `"sqlite"`, `"postgres"`
  - **Default:** `"sqlite"`
  - **Description:** Database driver for metadata storage

- **`dsn`** (string, required)  
  - **SQLite format:** File path (e.g., `~/.mycelium/mycelium.db`)
  - **PostgreSQL format:** `postgres://user:password@host/database`
  - **Default:** `~/.mycelium/mycelium.db`
  - **Description:** Database connection string

#### Examples

**SQLite (default):**
```yaml
storage:
  driver: sqlite
  dsn: ~/.mycelium/mycelium.db
```

**PostgreSQL:**
```yaml
storage:
  driver: postgres
  dsn: postgres://mycelium:password@localhost/mycelium_db
```

### Library Configuration

Defines skill libraries in layered resolution order. Libraries with higher priority override those with lower priority when skills have the same name.

```yaml
libraries:
  - name: "local"
    path: "~/.mycelium/skills"
    priority: 100
    description: "Local skills"
    
  - name: "company"
    url: "ssh://mycelium.company.com:23231"
    priority: 50
    description: "Company skill library"
    
  - name: "community"  
    url: "https://skills.mycelium.dev"
    priority: 10
    description: "Public skill library"
```

#### Fields

- **`name`** (string, required)
  - **Description:** Unique identifier for the library
  - **Example:** `"local"`, `"company"`, `"community"`

- **`path`** (string, optional)
  - **Description:** Local filesystem path to skill repository
  - **Format:** Absolute path or `~` for home directory
  - **Mutually exclusive with `url`**

- **`url`** (string, optional)
  - **Description:** Remote URL for skill repository  
  - **Formats:** `ssh://host:port`, `https://host`
  - **Mutually exclusive with `path`**

- **`priority`** (integer, required)
  - **Description:** Resolution priority (higher = higher priority)
  - **Range:** Non-negative integers
  - **Example:** Local=100, Company=50, Community=10

- **`description`** (string, optional)
  - **Description:** Human-readable description for the library

#### Resolution Order

Skills are resolved in priority order (highest first). If multiple libraries contain skills with the same name, the higher priority version is used.

**Example resolution:**
1. `local` library (priority 100) — checked first
2. `company` library (priority 50) — checked if not found in local
3. `community` library (priority 10) — checked last

### Server Configuration

Controls the built-in git server (`mycelium serve` command).

```yaml
server:
  host: "127.0.0.1"           # IP address to bind to  
  port: 23231                 # Port number
  dataDir: "~/.mycelium/data" # Server data directory
```

#### Fields

- **`host`** (string, required)
  - **Description:** IP address or hostname to bind the server to
  - **Default:** `"127.0.0.1"` (localhost only)
  - **Examples:** `"0.0.0.0"` (all interfaces), `"192.168.1.100"` (specific IP)

- **`port`** (integer, required)
  - **Description:** TCP port number for SSH git server
  - **Range:** 1-65535 
  - **Default:** `23231`
  - **Note:** Ports < 1024 typically require root/admin privileges

- **`dataDir`** (string, required)
  - **Description:** Directory for server data (git repositories, metadata)
  - **Default:** `~/.mycelium/data`
  - **Note:** Created automatically if it doesn't exist

#### Examples

**Local development:**
```yaml
server:
  host: "127.0.0.1"  # Localhost only
  port: 23231
  dataDir: ~/.mycelium/data
```

**Team server:**
```yaml
server:  
  host: "0.0.0.0"    # All network interfaces
  port: 23231
  dataDir: /var/lib/mycelium
```

### Extraction Configuration

Controls automatic skill extraction from agent sessions (Phase 2 feature).

```yaml
extraction:
  auto_extract: true        # Enable automatic extraction
  min_confidence: 0.5       # Minimum confidence threshold
  require_validation: true  # Require validation before storage
```

#### Fields

- **`auto_extract`** (boolean, optional)
  - **Description:** Enable automatic skill extraction from sessions
  - **Default:** `true`

- **`min_confidence`** (float, optional)
  - **Description:** Minimum confidence score for extracted skills
  - **Range:** 0.0-1.0
  - **Default:** `0.5`

- **`require_validation`** (boolean, optional)
  - **Description:** Require manual validation before storing extracted skills
  - **Default:** `true`

### Hooks Configuration

Controls pre-commit validation hooks for skill quality assurance.

```yaml
hooks:
  credential_scan: true      # Scan for leaked credentials
  format_validation: true    # Validate SKILL.md format
  llm_critic: false         # Enable LLM-based skill review
```

#### Fields

- **`credential_scan`** (boolean, optional)
  - **Description:** Scan skills for accidentally committed credentials (API keys, passwords, etc.)
  - **Default:** `true`
  - **Recommendation:** Always keep enabled

- **`format_validation`** (boolean, optional)  
  - **Description:** Validate skills conform to SKILL.md format specification
  - **Default:** `true`
  - **Recommendation:** Always keep enabled

- **`llm_critic`** (boolean, optional)
  - **Description:** Use LLM to review skill quality before committing
  - **Default:** `false`
  - **Note:** Requires LLM API configuration (coming in Phase 2)

### Defaults Configuration

Default values used when creating new skills.

```yaml
defaults:
  author: "your-email@example.com"  # Your identifier
  confidence: 0.7                   # Default confidence score
```

#### Fields

- **`author`** (string, optional)
  - **Description:** Default author identifier for new skills
  - **Examples:** `"user@example.com"`, `"username"`, `"agent:claude-3.5"`
  - **Used when:** Creating skills automatically or via templates

- **`confidence`** (float, optional)  
  - **Description:** Default confidence score for new skills
  - **Range:** 0.0-1.0
  - **Default:** `0.7`
  - **Note:** Can be overridden per skill

## Path Expansion

All path fields support tilde (`~`) expansion:

- `~` → User home directory (`/home/username` or `C:\Users\Username`)
- `~/path` → `$HOME/path`
- `~user/path` → `/home/user/path` (Unix-like systems only)

**Examples:**
```yaml
# These are equivalent (assuming user 'alice'):
dataDir: "~/.mycelium/data"
dataDir: "/home/alice/.mycelium/data"  # Linux
dataDir: "C:\\Users\\alice\\.mycelium\\data"  # Windows
```

## Environment Variables

Currently, environment variable overrides are not supported. Configuration must be specified in the YAML file.

**Roadmap:** Environment variable support is planned for container deployments.

## Configuration Examples

### Minimal Configuration

```yaml
# Only override what you need to change
defaults:
  author: "alice@company.com"
```

### Team Setup

```yaml
# Team member configuration
storage:
  driver: sqlite
  dsn: ~/.mycelium/mycelium.db

libraries:
  - name: local
    path: ~/.mycelium/skills
    priority: 100
    description: "My local skills"
    
  - name: team
    url: ssh://mycelium.company.com:23231
    priority: 50 
    description: "Company skill library"

defaults:
  author: "alice@company.com"
  confidence: 0.8

extraction:
  min_confidence: 0.6
  require_validation: false  # Trust team extraction
```

### Server Deployment

```yaml
# Production server configuration
server:
  host: "0.0.0.0"
  port: 23231
  dataDir: /var/lib/mycelium

storage:
  driver: postgres
  dsn: postgres://mycelium:secure_password@localhost/mycelium_prod

libraries:
  - name: main
    path: /var/lib/mycelium/skills
    priority: 100
    description: "Main skill repository"

hooks:
  credential_scan: true
  format_validation: true  
  llm_critic: true

extraction:
  auto_extract: true
  min_confidence: 0.7
  require_validation: true
```

## Configuration Validation

Mycelium validates your configuration on startup. Common validation errors:

### Invalid Port

```
Error: server port must be between 1 and 65535, got 70000
```

### Missing Required Fields

```  
Error: library[0] name cannot be empty
```

### Invalid Confidence Range

```
Error: defaults confidence must be between 0.0 and 1.0, got 1.5
```

### Mutually Exclusive Fields

```
Error: library[1] cannot have both path and URL
```

## Loading Configuration

Mycelium loads configuration in this order:

1. **Explicit path:** `mycelium serve --config /path/to/config.yaml`
2. **Default location:** `~/.mycelium/config.yaml`
3. **Built-in defaults:** If no config file exists

You can verify your configuration:

```bash
# Check current configuration (coming soon)
mycelium config show

# Validate configuration file  
mycelium config validate ~/.mycelium/config.yaml
```

## Migration

When upgrading Mycelium, configuration is automatically migrated to newer formats when possible. Always back up your config before major version upgrades.

## See Also

- **[Installation Guide](../getting-started/installation.md)** — Setting up Mycelium
- **[CLI Reference](cli-reference.md)** — Command-line interface
- **[Skill Format](skill-format.md)** — SKILL.md specification