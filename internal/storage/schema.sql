CREATE TABLE IF NOT EXISTS skills (
    id                    TEXT PRIMARY KEY,
    name                  TEXT NOT NULL,
    version               INTEGER NOT NULL DEFAULT 1,
    parent_id             TEXT REFERENCES skills(id),
    library_id            TEXT NOT NULL,
    category              TEXT NOT NULL CHECK (category IN ('foundational','tactical','contextual')),
    q_problem_specificity     INTEGER NOT NULL CHECK (q_problem_specificity BETWEEN 1 AND 5),
    q_solution_completeness   INTEGER NOT NULL CHECK (q_solution_completeness BETWEEN 1 AND 5),
    q_context_portability     INTEGER NOT NULL CHECK (q_context_portability BETWEEN 1 AND 5),
    q_reasoning_transparency  INTEGER NOT NULL CHECK (q_reasoning_transparency BETWEEN 1 AND 5),
    q_technical_accuracy      INTEGER NOT NULL CHECK (q_technical_accuracy BETWEEN 1 AND 5),
    q_verification_evidence   INTEGER NOT NULL CHECK (q_verification_evidence BETWEEN 1 AND 5),
    q_innovation_level        INTEGER NOT NULL CHECK (q_innovation_level BETWEEN 1 AND 5),
    q_composite_score         REAL NOT NULL,
    q_critic_confidence       REAL NOT NULL CHECK (q_critic_confidence BETWEEN 0.0 AND 1.0),
    injection_count       INTEGER NOT NULL DEFAULT 0,
    last_injected_at      TIMESTAMP,
    success_correlation   REAL NOT NULL DEFAULT 0.0,
    decay_score           REAL NOT NULL DEFAULT 1.0,
    source_session_id     TEXT,
    extracted_by          TEXT NOT NULL,
    file_path             TEXT NOT NULL,
    created_at            TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at            TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name, version, library_id)
);

CREATE TABLE IF NOT EXISTS skill_patterns (
    skill_id  TEXT NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    pattern   TEXT NOT NULL,
    PRIMARY KEY (skill_id, pattern)
);

CREATE TABLE IF NOT EXISTS skill_embeddings (
    skill_id   TEXT PRIMARY KEY REFERENCES skills(id) ON DELETE CASCADE,
    embedding  BLOB NOT NULL,
    model      TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sessions (
    id                   TEXT PRIMARY KEY,
    started_at           TIMESTAMP NOT NULL,
    ended_at             TIMESTAMP NOT NULL,
    duration_minutes     REAL NOT NULL,
    tool_call_count      INTEGER NOT NULL,
    error_count          INTEGER NOT NULL,
    message_count        INTEGER NOT NULL,
    error_rate           REAL NOT NULL,
    has_successful_exec  BOOLEAN NOT NULL,
    tokens_used          INTEGER NOT NULL DEFAULT 0,
    agent_model          TEXT NOT NULL DEFAULT '',
    user_id              TEXT NOT NULL DEFAULT '',
    library_id           TEXT NOT NULL,
    extraction_status    TEXT NOT NULL DEFAULT 'pending',
    rejected_at_stage    INTEGER NOT NULL DEFAULT 0,
    rejection_reason     TEXT NOT NULL DEFAULT '',
    extracted_skill_id   TEXT REFERENCES skills(id),
    log_content_path     TEXT NOT NULL DEFAULT '',
    retry_count          INTEGER NOT NULL DEFAULT 0,
    last_error           TEXT NOT NULL DEFAULT '',
    next_retry_at        TIMESTAMP,
    claimed_by           TEXT NOT NULL DEFAULT '',
    claimed_at           TIMESTAMP,
    created_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS injection_events (
    id              TEXT PRIMARY KEY,
    session_id      TEXT NOT NULL,
    skill_id        TEXT NOT NULL REFERENCES skills(id),
    rank_position   INTEGER NOT NULL,
    match_score     REAL NOT NULL,
    pattern_overlap REAL NOT NULL,
    cosine_sim      REAL NOT NULL,
    historical_rate REAL NOT NULL,
    injected_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    session_outcome TEXT DEFAULT NULL,
    ab_group        TEXT NOT NULL DEFAULT '',
    delivered       BOOLEAN NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS human_review_samples (
    id              TEXT PRIMARY KEY,
    session_id      TEXT NOT NULL REFERENCES sessions(id),
    extraction_result TEXT NOT NULL,
    reviewer        TEXT,
    verdict         TEXT,
    notes           TEXT,
    sampled_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    reviewed_at     TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_skills_category ON skills(category);
CREATE INDEX IF NOT EXISTS idx_skills_decay ON skills(decay_score);
CREATE INDEX IF NOT EXISTS idx_skills_library ON skills(library_id);
CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(extraction_status);
CREATE INDEX IF NOT EXISTS idx_sessions_library ON sessions(library_id);
CREATE INDEX IF NOT EXISTS idx_injection_events_skill ON injection_events(skill_id);
CREATE INDEX IF NOT EXISTS idx_injection_events_session ON injection_events(session_id);
CREATE INDEX IF NOT EXISTS idx_skills_name_library ON skills(name, library_id);
CREATE INDEX IF NOT EXISTS idx_sessions_queue ON sessions(extraction_status, next_retry_at);
CREATE INDEX IF NOT EXISTS idx_injection_events_ab_group ON injection_events(ab_group);
CREATE INDEX IF NOT EXISTS idx_injection_events_outcome ON injection_events(skill_id, session_outcome);
