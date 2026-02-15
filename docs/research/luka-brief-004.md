# Research Brief #004: Knowledge Compression

**Author:** Luka Jensen  
**Date:** 2026-02-15  
**Status:** Complete  
**Depends on:** Brief 001 (quality dimensions), Brief 002 (extraction filters), Brief 003 (delayed signals), Engineering Spec

---

## 1. What's the Optimal Structure for an Extracted Skill?

I surveyed how different fields structure reusable knowledge units. The patterns are remarkably convergent.

### Cross-Domain Comparison

| Format | Core Structure | What Makes It Reusable |
|---|---|---|
| **GoF Design Patterns** | Intent → Motivation → Structure → Participants → Consequences | Explicit "when to use" + tradeoffs |
| **Medical Protocols (NICE)** | Indication → Contraindications → Procedure → Monitoring → Escalation | Boundary conditions front-loaded |
| **Military SOPs** | Purpose → Conditions → Standards → Performance Steps → Notes | "Conditions" = trigger context |
| **Recipe (Serious Eats)** | Why It Works → Ingredients → Steps → Troubleshooting | Reasoning before procedure |
| **StackOverflow (top answers)** | Problem restatement → Solution → Why it works → Caveats | Implicit "I had the same problem" |
| **Unix man pages** | NAME → SYNOPSIS → DESCRIPTION → OPTIONS → EXAMPLES → SEE ALSO | Scannable layers of depth |
| **API docs (Stripe)** | Endpoint → Parameters → Response → Errors → Example | Machine-parseable + human-readable |
| **ADRs** | Context → Decision → Consequences → Alternatives Considered | Captures the "why not" |
| **Wikipedia** | Lead paragraph → Sections → References | Lead = standalone summary |

### Universal Elements

Every effective knowledge format includes these (in roughly this order):

1. **Name/Title** — what is this about (scannable)
2. **When to use** — trigger conditions, not just topic tags
3. **Core solution** — the actual knowledge, procedural or declarative
4. **Why it works** — reasoning, not just steps
5. **Boundaries** — when NOT to use, edge cases, failure modes

Optional but high-value:
6. **Alternatives considered** — what was tried and rejected
7. **Verification** — how to know it worked
8. **References** — source session, related skills

The man page insight is important: **layer depth**. The title and "when to use" should be enough to decide relevance. The solution should be enough to act. The reasoning/boundaries are for when things go sideways.

---

## 2. How Do You Distill Signal from Noise?

A 50K-token agent session is like raw footage, a court transcript, or a lab notebook. Every field that produces actionable knowledge from raw records uses a similar process.

### Cross-Domain Distillation Methods

**Court transcript → Legal brief**: Court reporters capture everything. Lawyers then extract: (1) key facts, (2) legal arguments, (3) rulings. The filtering criterion is *legal relevance* — everything else is noise regardless of how much time it consumed. A 3-day trial becomes a 10-page brief.

**Lab notebook → Paper**: Scientists record every failed experiment, calibration, dead end. The paper contains: what worked, why it worked, and enough method detail to reproduce. Failed approaches appear only if they're instructive ("we initially tried X, which failed because Y, leading us to Z").

**Raw SIGINT → Intelligence briefing**: The NSA's problem is exactly ours. Millions of intercepts, 99.9% noise. The pipeline: automated filtering (keywords, patterns) → analyst triage → structured brief (BLUF — Bottom Line Up Front — then supporting detail). Key principle: **lead with the conclusion**.

**Full game → Highlight reel**: Sports editors select by *impact on outcome*. A 3-hour game becomes 5 minutes. The criterion: did this moment change the score or create a turning point? For us: did this moment solve the problem or represent a reusable technique?

**Medical chart → Discharge summary**: A 2-week hospital stay generates thousands of data points. The discharge summary captures: diagnosis, what was done, what worked, medications on discharge, follow-up needed. It's ruthlessly outcome-oriented.

### The Universal Distillation Pattern

```
Raw Signal → Filter by relevance → Extract structure → Compress → Verify fidelity
```

For agent sessions, this maps to:

1. **Filter**: Strip retries, typos, debugging dead ends, small talk, status updates
2. **Extract structure**: Identify the problem-solution arc. What was the goal? What approach was taken? What was the result?
3. **Compress**: Rewrite in canonical form. The skill should read like it was written by someone who already knew the answer — not like a transcript of discovery.
4. **Verify fidelity**: Does the compressed version preserve enough detail to actually reproduce the solution?

The BLUF principle from intelligence briefings is critical: **the skill should start with the answer, not the journey**.

---

## 3. What's the Right Compression Ratio?

2KB is a good default but not a universal target. The right unit size depends on knowledge complexity.

### Cross-Domain Size Analysis

| System | Unit Size | Variance | What Controls Size |
|---|---|---|---|
| **Zettelkasten** | 1 idea per card (~100-300 words) | Low | Atomic by definition |
| **Unix man pages** | 0.5KB (true) to 50KB (bash) | Extreme | Complexity of the tool |
| **API reference entry** | 200 bytes (simple endpoint) to 5KB (complex) | High | Parameter count, edge cases |
| **Recipe** | 500 bytes (boiled egg) to 3KB (croissant) | Medium | Technique complexity |
| **StackOverflow answer** | 200 bytes (one-liner) to 5KB (deep explanation) | High | Problem complexity |
| **Design pattern (GoF)** | 3-8KB per pattern | Medium | Structural complexity |
| **Medical protocol** | 1-10KB | Medium | Decision tree depth |

### Key Insight: Tiered Compression

No single size fits all. But you can tier:

- **Atomic skills** (~200-500 bytes): Single technique, one-liner solutions, configuration snippets. "How to fix CORS in Express." These are like Zettelkasten cards.
- **Standard skills** (~1-3KB): Problem-solution pairs with context and reasoning. Most extracted skills will be this size. Like a good StackOverflow answer.
- **Complex skills** (~3-8KB): Multi-step procedures, architectural patterns, integration guides. Like a design pattern or medical protocol.

**Enforcement approach**: Don't enforce a hard cap. Instead, set a *soft target* of 2KB and flag skills that exceed 5KB for review — they may need to be decomposed into multiple atomic/standard skills. The quality rubric's "Context Portability" dimension naturally penalizes bloated skills (more context = less portable).

The Zettelkasten principle applies: **if a skill covers two distinct techniques, it should be two skills**. Atomicity is more valuable than completeness in a single unit.

---

## 4. What Gets Lost in Compression?

This is the hardest question. Compression is lossy by nature. The question is which losses matter.

### What Typically Gets Lost

| Lost Element | Value | How Other Fields Handle It |
|---|---|---|
| **Failed approaches** | High — prevents repeat mistakes | ADRs: explicit "Alternatives Considered" section. Postmortems: "What we tried" timeline. Medical case studies: differential diagnosis narrative. |
| **Context/environment** | Medium — affects applicability | Recipes: "altitude adjustments." Software: "tested on Ubuntu 22.04, Node 18." Military SOPs: "conditions" section. |
| **Reasoning chain** | High — enables adaptation | Scientific papers: Discussion section. Design patterns: "Consequences" section. Good code: comments explain *why*, not *what*. |
| **Edge cases** | Medium-High | API docs: "Errors" section. Medical protocols: "Special populations." Unix man pages: BUGS section. |
| **Emotional/social context** | Low for reuse | Generally discarded. Exception: postmortems preserve team dynamics when relevant to failure. |
| **Temporal sequence** | Low-Medium | Usually compressed to logical order, not chronological. Lab notebooks → papers reorder everything. |

### The ADR Insight

Architectural Decision Records are the best model for preserving "what NOT to do." Their structure:

```
## Context
What is the issue that we're seeing that is motivating this decision?

## Decision
What is the change that we're proposing and/or doing?

## Consequences
What becomes easier or more difficult because of this change?

## Alternatives Considered  ← THIS IS THE KEY SECTION
What other options were evaluated and why were they rejected?
```

The "Alternatives Considered" section is where the highest-value compressed information lives. Knowing that approach X was tried and failed because of Y is often more valuable than knowing that approach Z works.

### Proposal: Preserve Failed Approaches as Warnings

Don't try to preserve the full narrative. Instead, compress failures into a structured "Pitfalls" section:

```markdown
## Pitfalls
- **Don't use `npm install --force`** — resolves dependency conflicts but breaks peer dependencies silently. Discovered after 30 minutes of debugging.
- **`useEffect` with empty deps won't work here** — the callback needs the latest state. Use `useRef` to capture it.
```

This is cheap (adds ~200 bytes), high-value, and follows the intelligence briefing principle: lead with the conclusion ("don't do X"), then briefly explain why.

---

## 5. Concrete Proposal: SKILL.md Template

Based on everything above, here's the extraction output format.

### Template

```markdown
---
name: fix-postgres-connection-pool-exhaustion
version: 1
tags: [database, postgres, connection-pooling, performance]
author: mycelium-extractor
confidence: 0.82
created: 2026-02-15
dependencies: []
---

# Fix PostgreSQL Connection Pool Exhaustion

Brief one-line summary: what this skill helps you do.

## When to Use

Trigger conditions — not just topic, but SITUATION:
- You're seeing "too many connections" errors from PostgreSQL
- Connection count grows over time but doesn't shrink
- Application uses an ORM with connection pooling (e.g., Prisma, SQLAlchemy, TypeORM)

## Solution

The actual knowledge. Procedural (steps) or declarative (configuration), depending on the skill type.

1. Check current connection count: `SELECT count(*) FROM pg_stat_activity;`
2. Identify the leak: connections in `idle` state with old `backend_start` timestamps
3. Configure pool bounds in your ORM:
   ```
   pool:
     min: 2
     max: 10
     idleTimeoutMillis: 30000
     acquireTimeoutMillis: 10000
   ```
4. Add connection lifecycle logging to identify which code paths acquire but don't release

Key insight: the default pool size in most ORMs is unbounded or set to 100, which exceeds PostgreSQL's default `max_connections` (100). The fix is to set an explicit max below your PostgreSQL limit divided by the number of application instances.

## Why It Works

Reasoning — so the reader can adapt this to their situation:
- Connection pools without explicit bounds grow monotonically under load
- ORM defaults assume a single application instance; multi-instance deployments multiply the problem
- The `idleTimeoutMillis` is critical — without it, connections acquired during traffic spikes are never returned

## Pitfalls

What NOT to do and why (compressed from failed approaches):
- **Don't just increase `max_connections` in PostgreSQL** — each connection costs ~10MB RAM. At 500 connections that's 5GB just for connection overhead. Fix the leak, don't accommodate it.
- **Don't set `min: 0`** — cold-start latency for new connections is 50-100ms. Keep a small minimum pool for baseline traffic.
- **PgBouncer is not always the answer** — it helps with many short-lived connections but masks pool management bugs in your application. Fix the app first.

## Verification

How to confirm the skill was applied correctly:
- Connection count stabilizes at or below your configured `max`
- `pg_stat_activity` shows connections cycling (not accumulating)
- No `acquireTimeout` errors under normal load

## Context

Optional. Environment specifics, version constraints, scope limitations:
- Tested with PostgreSQL 14-16, Prisma 5.x, Node.js 18+
- Pattern applies to any connection-pooling ORM; specific config keys differ
- Source session: `session-abc123` (2026-02-14)
```

### Section Requirements

| Section | Required? | Purpose | Typical Size |
|---|---|---|---|
| **Front matter** | Required | Machine-parseable metadata | ~150 bytes |
| **Title + summary** | Required | Scannable identification | ~100 bytes |
| **When to Use** | Required | Trigger conditions for injection matching | ~200 bytes |
| **Solution** | Required | The actual knowledge | ~500-2000 bytes |
| **Why It Works** | Required | Reasoning for adaptation | ~200-500 bytes |
| **Pitfalls** | Recommended | Failed approaches, anti-patterns | ~200-500 bytes |
| **Verification** | Recommended | Confirm correct application | ~100-200 bytes |
| **Context** | Optional | Environment, versions, scope | ~100-200 bytes |

**Required sections**: Front matter, Title, When to Use, Solution, Why It Works  
**Recommended sections**: Pitfalls, Verification  
**Optional sections**: Context

### Why This Structure

- **When to Use** is the injection key. It's what the system matches against incoming prompts. Write it as trigger conditions, not topic descriptions.
- **Solution** is BLUF — lead with the answer. Not the journey.
- **Why It Works** is the adaptation layer. Without it, the skill is a recipe that breaks when ingredients change.
- **Pitfalls** preserves the most valuable signal from compression: what NOT to do. This section alone justifies the extraction — humans repeat the same mistakes.
- **Verification** closes the loop. A skill without verification criteria can't feed back into the quality system.

### Good vs Bad Extraction: Examples

**BAD — too vague, no trigger conditions:**
```markdown
# Database Connections
## When to Use
When you have database connection issues.
## Solution
Configure your connection pool properly. Set appropriate limits.
```

Why it's bad: "Database connection issues" matches everything and helps nothing. "Configure properly" contains zero information. This skill will be injected frequently and help never.

**BAD — too verbose, just a transcript:**
```markdown
# Fix PostgreSQL Connection Pool Exhaustion
## When to Use
The user was building a Node.js application and noticed that their PostgreSQL
database was running out of connections. They first tried increasing the
max_connections parameter but that didn't work because...
[continues for 3000 words renarrating the session]
```

Why it's bad: This is a transcript summary, not extracted knowledge. It preserves chronological noise instead of compressing to reusable structure.

**BAD — no reasoning, fragile:**
```markdown
# Fix PostgreSQL Connections
## When to Use
PostgreSQL connection errors with Prisma.
## Solution
Add this to your schema.prisma:
\`\`\`
datasource db {
  provider = "postgresql"
  url = env("DATABASE_URL")
  connectionLimit = 10
}
\`\`\`
```

Why it's bad: The magic number 10 has no justification. If the reader has 5 app instances, 10 per instance = 50, which may still be too many — or too few. Without "Why It Works," the skill can't be adapted. Also missing: what connectionLimit actually does, and the Prisma-specific deprecation of this field in favor of connection string parameters.

**GOOD** — the template example above. Specific trigger conditions, concrete solution with explanation, reasoning for adaptation, explicit anti-patterns, verification criteria.

---

## 6. Implementation Notes for Otso

### Changes to skill.go

The current `Validate()` requires `# Title`, `## When to Use`, and `## Solution`. This is already correct for the minimum viable format. I'd recommend:

1. **Add `## Why It Works` to required sections** — this is what separates a useful skill from a fragile one.
2. **Keep `## Pitfalls` and `## Verification` as recommended, not required** — the extractor should attempt to generate them, but some sessions genuinely lack failed approaches or clear verification criteria.
3. **Don't validate `## Context`** — it's optional metadata that the extractor adds when environment info is available.

### Extraction Prompt Design

The Stage 3 LLM critic should both evaluate AND extract in a single call. The extraction prompt should:

1. Take the session content + Stage 2 classification
2. Output a structured JSON with the skill sections
3. Use the template above as the output schema

Rough prompt skeleton:

```
You are extracting reusable knowledge from an agent session.

Output a skill with these sections:
- name: kebab-case identifier
- when_to_use: list of specific trigger conditions (situations, not topics)
- solution: the core technique or procedure (be concrete, include code if relevant)
- why_it_works: reasoning that enables adaptation
- pitfalls: approaches that were tried and failed, or common mistakes (if evident)
- verification: how to confirm correct application (if determinable)
- context: environment specifics, version constraints (if mentioned)

Rules:
- Write as if you already knew the answer. No narrative, no journey.
- "When to Use" must be matchable trigger conditions, not topic labels.
- "Solution" must contain enough detail to act on without the source session.
- "Pitfalls" should only include genuinely instructive failures, not every wrong turn.
- If a section has no content, omit it (except required sections).
```

### Size Budget

Target: 1-3KB for the body (excluding front matter). The extractor should:
- Flag skills >5KB for decomposition review
- Allow skills <500 bytes only if they're genuinely atomic (single config change, one-liner fix)
- Track size distribution in metrics to calibrate over time

### Compatibility with Existing Format

The proposed template is fully compatible with the current `skill.go` parser:
- Front matter: unchanged (name, version, tags, author, confidence, created, dependencies)
- Required body sections: `# Title`, `## When to Use`, `## Solution` (already validated)
- New required section `## Why It Works` needs one line added to `validateBody()`
- Recommended/optional sections are just Markdown — no parser changes needed

---

## Summary

Knowledge compression is a solved problem across many fields — we just need to apply the patterns. The key principles:

1. **BLUF**: Lead with the answer, not the journey
2. **Trigger conditions over topic labels**: "When to Use" is the injection key
3. **Preserve reasoning**: "Why It Works" enables adaptation
4. **Compress failures into warnings**: "Pitfalls" is the highest-value-per-byte section
5. **Layer depth**: Title → When to Use → Solution → Why → Pitfalls → Context (each layer adds depth, reader stops when they have enough)
6. **Variable size with soft targets**: 1-3KB typical, allow 200B-8KB range, flag outliers

The template above is ready for implementation as the extraction output format.
