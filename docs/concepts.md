# How Mycelium Thinks

A conceptual guide for contributors. This document explains *why* the system works the way it does, using four mental models that map to the four pipeline stages. For the technical specifics of each component, see [architecture.md](./architecture.md).

---

## The Problem

Every day, AI agents help people solve problems. Every solution dies when the session ends. The next person with the same problem starts from zero. Mycelium captures that knowledge automatically and delivers it to future sessions — without anyone having to write documentation, publish anything, or even know the system exists.

The hard part isn't storing knowledge. It's deciding what's worth keeping, how good it is, who should receive it, and when to let it go. Four mental models explain how Mycelium makes those decisions.

---

## 1. Gold Panning — Multi-Stage Extraction

**The model:** A gold miner doesn't analyze every grain of sand with a microscope. They start with a pan, washing away the obviously worthless material. What survives gets progressively finer treatment — magnetic separation, chemical assays — until only gold remains. Each stage is more precise *and* more expensive than the last.

**How it maps:** Most agent sessions don't produce reusable knowledge. They're routine work, failed experiments, or exploration with no conclusion. Running an expensive LLM critic on every session would be wasteful and slow.

Instead, Mycelium filters in stages:

1. **Panning** (Stage 1) — Cheap metadata checks. Is the session long enough? Did the agent actually execute anything successfully? Sessions that are obviously worthless get rejected instantly, at near-zero cost.

2. **Sifting** (Stage 2) — Content analysis with lightweight classifiers. Does the session contain a problem-solution structure? Is the solution novel compared to what we already know? This costs more than checking metadata, but far less than an LLM call.

3. **Assaying** (Stage 3) — An LLM critic examines the survivors with focused questions. Not "is this good?" but "is the reasoning explicit?" and "does this contradict known practices?" This is expensive, but it only runs on the small fraction that passed earlier stages.

**The key insight:** You can afford expensive quality checks if you first eliminate the noise cheaply. Each stage has a different cost-precision tradeoff, and they're ordered from cheapest to most precise.

**For contributors:** When working on extraction, think about which stage a filter belongs in. Can it be evaluated from metadata alone? Stage 1. Does it need content analysis? Stage 2. Does it require judgment? Stage 3. Moving a filter to an earlier stage saves cost. Moving it later improves precision.

---

## 2. Wine Tasting — Dimensional Quality Evaluation

**The model:** A professional wine taster doesn't sip a wine and say "7 out of 10." They evaluate specific dimensions independently — appearance, aroma, acidity, tannins, finish — each on its own scale, each answerable without reference to the others. The overall assessment emerges from the combination. Two wines can score identically overall while being excellent for completely different reasons.

**How it maps:** Asking "is this knowledge good?" is like asking "is this wine good?" — it's too vague to produce reliable, consistent answers. Different evaluators will weigh different things, and you can't debug why something scored poorly.

Mycelium evaluates skills on seven specific dimensions:

- **Problem Specificity** — Is the problem clearly defined?
- **Solution Completeness** — Could someone follow this and solve the problem?
- **Context Portability** — Does this work beyond its original environment?
- **Reasoning Transparency** — Does it explain *why*, not just *what*?
- **Technical Accuracy** — Are the details correct?
- **Verification Evidence** — Is there proof it works?
- **Innovation Level** — Is the approach novel?

Each dimension is scored 1–5 with explicit criteria at each level. An LLM answering "is this reasoning transparent?" is far more reliable than one answering "rate this knowledge."

**The key insight:** Structured evaluation with specific questions beats holistic judgment in every domain where it's been tried. It's more consistent, more debuggable, and more calibratable.

**For contributors:** When tuning quality assessment, work on one dimension at a time. If extraction is letting through poorly reasoned skills, improve the Reasoning Transparency classifier specifically. If it's rejecting niche but accurate solutions, check whether Context Portability is being overweighted. The dimensional model lets you diagnose and fix quality problems precisely.

---

## 3. Reference Librarian — Intelligent Injection Matching

**The model:** A good reference librarian doesn't just search for your keywords. If you ask about "debugging a slow React app," they don't hand you every document containing "React" and "slow." They think about what you actually need — maybe a guide to browser profiling tools, or a piece on database query optimization, or an article about CDN configuration. They understand the *problem beneath the question*.

**How it maps:** Text similarity is a blunt instrument for matching knowledge to needs. A prompt about "React performance" is most *textually* similar to other documents mentioning React and performance. But the most *useful* skill might be about JavaScript memory profiling or browser DevTools — related by problem pattern, not by vocabulary.

Mycelium matches in multiple steps:

1. **Understand the question** — Classify the prompt's intent (building, fixing, optimizing), domain (frontend, backend, infrastructure), and specific problem pattern.

2. **Search by pattern, not just text** — Find skills that address the same *type* of problem, even if they use different technology or terminology.

3. **Rank by multiple signals** — Combine pattern overlap, embedding similarity, and historical success rate. A skill that has consistently helped with similar problems ranks higher than one that merely uses similar words.

**The key insight:** The most useful knowledge often doesn't look like what you asked for. Matching by problem pattern surfaces solutions that keyword search would miss entirely.

**For contributors:** When working on injection, resist the temptation to optimize for embedding similarity alone. The problem pattern taxonomy is the librarian's expertise — it encodes the understanding of what problems are *actually* related, beyond surface text. Improving the taxonomy or the prompt classifier has a bigger impact on injection quality than tuning embedding weights.

---

## 4. Forest Fires — Knowledge Decay

**The model:** A healthy forest needs fire. Without periodic burns, dead wood and undergrowth accumulate until the forest is a tinderbox — and the eventual fire is catastrophic. Controlled burns clear the debris while preserving mature trees with deep root systems. Crucially, not everything burns the same way: old-growth trees survive fires that kill saplings, and some species *depend* on fire to germinate.

**How it maps:** A knowledge library that only grows eventually drowns in stale, outdated, or redundant information. API-specific workarounds go stale when the API changes. Deprecated library solutions become actively harmful. Without decay, the ratio of useful to useless knowledge degrades over time, and injection quality collapses under noise.

Mycelium applies different decay rules to different types of knowledge:

- **Foundational skills** (old-growth trees) — Core patterns that rarely change: debugging race conditions, designing database schemas, structuring error handling. These decay slowly and require strong evidence to deprecate.

- **Tactical skills** (undergrowth) — Specific solutions tied to current tool versions, API behaviors, or library quirks. These decay fast. If they haven't been used recently and validated by success, they lose ranking.

- **Contextual skills** (seasonal growth) — Environment-specific knowledge. Useful in the right context, irrelevant elsewhere. Medium decay rate.

Decay is gradual, not sudden. Skills don't get deleted — they lose ranking until they're effectively invisible. A decayed skill that suddenly gets used again and correlates with success is *rescued* — its ranking recovers. The system forgets gracefully and can remember again when evidence warrants it.

**The key insight:** Different knowledge types have different shelf lives, and the system must model that explicitly. Treating all knowledge the same — either never deleting (hoarding) or deleting on a fixed schedule (indiscriminate) — fails in different ways.

**For contributors:** When working on decay, think about what category a skill falls into before adjusting thresholds. A foundational skill that hasn't been injected in six months might still be perfectly valid — it's just niche. A tactical skill that hasn't been used in six months is probably stale. The category determines the appropriate response to inactivity.

---

## How the Models Connect

The four models form a coherent pipeline:

1. **Gold panning** gets the raw material — filtering the enormous volume of agent sessions down to candidates worth evaluating.
2. **Wine tasting** assesses the survivors — determining quality across specific dimensions rather than with a single thumbs-up/thumbs-down.
3. **The reference librarian** delivers the knowledge — matching skills to needs by understanding the problem, not just the words.
4. **Forest fires** keep the library healthy — ensuring that what stays in the system earns its place through continued relevance.

Each model addresses a different challenge (volume, quality, relevance, freshness), and each produces a different type of signal that the next stage depends on. Together, they describe a system where knowledge is extracted carefully, evaluated rigorously, delivered intelligently, and maintained honestly.

---

## Further Reading

- [Architecture](./architecture.md) — Technical component descriptions, data flow, and API boundaries
- [Glossary](./glossary.md) — Definitions of terms used throughout the project
- [Feedback Patterns](./feedback-patterns.md) — How developers interact with the system
