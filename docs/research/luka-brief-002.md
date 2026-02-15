# Luka's Research Brief #002
## Concrete Implementation Designs

*Luka Jensen, Research Engineer*  
*February 15, 2026*

---

## 1. Extraction Pre-Filters

### Multi-Stage Filtering Architecture

**Stage 1: Cheap Session Metadata Filters**
```
Decision Tree:
├─ Session duration < 2 minutes → REJECT (too brief for knowledge)
├─ Session duration > 3 hours → REJECT (likely exploratory, not conclusive)
├─ Tool usage count < 3 → REJECT (minimal interaction)
├─ Error rate > 70% → REJECT (mostly failed attempts)
├─ No successful exec() calls → REJECT (no actual work done)
└─ Passes basic filters → Stage 2

Thresholds (empirically tunable):
- Min duration: 2 minutes
- Max duration: 180 minutes  
- Min tool calls: 3
- Max error rate: 0.7
- Must have ≥1 successful exec with exit code 0
```

**Stage 2: Content Pattern Recognition**
```
Heuristics (boolean filters):
- Contains problem-solution structure (regex patterns)
- Has code examples with context
- Shows iterative refinement (multiple attempts → success)
- Includes verification/testing steps
- Explains "why" not just "how"

Pattern Detection:
- Problem statement: "error:", "failed to", "how do I", "need to"
- Solution attempt: exec(), edit(), write() with meaningful content  
- Verification: "works", "fixed", "successful", test execution
- Context explanation: "because", "since", "this is needed for"

Scoring: Must match ≥3 patterns to proceed to Stage 3
```

**Stage 3: LLM Quality Assessment (expensive, limited)**
Only for candidates passing Stages 1-2. Run dimensional evaluation (see Section 2).

### Implementation Reference: Academic Manuscript Screening

Based on **editorial decision systems** used by journals:
- **Nature/Science pre-screening**: 80% rejection based on scope/novelty before peer review
- **ArXiv moderation**: automated filters for formatting, completeness, basic coherence
- **Wikipedia featured article review**: structured checklist with specific criteria

### Pre-Filter Calibration Data

**Training approach**: Manually label 500 sessions as "extract/no-extract" based on outcomes:
- Sessions that led to reused patterns in later work → extract
- Sessions with solutions that worked for similar problems → extract  
- Pure exploration with no conclusion → no extract
- Failed attempts with no useful insights → no extract

**Otso can build this by:**
1. Creating session metadata extraction pipeline (duration, tool counts, error rates)
2. Building regex pattern matchers for problem-solution structure
3. Implementing staged filtering with configurable thresholds  
4. Adding feedback loop to track filter precision/recall against manual labels

---

## 2. Skill Quality Dimensions  

### 7-Dimensional Skill Quality Rubric

**Dimension 1: Problem Specificity**
- *Question*: "Does this solve a clearly defined problem?"
- *Scale*: 1-5 (1=vague, 5=specific problem statement)
- *Criteria*:
  - 5: Specific error message, API, or task with context
  - 4: Clear problem category with some context  
  - 3: General problem type (debugging, deployment, etc.)
  - 2: Broad category (frontend, backend, etc.)
  - 1: Unclear what problem this solves

**Dimension 2: Solution Completeness**
- *Question*: "Can someone follow this to solve the problem?"
- *Scale*: 1-5 (1=partial, 5=complete solution)
- *Criteria*:
  - 5: Complete solution with verification steps
  - 4: Solution with most steps, minor gaps
  - 3: Core solution present, missing some details
  - 2: Partial solution, significant gaps
  - 1: Incomplete or fragmented

**Dimension 3: Context Portability**
- *Question*: "How much context is needed to apply this elsewhere?"
- *Scale*: 1-5 (1=highly specific, 5=broadly applicable)
- *Criteria*:
  - 5: Works across environments/projects (general principles)
  - 4: Portable with minor environment adjustments
  - 3: Requires similar tech stack
  - 2: Very specific environment/version requirements
  - 1: Tied to exact original context

**Dimension 4: Reasoning Transparency**
- *Question*: "Does this explain WHY, not just WHAT?"  
- *Scale*: 1-5 (1=no explanation, 5=clear reasoning)
- *Criteria*:
  - 5: Explains underlying reasons and trade-offs
  - 4: Some reasoning provided for key decisions
  - 3: Basic explanation of approach
  - 2: Minimal reasoning, mostly procedural
  - 1: No explanation, just steps

**Dimension 5: Technical Accuracy**
- *Question*: "Are the technical details correct?"
- *Scale*: 1-5 (1=incorrect, 5=accurate)
- *Criteria*:
  - 5: Technically accurate, current best practices
  - 4: Accurate with minor outdated elements
  - 3: Mostly correct, some questionable practices
  - 2: Some technical errors or poor practices
  - 1: Technically incorrect or harmful

**Dimension 6: Verification Evidence**
- *Question*: "Is there proof this solution works?"
- *Scale*: 1-5 (1=no evidence, 5=strong evidence)
- *Criteria*:
  - 5: Shown working in session + testing described
  - 4: Demonstrated working in original session
  - 3: Logical solution but no execution proof
  - 2: Theoretical solution, unclear if tested
  - 1: No evidence of effectiveness

**Dimension 7: Innovation Level**
- *Question*: "How novel is this solution approach?"
- *Scale*: 1-5 (1=standard, 5=novel)  
- *Criteria*:
  - 5: Novel approach or insight, significantly different
  - 4: Creative variation on known approaches
  - 3: Standard approach with useful modifications
  - 2: Standard approach, well-executed
  - 1: Common/obvious solution

### Aggregate Scoring
- **Minimum viable skill**: ≥3 on Problem Specificity, Solution Completeness, Technical Accuracy
- **High-value skill**: ≥4 average across all dimensions
- **Priority injection**: High Context Portability × Verification Evidence scores

### Implementation Reference: Established Quality Frameworks

**Based on Wikipedia Featured Article Criteria**:
- Comprehensive (completeness)
- Well-researched (verification)  
- Neutral point of view (accuracy)
- Stable (context independence)

**Code Review Rubrics** (Google/Microsoft):
- Correctness, Design, Complexity, Tests, Naming, Comments
- Similar dimensional breakdown of technical work quality

**Educational Content Quality** (Bloom's Taxonomy adaptation):
- Knowledge level (what), Comprehension (why), Application (how to use)

**Otso can build this by:**
1. Creating structured evaluation forms with 1-5 scales for each dimension
2. Training lightweight classifiers on each dimension using manual examples
3. Implementing weighted scoring that prioritizes minimum viability filters
4. Building dashboard for human reviewers to calibrate and audit scores

---

## 3. Problem Pattern Taxonomy

### Software Development Task Classification  

**Tier 1: Primary Intent Categories**
```
BUILD        - Creating new functionality
FIX          - Resolving existing problems  
OPTIMIZE     - Improving performance/efficiency
INTEGRATE    - Connecting systems/components
CONFIGURE    - Setting up environments/tools
LEARN        - Understanding existing systems
```

**Tier 2: Technical Domain**
```
Frontend     - UI, browsers, user interaction
Backend      - APIs, servers, databases  
DevOps       - Infrastructure, deployment, monitoring
Data         - Processing, analysis, storage
Security     - Authentication, authorization, vulnerabilities
Performance  - Speed, memory, scalability
```

**Tier 3: Specific Patterns (Examples)**

**BUILD patterns:**
- `BUILD/Frontend/ComponentDesign` - Creating reusable UI components
- `BUILD/Backend/APIEndpoint` - Implementing REST/GraphQL endpoints  
- `BUILD/Data/Pipeline` - ETL and data processing workflows
- `BUILD/DevOps/Automation` - CI/CD and deployment scripts

**FIX patterns:**
- `FIX/Frontend/BrowserCompatibility` - Cross-browser issues
- `FIX/Backend/DatabaseConnection` - Connection/query problems
- `FIX/DevOps/DeploymentFailure` - Build/deploy pipeline issues
- `FIX/Performance/MemoryLeak` - Resource consumption problems

**Pattern Inheritance:**
- Skills tagged `FIX/Performance/*` match prompts about any performance debugging
- Skills tagged `BUILD/Frontend/*` match frontend construction tasks
- More specific patterns (Tier 3) get higher relevance weights than general patterns (Tier 1)

### Implementation Reference: Bug Classification Systems

**Mozilla Bug Categories**:
- Defect types: logic, UI, performance, security, compatibility
- Component mapping: browser engine, UI, networking, graphics
- Severity levels and priority rankings

**JIRA Issue Types** (widely adopted):  
- Epic/Story/Task/Bug/Improvement structure  
- Component and label taxonomies
- Custom fields for technical classification

**Stack Overflow Tag Hierarchy**:
- Technology tags (javascript, python, react)
- Concept tags (debugging, performance, deployment)  
- Problem type tags (error-handling, authentication)

### Pattern Assignment Strategy

**For Skills** (during extraction):
1. Extract mentioned technologies → Domain classification
2. Analyze session goal → Intent classification  
3. Pattern-match solution type → Specific pattern
4. Allow multiple patterns per skill (skills can solve multiple problem types)

**For User Prompts** (during injection):
1. Parse prompt for technology mentions → Domain hints
2. Extract action verbs → Intent classification
3. Identify problem symptoms → Pattern matching
4. Score skills by pattern overlap + embedding similarity

**Pattern-Based Relevance Scoring**:
```
Score = (Pattern_Match_Weight × Pattern_Overlap) + 
        (Embedding_Weight × Cosine_Similarity) + 
        (Usage_Weight × Historical_Success_Rate)

Where:
- Pattern_Overlap = Number of matching pattern tags
- Pattern_Match_Weight = 0.5 (tunable)
- Embedding_Weight = 0.3  
- Usage_Weight = 0.2
```

**Otso can build this by:**
1. Creating pattern extraction rules based on code analysis, tool usage, and session outcomes
2. Building pattern taggers that classify skills during extraction pipeline
3. Implementing prompt pattern matching with fallback to embedding similarity
4. Creating feedback loop to track which pattern matches lead to successful sessions

---

## System Integration

All three components work together:

1. **Extraction Pipeline**: Multi-stage filters → Dimensional quality assessment → Pattern classification → Skill creation
2. **Injection Pipeline**: User prompt → Pattern classification → Pattern-weighted skill ranking → Dimensional quality filtering → Injection
3. **Feedback Loop**: Session outcomes → Update pattern effectiveness scores → Adjust quality dimension weights → Refine filter thresholds

The key insight remains: **quality and relevance emerge from multiple independent measures**, not single-metric optimization. Each component provides different signal that combines into robust, game-resistant skill curation.

*Now we build the training data and test whether librarians, wine tasters, and geologists actually know something about AI knowledge management.*

---

## Addendum: Post-Session Extraction Signals (Egor + Hal, 2026-02-15)

The strongest extraction signals may not come from session analysis at all — they come from what happens AFTER the session:

**Return signal:** User reopens an old chat after days/weeks. Nobody returns to a dead session for fun. This is the highest-confidence signal that a session contains extractable knowledge.

**Artifact persistence:** Session produces files that get committed, READMEs that stay in repos, configs that persist. If the output survives, the knowledge is real.

**Cross-reference signal:** An artifact from session A appears in session B. A README written in one session gets referenced in another. This is citation count for agent sessions.

**Refinement depth:** Sessions with iterative back-and-forth (try → adjust → better → yes) contain more nuanced knowledge than single-shot Q&A.

**Implication for architecture:** Extraction shouldn't only happen at session end. We need a delayed extraction pass that evaluates sessions retroactively based on return visits, artifact survival, and cross-references. The best knowledge reveals itself over time, not at the moment of creation.

**Privacy implication:** The most valuable sessions (ones users return to) are often the most personal. Extraction must separate transferable patterns from personal context. "I debugged our billing race condition at 2 AM" → skill is "debugging race conditions in concurrent services." The story stays local, the pattern travels.