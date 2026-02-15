# Active Research Threads

## 2026-02-15 — Brief 001

### Extraction Quality
- Best approaches: multi-stage filtering (gold panning) + dimensional evaluation (wine tasting)
- Key insight: don't ask "is this good?" — ask specific dimensional questions (novel? reusable? clear context?)
- Cheap pre-filters first (session metadata), expensive LLM critic only on candidates
- Quorum sensing principle: quality emerges from consensus of independent measures
- Next: build training dataset of 500-1000 labeled sessions

### Injection Precision
- Best approaches: reference librarian thinking + problem pattern classification
- Key insight: match problem PATTERNS not text similarity. "Slow React app" pattern-matches optimization skills, not just React skills
- Multi-dimensional similarity: text + problem pattern + usage success rate
- Bayesian approach from clinical diagnosis: use base rates (how often is skill useful?) as priors
- Next: build problem pattern taxonomy from existing skills

### Knowledge Decay
- Best approaches: forest fire ecology + immune memory
- Key insight: different knowledge types have different half-lives. Foundational = slow decay, tactical = fast decay
- Usage-based reinforcement: skills that get used and succeed get refreshed
- Wikipedia-style monitoring for high-impact skills
- Radioactive decay model for probabilistic ranking decrease
- Next: analyze actual decay patterns in programming knowledge

### Meta-Insight
Quality is emergent from multiple independent measures. One-dimensional scoring fails. Every field that does quality well uses multi-dimensional, multi-stage approaches.

## 2026-02-15 — Brief 002 (COMPLETED)

Transformed the three winning analogies into concrete implementations:

### 1. Multi-Stage Extraction Filters
- **Stage 1**: Session metadata (duration 2-180min, tool usage >3, error rate <70%)
- **Stage 2**: Content patterns (problem-solution structure, code+context, verification steps)  
- **Stage 3**: Dimensional LLM evaluation (expensive, only for pre-filtered candidates)
- **Reference**: Academic manuscript screening, Wikipedia moderation systems
- **Calibration**: 500-session manual labeling for threshold tuning

### 2. 7-Dimensional Quality Rubric
- **Dimensions**: Problem Specificity, Solution Completeness, Context Portability, Reasoning Transparency, Technical Accuracy, Verification Evidence, Innovation Level
- **Scoring**: 1-5 scale per dimension, minimum viability thresholds, weighted aggregation
- **Reference**: Wikipedia Featured Article criteria, code review rubrics, Bloom's taxonomy
- **Priority**: High Context Portability × Verification Evidence for injection ranking

### 3. Problem Pattern Taxonomy  
- **3-Tier Structure**: Intent (BUILD/FIX/OPTIMIZE/etc) → Domain (Frontend/Backend/DevOps/etc) → Specific Patterns
- **Pattern-Based Scoring**: Pattern overlap + embedding similarity + historical success rate
- **Reference**: Mozilla bug classification, JIRA issue types, Stack Overflow tag hierarchy
- **Implementation**: Extract patterns during skill creation, match patterns during injection

### Key Implementation Insight
All three systems integrate: Extraction filters → Quality assessment → Pattern classification → Injection ranking with feedback loops to improve each component based on session outcomes.
