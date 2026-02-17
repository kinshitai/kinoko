# Design Rationale

This document explains **why** Mycelium's architecture is designed the way it is, linking key design decisions to research findings and empirical evidence.

## Research Foundation

Our architecture is informed by **SkillsBench** (Li et al., Feb 2026), the first comprehensive benchmark evaluating how Agent Skills affect task performance across 86 diverse tasks and 11 domains.

### Key SkillsBench Findings

1. **Curated Skills work dramatically** — +16.2pp average improvement
2. **Self-generated Skills don't work** — -1.3pp average (worse than no Skills)
3. **Less is more** — 2-3 Skills optimal (+18.6pp), 4+ Skills degrade (+5.9pp)
4. **Compact beats comprehensive** — detailed Skills (+18.8pp), comprehensive Skills (-2.9pp)
5. **Domain variation is massive** — Healthcare +51.9pp, Software Engineering +4.5pp
6. **Skills can hurt** — 16/84 tasks showed negative deltas even with curated Skills

These findings directly shaped our architectural decisions.

## Client/Server Split

**Decision**: Separate `kinoko run` (client) from `kinoko serve` (server) instead of a monolithic architecture.

### Rationale

**Extraction needs session data** — Knowledge extraction requires access to:
- Local session artifacts (code files, logs, terminal history)
- File system context (directory structure, permissions, symbolic links)
- Environment variables and runtime state
- User-specific configuration and preferences

This data is inherently **client-local** and often sensitive. Centralizing extraction would require:
- Uploading raw session data to servers (privacy/security risk)
- Complex access control and data isolation
- Network overhead for large artifact sets
- Synchronization complexity across multiple clients

**Discovery needs global index** — Knowledge discovery requires:
- Search across all extracted Skills from all users
- Semantic similarity computation via embeddings
- Quality scoring and ranking algorithms
- Fast query response times (<100ms)

This requires **centralized indexing** with:
- Shared embedding models and vector databases
- Aggregated quality signals across Skills
- Optimized search data structures
- Dedicated computational resources

**Different scaling characteristics**:
- **Extraction**: I/O bound, CPU spiky, session-driven, inherently parallel
- **Discovery**: CPU/memory bound, sustained load, request-driven, benefits from centralization

**Security boundaries**:
- **Client**: Handles sensitive session data, trusts local environment
- **Server**: Only sees sanitized extracted knowledge, shared across users

### Alternative Considered: Monolithic

A single binary handling both extraction and discovery was considered but rejected because:

1. **Resource contention** — Extraction jobs would interfere with query response times
2. **Deployment complexity** — Updates require coordinated restarts across all functions  
3. **Scaling mismatch** — Can't scale extraction and discovery independently
4. **Security mixing** — Session data and shared knowledge in the same process space

## Git-First Architecture

**Decision**: Use Git repositories as the source of truth with SQLite databases as derived caches.

### Write Path: Session → Extract → Git → Hook → Index

Instead of writing directly to databases, we:

1. **Extract** knowledge from sessions into standardized formats
2. **Commit** Skills to Git repositories (one repo per Skill)
3. **Trigger** post-receive hooks on push
4. **Index** new Skills into server SQLite database

### Read Path: Query → SQLite → Results

Discovery queries hit the SQLite index directly for performance, not Git repositories.

### Rationale

**Git provides better durability guarantees** than databases:
- **Distributed** — Every clone is a full backup
- **Cryptographically verified** — Tampering is detectable
- **Network resilient** — Works offline, syncs when available
- **Tooling mature** — Decades of operational knowledge

**Separation of concerns**:
- **Git** optimized for durability, versioning, distribution
- **SQLite** optimized for fast queries, complex indexes, search

**Recovery simplicity** — SQLite databases can be completely rebuilt from Git repositories. Data loss in the cache layer doesn't affect the source of truth.

**Standard workflows** — Git operations (clone, push, merge, branch) work naturally with Skills. No custom protocols required.

### SkillsBench Connection

The research shows that **Skill quality varies dramatically** (ecosystem mean: 6.2/12, only top quartile ≥9/12 useful). Our Git-first approach enables:

- **Versioning** — Skills can improve over time through standard Git workflows
- **Forking** — Good Skills can be adapted for new contexts
- **Auditing** — Full history of what was extracted and why
- **Quality control** — Bad Skills can be reverted or improved collaboratively

### Alternative Considered: Database-First

Direct database storage was rejected because:

1. **No versioning** — Updates overwrite previous versions
2. **Single point of failure** — Database corruption loses everything
3. **Complex backup/restore** — Requires database-specific procedures
4. **Vendor lock-in** — Tied to specific database technologies
5. **No standard tooling** — Custom protocols for access and collaboration

## Unified Discover Endpoint

**Decision**: Replace 15 specialized endpoints with 1 unified discovery API.

### Previous State (API Consolidation)

The original system had **15 endpoints**:
```
POST /api/v1/discover        # One of several query types
GET  /api/v1/discover        # Duplicate
POST /api/v1/search          # Raw similarity search  
POST /api/v1/match           # Context-based matching
POST /api/v1/novelty         # Duplicate detection
POST /api/v1/sessions        # Session tracking
...
```

### Current State (Consolidated)

**6 endpoints total**, with `POST /api/v1/discover` handling all discovery:

```json
{
  "prompt": "How to create pivot tables with pandas",
  "embedding": [0.1, 0.2, ...],           // optional
  "patterns": ["pivot_table", "DataFrame"], // optional  
  "library_ids": ["pandas-skills"],        // optional
  "min_quality": 0.8,                     // optional
  "top_k": 3                              // optional
}
```

### Rationale

**Client-side decision making** — The unified endpoint returns ranked results. **Clients decide** what to do with them:
- **Injection**: Use top 3 results for context augmentation
- **Novelty**: Check if any results have high similarity to current session
- **General search**: Present results to user for exploration

This follows the **SkillsBench principle** that **focused guidance outperforms comprehensive options**. One well-designed API beats many specialized ones.

**Simplified client implementation**:
- One API to learn instead of 15
- Consistent parameters across all use cases
- Better error handling and retry logic
- Easier testing and mocking

**Better optimization opportunities**:
- Single query path to optimize
- Consistent caching strategy  
- Centralized rate limiting and monitoring
- Easier to add new query parameters

### Alternative Considered: Keep Specialized Endpoints

Maintaining separate `/search`, `/match`, `/novelty` endpoints was rejected because:

1. **Implementation overlap** — All endpoints query the same underlying index
2. **Client complexity** — Different APIs for similar functionality
3. **Testing burden** — 3x the integration test matrix
4. **Optimization difficulty** — Hard to optimize multiple query paths consistently

## MaxSkills=3, Compact Format

**Decision**: Hard-cap injection at 3 Skills per session and target 1.5K tokens per Skill.

### SkillsBench Evidence

The research provides unambiguous guidance:

| Skills Count | Improvement |
|-------------|------------|
| 1 skill | +17.8pp |
| 2–3 skills | **+18.6pp** |
| 4+ skills | +5.9pp |

| Skill Complexity | Improvement |
|-----------------|------------|
| Detailed (~1.5K tokens) | **+18.8pp** |
| Comprehensive | **-2.9pp** |

**4+ Skills create "cognitive overhead and conflicting guidance."** More knowledge doesn't mean better performance — it means confusion.

**Comprehensive Skills "consume context budget without providing actionable guidance."** Verbose documentation actively hurts because agents can't find the relevant information.

### Implementation

**Hard cap in discovery API**:
```go
if req.TopK > 10 {
    req.TopK = 10  // Server-enforced maximum
}
```

**Hard cap in injection**:
```go
if len(skills) > 3 {
    skills = skills[:3]  // Client-enforced injection limit
}
```

**Extraction targeting**:
- Critic prompt explicitly penalizes verbosity without substance
- Target skill length: 1,000-2,000 tokens (detailed tier)
- Reject extractions >3,000 tokens as too comprehensive

### Rationale

This isn't an arbitrary limit — it's **empirically optimal**. The SkillsBench data shows peak performance at 2-3 Skills, and we implement the upper bound.

**Quality over quantity** — Better to inject 3 highly relevant, actionable Skills than 10 mediocre ones.

**Respects context limits** — Even large context models benefit from focused, relevant content over comprehensive dumps.

### Alternative Considered: Dynamic Limits

Adaptive skill counts based on context size or complexity were rejected because:

1. **Empirical data** shows 2-3 is optimal across all tested conditions
2. **Implementation complexity** — Dynamic limits are harder to predict and test
3. **Cognitive consistency** — Agents perform better with consistent input patterns

## No MCP (Model Context Protocol)

**Decision**: Use file-based injection + Git hooks instead of implementing MCP.

### Rationale

**Universal platform support** — File system operations work everywhere:
- Local development environments (any OS)
- Remote servers (SSH, containers)  
- CI/CD systems (GitHub Actions, GitLab CI)
- Embedded systems and edge devices

**MCP requires platform-specific integration**:
- Claude Desktop support only
- Protocol implementation per platform
- Version compatibility management
- Limited adoption outside Anthropic ecosystem

**Simplicity and debuggability**:
- Files are human-readable and inspectable
- Git operations use standard, well-understood tools
- No protocol negotiation or version mismatches
- Clear audit trail of what was injected when

**Durability**:
- Files persist across sessions automatically
- Git history provides complete change tracking
- No session state to manage or recover
- Works offline and syncs when network is available

### Performance Benefits

**Local file access** is faster than protocol round-trips:
- Injection: ~5ms to read 3 Skills from disk
- MCP round-trip: ~50-200ms depending on network and server load
- Scales linearly with skill count vs. exponentially with protocol overhead

**Caching simplicity**:
- File system caching handled by OS
- Git objects cached automatically
- No custom caching protocol needed

### Alternative Considered: MCP Implementation

MCP was evaluated but rejected because:

1. **Platform fragmentation** — Different MCP implementations per platform
2. **Complexity overhead** — Protocol negotiation, session management, error handling
3. **Network dependency** — Requires network connectivity for local operations
4. **Limited ecosystem** — Few tools support MCP compared to file system + Git
5. **Debugging difficulty** — Protocol exchanges harder to inspect than file contents

**The pragmatic choice** — File-based + Git provides MCP-like functionality (context augmentation, knowledge sharing) through proven, universal mechanisms.

## Retrospective Quality Signals

**Decision**: Use artifact persistence, return visits, and cross-references instead of extraction-time quality assessment alone.

### SkillsBench Context

The research shows **even curated Skills can hurt performance** (16/84 tasks). Human curation doesn't guarantee quality. Extraction-time assessment is **insufficient**.

Our **3-stage extraction pipeline** provides initial quality filtering:
1. **Stage 1**: Metadata filtering (80% rejection)
2. **Stage 2**: Pattern matching (60% of remaining rejected) 
3. **Stage 3**: LLM critic evaluation (40% of remaining rejected)

But this only captures **extraction-time quality**, not **real-world usefulness**.

### Retrospective Signals

**Layer 2 signals** (days to weeks after extraction):
- **Artifact persistence** — Do the code/files produced by this Skill still exist in the user's workspace?
- **Return visits** — Did the user come back to the same problem domain?
- **Cross-references** — Do other extractions reference this Skill's patterns?

**Layer 3 signals** (weeks to months after extraction):
- **Git activity** — Are the Skill's Git artifacts still active?
- **Community adoption** — Do other users discover and use this Skill?
- **Update frequency** — Is the Skill being improved over time?

### Decay Scoring

Skills receive **decay scores** based on these signals:
- **High decay** — Artifacts deleted, no return visits, no cross-references
- **Low decay** — Artifacts persist, multiple return visits, referenced by other Skills
- **Decay endpoints** allow server to surface and manage low-quality Skills

### Rationale

**Time reveals quality** — A Skill that looked good at extraction but produces poor outcomes will show decay signals within days.

**Community validation** — Skills used by multiple people are more likely to be genuinely useful than Skills used once.

**Prevents crud accumulation** — Bad Skills get deprioritized or removed instead of accumulating indefinitely.

### SkillsBench Connection

This addresses the research finding that **Skills can actively hurt performance**. Our retrospective signals detect these harmful Skills post-deployment and reduce their prominence.

**Better than human curation** — Humans can't predict which Skills will be useful in practice. Retrospective signals measure actual utility.

### Alternative Considered: Extraction-Only Quality

Relying only on LLM critic evaluation was rejected because:

1. **SkillsBench shows limits** — Even human curation allows harmful Skills
2. **Context blindness** — Extraction-time assessment lacks deployment context
3. **No feedback loop** — Bad Skills persist indefinitely without correction mechanism
4. **Static assessment** — Quality can change as the domain evolves

## Summary

Mycelium's architecture emerged from empirical research on what makes Agent Skills effective:

1. **Client/server split** — Extraction needs local context, discovery needs global index
2. **Git-first** — Durability and versioning beat database convenience
3. **Unified discovery** — One focused API beats many specialized ones  
4. **MaxSkills=3** — Empirically optimal, prevents cognitive overload
5. **Compact format** — Detailed beats comprehensive, focus beats exhaustive
6. **No MCP** — Files + Git work everywhere, simpler than protocols
7. **Retrospective signals** — Time reveals quality better than extraction-time assessment

Each decision addresses specific failure modes identified in the research while maintaining simplicity and universality.

---

*Key research: SkillsBench (Li et al., Feb 2026). Full analysis: `internal-docs/research/luka-brief-012-auto-skills-failure-modes.md`*