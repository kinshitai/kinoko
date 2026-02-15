# Spec: `kinoko init` for Client Adoption

**Ticket:** G3  
**Author:** Hal (CTO)  
**Priority:** P1  
**Size:** M (2-3 hours)  
**Status:** Draft

---

## Problem

"Zero friction or zero adoption." Currently, connecting an agent runtime (OpenClaw, Claude Code) to a Kinoko server requires:
1. Knowing the server exists
2. Understanding the hook model
3. Manually integrating session start/end hooks
4. Configuring the agent to send sessions somewhere

That's not zero friction. `kinoko init` should make adoption a one-command affair.

The existing `init.go` creates `~/.kinoko/` with a config and local git repo. That's the **server** setup. We need **client** setup — the thing that connects an agent to an existing Kinoko server.

## Design

### Two Modes

**`kinoko init` (existing)** — server mode. Creates `~/.kinoko/`, config, local skills dir. Already works. Keep it.

**`kinoko init --connect <url>`** — client mode. Connects this machine to a remote Kinoko server.

```bash
# Connect to a remote server
kinoko init --connect ssh://kinoko.internal:23231

# Connect to localhost (dev mode)
kinoko init --connect localhost
```

### Client Mode Flow

```
$ kinoko init --connect ssh://kinoko.internal:23231

🍄 Connecting to Kinoko server...

✓ Server reachable at ssh://kinoko.internal:23231
✓ 3 skill libraries available (local: 47 skills, company: 123 skills)
✓ SSH key generated at ~/.kinoko/client_ed25519
✓ Config written to ~/.kinoko/config.yaml

Next steps:
  • Add this to your agent config to enable injection:
    
    # OpenClaw (openclaw.yaml)
    skills:
      kinoko: ssh://kinoko.internal:23231
    
    # Claude Code (.claude/settings.json)  
    "kinoko": { "server": "ssh://kinoko.internal:23231" }
    
  • Or run the auto-installer:
    kinoko hook install openclaw
    kinoko hook install claude-code
```

### Hook Installation

**`kinoko hook install <runtime>`** — installs session hooks into agent runtimes.

Supported runtimes (start with these):

**OpenClaw:**
- Detect OpenClaw config at `~/.openclaw/` or `$OPENCLAW_HOME`
- Add a skill entry pointing to the Kinoko server
- Or: write a hook script that `kinoko` calls on session start/end

**Claude Code:**
- Detect Claude Code config at `~/.claude/`
- Write hook configuration

**Generic (webhook):**
- Output a curl-compatible webhook URL
- Any agent that can POST session logs can integrate

### Hook Protocol

The hook model is simple — two events:

```
SESSION_START:
  Input: prompt text, agent context
  Output: injected skills (SKILL.md content to prepend)
  Latency budget: <500ms

SESSION_END:
  Input: session log (conversation transcript)
  Output: nothing (fire-and-forget, <10ms)
  The server enqueues for async extraction
```

For the client, this means:

```go
// internal/client/client.go

type Client struct {
    serverURL string
    sshKey    string
    httpURL   string // for REST API fallback
}

// OnSessionStart calls the server for skill injection.
func (c *Client) OnSessionStart(ctx context.Context, req *model.InjectionRequest) (*model.InjectionResponse, error)

// OnSessionEnd sends the session log for async extraction.
func (c *Client) OnSessionEnd(ctx context.Context, sessionLog []byte) error
```

### Transport: Git-First

**Git (primary path — skill delivery):**
- Client clones skill repos via SSH from Soft Serve
- Reads SKILL.md files locally — injection happens client-side, offline-capable
- Periodic `git pull` keeps local cache fresh
- This is how skills are delivered. Not HTTP. Git.

**HTTP (discovery + ingestion only):**
- Discovery: "given this prompt, which skills match?" → returns repo names + scores
- Ingestion: POST session logs for async extraction
- Health/status checks
- HTTP does NOT deliver skill content — git does.

```
┌─────────────────┐          ┌──────────────────────────┐
│     Client      │          │     Kinoko Server      │
│                 │          │                          │
│  1. POST /discover ──────► │  Query SQLite+embeddings │
│     {prompt}    │          │  Return [{repo, score}]  │
│                 │ ◄──────  │                          │
│  2. git clone   │          │                          │
│     ssh://server│ ◄──────► │  Soft Serve              │
│     /lib/skill  │          │  (serves git repos)      │
│                 │          │                          │
│  3. Read SKILL.md locally  │                          │
│     Inject into prompt     │                          │
│                 │          │                          │
│  4. POST /ingest ────────► │  Queue for extraction    │
│     {session_log}          │                          │
└─────────────────┘          └──────────────────────────┘
```

### `kinoko serve` API (discovery + ingestion)

```go
// Discovery — client asks "what skills match this prompt?"
POST /api/v1/discover
  Request:  { "prompt": "...", "limit": 5 }
  Response: { "skills": [{"repo": "local/fix-nplus1", "score": 0.87, "clone_url": "ssh://..."}] }

// Ingestion — client sends session log for extraction
POST /api/v1/ingest
  Request:  { "session_id": "...", "log": "..." }
  Response: { "queued": true }

// Health
GET /api/v1/health
  Response: { "status": "ok", "skills": 47, "libraries": 2 }

// Library listing
GET /api/v1/libraries
  Response: { "libraries": [{"name": "local", "skills": 47}] }
```

### Client Skill Cache

Clients maintain a local cache of cloned repos:

```
~/.kinoko/cache/
  └── local/
      ├── fix-n-plus-one-queries/    (git clone)
      │   └── v1/SKILL.md
      ├── retry-with-exponential-backoff/
      │   └── v1/SKILL.md
      └── ...
```

- On first discovery hit → `git clone` into cache
- On subsequent hits → `git pull` if stale (configurable staleness window)
- Injection reads from local cache — no network call needed
- Works offline after initial population

### Config Changes

```yaml
# Client-side config (~/.kinoko/config.yaml)
client:
  server: ssh://kinoko.internal:23231    # Soft Serve (git)
  api: http://kinoko.internal:23232      # Discovery + ingestion API
  ssh_key: ~/.kinoko/client_ed25519
  cache_dir: ~/.kinoko/cache             # Local skill cache
  pull_interval: 5m                        # How often to refresh cached repos
  prefetch: true                           # Clone all skills on init, not just on demand

# Server-side addition
server:
  api:
    enabled: true
    port: 23232
    auth: bearer
    bearer_token: ""
```

### Implementation Phases

**Phase 1 (this ticket):**
- Discovery API on serve (`/discover`, `/ingest`, `/health`, `/libraries`)
- `kinoko init --connect <url>` (test connection, generate SSH key, write client config)
- Client library (`internal/client/`) — discover → clone → read → inject cycle
- Local skill cache with git clone/pull

**Phase 2 (follow-up):**
- `kinoko hook install openclaw`
- `kinoko hook install claude-code`
- Background goroutine for periodic `git pull` on cached repos
- Prefetch mode (clone everything on init)

**Phase 3 (follow-up):**
- Authentication (SSH key verification, bearer tokens)
- Rate limiting on ingest/discover endpoints
- Client SDK in Python/TypeScript for non-Go agents

## Testing

- Unit: client construction, config generation
- Integration: start serve → init --connect → inject via HTTP → verify skills returned
- Integration: start serve → ingest session via HTTP → verify it appears in queue
- E2E: full flow — serve + init + ingest + extract + inject

## RFC Alignment Check

| RFC-002 Requirement | This Spec | Status |
|---|---|---|
| `kinoko init` sets up local config + hooks | ✅ `init --connect` for client setup | Aligned |
| `kinoko remote add home ssh://...` | ⚠️ Simplified — `init --connect` handles it in one step instead of separate `remote add` | Simpler but equivalent |
| Three agents, one server (Phase 1 users) | ✅ Hal (OpenClaw) + Egor (Claude Code × 2) | Aligned |
| Framework-agnostic hook spec (Phase 3) | ⚠️ `hook install` starts with OpenClaw + Claude Code, generic webhook as fallback | Phase 1 covers our needs, generic comes later |
| Layered libraries — adding cloud = adding a URL | ✅ Client config supports multiple remotes with priority | Aligned |
| Self-hostable first | ✅ Everything runs locally, no cloud deps | Aligned |
| One config file | ✅ `~/.kinoko/config.yaml` controls everything | Aligned |

**Gap from RFC:** RFC-002 shows `kinoko remote add home ssh://...` as a separate command. This spec folds that into `init --connect` for simplicity. If we need multi-remote later (connect to both home server AND company server), we should add `kinoko remote add/remove/list` commands. Not needed for Phase 1 — one server is enough.

**Transport decision:** Git-first, as the RFC intended. Git delivers skills (clone/pull repos). HTTP is only for discovery ("which repos match this prompt?") and ingestion (sending session logs). Clients work offline after initial clone. This is the right architecture — git is the truth all the way to the client.

## Dependencies

- G1 (git integration) enables the SSH clone path but is not required — HTTP API works independently
- Standard library `net/http` for the API — no framework needed
