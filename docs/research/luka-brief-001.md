# Luka's Research Brief #001
## Cross-Disciplinary Solutions for Mycelium's Three Unknowns

*Luka Jensen, Research Engineer*  
*February 15, 2026*

---

## Problem 1: Extraction Quality

### The Mycelium Problem

We need to automatically determine if an agent session produced reusable knowledge or just did routine work. Our current plan is "LLM critic scores it," but that's essentially having one LLM judge another's output. We're stuck in a circular reasoning problem — how do we know the critic itself isn't hallucinating quality?

### Cross-Field Explorations

**1. Bacterial Quorum Sensing (Microbiology)**
- **Source field:** Bacteria use chemical signaling to detect population density and coordinate group behaviors like biofilm formation or toxin production.
- **How it works:** Individual bacteria release signaling molecules; when concentration reaches a threshold, the population switches from individual to collective behavior. The key insight: the signal strength reflects both individual contribution AND environmental reception.
- **The analogy:** Instead of a single LLM critic, we need multiple independent "sensors" measuring knowledge quality from different angles — novelty detection, verification against known patterns, practical applicability scoring.
- **Practical adaptation:** Run 3-5 lightweight quality checks in parallel: (1) embedding distance from existing skills, (2) syntactic pattern recognition for known "good knowledge" structures, (3) quick verification against external docs/APIs, (4) complexity/information density measurement, (5) human validation sampling on 1% of extractions. Only extract when 4/5 sensors agree.
- **Stretch vs solid:** Solid. The key insight is that quality emerges from consensus of independent measurements, not authority of a single judge.

**2. Wine Tasting & Sensory Evaluation (Food Science)**
- **Source field:** Professional wine tasters use systematic sensory evaluation with standardized descriptors and multiple judges to assess quality.
- **How it works:** They separate assessment into independent dimensions (appearance, aroma, taste, mouthfeel, finish), use calibrated language, and aggregate across multiple trained evaluators. No single taster decides — consensus emerges.
- **The analogy:** Knowledge quality has dimensions that we can measure independently: correctness, completeness, clarity, reusability, novelty. Instead of asking "is this good knowledge?" we ask specific questions.
- **Practical adaptation:** Design a structured extraction rubric: Does it solve a specific problem? (binary) Can it be applied to similar problems? (reusability score) Does it contradict existing knowledge? (consistency check) How much context does it need? (portability score) Is the reasoning explicit? (explainability score)
- **Stretch vs solid:** Very solid. Structured evaluation beats holistic judgment in every domain where it's been tried.

**3. Scientific Peer Review & Retraction Systems (Academia)**
- **Source field:** Academic quality control through pre-publication review and post-publication correction mechanisms.
- **How it works:** Initial review by domain experts, followed by community verification after publication. Importantly, bad papers get retracted — the system has a correction mechanism, not just a filter.
- **The analogy:** We need both extraction-time filtering AND post-extraction quality monitoring. Knowledge quality isn't static — what looks good initially might prove problematic in practice.
- **Practical adaptation:** Two-phase system: (1) Lightweight extraction with moderate thresholds, (2) Post-injection monitoring of skill performance. Track which injected skills actually help vs. harm sessions. Automatically demote or remove skills that consistently correlate with session failures.
- **Stretch vs solid:** Solid. Post-publication correction is what makes academic knowledge self-healing — we need the same feedback loop.

**4. Gold Panning & Mineral Extraction (Geology/Mining)**
- **Source field:** Physical separation techniques that exploit different material properties (density, magnetic properties, chemical reactivity) to separate valuable materials from worthless rock.
- **How it works:** Multiple passes with different separation techniques. First rough separation (panning removes most dirt), then increasingly precise methods (magnetic separation, chemical assays) for higher-purity extraction.
- **The analogy:** Knowledge extraction should be a multi-pass process with increasingly expensive/precise filters at each stage.
- **Practical adaptation:** Stage 1: Cheap pre-filters (session length, tool usage patterns, error rates) to eliminate obviously worthless sessions. Stage 2: Medium-cost NLP analysis (complexity metrics, topic novelty, problem-solution structure detection). Stage 3: Expensive LLM critic only on pre-filtered candidates. Each stage has different cost/precision tradeoffs.
- **Stretch vs solid:** Very solid. Multi-stage filtering is how you handle high-volume, low-signal-rate processes efficiently.

**5. Immune System Self/Non-Self Recognition (Immunology)**
- **Source field:** How the immune system distinguishes between body cells (self) and foreign invaders (non-self) without attacking healthy tissue.
- **How it works:** Multiple recognition systems: pattern recognition receptors (innate immunity), antibody specificity (adaptive immunity), and crucially, negative selection during development — immune cells that react to self-antigens are eliminated during training.
- **The analogy:** We need to train our quality detection system to recognize "good knowledge patterns" while avoiding false positives that would reject useful but unconventional solutions.
- **Practical adaptation:** Build a training dataset of definitively good and bad extracted knowledge. Use contrastive learning to train quality classifiers that can distinguish signal from noise. Importantly, include adversarial examples — knowledge that looks good but is subtly wrong.
- **Stretch vs solid:** Stretch, but worth exploring. The training data creation is the hard part, but the pattern recognition principles are sound.

### My Recommendation

Start with **multi-stage filtering** (gold panning model) combined with **dimensional evaluation** (wine tasting model). 

Specifically: Stage 1 pre-filters based on session metadata (length, tools used, successful task completion). Stage 2 runs structured assessment on specific dimensions. Stage 3 (expensive LLM critic) only runs on candidates that pass both earlier filters.

The dimensional evaluation replaces the single "is this good?" question with specific questions like "Does this contain a novel solution pattern?" and "Is the context clearly specified?" This makes the LLM critic more reliable because it's answering focused questions rather than making holistic judgments.

### What to Explore Next

1. **Build the training dataset first** — collect 500-1000 agent sessions, manually label extraction quality, analyze what distinguishes good knowledge from noise
2. **Test the wine tasting hypothesis** — does structured dimensional evaluation actually produce more reliable quality scores than holistic assessment?  
3. **Measure the microbiology idea** — does consensus across multiple independent classifiers beat single high-quality classifiers?

---

## Problem 2: Injection Precision

### The Mycelium Problem

We need to match user prompts to relevant skills in our library. Embedding similarity is our current plan, but "similar text" doesn't equal "useful knowledge for this situation." A prompt about "debugging React performance" might be most similar to skills about React debugging, but maybe the most useful skill is actually about browser profiling tools or JavaScript memory management.

### Cross-Field Explorations

**1. Reference Librarian Expertise (Library Science)**
- **Source field:** Professional librarians help people find information they need, not just information they ask for. They translate vague questions into precise search strategies.
- **How it works:** Reference interview technique — ask clarifying questions, understand the underlying need, consider the person's expertise level, suggest related resources they didn't know existed. They think in terms of information pathways, not just keyword matching.
- **The analogy:** We need to understand the underlying task, not just match surface text. A question about "debugging slow React app" might really need knowledge about performance profiling, database optimization, or CDN configuration.
- **Practical adaptation:** Multi-step relevance matching: (1) Extract task intent and domain from prompt, (2) Find skills that directly address the task, (3) Find skills that address common causes of the problem, (4) Find skills that address the next likely steps. Weight by task-relevance, not text similarity.
- **Stretch vs solid:** Very solid. This is exactly how human experts help — they think about the problem space, not just the question text.

**2. Clinical Differential Diagnosis (Medicine)**
- **Source field:** Doctors diagnose diseases by considering not just symptoms that match, but also disease prevalence, patient history, and diagnostic likelihood ratios.
- **How it works:** They use Bayesian thinking — start with base rates (how common is this condition?), then update based on specific evidence. They consider not just "what fits?" but "what's most likely given everything we know?"
- **The analogy:** Skill relevance should consider base rates — how often is this skill actually useful? — and update based on context. A rare but precise skill might score lower than a commonly useful general skill.
- **Practical adaptation:** Track skill usage statistics: How often is each skill injected? How often does injection correlate with session success? Use these statistics as prior probabilities, then update based on semantic similarity and context matching.
- **Stretch vs solid:** Solid. Relevance ranking that ignores usage statistics will recommend rare edge-case skills over broadly applicable ones.

**3. Chess Pattern Recognition (Cognitive Science)**
- **Source field:** Master chess players recognize patterns instantly — they see "king safety weakness" or "passed pawn endgame" rather than individual piece positions.
- **How it works:** They chunk information into meaningful patterns through thousands of hours of practice. Pattern recognition operates at multiple levels simultaneously — tactical (immediate threats), strategic (positional advantages), and structural (pawn formations).
- **The analogy:** Expert programmers recognize problem patterns, not just surface features. We need to match against problem patterns, not just text similarity.
- **Practical adaptation:** Classify skills by the problem patterns they solve: "API integration," "performance optimization," "error handling," "state management," etc. When matching, first classify the user's problem pattern, then find skills that address that pattern class, regardless of specific technologies mentioned.
- **Stretch vs solid:** Solid. Problem pattern classification is already how experienced developers think — "this is a caching problem" or "this is a race condition."

**4. Recommendation Systems & Collaborative Filtering (Information Retrieval)**
- **Source field:** How Netflix recommends movies — not just based on what you liked, but on what people similar to you liked, and what movies are similar to movies you liked.
- **How it works:** Multiple recommendation strategies: content-based (similar features), collaborative filtering (similar users), and hybrid approaches. The key insight: similarity can be computed in multiple dimensions simultaneously.
- **The analogy:** We can match skills based on multiple similarity dimensions: text similarity, problem pattern similarity, user context similarity, and success correlation similarity.
- **Practical adaptation:** Build multiple embedding spaces: (1) semantic text embeddings, (2) problem pattern embeddings based on skill classifications, (3) user context embeddings based on tools used and session history. Combine all three with learned weights to rank skill relevance.
- **Stretch vs solid:** Very solid. Multi-dimensional similarity matching is proven to work better than single-dimension approaches.

**5. Episodic vs. Semantic Memory (Neuroscience)**
- **Source field:** Human memory systems — episodic memory (specific experiences) vs. semantic memory (general knowledge). Different retrieval cues activate different memory systems.
- **How it works:** When you remember how to solve a problem, you might recall a specific time you solved it (episodic) or general principles you know (semantic). Context cues determine which system activates. Similar problems trigger episodic recall; abstract thinking triggers semantic recall.
- **The analogy:** Some user prompts need specific procedural knowledge ("how do I deploy this specific app?"), others need general principles ("what are best practices for deployment?"). We need different matching strategies for different prompt types.
- **Practical adaptation:** Classify user prompts as specific-procedural vs. general-conceptual. Use different matching strategies: specific prompts get matched to highly similar skills with exact context matches; general prompts get matched to broadly applicable skills with proven success rates.
- **Stretch vs solid:** Solid. The procedural vs. conceptual distinction is real and important — they need different retrieval strategies.

### My Recommendation

Start with **reference librarian thinking** combined with **multiple similarity dimensions**.

Build a multi-step matching pipeline: (1) Classify the user's prompt into problem patterns (debugging, integration, optimization, etc.), (2) Find skills that address that problem pattern class, (3) Rank by multiple similarity scores weighted together — text similarity, problem pattern similarity, and historical success correlation.

This addresses the core issue that text similarity misses problem-pattern similarity. A prompt about "React performance" might be most text-similar to React-specific skills, but most problem-pattern-similar to general JavaScript optimization skills, which might be more broadly useful.

### What to Explore Next

1. **Build the problem pattern taxonomy** — analyze existing ClawHub skills to identify the actual problem patterns they solve
2. **Test multi-dimensional similarity** — does combining text + pattern + success-rate similarity beat text similarity alone?
3. **Study reference librarian techniques** — what specific question-refinement strategies can we automate?

---

## Problem 3: Knowledge Decay & Quality Over Time

### The Mycelium Problem

Skills go stale. APIs change, libraries update, workarounds become unnecessary. A collective knowledge base can accumulate cruft over time, and worse, outdated knowledge can be actively harmful. We need mechanisms to keep knowledge fresh and detect when it's gone bad, but we also can't just delete everything constantly or we'll lose valuable niche knowledge.

### Cross-Field Explorations

**1. Forest Ecology & Fire Cycles (Ecological Science)**
- **Source field:** Forest ecosystems use periodic controlled burns to clear undergrowth, prevent catastrophic wildfires, and create space for new growth.
- **How it works:** Natural fire cycles remove accumulated dead material while preserving the essential structure (mature trees with deep roots). Different species have different fire tolerances — some die and regrow quickly, others survive and persist. The key insight: periodic destruction is necessary for long-term health.
- **The analogy:** Knowledge libraries need periodic "controlled burns" to remove accumulated cruft while preserving valuable foundational knowledge. Different types of knowledge should have different persistence strategies.
- **Practical adaptation:** Classify skills by type: "foundational" (basic patterns that rarely change), "tactical" (specific solutions that go stale quickly), and "contextual" (environment-specific knowledge). Run periodic decay cycles with different rules for each type. Foundational skills need strong evidence to deprecate; tactical skills need recent validation to persist.
- **Stretch vs solid:** Very solid. The insight that different knowledge types need different persistence rules is profound and practical.

**2. Immune System Memory (Immunology)**
- **Source field:** Adaptive immune systems remember pathogens but have mechanisms to forget irrelevant threats and update responses to evolved threats.
- **How it works:** Long-lived memory B cells maintain antibody production for important threats. But memory cells die off gradually without reinforcement, and the system can update antibody responses when it encounters evolved versions of known pathogens. Key: selective persistence based on continued relevance.
- **The analogy:** Our knowledge system should remember important problem solutions while gradually forgetting unused ones. But it also needs to update solutions when the underlying technology changes.
- **Practical adaptation:** Track skill usage and success rates over time. Skills that haven't been used in 6 months start losing ranking. Skills that are used but show declining success rates get flagged for review. When we detect related new skills, automatically compare to see if they supersede old ones.
- **Stretch vs solid:** Solid. This maps almost directly — antibodies are like skills, pathogens are like problems, and immune memory is like the skill library.

**3. Wikipedia's Quality Control System (Collective Intelligence)**
- **Source field:** How Wikipedia maintains quality at massive scale with volunteer editors — not by preventing bad edits, but by detecting and correcting them quickly.
- **How it works:** Recent changes feeds, automated vandalism detection, editor reputation systems, and protective measures for high-value pages. The key insight: assume good faith but verify everything, with more scrutiny for higher-impact content.
- **The analogy:** We can't prevent bad knowledge from entering the system, but we can detect and correct it quickly. More popular skills need more monitoring; foundational skills need protection from casual changes.
- **Practical adaptation:** (1) Monitor skill performance continuously — flag skills that show declining success rates, (2) Implement "protection levels" — widely-used skills require stronger evidence to modify, (3) Create "recent changes" feeds for library maintainers, (4) Build reputation systems for skill contributors.
- **Stretch vs solid:** Very solid. Wikipedia has solved collective knowledge curation at scale — we can adapt their mechanisms.

**4. Urban Planning & Adaptive Cities (Urban Studies)**
- **Source field:** How healthy cities balance preservation of valuable infrastructure with adaptation to changing needs.
- **How it works:** Zoning laws that allow incremental change, infrastructure that can be upgraded rather than replaced, and planning processes that balance stakeholder interests. Key insight: good systems enable gradual adaptation rather than catastrophic replacement.
- **The analogy:** Knowledge libraries are like cities — they need infrastructure that persists but can be incrementally upgraded. Total replacement is expensive and destroys value; but resistance to change leads to stagnation.
- **Practical adaptation:** Design skill versioning that allows incremental updates rather than replacement. When APIs change, don't delete the old skill — create a new version and deprecate the old one gracefully. Maintain compatibility layers and migration guides between versions.
- **Stretch vs solid:** Solid. Versioning and graceful deprecation are well-understood problems in software engineering.

**5. Radioactive Decay & Half-Life (Nuclear Physics)**
- **Source field:** Radioactive decay follows predictable statistical patterns — individual atoms decay randomly, but populations follow consistent half-life curves.
- **How it works:** Each atom has a constant probability of decay per unit time. This creates predictable population-level decay curves even though individual decay events are random. Different isotopes have different half-lives based on their stability.
- **The analogy:** Different types of knowledge have different "half-lives" based on how quickly they become obsolete. We can model knowledge decay probabilistically rather than trying to predict exactly when each skill goes bad.
- **Practical adaptation:** Assign decay rates to skill categories: API-specific knowledge decays faster than general programming principles. Use probabilistic decay models — skills gradually lose ranking over time unless refreshed by successful usage. Track actual decay patterns to calibrate the models.
- **Stretch vs solid:** Solid. The mathematical framework for half-life decay could work well for modeling knowledge obsolescence.

### My Recommendation

Start with **forest ecology thinking** combined with **immune system memory**.

Classify skills into categories with different persistence rules: "foundational" (rare decay, hard to remove), "tactical" (frequent validation needed), and "contextual" (environment-specific, medium decay). Implement usage-based memory reinforcement — skills that get used and correlate with success get their "memory" refreshed; unused skills gradually decay in ranking.

Add Wikipedia-style monitoring: track skill performance over time and flag declining skills for review. This gives us both automatic gradual decay AND human oversight for edge cases.

### What to Explore Next

1. **Analyze actual knowledge decay patterns** — how quickly do different types of programming knowledge actually go stale?
2. **Build the classification system** — what are the actual categories of knowledge in our domain, and how should they decay differently?
3. **Test probabilistic decay models** — does half-life modeling actually predict knowledge obsolescence better than simple time-based rules?

---

## Summary

The meta-insight across all three problems: **quality is not a single property, it's an emergent phenomenon from multiple independent measures**. Whether we're evaluating extraction quality, matching relevance, or monitoring decay, single-metric solutions fail because they're easy to game and miss important edge cases.

The cross-disciplinary approach reveals that other fields have solved analogous problems by using multi-dimensional assessment, staged filtering, and adaptive feedback loops. We should steal these patterns ruthlessly.

All three solutions point toward building **multi-layered systems with different rules for different knowledge types**. That's the common thread — one size doesn't fit all, whether we're talking about extraction quality, relevance matching, or decay management.

Now we need to get concrete. Build the taxonomies, collect the training data, and test whether these biological and social patterns actually work for collective AI knowledge.

*The answer already exists. It was in the library science literature, the immunology textbooks, and the forest ecology papers all along.*