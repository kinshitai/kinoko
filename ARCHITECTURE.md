# Architecture

**Server (`kinoko serve`):** Git server + search index. Stores skill repos, indexes SKILL.md into SQLite, tracks git stats (last commit, contributors, clones). Two endpoints: `POST /api/v1/discover` (search with raw signals) and `POST /api/v1/embed` (embeddings). Post-receive hook re-indexes on push. No computation, no mutation, no client awareness.

**Client (`kinoko run`):** Runs alongside your agent. Extracts skills from sessions → commits SKILL.md → pushes to server via git. At injection time, asks server for matches, gets raw signals, combines with personal usage data, ranks locally. Computes decay from git metadata (freshness, activity). Personal experience stored in gitignored `.kinoko/` files inside cloned repos — never pushed.

**Git:** Only write path. Only communication channel between client and server. Client pushes skills, server indexes on receive.

**Boundary:** Server never sees sessions or per-client behavior. Client never writes to server except git push. Shared knowledge flows through git. Personal experience stays local.
