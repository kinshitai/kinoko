-- Migration note: existing databases from pre-v0.x may contain extra columns
-- (e.g. decay_score, injection_count, last_injected_at, success_correlation,
-- source_session_id). SQLite ignores unused columns gracefully, so no
-- explicit migration is needed.
CREATE TABLE IF NOT EXISTS skills (
    id                    TEXT PRIMARY KEY,
    name                  TEXT NOT NULL,
    description           TEXT NOT NULL DEFAULT '',
    version               INTEGER NOT NULL DEFAULT 1,
    parent_id             TEXT REFERENCES skills(id),
    library_id            TEXT NOT NULL,
    category              TEXT NOT NULL CHECK (category IN ('foundational','tactical','contextual')),
    q_problem_specificity     INTEGER NOT NULL CHECK (q_problem_specificity BETWEEN 0 AND 5),
    q_solution_completeness   INTEGER NOT NULL CHECK (q_solution_completeness BETWEEN 0 AND 5),
    q_context_portability     INTEGER NOT NULL CHECK (q_context_portability BETWEEN 0 AND 5),
    q_reasoning_transparency  INTEGER NOT NULL CHECK (q_reasoning_transparency BETWEEN 0 AND 5),
    q_technical_accuracy      INTEGER NOT NULL CHECK (q_technical_accuracy BETWEEN 0 AND 5),
    q_verification_evidence   INTEGER NOT NULL CHECK (q_verification_evidence BETWEEN 0 AND 5),
    q_innovation_level        INTEGER NOT NULL CHECK (q_innovation_level BETWEEN 0 AND 5),
    q_composite_score         REAL NOT NULL,
    q_critic_confidence       REAL NOT NULL CHECK (q_critic_confidence BETWEEN 0.0 AND 1.0),
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

CREATE INDEX IF NOT EXISTS idx_skills_category ON skills(category);
CREATE INDEX IF NOT EXISTS idx_skills_library ON skills(library_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_skills_name_library ON skills(name, library_id);
