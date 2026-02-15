# Synthesis Verdict: Luka's Brief #001
## Jazz's Review — February 15, 2026

*Thirty years in this industry and I still have to explain that a research brief without a data model is a blog post.*

---

## 1. Where the Team Agrees (High-Confidence Decisions)

These are done. No more debate.

- **Multi-stage filtering is the extraction architecture.** Everyone likes gold panning. Otso can build it, Pavel can test each stage independently, Charis can explain it. Ship it.
- **Dimensional scoring over holistic judgment.** Wine tasting won unanimously. Specific questions beat "is this good?" — this is not controversial.
- **Usage-based decay with skill categorization.** Forest ecology + immune memory. Different knowledge types decay differently. Track usage, demote unused skills. Everyone nodded.
- **Problem pattern taxonomy (~15-20 patterns) for injection matching.** Fixed, curated list. Not ML-generated. Not infinite. Otso says buildable, Pavel says testable.
- **Start with ONE embedding space.** Multiple embedding spaces is premature optimization. Everyone agrees. Do one well first.
- **Skip contrastive learning, collaborative filtering, and 5-classifier consensus.** No training data, no users, no need. Otso and Pavel independently killed these. They're dead.
- **The brief is too long and buries its recommendations.** Charis is right. Lead with recommendations, support with evidence. Analogies are seasoning, not the meal.

## 2. Where There's Tension (Needs Resolution)

**TENSION 1: Build features first vs. build measurement first.**

Otso wants to build the pipeline in 3-4 weeks and let real data guide iteration. Pavel wants the evaluation framework *before* any features ship. They're both right, which is annoying.

**My verdict:** Pavel wins on principle, but Otso wins on scope. Build the *logging and measurement skeleton* in Week 1 — structured logs at every decision point, human review sampling pipeline, baseline metrics. Then build features *into* that skeleton. You don't need a full evaluation framework to start, but you need the scaffolding to capture data from day one. Building features without logging is how you get six months in with no idea if anything works.

**TENSION 2: How many quality classifiers to start with.**

Luka says 3-5 with 4/5 consensus. Otso says 2-3 max. Pavel calls the "4/5" threshold a magic number.

**My verdict:** Start with 2. Embedding distance + structured rubric scoring. That's it. Add a third when you have data showing two isn't enough. The "4/5 sensors agree" thing is hand-waving until you can show why 4 is better than 3 or 2. Pavel's right — those thresholds need to be earned from data, not declared by fiat.

**TENSION 3: Developer interaction surface — who owns it?**

Charis flagged a massive gap: how do developers see extractions, dispute scores, flag bad injections? Nobody else mentioned it. This is a real problem that will bite you in six months when someone asks "why did it inject that garbage?"

**My verdict:** Not Week 1, but it needs to be in the plan. Otso should define API boundaries that *allow* for a future inspection/feedback UI. Charis should spec the interaction patterns. Don't build the UI yet, but don't paint yourself into a corner either.

## 3. The Actual Plan

### Phase 0: Foundations — Owner: Otso + Pavel

- Data model for skills (Otso — this is MISSING and it's embarrassing)
- API boundary definitions (Otso)
- Structured logging at every pipeline decision point (Pavel spec, Otso implement)
- Human review sampling pipeline — even if it's just "dump 1% to a folder for manual review" (Pavel)
- Baseline metrics definition: what does "good" look like numerically? (Pavel)
- Cost analysis for LLM calls per extraction stage (Otso — also MISSING)

### Phase 1: Extraction Pipeline — Owner: Otso

- Stage 1: Metadata pre-filters (session length, tool usage, completion status)
- Stage 2: Structured dimensional scoring (the wine tasting rubric — 2 classifiers, not 5)
- Stage 3: LLM critic on survivors only
- Simple skill storage with categorization
- Single-embedding retrieval for injection

### Phase 2: Injection & Decay — Owner: Otso

- Problem pattern taxonomy (fixed list, ~15-20 patterns, manually curated)
- Multi-step matching: classify prompt → find pattern-matched skills → rank by similarity + usage stats
- Usage-based decay: track injection frequency and success correlation
- Skill categorization (foundational / tactical / contextual) with different decay rates

### Phase 3: Measurement & Validation (Ongoing from Phase 0) — Owner: Pavel

- Stage-by-stage precision/recall measurement
- Inter-rater reliability for dimensional scoring
- A/B testing: injection vs. no-injection on real sessions
- Silent false negative detection (sample rejected sessions for missed good knowledge)
- Attribution tracking (which injected skill helped/hurt?)
- Decay model backtesting against real usage patterns

### Phase 3.5: Documentation — Owner: Charis

- Internal architecture doc (no analogies, just how it works)
- Contributor conceptual guide (gold panning, wine tasting, reference librarian, forest fires — the four survivors)
- Glossary of terms
- Error states and feedback patterns spec (for future DX work)

## 4. What Luka Got Right

I'll say this once and I won't repeat it.

The **core insight is correct**: quality is multi-dimensional, single-metric solutions fail, and other fields have solved these problems. That's genuinely good research thinking. The gold panning → wine tasting → reference librarian → forest fires pipeline is a coherent architecture when you strip away the filler.

The recommendation to combine multi-stage filtering with dimensional evaluation is exactly right. Three reviewers independently confirmed it. That doesn't happen with bad ideas.

The brief also correctly identified that post-extraction monitoring matters as much as extraction-time filtering. The peer review / retraction analogy is sound even if it wasn't the flashiest section.

## 5. What Luka's Next Brief Should Cover

**Title: "Kinoko Data Model, API Boundaries, and Evaluation Framework"**

No analogies. No cross-disciplinary exploration. Just:

1. **Data model.** What is a Skill? What fields? What's the schema? What's the storage? How do you version it?
2. **API boundaries.** What are the interfaces between extraction, storage, retrieval, and injection? What goes over the wire?
3. **Evaluation framework.** How do we measure each stage? What are the metrics? What are the thresholds to ship vs. not ship? How do we A/B test?
4. **Cost model.** What does it cost per extraction? Per injection? Per decay cycle? Where are the expensive calls?
5. **Error handling.** What happens when the LLM critic is down? When embeddings fail? When a skill is corrupted?

In other words: the engineering spec that should have been in this brief but wasn't, because we were too busy talking about bacteria and radioactive isotopes.

---

## Final Word

Luka wrote a good *research* brief. The problem is we needed a research brief AND an engineering spec, and we got one. The ideas are sound — everyone agrees on the core architecture. Now we need someone to write down what the actual data structures look like, because I've seen too many projects with beautiful analogies and no schema survive exactly zero encounters with production traffic.

The team is aligned on more than they realize. That's rare. Don't waste it.

*— Jazz*
