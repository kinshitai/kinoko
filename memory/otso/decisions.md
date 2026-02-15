# Decisions

## 2026-02-15
- Soft Serve integration: managed subprocess, NOT library embedding. Soft Serve is deeply coupled to its own context injection — embedding is a nightmare.
- SSH repo management: use `ssh localhost -p {port} repo create/list/delete` commands via admin keypair.
- HTTP port convention: SSH port + 1 (e.g., SSH 23231, HTTP 23232).
- Admin keypair: ed25519, generated via ssh-keygen, stored in dataDir.
- Buffer limit fix: bufio.Scanner needs explicit `scanner.Buffer()` call for large skills (set to 1MB).
- Tilde expansion: handle both `~/path` and `~user/path` via os/user.Lookup().
