# Decisions

## 2026-02-15
- Soft Serve integration: managed subprocess, NOT library embedding. Soft Serve is deeply coupled to its own context injection — embedding is a nightmare.
- SSH repo management: use `ssh localhost -p {port} repo create/list/delete` commands via admin keypair.
- HTTP port convention: SSH port + 1 (e.g., SSH 23231, HTTP 23232).
- Admin keypair: ed25519, generated via ssh-keygen, stored in dataDir.
- Buffer limit fix: bufio.Scanner needs explicit `scanner.Buffer()` call for large skills (set to 1MB).
- Tilde expansion: handle both `~/path` and `~user/path` via os/user.Lookup().
- UUIDv7 via google/uuid (already in go.mod) for skill IDs — sortable by creation time per spec §1.1.
- Skill names from Stage2 classification patterns, not raw content. CamelCase kebab conversion handles FIX/Backend/DatabaseConnection → fix-backend-database-connection.
- Stratified sampling: double the per-pool rate so extracted and rejected each contribute ~50% of samples. Simpler than tracking separate counters.
- SKILL.md template follows Luka's brief-004: front matter with id/version/quality/confidence/extracted_by, body with When to Use / Solution / Why It Works / Pitfalls sections.
