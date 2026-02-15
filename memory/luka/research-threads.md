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

## 2026-02-15 — Brief 003 (COMPLETED)

Explored post-session extraction signals - the insight that the strongest indicator of valuable knowledge is what happens AFTER the session ends, not during it.

### Cross-Field Analysis: Retrospective Quality Systems
- **Academic Citations**: Quality emerges through citation networks over time, not peer review
- **Vintage Wine**: Value revealed through aging and expert consensus over years
- **Collective Memory**: Societies remember what gets retold and referenced across contexts
- **Search Engines**: Click-through, dwell time, and return visits reveal true relevance
- **Financial Markets**: Price discovery through trading behavior and performance persistence
- **Archaeology**: Artifact value through durability, uniqueness, and connection to other finds

**Universal retrospective signals**: Persistence through time, network references, return engagement, context bridging, performance under pressure

### 3-Layer Delayed Extraction Architecture
- **Layer 1** (Session End): Cheap metadata capture, provisional quality assessment (<$0.01/session)
- **Layer 2** (1-7 days): Return visits, artifact persistence, cross-references (<$0.05/promoted)
- **Layer 3** (30+ days): Deep quality assessment and skill finalization (<$0.50/skill)

**Key insight**: Quality assessment is continuous, not one-time. Best knowledge reveals itself over weeks/months.

### Privacy/Value Paradox Solutions
- **Medical Research Model**: Anonymize individual data, learn from population patterns
- **Journalism Model**: Separate content from source, protect individual context
- **Aggregate Analytics**: Differential privacy, k-anonymity, federated learning
- **Wikipedia Model**: Focus on transferable patterns, not personal attribution

### Concrete Signal Specifications
- **Return Visits**: Weighted by time decay, multiple returns > single returns
- **Artifact Persistence**: File survival, git commit integration, deployment uptime
- **Cross-References**: Code reuse, approach similarity, problem pattern matching
- **Composite Scoring**: 40% return behavior, 30% artifact survival, 20% citations, 10% initial filters

**Quality thresholds**: ≥2.0 high-value extraction, 1.0-2.0 candidate monitoring, 0.5-1.0 low-confidence patterns, <0.5 no extraction

### Meta-Insight
The retrospective approach solves the "everyone starts from zero" problem by identifying the 1% of sessions that have already proven their value through real-world usage patterns. Time reveals quality—we just need infrastructure to wait for the revelation.
