# Spec: Credential Scanning

**Ticket:** G2  
**Author:** Hal (CTO)  
**Priority:** P0  
**Size:** M (2-3 hours)  
**Status:** Draft

---

## Problem

The manifesto says "security is not a feature — it's a precondition. Sanitization and verification ship on day one or we don't ship." The config has `hooks.credential_scan: true` but there's no backing implementation. Session logs may contain API keys, passwords, database URLs, and tokens that flow unscanned through extraction into SKILL.md files.

Two attack surfaces:
1. **Session logs → extraction pipeline** — secrets in session content get embedded into skills
2. **SKILL.md files → git repos** — if G1 (git integration) ships first, secrets get committed to git

## Design

### Package: `internal/sanitize`

```go
// Scanner detects credentials and secrets in text.
type Scanner struct {
    patterns []compiledPattern
    logger   *slog.Logger
}

type Finding struct {
    Type       string // "aws_key", "github_token", "generic_password", etc.
    Line       int
    Column     int
    Match      string // redacted preview: "AKIA****EXAMPLE"
    Confidence float64 // 0.0-1.0
}

// Scan returns all credential findings in the given text.
func (s *Scanner) Scan(text string) []Finding

// Redact replaces detected credentials with [REDACTED:{type}] placeholders.
func (s *Scanner) Redact(text string) string

// HasSecrets returns true if any high-confidence findings exist.
func (s *Scanner) HasSecrets(text string) bool
```

### Detection Patterns

Start with high-confidence regex patterns (no ML, no external deps):

| Type | Pattern | Confidence |
|------|---------|------------|
| AWS Access Key | `AKIA[0-9A-Z]{16}` | 0.95 |
| AWS Secret Key | `[0-9a-zA-Z/+]{40}` near "aws_secret" | 0.85 |
| GitHub Token | `gh[ps]_[A-Za-z0-9_]{36,}` | 0.95 |
| GitHub Fine-grained | `github_pat_[A-Za-z0-9_]{22,}` | 0.95 |
| Generic API Key | `[a-zA-Z0-9_-]{32,}` near "api_key", "apikey", "api-key" | 0.60 |
| Bearer Token | `Bearer [A-Za-z0-9\-._~+/]+=*` | 0.80 |
| Private Key | `-----BEGIN (RSA|EC|OPENSSH) PRIVATE KEY-----` | 0.99 |
| Database URL | `(postgres|mysql|mongodb)://[^\s]+@[^\s]+` | 0.90 |
| Slack Token | `xox[baprs]-[0-9a-zA-Z-]+` | 0.95 |
| OpenAI Key | `sk-[A-Za-z0-9]{48}` | 0.95 |
| Generic Password | `password\s*[:=]\s*[^\s]{8,}` (case-insensitive) | 0.50 |
| Generic Secret | `secret\s*[:=]\s*[^\s]{8,}` (case-insensitive) | 0.50 |
| Hex Token (long) | `[0-9a-f]{64}` (SHA256-like) | 0.40 |

Confidence < 0.5 = informational only, don't block.

### Integration Points

Three layers — belt, suspenders, and a safety net:

**1. Session ingestion (worker queue):**

When `queue.Enqueue()` writes the session log to disk, scan it. If high-confidence secrets found:
- Redact the log file (replace secrets with `[REDACTED:{type}]`)
- Log a warning with finding types (not the actual secrets)
- Continue extraction with redacted content

```go
// internal/worker/queue.go — Enqueue()
if scanner != nil {
    content = scanner.Redact(content)
}
```

**2. SKILL.md before git push (extraction pipeline):**

Scan the generated SKILL.md body before `SkillCommitter.CommitSkill()`. If secrets found, something went wrong upstream — log error, redact, continue.

```go
// internal/extraction/pipeline.go — Extract()
if p.scanner != nil && p.scanner.HasSecrets(skillBody) {
    p.log.Error("credentials detected in generated skill", "skill", name)
    skillBody = p.scanner.Redact(skillBody)
}
```

**3. Application-level scan before push (in GitCommitter):**

Since Soft Serve doesn't support native git hooks, we scan at the application level before pushing:

```go
// internal/gitserver/committer.go — CommitSkill()
// Before git push:
if scanner != nil && scanner.HasSecrets(string(body)) {
    body = []byte(scanner.Redact(string(body)))
    log.Warn("credentials redacted before git push", "repo", repoName)
}
// Then git add + commit + push with clean content
```

For Phase 3 (external contributors pushing via `git push`), the RepoWatcher (see G1) scans after push and can quarantine/flag repos with credentials. Not as strong as a pre-receive rejection, but Soft Serve doesn't give us that hook point. If this becomes a real concern, we fork Soft Serve or switch to a server that supports hooks.

**4. `mycelium scan` CLI command:**

Manual scanning for existing content:
```bash
mycelium scan <file>           # Scan a single file
mycelium scan --dir <path>     # Scan directory recursively
mycelium scan --skills         # Scan all skills in git
```

Output: findings per file with line numbers and types. Exit code 1 if high-confidence findings.

### Configuration

```yaml
# config.yaml
hooks:
  credential_scan: true       # Enable/disable (default: true)
  scan_confidence: 0.7        # Minimum confidence to redact (default: 0.7)
  scan_block: false           # Block extraction if secrets found (default: false, just redact)
```

### What NOT to Do

- Don't use external scanning tools (gitleaks, trufflehog) — we want zero deps
- Don't try to be comprehensive — high-confidence patterns only, expand over time
- Don't block by default — redact and warn. Users can set `scan_block: true` for strict mode
- Don't scan embeddings (binary data, not text)
- Don't store findings in the database — log them only

## Testing

- Unit: test each pattern against known credential formats + false positives
- Unit: test redaction preserves document structure
- Unit: test confidence thresholds
- Integration: extract a session containing fake credentials → verify SKILL.md is clean
- Edge cases: credentials split across lines, credentials in code blocks, base64-encoded credentials

## RFC Alignment Check

| RFC-002 Requirement | This Spec | Status |
|---|---|---|
| Pre-commit credential scanning | ⚠️ Application-level scan before push (Soft Serve lacks native git hooks) | Phase 1: scan in pipeline + GitCommitter. Phase 3: RepoWatcher for external pushes. |
| Pre-commit format validation | ⚠️ Application-level validation before push | Same approach as credential scanning |
| Pre-commit prompt injection detection | ⚠️ Stage 3 has delimiter sanitization for its own prompts, but no general input scanning | Partial — extend later |
| "Security is not a feature" / "ships on day one" | ✅ This spec addresses it | Overdue but correct |

**Alignment note:** RFC says "pre-commit hooks run on contributor's machine." Soft Serve doesn't support native git hooks (pre-receive/post-receive). For Phase 1, we scan at the application level before pushing — our pipeline controls all pushes, so nothing bypasses it. For Phase 3 (external contributors), we need either a RepoWatcher that scans after push, or a git server that supports hooks. This is a known gap — see G1 open question.

## Dependencies

None. Pure Go regex. No external tools.
