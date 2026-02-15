# Glossary

Terms used throughout the Mycelium project with specific meanings.

---

**Category**
A classification of a skill's persistence characteristics. One of three values:
- **Foundational** — Core patterns that rarely change. Slow decay.
- **Tactical** — Solutions tied to specific tool versions or APIs. Fast decay.
- **Contextual** — Environment-specific knowledge. Medium decay.

See [architecture.md → Decay System](./architecture.md#4-decay-system).

**Critic**
The LLM evaluator that runs at Stage 3 of the extraction pipeline. Answers focused, dimensional questions about candidate skills rather than making holistic quality judgments. The most expensive extraction stage — only runs on sessions that pass Stages 1 and 2.

**Decay**
The process by which skills lose ranking over time due to inactivity or declining success correlation. Decay is gradual (demotion, not deletion) and category-dependent. Decayed skills can be rescued by renewed successful usage.

**Dimensional Scoring**
The evaluation method used to assess skill quality. Seven independent dimensions (Problem Specificity, Solution Completeness, Context Portability, Reasoning Transparency, Technical Accuracy, Verification Evidence, Innovation Level), each scored 1–5 with explicit criteria. Replaces holistic "is this good?" judgments.

**Embedding**
A vector representation of a skill's content, used for similarity-based retrieval during injection. Mycelium uses a single embedding space per skill.

**Extraction**
The process of identifying reusable knowledge in an agent session and distilling it into a skill. A three-stage pipeline: metadata pre-filters → structured dimensional scoring → LLM critic. Most sessions are rejected; only high-quality candidates become skills.

**Injection**
The process of delivering relevant skills as context to an agent at the start of a session. Matching uses problem pattern classification, embedding similarity, and historical success rates. The skill's content is prepended to the agent's context.

**Knowledge Library**
The collection of skills available to the system. Libraries are layered by scope: local (user-specific) → team/company → public. Local skills take precedence during injection.

**Pattern (Problem Pattern)**
A tag from a fixed taxonomy that classifies the type of problem a skill addresses. Three tiers: Intent (`BUILD`, `FIX`, `OPTIMIZE`, `INTEGRATE`, `CONFIGURE`, `LEARN`), Domain (`Frontend`, `Backend`, `DevOps`, `Data`, `Security`, `Performance`), and Specific Pattern (e.g., `FIX/Backend/DatabaseConnection`). Used for injection matching. The taxonomy is manually curated, ~15–20 patterns total.

**Post-Session Signal**
An extraction indicator that emerges after a session ends: return visits, artifact persistence, cross-references from other sessions. These retroactive signals can trigger delayed extraction.

**Session**
A single interaction between a user and an AI agent. The raw input to the extraction pipeline. Sessions include conversation history, tool calls, outputs, and metadata (duration, error rates, tool usage counts).

**Skill**
The atomic unit of knowledge in Mycelium. A Markdown file (`SKILL.md`) with YAML front matter containing quality scores, problem pattern tags, category, and usage statistics. Stored in a Git repository. Versioned. The output of extraction and the input to injection.

**Success Correlation**
A metric tracking how often a skill's injection correlates with positive session outcomes. Used in injection ranking (higher correlation = higher priority) and decay (declining correlation triggers review).

**Usage Tracking**
The system that records when and how skills are injected, and whether those injections correlate with session success. Feeds both the injection ranking algorithm and the decay system.
