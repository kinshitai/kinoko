# RFC-001: Vision & References

| Field   | Value       |
|---------|-------------|
| Status  | Living      |
| Date    | 2026-02-14  |

---

## What Is Kinoko

A collective knowledge system where agents automatically extract reusable knowledge from work sessions, share it through a version-controlled library, and transparently inject relevant knowledge into future sessions. Humans are the source and the beneficiary. Agents are the transport layer.

## The Core Loop

```
Person works with agent
  → Agent extracts knowledge from the session
  → Knowledge lands in a shared, versioned library
  → Another person's agent absorbs relevant knowledge
  → That person gets a better answer without knowing why
```

No human writes documentation. No human publishes anything. Knowledge sharing is a byproduct of work.

## Inspirations & References

### Voyager (2023, NVIDIA/Stanford)

LLM-powered Minecraft agent that builds its own skill library. Skills are executable code, indexed by embedding, composed into complex behaviors. A critic loop verifies skills before committing them — without it, noise accumulates fast.

The big idea: agents can extract, verify, and reuse their own knowledge. The extraction loop works.

Paper: https://arxiv.org/abs/2305.16291

### MOSAIC (2025, Soltoggio et al.)

Multi-agent collective learning. Agents share knowledge selectively based on task similarity (Wasserstein embeddings). Collective consistently outperforms isolated learners. An emergent curriculum appears — easy tasks solved first, their knowledge enables harder tasks.

The big idea: sharing works, but unfiltered sharing hurts. Quality gates and selective sharing based on relevance are essential.

Paper: https://arxiv.org/html/2506.05577v1

### OpenClaw / ClawHub (2025-2026)

Community skill registry with 3000+ skills in SKILL.md format. Vector search for discovery. Proves the format works and demand exists. But every skill is human-authored — it's an app store, not a learning system.

The big idea: SKILL.md as a standard works. The gap is agent authorship.

### Anthropic Agent Skills (2025)

Skills as folders with SKILL.md + supporting files. Progressive disclosure — metadata loaded at startup, full content on demand. Efficient context window management.

The big idea: folder-per-skill is the right structure. Progressive disclosure matters for agents with finite context.

### Entire.io (2025-2026)

Session-level version control for AI coding agents. Captures full agent sessions via dual hooks (UserPromptSubmit + Stop). Programmatic hook installation (`entire enable`) drops adoption friction to near zero. Transcript sanitization strips secrets before pushing.

The big idea: dual hooks capture the full agent lifecycle. Programmatic installation is critical for adoption. Sanitization is non-negotiable.

### Rowboat (RowboatLabs, 2025-2026)

Local-first AI coworker building a knowledge graph as an Obsidian-compatible Markdown vault with wikilinks. Compounding memory beats cold retrieval. Incremental updates over append-only. Background agents handle maintenance proactively.

The big idea: knowledge that accumulates and updates beats search that starts fresh. Markdown-as-graph works. Background maintenance is infrastructure, not a nice-to-have.

### Moltbook (2026)

Social network for AI agents (1.5M+ agents). Agents naturally share technical tips without being designed to — emergent knowledge exchange. But unstructured, terrible signal-to-noise ratio, security nightmare.

The big idea: agents want to share knowledge (it emerges naturally). But without structure and quality gates, it's chaos.

### A2A Protocol (2025, Google → Linux Foundation)

Agent-to-agent communication standard. Agent Cards for discovery, task delegation, structured artifact exchange. 150+ organizations.

The big idea: knowledge contribution as structured artifact. Standardization creates ecosystem momentum.

## Ideas Worth Exploring

- **Compositional skills** (Voyager) — complex skills built from simpler ones. Not day one, but the library should support it eventually.
- **Emergent curriculum** (MOSAIC) — the order agents learn matters. Easy knowledge enables hard knowledge. Can we influence the learning path?
- **Knowledge graphs over flat search** (Rowboat) — wikilinks between skills create navigable structure. When does flat search stop being enough?
- **Agent identity and reputation** (A2A, AT Protocol) — DIDs for authorship, trust scores that travel with an agent across libraries.
- **Federation** — multiple skill libraries that discover and cross-reference each other. Private company libraries that selectively share with public ones.
- **Session capture as input** (Entire.io) — their transcript capture could feed our extraction pipeline. Complementary tools.

---

*This document collects ideas and inspirations. It is not a spec. Not everything here will be built. Some of it might be wrong. That's fine — it's a reference, not a commitment.*
