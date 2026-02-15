# Luka's Research Brief #003
## Retrospective Quality Signals: Architecture for Delayed Knowledge Extraction

*Luka Jensen, Research Engineer*  
*February 15, 2026*

---

## The Core Insight

**Papers aren't valuable because peer reviewers said so. They're valuable because other papers cite them.**

This applies directly to agent sessions: the strongest signal that a session contains extractable knowledge isn't what happened during the session—it's what happens after. Return visits, artifact persistence, cross-references from other sessions. We need an architecture that evaluates knowledge quality retrospectively, not just at session-end.

---

## 1. Cross-Field Analysis: Retrospective Quality Systems

### Academic Citations: The Gold Standard
**How it works**: Papers gain authority through citation count, citation quality (who cites them), and citation context (why they're cited).

**Key patterns**:
- **Time delay**: Quality emerges months/years after publication
- **Network effects**: High-quality papers create citation cascades
- **Context sensitivity**: Same paper has different value in different fields
- **Authority propagation**: Citations from prestigious sources carry more weight

**Implementation insight**: Track session "citations"—when artifacts from session A appear in session B, treat it as a quality signal.

### Vintage Wine Classification: Quality Through Time
**How it works**: Wine quality isn't determined at bottling. Critical assessments happen years later. The best vintages are those that age well and maintain/improve quality over time.

**Key patterns**:
- **Survivorship**: Only wines worth keeping get aged
- **Environmental stability**: Quality depends on consistent storage conditions
- **Expert consensus**: Multiple independent tastings over time
- **Scarcity premium**: Aged quality compounds value

**Implementation insight**: Sessions that produce artifacts still in use months later have proven their value through "aging"—they've remained relevant and useful.

### Collective Memory Formation (Anthropology)
**How societies decide what to remember**: Not based on immediate importance, but on what gets retold, referenced, and built upon over generations.

**Key patterns**:
- **Narrative persistence**: Stories that get retold in new contexts
- **Practical utility**: Knowledge that helps solve recurring problems
- **Cultural resonance**: Information that fits existing mental models
- **Transmission cost**: Simple, memorable patterns survive better

**Implementation insight**: Sessions that get "retold" (referenced in later sessions, artifacts reused) demonstrate cultural/practical value.

### Search Engine Click-Through Data: Revealed Preference
**How Google uses retrospective signals**: Initial rankings based on content analysis, refined through user behavior. Pages that get clicked, stayed on, and returned to gain authority.

**Key patterns**:
- **Immediate feedback**: Click-through rate signals relevance
- **Dwell time**: How long users stay indicates quality  
- **Return visits**: Users bookmarking/returning indicates lasting value
- **Query refinement**: Users' subsequent searches reveal satisfaction

**Implementation insight**: Sessions users return to, spend time in, and reference later demonstrate high practical value.

### Financial Market Price Discovery
**How markets determine "true" value**: Initial pricing based on fundamentals, corrected through trading behavior, information flow, and time-based performance.

**Key patterns**:
- **Liquidity premium**: Frequently traded assets are better price-discovered
- **Information incorporation**: New information changes valuations
- **Risk adjustment**: Value relative to alternatives over time
- **Performance persistence**: Assets that perform well continue to attract attention

**Implementation insight**: Sessions that remain "liquid" (actively referenced, artifacts reused) maintain high knowledge value.

### Archaeological Artifact Preservation
**How we decide what's worth preserving**: Not based on initial assessment (we can't know historical significance), but on durability, uniqueness, and connection to other finds.

**Key patterns**:
- **Material durability**: Physical persistence despite environment
- **Contextual significance**: Artifacts that illuminate other finds
- **Rarity value**: Unique examples of common patterns
- **Reconstruction utility**: Pieces that help complete larger pictures

**Implementation insight**: Session artifacts that physically persist (committed files, deployed systems) and connect to other work demonstrate archaeological value.

## Cross-Field Pattern Summary

**Universal retrospective quality signals**:
1. **Persistence through time** (aging, survival)
2. **Network references** (citations, reuse)  
3. **Return engagement** (revisits, bookmarks)
4. **Context bridging** (connecting different domains)
5. **Performance under pressure** (working when it matters)

**Implementation sketch**: Track all five signal types. Weight them based on field-specific research. Quality score emerges from signal convergence over time.

---

## 2. Practical Architecture: Delayed Extraction System

### Three-Layer Extraction Pipeline

**Layer 1: Immediate Extraction (Session End)**
*Low confidence, cheap computation*

```
Session metadata capture:
- Duration, tool usage, error rates
- Artifact outputs (files, commits, deployments)
- Problem-solution structure detection
- Context tags (technologies, domains, patterns)

Quality assessment: PROVISIONAL
- Basic filters only (duration >2min, tools >3, success signals present)
- Pattern classification but no quality scoring
- Storage: candidate pool with LOW confidence scores
- Cost: <$0.01 per session
```

**Layer 2: Early Signal Detection (1-7 days)**
*Medium confidence, moderate computation*

```
Retrospective signal monitoring:
- Return visits: user reopens session
- Artifact persistence: files still exist, code still deployed
- Cross-references: artifacts mentioned in other sessions
- Usage patterns: how user engages with produced artifacts

Quality re-evaluation: PROVISIONAL → CANDIDATE
- Return visit within 7 days: +50% confidence
- Artifact still in repo after 7 days: +30% confidence  
- Referenced in another session: +60% confidence
- Combined signals can promote to CANDIDATE status
- Cost: <$0.05 per promoted session
```

**Layer 3: Long-Term Quality Assessment (30+ days)**
*High confidence, expensive computation*

```
Deep retrospective analysis:
- Multiple return visits over time
- Artifact survival and active use  
- Cross-session citation networks
- Similar problem-solving success

Quality finalization: CANDIDATE → SKILL
- Multi-dimensional LLM evaluation (from brief-002)
- Privacy sanitization and anonymization
- Pattern extraction and generalization
- Integration into skill database
- Cost: <$0.50 per finalized skill
```

### Signal Detection Infrastructure

**Return Visit Tracking**:
```python
class SessionReturnTracker:
    def track_return(self, session_id: str, days_since_end: int):
        # Weight returns by recency and frequency
        return_weight = max(0.1, 1.0 / days_since_end)
        self.signal_db.increment("return_score", session_id, return_weight)
        
    def get_return_pattern(self, session_id: str) -> ReturnPattern:
        # Distinguish one-offs from ongoing reference sessions
        visits = self.signal_db.get_returns(session_id)
        if len(visits) == 1: return ReturnPattern.SINGLE_REVISIT
        if max(visits) - min(visits) > 30: return ReturnPattern.ONGOING_REFERENCE
        return ReturnPattern.CLUSTER_REVISIT
```

**Artifact Persistence Monitoring**:
```python
class ArtifactSurvivalTracker:
    def monitor_artifacts(self, session_artifacts: List[Artifact]):
        for artifact in session_artifacts:
            if artifact.type == "file":
                self.schedule_file_existence_checks(artifact, [1, 7, 30, 90])
            elif artifact.type == "git_commit":  
                self.monitor_commit_references(artifact)
            elif artifact.type == "deployment":
                self.track_deployment_status(artifact)
                
    def calculate_persistence_score(self, artifact: Artifact) -> float:
        age_days = (datetime.now() - artifact.created).days
        if artifact.still_exists and age_days > 90:
            return 1.0  # High-value persistence
        elif artifact.still_exists and age_days > 30:
            return 0.7  # Medium persistence  
        elif artifact.still_exists and age_days > 7:
            return 0.4  # Early persistence
        else:
            return 0.0  # Failed to persist
```

**Cross-Reference Detection**:
```python
class CrossReferenceTracker:
    def detect_citations(self, new_session: Session, historical_sessions: List[Session]):
        citations = []
        for hist_session in historical_sessions:
            similarity_score = self.calculate_content_overlap(new_session, hist_session)
            if similarity_score > 0.3:  # Significant content reuse
                citations.append(Citation(
                    citing_session=new_session.id,
                    cited_session=hist_session.id,  
                    strength=similarity_score,
                    citation_type=self.classify_citation_type(overlap_analysis)
                ))
        return citations
        
    def classify_citation_type(self, analysis) -> CitationType:
        if analysis.has_code_reuse: return CitationType.IMPLEMENTATION_REUSE
        if analysis.has_approach_similarity: return CitationType.APPROACH_REFERENCE  
        if analysis.has_problem_match: return CitationType.PROBLEM_PATTERN
        return CitationType.TANGENTIAL_REFERENCE
```

### Privacy-Safe Signal Collection

**Differential Privacy for Signals**:
- Add noise to return visit counts (prevent user behavior fingerprinting)
- Anonymize artifact hashes (track persistence without exposing content)
- Aggregate signals across users before quality assessment
- Local processing of sensitive signals, federated aggregation of quality scores

**Implementation sketch**: Build signal collection as privacy-preserving system from day one. Collect behavioral signals locally, aggregate quality indicators centrally without exposing individual patterns.

---

## 3. The Privacy/Value Paradox: Learning from Other Fields

### Medical Research: Knowledge from Patient Data
**The paradox**: The most medically valuable data is the most personally sensitive. Cancer research needs detailed patient histories, but patients need privacy protection.

**Solutions implemented**:
- **Anonymization**: Remove identifying information, keep medical patterns
- **Aggregate analysis**: Learn from population patterns, not individual cases
- **Differential privacy**: Add noise to datasets to prevent re-identification
- **Federated learning**: Models learn from distributed data without centralizing it
- **Purpose limitation**: Data only used for specific approved research questions

**Implementation insight**: Apply medical anonymization to session data. Extract transferable problem-solving patterns while removing personal context.

### Journalism: Source Protection
**The paradox**: The most newsworthy information comes from sources who need the most protection. Value depends on credibility, credibility depends on source identification, but identification destroys protection.

**Solutions implemented**:
- **Content/source separation**: Publish the story, protect the source
- **Verification independence**: Multiple sources confirm facts independently
- **Contextual anonymization**: Remove identifying details while preserving newsworthiness
- **Time delays**: Publish information when source protection risk decreases

**Implementation insight**: Separate "what was learned" from "who learned it and why." Technical patterns can transfer without personal context.

### Aggregate Analytics: Individual Behavior, Population Insights
**The paradox**: Platforms need to understand user behavior to provide value, but individual behavior tracking violates privacy. Google/Apple solved this with privacy-preserving analytics.

**Solutions implemented**:
- **Local differential privacy**: Add noise to individual data points
- **K-anonymity**: Only report patterns seen in k+ individuals
- **Aggregate reporting**: Population statistics, not individual profiles
- **Temporal aggregation**: Patterns over time windows, not moment-by-moment tracking

**Implementation insight**: Build population-level skill quality from individual session signals without storing individual behavioral profiles.

### Wikipedia: Collaborative Knowledge from Personal Expertise
**The paradox**: Best Wikipedia articles come from experts with deep personal knowledge, but Wikipedia aims for neutral, impersonal content.

**Solutions implemented**:
- **Content/contributor separation**: Focus on article quality, not author identity
- **Peer review process**: Multiple editors verify and improve content
- **Source attribution**: Reference external sources, not personal experience
- **Version control**: Track changes and quality evolution over time

**Implementation insight**: Extract impersonal technical patterns from personal problem-solving sessions. Focus on "what works" not "who did it."

## Privacy-Preserving Extraction Architecture

**Stage 1: Local Anonymization**
```python
class PrivacySanitizer:
    def sanitize_session(self, session: Session) -> AnonymizedSession:
        # Remove personal identifiers
        sanitized = session.copy()
        sanitized.remove_personal_data()  # names, emails, IPs, etc.
        
        # Generalize specific context
        sanitized.generalize_paths()      # /Users/john/project → /path/to/project
        sanitized.generalize_urls()       # mycompany.com → example.com
        sanitized.generalize_accounts()   # john@company → user@domain
        
        # Keep technical patterns  
        sanitized.preserve_error_patterns()
        sanitized.preserve_solution_structures()
        sanitized.preserve_tool_usage_patterns()
        
        return sanitized
```

**Stage 2: Differential Privacy in Signal Aggregation**
```python  
class DifferentialPrivateSignals:
    def aggregate_return_visits(self, session_ids: List[str]) -> float:
        true_return_rate = sum(self.get_return_count(sid) for sid in session_ids) / len(session_ids)
        noise = self.laplace_noise(sensitivity=1.0/len(session_ids), epsilon=1.0)
        return max(0.0, true_return_rate + noise)
        
    def aggregate_persistence_scores(self, artifacts: List[Artifact]) -> Dict[str, float]:
        # Add noise to prevent inference of individual artifact survival
        return {
            artifact_type: self.noisy_average([a.persistence_score for a in artifacts if a.type == artifact_type])
            for artifact_type in ["file", "commit", "deployment"]
        }
```

**Stage 3: Federated Quality Assessment**
```python
class FederatedQualityLearning:
    def train_quality_model(self, local_datasets: List[LocalDataset]) -> QualityModel:
        # Each organization trains locally, shares model updates only
        local_models = []
        for dataset in local_datasets:
            local_model = self.train_local_model(dataset)
            local_models.append(local_model.get_weights())
            
        # Aggregate model weights without sharing raw data
        global_weights = self.federated_averaging(local_models)
        return QualityModel(weights=global_weights)
```

**Implementation sketch**: Build privacy as a core architectural principle. Personal context stays local, transferable patterns aggregate globally with mathematical privacy guarantees.

---

## 4. Concrete Signal Specifications: Measurable Thresholds

### Return Visit Signals

**Signal Definition**: User reopens a completed session after it ended.

**Measurement**:
```
Return Score = Σ(return_weight_i) where:
return_weight_i = min(1.0, days_since_session_end_i^(-0.5))

Thresholds:
- Single return within 7 days: +0.5 quality points
- Multiple returns within 30 days: +1.0 quality points  
- Returns after 30+ days: +1.5 quality points (aging bonus)
- No returns after 90 days: -0.2 quality points (decay penalty)
```

**Implementation**:
```python
def calculate_return_score(returns: List[datetime], session_end: datetime) -> float:
    score = 0.0
    for return_time in returns:
        days_elapsed = (return_time - session_end).days
        weight = min(1.0, days_elapsed ** -0.5)
        score += weight
    return score

def classify_return_pattern(score: float) -> ReturnPattern:
    if score == 0: return ReturnPattern.NEVER_RETURNED
    if score < 0.7: return ReturnPattern.SINGLE_RETURN
    if score < 1.5: return ReturnPattern.MULTIPLE_RETURNS
    return ReturnPattern.REFERENCE_SESSION  # Highly valuable
```

### Artifact Persistence Signals

**Signal Definition**: Files, commits, deployments, or configurations produced during session still exist and are actively used.

**Measurement Categories**:

**File Persistence**:
```
File Score = existence_bonus × usage_bonus × age_bonus

Where:
- existence_bonus: 1.0 if file exists, 0.0 if deleted
- usage_bonus: 1.0 + (git_commit_frequency / 30_days)
- age_bonus: min(1.5, sqrt(days_survived / 30))

Thresholds:
- File deleted within 7 days: 0.0 points (failure)
- File exists at 30 days: +0.5 points
- File actively modified at 30+ days: +1.0 points  
- File becomes part of core codebase: +1.5 points
```

**Git Commit Persistence**:
```
Commit Score = branch_integration × reference_count × time_weight

Where:
- branch_integration: 2.0 if merged to main, 1.0 if branch exists, 0.0 if deleted
- reference_count: number of later commits that reference/build on this commit
- time_weight: log(1 + days_since_commit)

Thresholds:
- Commit merged to main branch: +1.0 points
- Commit referenced by later work: +0.5 per reference
- Commit survives >90 days in active branch: +0.8 points
```

**Deployment Persistence**:
```
Deployment Score = uptime_ratio × traffic_weight × configuration_stability

Where:  
- uptime_ratio: percentage of time deployment is healthy
- traffic_weight: log(1 + requests_per_day)
- configuration_stability: 1.0 - (config_changes / days_deployed)

Thresholds:
- Deployment running >95% uptime for 30+ days: +1.2 points
- Deployment handling real user traffic: +0.8 points
- Configuration unchanged for 60+ days: +0.5 points (stability bonus)
```

### Cross-Reference Signals

**Signal Definition**: Content, approaches, or artifacts from session A appear in later session B.

**Reference Types and Scores**:

**Direct Code Reuse**:
```
if cosine_similarity(session_A.code, session_B.code) > 0.7:
    points = 1.5  # High-confidence knowledge transfer

if edit_distance_ratio(session_A.code, session_B.code) > 0.5:
    points = 1.0  # Moderate code reuse
```

**Approach Similarity**:
```
if session_A.solution_approach == session_B.solution_approach and 
   session_A.problem_domain == session_B.problem_domain:
    points = 0.8  # Same approach to similar problem
    
if session_A.tools_used.overlap(session_B.tools_used) > 0.6:
    points = 0.5  # Similar tool usage pattern
```

**Problem Pattern Matching**:
```
if session_A.problem_signature.matches(session_B.problem_signature):
    points = 1.2  # Same problem type solved similarly
    
if session_A.error_patterns.overlap(session_B.error_patterns) > 0.4:
    points = 0.6  # Related debugging patterns
```

**Temporal Decay in Citations**:
```
time_decay_factor = exp(-0.1 × days_between_sessions)
final_citation_score = base_citation_score × time_decay_factor

# Recent citations worth more than old citations
# Prevents stale knowledge from accumulating credit
```

### Composite Quality Score

**Final Score Calculation**:
```
Session Quality Score = 
    0.4 × return_score +           # User behavior most important
    0.3 × artifact_persistence +   # Real-world survival matters
    0.2 × cross_reference_score +  # Network effects
    0.1 × initial_quality_filters  # Basic session viability

Quality Thresholds:
- Score ≥ 2.0: HIGH-VALUE EXTRACTION (expensive LLM processing)
- Score 1.0-2.0: CANDIDATE (monitor for additional signals)  
- Score 0.5-1.0: LOW-CONFIDENCE (cheap pattern extraction only)
- Score < 0.5: NO EXTRACTION (filter out)
```

**Quality Evolution Over Time**:
```python
def evolve_quality_score(session_id: str, current_score: float, days_elapsed: int) -> float:
    # Signals can accumulate or decay over time
    new_signals = self.collect_recent_signals(session_id, days_elapsed)
    signal_boost = sum(signal.value for signal in new_signals)
    
    # Natural decay for sessions without fresh signals
    decay_factor = exp(-0.01 × days_elapsed) if not new_signals else 1.0
    
    evolved_score = (current_score * decay_factor) + signal_boost
    return max(0.0, evolved_score)  # Floor at 0
```

**Implementation sketch**: Build a comprehensive signal collection and scoring system that combines multiple independent measures. Quality emerges from convergent evidence across return visits, artifact survival, and network effects. Scores evolve over time as new signals accumulate or natural decay occurs.

---

## System Integration: End-to-End Architecture

The complete delayed extraction system combines all components:

1. **Immediate extraction pipeline** (from brief-002) creates extraction candidates
2. **Retrospective signal monitoring** tracks return visits, artifact persistence, cross-references  
3. **Privacy-preserving aggregation** sanitizes personal context while preserving technical patterns
4. **Quality score evolution** continuously updates session value based on accumulated signals
5. **Promotion pipeline** elevates high-scoring sessions to full skill extraction
6. **Feedback integration** uses extraction success to tune signal weights and thresholds

**Key architectural insight**: Quality assessment is not a one-time decision but a continuous process. The best knowledge reveals itself over weeks and months, not minutes and hours. Build for gradual signal accumulation, not instant classification.

**The retrospective extraction advantage**: By waiting for return visits, artifact survival, and cross-references, we can identify the 1% of sessions that contain genuinely transferable knowledge—knowledge that has already proven its value through real-world usage and reference patterns.

This is how we scale beyond the "everyone starts from zero" problem. Not by guessing what's valuable, but by measuring what has already proven valuable.

*Time reveals quality. We just need the patience and infrastructure to wait for the revelation.*