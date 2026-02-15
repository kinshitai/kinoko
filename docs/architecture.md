# Internal Architecture

This document describes Mycelium's technical architecture: how knowledge flows from agent sessions into a shared library and back out into future sessions. No analogies — just components, data flow, and boundaries.

---

## System Overview

Mycelium is a pipeline with four stages:

```
Agent Session → EXTRACTION → STORAGE → INJECTION → Agent Session
                                ↑                       |
                                └── DECAY ──────────────┘
                                    (ongoing)
```

A session produces work. Extraction decides whether that work contains reusable knowledge and, if so, distills it into a **skill**. Storage persists skills in a versioned, searchable library. Injection matches relevant skills to new sessions and delivers them as context. Decay continuously demotes or removes skills that are no longer useful.

---

## Components

### 1. Extraction Pipeline

Extraction is a multi-stage filter. Each stage is cheaper than the next; most sessions are rejected early.

**Stage 1: Metadata Pre-Filters**

Operates on session metadata only — no content analysis. Rejects sessions that are too short, too long, or lack meaningful interaction.

| Filter | Threshold | Rationale |
|---|---|---|
| Session duration | 2–180 minutes | Too brief = no depth; too long = likely exploratory |
| Tool call count | ≥ 3 | Minimal interaction unlikely to produce knowledge |
| Error rate | ≤ 70% | Mostly-failed sessions rarely contain solutions |
| Successful execution | ≥ 1 exit code 0 | No successful execution = no verified work |

All thresholds are configurable. Stage 1 is a decision tree — every session either passes or is rejected.

**Stage 2: Structured Dimensional Scoring**

Runs two classifiers on session content:

1. **Embedding distance** — measures how far the session's content is from existing skills. Sessions too similar to existing knowledge score low on novelty; sessions too dissimilar may be noise.
2. **Structured rubric scoring** — evaluates the session against specific quality dimensions (see [Dimensional Scoring](#dimensional-scoring) below).

Stage 2 produces a numeric score per dimension. Sessions must meet minimum thresholds on Problem Specificity, Solution Completeness, and Technical Accuracy (≥ 3/5 each) to proceed.

**Stage 3: LLM Critic**

An LLM evaluates surviving candidates with focused, dimensional questions — not a holistic "is this good?" judgment. The critic answers specific questions:

- Does this contain a reusable solution pattern?
- Is the reasoning explicit enough to apply elsewhere?
- Does it contradict known best practices?

Stage 3 is expensive. By filtering at Stages 1 and 2, we limit LLM calls to a small fraction of total sessions.

**Output:** A candidate skill with quality scores, or rejection.

### 2. Skill Storage

A skill is the atomic unit of knowledge. It is a Markdown file (`SKILL.md`) with YAML front matter, stored in a Git repository.

**Skill data model:**

```yaml
---
id: <uuid>
title: <human-readable title>
version: <semver>
patterns:
  - <problem pattern tags, e.g. FIX/Backend/DatabaseConnection>
quality:
  problem_specificity: <1-5>
  solution_completeness: <1-5>
  context_portability: <1-5>
  reasoning_transparency: <1-5>
  technical_accuracy: <1-5>
  verification_evidence: <1-5>
  innovation_level: <1-5>
category: foundational | tactical | contextual
usage:
  injection_count: <int>
  last_injected: <timestamp>
  success_correlation: <float>
created: <timestamp>
updated: <timestamp>
---

<Markdown body: problem description, solution, context, reasoning>
```

**Storage topology:**

Skills are stored in subdirectories of `~/.mycelium/skills/`. Libraries are layered:

```
Local skills  →  Company/team skills  →  Public library
(highest priority)                      (lowest priority)
```

When multiple skills match a query, local skills take precedence over shared ones.

**Embeddings:** Each skill has a single embedding vector computed from its content. Stored alongside the skill metadata. One embedding space (not multiple — that's premature optimization at this stage).

### 3. Injection Pipeline

Injection delivers relevant skills as additional context when an agent starts a session.

**Matching steps:**

1. **Prompt classification** — Parse the user's prompt to extract intent (BUILD, FIX, OPTIMIZE, INTEGRATE, CONFIGURE, LEARN), technical domain (Frontend, Backend, DevOps, Data, Security, Performance), and specific problem signals.

2. **Pattern matching** — Find skills whose problem pattern tags overlap with the classified prompt. Uses the [Problem Pattern Taxonomy](#problem-pattern-taxonomy).

3. **Similarity ranking** — Rank candidate skills using a weighted score:

   ```
   Score = 0.5 × Pattern_Overlap
         + 0.3 × Cosine_Similarity(prompt_embedding, skill_embedding)
         + 0.2 × Historical_Success_Rate
   ```

4. **Quality filtering** — Exclude skills below quality thresholds. Prefer skills with high Context Portability and Verification Evidence.

5. **Delivery** — Top-ranked skills are injected into the agent's context window. Number of injected skills is bounded by context budget.

**What "injection" means concretely:** The skill's Markdown content is prepended to the agent's system prompt or included as reference material, depending on the agent integration.

### 4. Decay System

Decay prevents the knowledge library from accumulating stale or unused skills.

**Skill categories and decay rates:**

| Category | Description | Decay behavior |
|---|---|---|
| **Foundational** | Core patterns that rarely change (e.g., "how to debug race conditions") | Slow decay. Requires strong evidence to deprecate. |
| **Tactical** | Specific solutions tied to current tool versions or APIs | Fast decay. Must be validated by recent usage to persist. |
| **Contextual** | Environment-specific knowledge | Medium decay. Loses relevance as environments change. |

**Decay mechanics:**

- **Usage tracking:** Every injection is logged. Skills that haven't been injected in a configurable period (default: 6 months) begin losing ranking.
- **Success correlation:** Skills whose injections correlate with session failures get flagged for review and deprioritized.
- **Gradual demotion:** Decaying skills aren't deleted — they drop in ranking until they're effectively invisible to injection. They can be rescued by successful usage or manual intervention.
- **Version supersession:** When a new skill is extracted that covers the same problem pattern as an existing skill, the old skill is compared and may be deprecated in favor of the new one.

### 5. Logging and Measurement

Every pipeline decision point emits structured logs. This is not optional — it ships with the pipeline, not after.

**What's logged:**

- Stage 1: filter pass/fail per criterion, per session
- Stage 2: dimensional scores per classifier, per session
- Stage 3: LLM critic verdict and reasoning
- Injection: which skills matched, ranking scores, which were delivered
- Decay: demotion events, rescues, deprecations
- Outcomes: session success/failure correlated with injected skills

**Human review sampling:** 1% of processed sessions (both accepted and rejected) are flagged for manual review to calibrate filter accuracy.

**Baseline metrics:**

- Extraction precision: what fraction of extracted skills are actually useful?
- Extraction recall: what fraction of useful knowledge is captured?
- Injection relevance: do injected skills help the session?
- Decay accuracy: are we keeping good skills and removing bad ones?

---

## Dimensional Scoring

Skills are evaluated on seven dimensions, each scored 1–5:

| Dimension | Question |
|---|---|
| Problem Specificity | Does this solve a clearly defined problem? |
| Solution Completeness | Can someone follow this to solve the problem? |
| Context Portability | How broadly applicable is this beyond its original context? |
| Reasoning Transparency | Does this explain *why*, not just *what*? |
| Technical Accuracy | Are the technical details correct and current? |
| Verification Evidence | Is there proof this solution works? |
| Innovation Level | How novel is the approach? |

**Minimum viable skill:** ≥ 3 on Problem Specificity, Solution Completeness, and Technical Accuracy.

**High-value skill:** ≥ 4 average across all dimensions.

**Injection priority weighting:** Context Portability × Verification Evidence.

---

## Problem Pattern Taxonomy

Skills and prompts are classified into a three-tier taxonomy:

**Tier 1 — Intent:**
`BUILD` · `FIX` · `OPTIMIZE` · `INTEGRATE` · `CONFIGURE` · `LEARN`

**Tier 2 — Domain:**
`Frontend` · `Backend` · `DevOps` · `Data` · `Security` · `Performance`

**Tier 3 — Specific pattern** (examples):
`FIX/Backend/DatabaseConnection` · `BUILD/Frontend/ComponentDesign` · `OPTIMIZE/Performance/MemoryLeak`

Skills can have multiple pattern tags. More specific tags receive higher relevance weight during injection matching.

The taxonomy is a fixed, manually curated list of ~15–20 patterns. It is not ML-generated.

---

## Component Boundaries

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  Extraction  │────▶│   Storage   │◀────│    Decay    │
│   Pipeline   │     │  (Git +     │     │   System    │
│              │     │  Embeddings)│     │             │
└─────────────┘     └──────┬──────┘     └─────────────┘
                           │
                    ┌──────▼──────┐
                    │  Injection  │
                    │  Pipeline   │
                    └─────────────┘
```

**Extraction → Storage:** Extraction produces a `SKILL.md` file with metadata. Storage accepts it, computes the embedding, and commits it to the skill repository.

**Storage → Injection:** Injection queries storage by pattern tags and embedding similarity. Storage returns ranked candidates.

**Decay → Storage:** Decay reads usage statistics from storage, computes demotion/deprecation decisions, and writes updated metadata back.

**Injection → Decay (indirect):** Injection logs usage events. Decay reads those logs to determine which skills are active.

Each component communicates through defined interfaces. Extraction does not know about injection. Injection does not trigger extraction. Decay operates independently on a schedule.

---

## Post-Session Signals

Extraction doesn't only happen at session end. A delayed extraction pass can evaluate sessions retroactively based on:

- **Return signal** — User reopens an old session. High-confidence indicator of extractable knowledge.
- **Artifact persistence** — Session outputs (files, configs) that survive in the user's environment.
- **Cross-reference signal** — Artifacts from one session appearing in another.

These signals require separating transferable patterns from personal context during extraction. The story stays local; the pattern travels.
