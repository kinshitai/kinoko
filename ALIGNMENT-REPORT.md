# Mycelium Alignment Report

**Author:** Luka Jensen  
**Date:** 2026-02-15  
**Purpose:** Reality check — does what we built match what we set out to build?

---

## 1. Manifesto Alignment

### "Humans are the point"

**Status: Aligned in design, untested in practice.**

The extraction pipeline is explicitly designed around human benefit — skills flow from human work sessions to other humans' sessions. The human review sampling system (1% stratified sampling, weekly review cadence) shows we take human judgment seriously. The `human_review_samples` table exists and is populated.

But here's the thing: we have zero users. The manifesto paints a picture of a junior developer in São Paulo getting better answers without knowing why. Right now, the system is a pipeline that *could* serve that developer. The "humans are the point" principle is architecturally correct but experientially unvalidated.

**No violations.** But also no evidence it's working yet.

### "Zero friction or zero adoption"

**Status: Partially compromised.**

The extraction side is genuinely zero-friction — background workers process sessions async, the session-end hook is <10ms, no human tags or reviews are required for the default flow. Good.

The injection side requires... well, it requires the system to exist and be running. `mycelium serve` starts everything, `mycelium init` sets up a client. That's two commands. Not zero. The RFC-002 vision of `mycelium init` + `mycelium remote add home ssh://...` is designed but I don't see `mycelium init` fully implemented in the codebase — I see `serve`, config loading, and gitserver, but the client-side setup flow isn't complete.

More fundamentally: "zero friction" implies the system installs itself into existing workflows. The Entire.io reference in RFC-001 mentions `entire enable` as a model — one command, hooks installed. We have the architecture for hooks (`OnSessionStart`, `OnSessionEnd` on the gitserver) but the actual hook installation into agent runtimes (OpenClaw, Claude Code) isn't in this codebase. That's the adoption-critical path and it's not here.

**Verdict:** The pipeline is zero-friction. Getting *into* the pipeline is not.

### "Quality over quantity. Always."

**Status: Strongly aligned. Possibly the best-implemented principle.**

This is where the implementation shines. The 3-stage extraction pipeline is a genuine quality gate:

- Stage 1 eliminates ~60-80% of sessions at zero cost
- Stage 2 runs a 7-dimensional rubric with validated scoring
- Stage 3 uses an LLM critic with contradiction detection, retry logic, circuit breakers
- `MinimumViable()` threshold requires three dimensions ≥ 3
- Human review sampling catches quality drift

The dimensional scoring model (wine tasting metaphor in concepts.md) is well-thought-out and well-implemented. The composite score weights are reasonable. The A/B testing framework for injection effectiveness is built and ready.

The cost model in the extraction spec shows we thought about this carefully — $6.08 per 1000 sessions, with Stage 3 dominating.

**One concern:** Quality is currently defined entirely by the extraction pipeline. There's no post-injection quality signal beyond the heuristic success correlation. The manifesto says "every piece of knowledge earns its place through verification" — but verification is a one-time gate, not continuous validation. Decay partially addresses this, but it's time-based, not evidence-based (except for the rescue mechanic).

### "Security is not a feature"

**Status: Architecturally present, implementation incomplete.**

The RFC-002 lists pre-commit hooks for credential scanning and prompt injection detection. The config has `hooks.credential_scan: true` and `hooks.format_validation: true`. The Stage 3 critic uses delimiter injection with random nonces to prevent prompt injection in its own prompts.

But: I don't see the actual credential scanning implementation. The `HooksConfig` struct exists in config but there's no `internal/hooks/` or `internal/sanitization/` package. The gitserver manages repos and has session hooks, but the pre-commit security pipeline isn't implemented.

This is concerning. The manifesto says "sanitization and verification ship on day one or we don't ship." We're shipping extraction and injection without credential scanning on the content flowing through them. Session logs could contain API keys, passwords, tokens — and they get stored as log files in the queue directory and potentially embedded into SKILL.md files.

**This is a manifesto violation.** Not a catastrophic one — the system isn't public yet — but it's a gap between "security is a precondition" and what's actually built.

### "Knowledge has a half-life"

**Status: Well implemented.**

The decay system is clean. Category-specific half-lives (365/90/180 days), rescue mechanics for skills that prove themselves, deprecation threshold, configurable everything. The scheduler runs daily at 3am. The `DecayCycleResult` tracks processed/demoted/deprecated/rescued counts.

The decay formula is mathematically sound — exponential decay anchored to `LastInjectedAt` rather than `UpdatedAt` (avoiding the double-counting bug the code comments explicitly warn about). The rescue mechanic adds a nice feedback loop.

**One nuance:** Decay is currently time-based + usage-based. The manifesto implies content-based obsolescence too — "what works today may be wrong tomorrow." A skill about a deprecated API should decay faster than its half-life suggests. There's no mechanism for external signals (dependency updates, breaking changes) to accelerate decay. This is probably fine for now but worth noting.

### "Open by default"

**Status: Architecturally supported, not yet realized.**

The layered library design (local > company > public, add a URL to config) supports open sharing. Git-based storage means skills are forkable, clonable, inspectable. No paywalls or token gates in the code.

But "open by default" implies a default-public posture. Right now, a Mycelium instance is a private server. There's no public skill library, no federation, no discovery mechanism. The "open" part requires infrastructure that doesn't exist yet. The RFC lists "cloud hosted layer" and "federation" as Beyond-phase ideas.

**Not violated, but not actualized.** The architecture doesn't prevent openness; it just doesn't enable it yet.

---

## 2. RFC Alignment

### What was planned and built as designed

| RFC-002 Plan | Implementation | Fidelity |
|---|---|---|
| 3-stage extraction pipeline | `internal/extraction/` — Stage 1, 2, 3, Pipeline | Exact match |
| SQLite behind abstraction | `internal/storage/` — `SkillStore` interface, SQLite impl | Built as designed |
| Embedding service | `internal/embedding/` — OpenAI client with retry + circuit breaker | Built as designed |
| Decay system | `internal/decay/` — Half-life per category, rescue, deprecation | Built as designed, extended with RescueWindowDays |
| Background workers | `internal/worker/` — Queue, Pool, Scheduler | Built as designed |
| Soft Serve git server | `internal/gitserver/` — Process lifecycle, SSH commands, repo CRUD | Built as designed |
| Config management | `internal/config/` — YAML config, validation, defaults, path expansion | Built as designed |
| Injection pipeline | `internal/injection/` — Prompt classification, multi-signal ranking, degraded mode | Built as designed |
| A/B testing for injection | `internal/injection/ab.go` — ABInjector decorator | Built as designed |

**Verdict:** The core architecture from RFC-002 was followed with remarkable fidelity. The three-component model (BG Worker + Metadata Server + Git Server) maps cleanly to the implementation.

### What was planned but built differently

| Planned | Actual | Why |
|---|---|---|
| `StatusPending` for "in queue" | `StatusQueued` (new) + `StatusPending` for "being processed" | Conscious improvement — the BG Worker spec added finer state tracking. Good pivot. |
| Pre-commit hooks run on contributor's machine | Session hooks run on server (gitserver `OnSessionEnd`) | The system doesn't have a client SDK distributing hooks. Extraction happens server-side. This is a significant architectural difference — the RFC envisioned distributed compute, the implementation centralizes it. |
| Repo-per-skill in git | Skills stored as files in a flat directory + DB rows | The gitserver exists and can create repos, but `SkillStore.Put()` writes to a local file path, not a git commit. The "git is the truth" principle from RFC-002 is partially inverted — SQLite is the operational truth, files on disk are a side effect. |
| Layered libraries like Docker images | Config supports library definitions with priority, but injection only queries by `LibraryIDs` | The layering is config-level, not runtime-resolved. No shadowing-by-name logic exists. |

**The biggest drift:** The repo-per-skill git model. RFC-002 says "each skill is its own git repo," "git is the truth," and "everything else is a derived cache." In practice, SQLite is the truth, skills are file blobs, and git hosting is a subprocess that manages repos but isn't integrated into the skill lifecycle. `SkillStore.Put()` doesn't call `gitserver.CreateRepo()` or commit anything to git.

This drift was probably pragmatic — building a proper git-backed store is complex, and SQLite gives you ACID transactions cheaply. But it means the "blow away the DB and rebuild from git" recovery story doesn't work. The git server and the skill store are decoupled.

### What was planned but not built yet

| RFC Item | Status | Still needed? |
|---|---|---|
| Phase 3: Trust scoring | Not started | Yes — essential for multi-contributor scenarios |
| Phase 3: Weighted voting / PR-like workflows | Not started | Yes, but the Soft Serve choice makes this custom work |
| Phase 3: Server-side validation (dedup, conflict detection) | Embedding novelty in Stage 2 is partial dedup | Dedup exists; conflict detection doesn't |
| Phase 3: Framework-agnostic hook spec | Not started | Yes — currently tied to the gitserver's hook model |
| Pre-commit credential scanning | Config flag exists, no implementation | Yes — manifesto says day one |
| Pre-commit prompt injection detection | Partial (Stage 3 delimiter sanitization) | Input sanitization needed |
| `mycelium init` client setup | Not found in codebase | Yes — critical for adoption |
| `mycelium extract <session-log>` manual extraction | Not found | Nice to have for debugging |
| `mycelium decay --dry-run` | Not found | Nice to have |
| `mycelium queue` inspection commands | Not found | Useful for operations |
| `mycelium stats` metrics command | Not found | `metrics.Collector` exists but no CLI |

### What was built that wasn't in the plan

| Built | In RFC? | Assessment |
|---|---|---|
| `internal/circuitbreaker/` — standalone package | Not explicitly, mentioned in error handling | Good engineering — reusable across embedding + extraction |
| `internal/metrics/collector.go` — full metrics with A/B z-test | Mentioned in Phase 9 | Pulled forward, good call |
| `internal/llm/` — LLM client abstraction with error types | Implied but not specified | Clean separation, good |
| Stratified human review sampling with 50/50 balance | Spec said 1% sampling; stratification is an implementation detail | Better than specified |
| Content truncation with UTF-8-safe boundaries in Stage 3 | Not in spec | Practical necessity, good |
| `model.ValidateDomain()` default fallback | Not in spec | Defensive coding, fine |

No scope creep. Everything unplanned was either pulled forward from later phases or is good engineering practice.

---

## 3. Vision Gaps

### Are we building a knowledge sharing system or just an extraction pipeline?

Right now: **an extraction pipeline with injection plumbing.**

The extraction system is thorough — 3 stages, quality scoring, human review, cost modeling, A/B testing. It's the most complete part of the system. The injection pipeline is implemented. The decay system works.

But knowledge *sharing* implies multiple agents, multiple users, knowledge flowing between them. The system currently supports one server with one skill library. There's no mechanism for Agent A's extraction to reach Agent B's injection unless they share the same server. The "collective" in "collective intelligence" requires a network. We have a node.

### Where's the "collective" in collective intelligence?

The manifesto says: "Agents extract what was learned. Other people's agents absorb it." 

The architecture supports this via layered libraries — add a remote URL, get someone else's skills. But:

1. There is no remote library fetching implementation. `SkillStore.Query()` queries a local SQLite database. It doesn't reach out to remote servers.
2. There is no skill synchronization protocol.
3. There is no public skill registry.

The "collective" is currently scoped to "whoever has access to the same Mycelium server." For the founding team (Hal + Egor, 3 agents), that's fine. For the manifesto vision, it's a long way off.

### Is the git-based sharing mechanism real or theoretical?

**Theoretical.** 

The gitserver can create repos, list them, delete them. SSH access works. But:

- Skills are not committed to git repos by the extraction pipeline
- There's no `git clone` / `git pull` mechanism for remote libraries
- The "fork one skill without pulling the whole library" story requires repo-per-skill, which isn't wired up
- The "blow away the DB and rebuild from git" story doesn't work because the DB is the primary store

Git is currently an infrastructure component (Soft Serve runs), not a knowledge transport layer. The vision of git-as-truth is architecturally present (the gitserver exists) but operationally disconnected from the skill lifecycle.

### Is the "zero friction" promise met?

**For extraction: yes.** Background workers, async queue, <10ms hook. Excellent.

**For adoption: no.** There's no `mycelium init` to set up a client. There's no hook installer for agent frameworks. The server runs, but connecting an agent to it requires manual integration that isn't documented or automated.

**For injection: partially.** The injection pipeline runs, but it requires an LLM call for every prompt classification. That's not zero-friction — it's a latency and cost addition to every session start. The degraded mode (pattern-only) is a good fallback, but the primary path adds ~300ms to session startup.

### Is this self-hostable-first as claimed?

**Yes.** This is one of the best-aligned principles.

- Single binary (Go)
- SQLite as default storage (one file)
- `mycelium serve` starts everything
- Soft Serve for git (embedded subprocess)
- No required cloud dependencies (embedding API is external but optional with degraded mode)
- Config in one YAML file

The self-hosting story is clean. A Raspberry Pi could genuinely run this. The cloud-as-optional-layer design (add a URL to config) is there in spirit even if not implemented.

---

## 4. What's Missing from the Vision

### Multi-agent collaboration

**Status: Not started. No implementation path.**

The manifesto talks about agents extracting, transporting, and delivering knowledge between people. The system processes sessions from agents, but there's no concept of agent identity, agent-to-agent communication, or collaborative skill refinement.

RFC-001 references MOSAIC (selective sharing based on task similarity), A2A Protocol (agent cards, task delegation), and Moltbook (social network for agents). None of these ideas have any implementation foothold.

For Phase 1-2 (founding team), this is fine. But the manifesto vision is fundamentally multi-agent, and the architecture is currently single-pipeline.

### Community/public skill libraries

**Status: Conceptual only.**

The layered library config supports a `url` field. That's it. There's no:
- Public skill registry
- Discovery mechanism
- Authentication for remote libraries
- Skill format negotiation
- Version compatibility checking

### Trust and reputation

**Status: Not started.**

RFC-002 Phase 3 lists trust scoring per contributor and weighted voting. The `SkillRecord` has `ExtractedBy` (pipeline version) but no contributor identity, no trust score, no reputation system.

This matters because the quality story currently depends entirely on the extraction pipeline being right. With multiple contributors, you need trust signals independent of content quality — a contributor whose skills consistently succeed should be trusted more.

### Cross-organization knowledge sharing

**Status: Not architected.**

The layered library model could support this (company library + public library), but there's no:
- Access control per library
- Selective sharing (share some skills, not others)
- Privacy-preserving knowledge exchange
- Organizational boundaries

### The "mycelial network" metaphor

**Is it just a metaphor?**

Yes. Completely.

A mycelial network is decentralized, self-organizing, and grows through substrate connections. The implementation is a centralized server with a pipeline architecture. There's no:
- Peer-to-peer discovery
- Emergent network topology
- Self-organizing knowledge routing
- Substrate-level connections between instances

The name is evocative but the architecture is a hub, not a network. For Phase 1-2, that's appropriate. But if the project scales, the metaphor will become increasingly misleading unless the architecture evolves toward actual network properties.

---

## 5. Honest Assessment

### If you showed this codebase to someone who only read the manifesto, would they recognize it?

**Partially.**

They'd recognize the extraction quality focus ("quality over quantity"), the self-hosting story, and the knowledge-has-a-half-life principle. The pipeline architecture is a credible implementation of "knowledge sharing as a byproduct of work."

They would *not* recognize:
- The "collective" — there's no collective yet, just a single server
- The "every problem solved once is solved for everyone" — it's solved for everyone on the same server
- The "mycelial network" — it's a pipeline, not a network
- The São Paulo developer story — the system can't reach them yet

### What's the gap between marketing and reality?

The manifesto is aspirational. The implementation is foundational. The gap is:

| Manifesto Claims | Reality |
|---|---|
| "Solved for everyone" | Solved for people on the same server |
| "Agents extract, transport, and deliver" | Agents extract and deliver; transport is local |
| "No one even knows it's happening" | You need to set up and run Mycelium |
| "Open by default" | Private by default, open architecturally possible |
| "Security is a precondition" | Security hooks not yet implemented |
| "The value is in the network" | There is no network yet |

**This gap is normal for Phase 1.** The manifesto describes the end state. The codebase is early infrastructure. The question isn't whether there's a gap — there always is — but whether the architecture can grow into the vision. And on that front: **mostly yes**, with one structural concern.

### The structural concern

The biggest risk is that the git-based knowledge transport layer — the thing that would make this a network rather than a pipeline — is the most disconnected component. The gitserver runs but isn't integrated with skill storage. If this drift continues, the system will become a very good local extraction-injection engine that can't federate, can't share, and can't grow into the manifesto vision.

### What should we build next?

**Priority 1: Wire git into the skill lifecycle.**

`SkillStore.Put()` should commit to a git repo. This makes "git is the truth" real, enables the recovery story, and is the foundation for everything network-related (cloning, forking, federation).

**Priority 2: Credential scanning.**

The manifesto says "ship on day one." We're past day one. Session logs contain secrets. Scan them before they hit the extraction pipeline.

**Priority 3: Client-side hook installation.**

`mycelium init` that installs hooks into OpenClaw/Claude Code. Without this, adoption requires manual integration, which violates zero-friction.

**Priority 4: Remote library fetching.**

Even a simple `git clone` + periodic `git pull` for remote libraries would move us from "single server" to "federated servers." The config already supports it. The plumbing doesn't.

---

## Summary

**What we got right:**
- Extraction pipeline quality architecture — genuinely well-designed
- Self-hostable-first — clean, single-binary, SQLite-default
- Knowledge decay — mathematically sound, category-aware, with rescue
- Background worker system — production-quality async processing
- Dimensional quality scoring — better than any "rate this 1-10" approach
- A/B testing framework — ready for evidence-based validation

**What drifted:**
- Git is not the truth (SQLite is)
- Pre-commit security hooks not implemented
- Client SDK / hook installation not built
- Repo-per-skill not wired up

**What's missing from the vision:**
- Any form of network / federation / collective
- Multi-agent collaboration
- Trust and reputation
- Public skill libraries
- The "mycelial" part of Mycelium

**Overall:** We built an excellent extraction-injection engine. We haven't built a knowledge network yet. The foundation is solid — the architecture can grow into the vision if we prioritize the git integration and network layer next. The risk is that we keep polishing the pipeline (which is already good) and never build the network (which is the whole point).

The manifesto should probably be updated to acknowledge a phased approach — "we're building the extraction engine first, the network second" — rather than implying the full vision exists today. Honesty about where we are doesn't diminish where we're going. It actually makes the vision more credible.

---

*"The best time to check alignment is before you've drifted too far to course-correct. The second best time is now."*
