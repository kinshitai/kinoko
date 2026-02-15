# Feedback Patterns

How developers interact with the Mycelium system: inspecting what it does, correcting mistakes, and maintaining knowledge quality. This is a DX specification — it describes the interaction patterns the system should support, not their implementation.

---

## Design Principle

Mycelium operates automatically by default. Developers shouldn't *need* to interact with it. But when they want to — because something went wrong, because they're curious, or because they know something the system doesn't — the interaction should be clear, fast, and effective.

Every automated decision should be inspectable. Every inspectable decision should be correctable.

---

## 1. Extraction Visibility

**What developers need to see:**

- Which sessions were evaluated for extraction
- Which stage rejected a session (and why)
- Which sessions produced skills (and the resulting skill)
- Dimensional quality scores assigned to extracted skills

**Interaction patterns:**

### Reviewing Extractions

A developer should be able to list recent extraction decisions:

```
Session abc123  →  Rejected at Stage 1 (duration: 47s, below 2min threshold)
Session def456  →  Rejected at Stage 2 (Technical Accuracy: 2/5, below minimum)
Session ghi789  →  Extracted → skill "Debugging PostgreSQL connection pooling"
```

Each entry links to the full decision trace: which filters fired, what scores were assigned, what the critic said (if Stage 3 was reached).

### Flagging Missed Extractions

A developer believes a rejected session contained valuable knowledge. They should be able to:

1. Identify the session
2. See why it was rejected
3. Override the rejection and trigger re-evaluation, or manually promote it to extraction

This is the "false negative" correction path. It feeds back into filter calibration.

### Flagging Bad Extractions

A developer sees a skill that shouldn't have been extracted — it's wrong, misleading, or too context-specific. They should be able to:

1. Flag the skill with a reason
2. See the extraction rationale (scores, critic reasoning)
3. Demote or remove the skill

This is the "false positive" correction path.

---

## 2. Quality Score Disputes

**What developers need to see:**

- Per-dimension scores for any skill
- The evidence or reasoning behind each score

**Interaction patterns:**

### Inspecting Scores

For any skill, a developer can view:

```
"Debugging PostgreSQL connection pooling"
  Problem Specificity:      5/5
  Solution Completeness:    4/5
  Context Portability:      2/5  ← why?
  Reasoning Transparency:   4/5
  Technical Accuracy:       5/5
  Verification Evidence:    4/5
  Innovation Level:         2/5
```

Each score should be expandable to show *why* that score was assigned — the classifier's evidence or the critic's reasoning.

### Disputing a Score

A developer disagrees with a dimension score. They should be able to:

1. Select the dimension
2. Provide a corrected score and rationale
3. Submit the dispute

Disputes are logged. If a skill accumulates disputes, it's flagged for re-evaluation. Developer corrections feed into classifier calibration over time.

### Requesting Re-Evaluation

A developer can trigger full re-scoring of a skill — useful after the skill content has been manually edited, or when classifiers have been updated.

---

## 3. Injection Feedback

**What developers need to see:**

- Which skills were injected into a session
- Why each skill was selected (pattern match, similarity score, success rate)
- Whether the injection was helpful

**Interaction patterns:**

### Reviewing Injections

After a session, a developer can see:

```
Injected skills:
  1. "Debugging PostgreSQL connection pooling"
     Pattern: FIX/Backend/DatabaseConnection (match: 0.92)
     Similarity: 0.78  |  Success rate: 0.84  |  Combined: 0.86

  2. "General database connection troubleshooting"
     Pattern: FIX/Backend/DatabaseConnection (match: 0.88)
     Similarity: 0.65  |  Success rate: 0.71  |  Combined: 0.73
```

### Flagging Bad Injections

A skill was injected but was irrelevant or harmful to the session. The developer should be able to:

1. Mark the injection as unhelpful (with optional reason)
2. This negative signal feeds into the skill's success correlation
3. Persistent negative feedback triggers skill review

### Flagging Missing Injections

The developer knows of a skill that *should* have been injected but wasn't. They should be able to:

1. Identify the skill and the session
2. See why it wasn't matched (pattern mismatch? low similarity? below threshold?)
3. Submit a relevance assertion: "this skill is relevant to this type of problem"

This feeds into pattern taxonomy refinement and matching calibration.

---

## 4. Decay Intervention

**What developers need to see:**

- Skills currently in decay (losing ranking)
- Skills approaching decay thresholds
- Recently deprecated or removed skills

**Interaction patterns:**

### Monitoring Decay

A developer can view the decay status of the library:

```
Decaying (3):
  "Webpack 4 tree-shaking workaround"    — last used 8 months ago, tactical
  "Docker Compose v2 migration steps"    — last used 5 months ago, tactical
  "REST API pagination patterns"         — last used 7 months ago, foundational ← unusual

Active (142):
  ...

Recently deprecated (1):
  "Node.js 14 async_hooks workaround"    — superseded by "Node.js 18+ diagnostics_channel"
```

### Rescuing a Skill

A developer knows a decaying skill is still valid — it's just niche. They should be able to:

1. Select the decaying skill
2. Provide justification ("this is still relevant for teams on PostgreSQL 12")
3. Reset or extend the decay timer

Rescued skills are marked as manually validated, which affects future decay behavior.

### Expediting Decay

A developer knows a skill is stale — the API changed, the library was deprecated, the approach is now an anti-pattern. They should be able to:

1. Flag the skill for immediate deprecation
2. Optionally link to a superseding skill
3. Provide reason (for audit trail)

---

## 5. Library Maintenance

**Interaction patterns beyond individual skills:**

### Bulk Review

A developer can filter and review skills by:

- Category (foundational / tactical / contextual)
- Pattern (e.g., all `FIX/Backend/*` skills)
- Quality score range (e.g., all skills with Technical Accuracy < 3)
- Decay status
- Age or last-used date

### Audit Trail

Every human intervention (flag, dispute, rescue, deprecation) is logged with:

- Who did it
- When
- What they changed
- Why (their stated reason)

This trail is essential for understanding how the library evolves and for detecting systematic quality issues.

### Sampling Dashboard

The 1% human review sample (see [architecture.md → Logging and Measurement](./architecture.md#5-logging-and-measurement)) should be presented as a review queue:

- Sessions awaiting review
- Quick accept/reject interface
- Option to add notes or adjust scores
- Aggregate statistics on filter accuracy over time

---

## Summary of Feedback Loops

| What went wrong | Developer action | System effect |
|---|---|---|
| Good session wasn't extracted | Flag missed extraction | Re-evaluate; calibrate filters |
| Bad session was extracted | Flag bad extraction | Demote skill; calibrate filters |
| Quality score is wrong | Dispute specific dimension | Log correction; re-score if disputed often |
| Irrelevant skill was injected | Flag bad injection | Lower success correlation; review matching |
| Relevant skill wasn't injected | Flag missing injection | Refine pattern taxonomy; review thresholds |
| Valid skill is decaying | Rescue from decay | Reset decay timer; mark as validated |
| Stale skill is still active | Expedite deprecation | Immediate demotion; link superseding skill |

Each feedback action generates data that improves the system's automated decisions over time. The goal is a system that needs less human correction as it learns from the corrections it receives.
